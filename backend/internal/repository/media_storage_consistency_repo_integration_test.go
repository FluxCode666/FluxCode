//go:build integration

package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMediaStorageConsistencySerializesConfigAndArtifactCommits(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	unique := uuid.NewString()
	user := mustCreateUser(t, client, &service.User{
		Email: "media-storage-consistency-" + unique + "@example.com",
	})
	group := mustCreateGroup(t, client, &service.Group{
		Name: unique, Platform: service.PlatformMedia, Status: service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard, RateMultiplier: 1,
	})
	apiKey := mustCreateApiKey(t, client, &service.APIKey{
		UserID: user.ID, GroupID: &group.ID, Key: "sk-" + unique, Name: unique, Status: service.StatusAPIKeyActive,
	})
	task, err := NewMediaTaskRepository(client).Create(ctx, &service.MediaTask{
		PublicID: "media-storage-" + unique, UserID: user.ID, APIKeyID: apiKey.ID, GroupID: group.ID,
		MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
		RequestedModel: "storage-test", ClientAsync: true,
		Status: service.MediaTaskStatusQueued, Stage: service.MediaTaskStageQueued,
		RequestSpec: []byte(`{"image":{"prompt":"test","n":1}}`), CandidateSnapshot: []byte(`[]`),
		RequestFingerprint: unique, BillingStatus: service.MediaBillingStatusPending,
	})
	require.NoError(t, err)

	_, err = integrationDB.ExecContext(ctx, "DELETE FROM settings WHERE key = $1", service.MediaStorageSettingKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM media_artifacts WHERE task_id = $1", task.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM settings WHERE key = $1", service.MediaStorageSettingKey)
	})

	expectedRevision := service.MediaStorageSettingRevision("", false)
	configRepo := NewMediaStorageConsistencyRepository(integrationDB)
	artifactRepo := NewMediaStorageConsistencyRepository(integrationDB)
	artifact := &service.MediaArtifact{
		TaskID: task.ID, Direction: "output", Position: 0, MediaType: service.MediaTypeImage,
		ContentType: "image/png", SizeBytes: 3, ChecksumSHA256: "abc",
		StorageStatus: "stored", StorageProvider: service.MediaStorageProviderLocal, ObjectKey: "media/test.png",
		StorageRevision: expectedRevision,
	}
	const firstConfig = `{"provider":"local","local_path":"/new/path","minio":{}}`

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var configErr, artifactErr error
	var created *service.MediaArtifact
	go func() {
		defer wg.Done()
		<-start
		configErr = configRepo.CommitConfig(
			ctx,
			expectedRevision,
			firstConfig,
			[]string{service.MediaStorageProviderLocal},
		)
	}()
	go func() {
		defer wg.Done()
		<-start
		created, artifactErr = artifactRepo.CommitArtifact(ctx, expectedRevision, artifact)
	}()
	close(start)
	wg.Wait()

	configWon := configErr == nil && errors.Is(artifactErr, service.ErrMediaStorageConfigChanged)
	artifactWon := artifactErr == nil && errors.Is(configErr, service.ErrMediaStorageLocationInUse)
	require.Truef(t, configWon || artifactWon, "configErr=%v artifactErr=%v", configErr, artifactErr)

	var artifactCount, settingCount int
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(*) FROM media_artifacts WHERE task_id = $1", task.ID,
	).Scan(&artifactCount))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"SELECT count(*) FROM settings WHERE key = $1", service.MediaStorageSettingKey,
	).Scan(&settingCount))
	if configWon {
		require.Zero(t, artifactCount)
		require.Equal(t, 1, settingCount)
		expectedRevision = service.MediaStorageSettingRevision(firstConfig, true)
		artifact.StorageRevision = expectedRevision
		created, err = artifactRepo.CommitArtifact(ctx, expectedRevision, artifact)
		require.NoError(t, err)
	} else {
		require.Equal(t, 1, artifactCount)
		require.Zero(t, settingCount)
	}
	require.NotNil(t, created)

	const rotatedConfig = `{"provider":"local","local_path":"/new/path","minio":{"access_key_id":"rotated"}}`
	require.NoError(t, configRepo.CommitConfig(ctx, expectedRevision, rotatedConfig, nil))
	retried, err := artifactRepo.CommitArtifact(ctx, expectedRevision, artifact)
	require.NoError(t, err, "an exact idempotent retry must win before the stale revision check")
	require.Equal(t, created.ID, retried.ID)
	require.Equal(t, artifact.StorageRevision, retried.StorageRevision)

	concrete := artifactRepo.(*mediaStorageConsistencyRepository)
	require.NoError(t, concrete.reconcileConfigCommit(ctx, rotatedConfig, service.ErrMediaStorageCommitOutcomeUnknown))
	require.ErrorIs(t,
		concrete.reconcileConfigCommit(ctx, `{"provider":"local","local_path":"/different"}`, service.ErrMediaStorageCommitOutcomeUnknown),
		service.ErrMediaStorageConfigChanged,
	)
	reconciled, err := concrete.reconcileArtifactCommit(ctx, artifact, service.ErrMediaStorageCommitOutcomeUnknown)
	require.NoError(t, err)
	require.Equal(t, created.ID, reconciled.ID)
	missing := *artifact
	missing.Position = 99
	missing.ObjectKey = "media/missing.png"
	_, err = concrete.reconcileArtifactCommit(ctx, &missing, service.ErrMediaStorageCommitOutcomeUnknown)
	require.ErrorIs(t, err, service.ErrMediaStorageIntegrity)
	require.NotErrorIs(t, err, service.ErrMediaStorageCommitOutcomeUnknown)
}

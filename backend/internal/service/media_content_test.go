package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mediaContentTaskRepoStub struct {
	task *MediaTask
}

func (s *mediaContentTaskRepoStub) GetByPublicIDForUser(_ context.Context, publicID string, userID int64) (*MediaTask, error) {
	if s.task == nil || s.task.PublicID != publicID || s.task.UserID != userID {
		return nil, ErrMediaTaskNotFound
	}
	copy := *s.task
	return &copy, nil
}

type mediaContentArtifactRepoStub struct {
	items []MediaArtifact
}

func (s *mediaContentArtifactRepoStub) Create(_ context.Context, artifact *MediaArtifact) (*MediaArtifact, error) {
	copy := *artifact
	copy.ID = int64(len(s.items) + 1)
	s.items = append(s.items, copy)
	return &copy, nil
}

func (s *mediaContentArtifactRepoStub) ListByTaskID(_ context.Context, taskID int64) ([]MediaArtifact, error) {
	var result []MediaArtifact
	for _, artifact := range s.items {
		if artifact.TaskID == taskID {
			result = append(result, artifact)
		}
	}
	return result, nil
}

type mediaContentSettingsStub struct {
	settings *SystemSettings
}

func (s mediaContentSettingsStub) GetAllSettings(context.Context) (*SystemSettings, error) {
	return s.settings, nil
}

type mediaContentAccountRepoStub struct{}

func (mediaContentAccountRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return nil, ErrMediaContentUnavailable
}

type mediaContentHTTPReaderStub struct{}

func (mediaContentHTTPReaderStub) ValidateURL(raw string) (string, error) { return raw, nil }
func (mediaContentHTTPReaderStub) Open(context.Context, MediaHTTPContentRequest) (*MediaContent, error) {
	return nil, ErrMediaContentUnavailable
}

type mediaContentObjectStoreStub struct {
	put func(context.Context, MediaArtifactInput) (*MediaArtifact, error)
}

func (s mediaContentObjectStoreStub) Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
	return s.put(ctx, input)
}

func (mediaContentObjectStoreStub) Open(context.Context, *MediaArtifact, string) (*MediaContent, error) {
	return nil, ErrMediaArtifactObjectStoreDisabled
}

func TestMediaContentServiceDecodesDataURLAndAppliesRange(t *testing.T) {
	accountID := int64(9)
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, AccountID: &accountID,
		MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
	}}
	artifacts := &mediaContentArtifactRepoStub{items: []MediaArtifact{{
		ID: 2, TaskID: 1, Direction: "output", Position: 0, MediaType: MediaTypeVideo,
		ContentType: "video/mp4", UpstreamReference: "data:video/mp4;base64,MDEyMzQ1Njc4OQ==",
	}}}
	svc := NewMediaContentService(
		tasks, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: true}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)

	content, err := svc.OpenVideo(context.Background(), "task_public", 42, "bytes=2-5")
	require.NoError(t, err)
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusPartialContent, content.StatusCode)
	require.Equal(t, "bytes 2-5/10", content.ContentRange)
	require.Equal(t, []byte("2345"), body)
}

func TestMediaContentServiceHidesTaskOwnedByAnotherUser(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusCompleted,
	}}
	svc := NewMediaContentService(
		tasks, &mediaContentArtifactRepoStub{},
		mediaContentSettingsStub{settings: &SystemSettings{}}, mediaContentAccountRepoStub{},
		NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{}, NewDisabledMediaArtifactObjectStore(),
	)

	_, err := svc.OpenVideo(context.Background(), "task_public", 99, "")
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestMediaContentServiceRejectsUnsatisfiableRange(t *testing.T) {
	content, err := sliceMediaContent([]byte("0123"), "video/mp4", "bytes=9-10")
	require.Nil(t, content)
	require.ErrorIs(t, err, ErrMediaRangeNotSatisfiable)
}

func TestMediaContentServiceStageUploadClearsDataAndKeepsChecksum(t *testing.T) {
	data := []byte("fake-image")
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, &mediaContentArtifactRepoStub{}, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{put: func(_ context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
			require.Equal(t, data, input.Data)
			require.NotEmpty(t, input.ChecksumSHA256)
			return &MediaArtifact{MediaType: input.MediaType, ContentType: input.ContentType, ObjectKey: "objects/input"}, nil
		}},
	)

	staged, err := svc.Stage(context.Background(), 42, MediaArtifactInput{
		Direction: "input", MediaType: MediaTypeImage, ContentType: "image/png", Data: data,
	})
	require.NoError(t, err)
	require.Nil(t, staged.Data)
	require.Equal(t, "objects/input", staged.ObjectKey)
	sum := sha256.Sum256(data)
	require.Equal(t, hex.EncodeToString(sum[:]), staged.ChecksumSHA256)
}

func TestDisabledMediaArtifactObjectStoreRejectsPutAndOpen(t *testing.T) {
	store := NewDisabledMediaArtifactObjectStore()
	_, err := store.Put(context.Background(), MediaArtifactInput{Data: []byte("video")})
	require.ErrorIs(t, err, ErrMediaArtifactObjectStoreDisabled)
	_, err = store.Open(context.Background(), &MediaArtifact{}, "")
	require.ErrorIs(t, err, ErrMediaArtifactObjectStoreDisabled)
}

func TestMediaContentServiceVideoUploadRequiresObjectStorage(t *testing.T) {
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, &mediaContentArtifactRepoStub{}, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	_, err := svc.Stage(context.Background(), 42, MediaArtifactInput{MediaType: MediaTypeVideo, Data: []byte("video")})
	require.ErrorIs(t, err, ErrMediaVideoObjectStorageRequired)
}

func TestMediaContentServicePersistsImageProxyReferenceWithoutPublicURL(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10}, []MediaArtifactInput{{
		MediaType: MediaTypeImage, ContentType: "image/png", ExternalURL: "https://upstream.example/image?token=secret",
	}})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, "proxy", stored[0].StorageStatus)
	require.Empty(t, stored[0].PublicURL)
	require.Equal(t, "https://upstream.example/image?token=secret", stored[0].UpstreamReference)
}

func TestMediaContentServiceRejectsVideoProxyWhenFallbackDisabled(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	_, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10}, []MediaArtifactInput{{
		MediaType: MediaTypeVideo, ContentType: "video/mp4", UpstreamReference: "internal-reference",
	}})
	require.ErrorIs(t, err, ErrMediaContentUnavailable)
	require.Empty(t, artifacts.items)
}

func TestMediaContentServiceStoredOutputKeepsInputMetadata(t *testing.T) {
	artifacts := &mediaContentArtifactRepoStub{}
	svc := NewMediaContentService(
		&mediaContentTaskRepoStub{}, artifacts,
		mediaContentSettingsStub{settings: &SystemSettings{MediaVideoProxyFallbackEnabled: false}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		mediaContentObjectStoreStub{put: func(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
			return &MediaArtifact{ObjectKey: "objects/output"}, nil
		}},
	)
	stored, err := svc.PersistOutputs(context.Background(), &MediaTask{ID: 10}, []MediaArtifactInput{{
		MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: 123, ChecksumSHA256: "abc123",
	}})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, MediaTypeVideo, stored[0].MediaType)
	require.Equal(t, "video/mp4", stored[0].ContentType)
	require.Equal(t, int64(123), stored[0].SizeBytes)
	require.Equal(t, "abc123", stored[0].ChecksumSHA256)
}

func TestMediaContentServiceOpenVideoRequiresCompletedVideoOwnedByUser(t *testing.T) {
	tasks := &mediaContentTaskRepoStub{task: &MediaTask{
		ID: 1, PublicID: "task_public", UserID: 42, MediaType: MediaTypeVideo, Status: MediaTaskStatusInProgress,
	}}
	svc := NewMediaContentService(
		tasks, &mediaContentArtifactRepoStub{}, mediaContentSettingsStub{settings: &SystemSettings{}},
		mediaContentAccountRepoStub{}, NewMediaAdapterRegistry(), mediaContentHTTPReaderStub{},
		NewDisabledMediaArtifactObjectStore(),
	)
	_, err := svc.OpenVideo(context.Background(), "task_public", 42, "")
	require.ErrorIs(t, err, ErrMediaTaskNotFound)
}

func TestMediaContentServiceRejectsInvalidBase64DataURL(t *testing.T) {
	data, contentType, ok := decodeMediaDataReference("data:video/mp4;base64,%%%not-base64%%%", "video/mp4")
	require.False(t, ok)
	require.Nil(t, data)
	require.Empty(t, contentType)
}

var (
	_ MediaArtifactWriter = (*MediaContentService)(nil)
	_ MediaInputStager    = (*MediaContentService)(nil)
)

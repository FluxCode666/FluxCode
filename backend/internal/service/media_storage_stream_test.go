package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type mediaRouterStreamStoreStub struct {
	input MediaArtifactInput
	body  []byte
}

func (*mediaRouterStreamStoreStub) Put(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
	return nil, errors.New("unexpected buffered put")
}

func (s *mediaRouterStreamStoreStub) PutStream(
	_ context.Context,
	input MediaArtifactInput,
	body io.Reader,
) (*MediaArtifact, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	s.input = input
	s.body = data
	return &MediaArtifact{
		MediaType: input.MediaType, ContentType: input.ContentType,
		StorageProvider: input.StorageProvider, ObjectKey: "media/output/stream.mp4",
	}, nil
}

func (*mediaRouterStreamStoreStub) Open(context.Context, *MediaArtifact, string) (*MediaContent, error) {
	return nil, ErrMediaArtifactNotFound
}

func (*mediaRouterStreamStoreStub) Discard(context.Context, MediaArtifactInput) error { return nil }

func TestMediaArtifactObjectStoreRouterForwardsStreamingWrites(t *testing.T) {
	store := &mediaRouterStreamStoreStub{}
	router, err := NewMediaArtifactObjectStoreRouter(
		MediaStorageProviderResolverFunc(func(context.Context) (string, error) {
			return MediaStorageProviderLocal, nil
		}),
		map[string]MediaArtifactObjectStore{MediaStorageProviderLocal: store},
	)
	require.NoError(t, err)
	body := testMediaMP4(1024)

	artifact, err := router.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
	}, bytes.NewReader(body))
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, MediaStorageProviderLocal, artifact.StorageProvider)
	require.Equal(t, MediaStorageProviderLocal, store.input.StorageProvider)
	require.Equal(t, body, store.body)
}

func TestConfiguredMediaArtifactStoreStreamsWithRevisionSnapshot(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	settings := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	configureMediaStorageSettingsTestService(settings, &mediaStorageTesterStub{})
	root := t.TempDir()
	_, err := settings.UpdateConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderLocal, LocalPath: root,
	})
	require.NoError(t, err)
	store, err := NewConfiguredMediaArtifactObjectStore(settings, 1<<20)
	require.NoError(t, err)
	body := testMediaMP4(2048)

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: int64(len(body)),
	}, bytes.NewReader(body))
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, MediaStorageProviderLocal, artifact.StorageProvider)
	require.NotEmpty(t, artifact.StorageRevision)

	require.NoError(t, store.Discard(context.Background(), mediaArtifactInputFromStored(artifact)))
}

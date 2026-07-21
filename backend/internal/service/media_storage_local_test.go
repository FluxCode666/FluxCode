package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalMediaArtifactStorePutRangeOpenAndDiscard(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalMediaArtifactObjectStore(root, 1<<20)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	png := testMediaPNG(t)

	artifact, err := store.Put(context.Background(), MediaArtifactInput{
		Direction: "input", MediaType: MediaTypeImage, ContentType: "image/png", Data: png,
	})
	require.NoError(t, err)
	require.Equal(t, MediaStorageProviderLocal, artifact.StorageProvider)
	require.NotEmpty(t, artifact.ObjectKey)
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(artifact.ObjectKey)))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	content, err := store.Open(context.Background(), artifact, "bytes=0-7")
	require.NoError(t, err)
	require.Equal(t, 206, content.StatusCode)
	read, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.NoError(t, content.Body.Close())
	require.Equal(t, png[:8], read)
	require.Equal(t, "bytes 0-7/68", content.ContentRange)

	require.NoError(t, store.Discard(context.Background(), mediaArtifactInputFromStored(artifact)))
	require.NoError(t, store.Discard(context.Background(), mediaArtifactInputFromStored(artifact)))
}

func TestLocalMediaArtifactStoreRejectsTraversalAndIntegrityMismatch(t *testing.T) {
	store, err := NewLocalMediaArtifactObjectStore(t.TempDir(), 1<<20)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	_, err = store.Open(context.Background(), &MediaArtifact{
		StorageProvider: MediaStorageProviderLocal, ObjectKey: "../escape", MediaType: MediaTypeImage, ContentType: "image/png",
	}, "")
	require.ErrorIs(t, err, ErrMediaArtifactNotFound)

	_, err = store.Put(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeImage, ContentType: "image/png",
		Data: testMediaPNG(t), ChecksumSHA256: "incorrect",
	})
	require.ErrorIs(t, err, ErrInvalidMediaInput)
}

func TestConfiguredMediaStoreReadsLocalArtifactAfterSwitchingWriteProvider(t *testing.T) {
	repo := &mediaStorageSettingRepoStub{values: map[string]string{}}
	settings := NewMediaStorageSettingsService(repo, nil, mediaStorageEncryptorStub{})
	configureMediaStorageSettingsTestService(settings, &mediaStorageTesterStub{})
	_, err := settings.UpdateConfig(context.Background(), MediaStorageConfig{
		Provider: MediaStorageProviderLocal, LocalPath: t.TempDir(),
	})
	require.NoError(t, err)
	store, err := NewConfiguredMediaArtifactObjectStore(settings, 1<<20)
	require.NoError(t, err)
	artifact, err := store.Put(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeImage, ContentType: "image/png", Data: testMediaPNG(t),
	})
	require.NoError(t, err)

	effective, err := settings.LoadEffectiveConfig(context.Background())
	require.NoError(t, err)
	effective.Provider = MediaStorageProviderMinIO
	effective.MinIO = MediaMinIOConfig{
		Endpoint: "https://minio.example.com", Bucket: "media", AccessKeyID: "access",
		SecretAccessKey: "secret", UseSSL: true, ForcePathStyle: true,
	}
	_, err = settings.UpdateConfig(context.Background(), *effective)
	require.NoError(t, err)

	content, err := store.Open(context.Background(), artifact, "")
	require.NoError(t, err)
	read, err := io.ReadAll(content.Body)
	require.NoError(t, err)
	require.NoError(t, content.Body.Close())
	require.Equal(t, testMediaPNG(t), read)
}

type boundedReadRequestReader struct {
	reader       *bytes.Reader
	maximumRead  int
	largestRead  int
	requestError error
}

func (r *boundedReadRequestReader) Read(buffer []byte) (int, error) {
	if len(buffer) > r.largestRead {
		r.largestRead = len(buffer)
	}
	if len(buffer) > r.maximumRead {
		return 0, r.requestError
	}
	return r.reader.Read(buffer)
}

func testMediaMP4(size int) []byte {
	if size < 12 {
		size = 12
	}
	data := make([]byte, size)
	copy(data, []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'})
	return data
}

func TestLocalMediaArtifactStoreStreamsLargeObjectsWithBoundedReads(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalMediaArtifactObjectStore(root, 4<<20)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	video := testMediaMP4(2 << 20)
	sum := sha256.Sum256(video)
	readErr := errors.New("requested an oversized read buffer")
	reader := &boundedReadRequestReader{
		reader: bytes.NewReader(video), maximumRead: 128 << 10, requestError: readErr,
	}

	artifact, err := store.PutStream(context.Background(), MediaArtifactInput{
		Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
		SizeBytes: int64(len(video)), ChecksumSHA256: hex.EncodeToString(sum[:]),
	}, reader)
	require.NoError(t, err)
	require.NotNil(t, artifact)
	require.Equal(t, int64(len(video)), artifact.SizeBytes)
	require.Equal(t, hex.EncodeToString(sum[:]), artifact.ChecksumSHA256)
	require.LessOrEqual(t, reader.largestRead, 128<<10)

	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(artifact.ObjectKey)))
	require.NoError(t, err)
	require.Equal(t, int64(len(video)), info.Size())
}

func TestLocalMediaArtifactStoreRejectsInvalidStreamsWithoutCommittingFiles(t *testing.T) {
	video := testMediaMP4(2048)
	sum := sha256.Sum256(video)
	for _, tt := range []struct {
		name     string
		maxBytes int64
		input    MediaArtifactInput
		body     []byte
		wantErr  error
	}{
		{
			name: "maximum exceeded", maxBytes: 1024,
			input: MediaArtifactInput{Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4"},
			body:  video, wantErr: ErrMediaContentTooLarge,
		},
		{
			name: "declared length mismatch", maxBytes: 4096,
			input: MediaArtifactInput{
				Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4", SizeBytes: int64(len(video) - 1),
			},
			body: video, wantErr: ErrMediaStorageIntegrity,
		},
		{
			name: "mime mismatch", maxBytes: 4096,
			input: MediaArtifactInput{Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/webm"},
			body:  video, wantErr: ErrInvalidMediaInput,
		},
		{
			name: "checksum mismatch", maxBytes: 4096,
			input: MediaArtifactInput{
				Direction: "output", MediaType: MediaTypeVideo, ContentType: "video/mp4",
				ChecksumSHA256: hex.EncodeToString(sum[:31]),
			},
			body: video, wantErr: ErrMediaStorageIntegrity,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			store, err := NewLocalMediaArtifactObjectStore(root, tt.maxBytes)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, store.Close()) })

			artifact, err := store.PutStream(context.Background(), tt.input, bytes.NewReader(tt.body))
			require.Nil(t, artifact)
			require.ErrorIs(t, err, tt.wantErr)
			require.Zero(t, countRegularFiles(t, root))
		})
	}
}

func TestCommitLocalMediaAtomicObject(t *testing.T) {
	t.Run("commits after directory sync", func(t *testing.T) {
		calls := make([]string, 0, 2)
		err := commitLocalMediaAtomicObject(localMediaAtomicCommitOperations{
			rename: func(oldName, newName string) error {
				calls = append(calls, "rename:"+oldName+":"+newName)
				return nil
			},
			remove: func(name string) error {
				calls = append(calls, "remove:"+name)
				return nil
			},
			syncDirectory: func(directory string) error {
				calls = append(calls, "sync:"+directory)
				return nil
			},
		}, "dir/.tmp", "dir/object", "dir")
		require.NoError(t, err)
		require.Equal(t, []string{"rename:dir/.tmp:dir/object", "sync:dir"}, calls)
	})

	t.Run("removes committed object and resyncs after first sync failure", func(t *testing.T) {
		firstSyncErr := errors.New("first directory sync failed")
		calls := make([]string, 0, 4)
		syncCalls := 0
		err := commitLocalMediaAtomicObject(localMediaAtomicCommitOperations{
			rename: func(oldName, newName string) error {
				calls = append(calls, "rename:"+oldName+":"+newName)
				return nil
			},
			remove: func(name string) error {
				calls = append(calls, "remove:"+name)
				return nil
			},
			syncDirectory: func(directory string) error {
				syncCalls++
				calls = append(calls, "sync:"+directory)
				if syncCalls == 1 {
					return firstSyncErr
				}
				return nil
			},
		}, "dir/.tmp", "dir/object", "dir")
		require.ErrorIs(t, err, firstSyncErr)
		require.Equal(t, []string{
			"rename:dir/.tmp:dir/object", "sync:dir", "remove:dir/object", "sync:dir",
		}, calls)
	})

	t.Run("preserves remove failure and does not claim durable rollback", func(t *testing.T) {
		firstSyncErr := errors.New("first directory sync failed")
		removeErr := errors.New("remove committed object failed")
		calls := make([]string, 0, 3)
		err := commitLocalMediaAtomicObject(localMediaAtomicCommitOperations{
			rename: func(oldName, newName string) error {
				calls = append(calls, "rename:"+oldName+":"+newName)
				return nil
			},
			remove: func(name string) error {
				calls = append(calls, "remove:"+name)
				return removeErr
			},
			syncDirectory: func(directory string) error {
				calls = append(calls, "sync:"+directory)
				return firstSyncErr
			},
		}, "dir/.tmp", "dir/object", "dir")
		require.ErrorIs(t, err, firstSyncErr)
		require.ErrorIs(t, err, removeErr)
		require.Equal(t, []string{
			"rename:dir/.tmp:dir/object", "sync:dir", "remove:dir/object",
		}, calls)
	})

	t.Run("preserves rollback directory sync failure", func(t *testing.T) {
		firstSyncErr := errors.New("first directory sync failed")
		rollbackSyncErr := errors.New("rollback directory sync failed")
		syncCalls := 0
		err := commitLocalMediaAtomicObject(localMediaAtomicCommitOperations{
			rename: func(string, string) error { return nil },
			remove: func(string) error { return nil },
			syncDirectory: func(string) error {
				syncCalls++
				if syncCalls == 1 {
					return firstSyncErr
				}
				return rollbackSyncErr
			},
		}, "dir/.tmp", "dir/object", "dir")
		require.ErrorIs(t, err, firstSyncErr)
		require.ErrorIs(t, err, rollbackSyncErr)
		require.Equal(t, 2, syncCalls)
	})

	t.Run("stops after rename failure", func(t *testing.T) {
		renameErr := errors.New("rename failed")
		removeCalls := 0
		syncCalls := 0
		err := commitLocalMediaAtomicObject(localMediaAtomicCommitOperations{
			rename: func(string, string) error { return renameErr },
			remove: func(string) error {
				removeCalls++
				return nil
			},
			syncDirectory: func(string) error {
				syncCalls++
				return nil
			},
		}, "dir/.tmp", "dir/object", "dir")
		require.ErrorIs(t, err, renameErr)
		require.Zero(t, removeCalls)
		require.Zero(t, syncCalls)
	})
}

func countRegularFiles(t *testing.T, root string) int {
	t.Helper()
	count := 0
	require.NoError(t, filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			count++
		}
		return nil
	}))
	return count
}

package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	localMediaDirectoryMode = 0o750
	localMediaFileMode      = 0o600
)

// LocalMediaArtifactObjectStore stores private media beneath one rooted file
// descriptor. os.Root prevents object keys and symlinks from escaping the
// configured directory.
type LocalMediaArtifactObjectStore struct {
	root            *os.Root
	rootPath        string
	maxContentBytes int64
}

func NewLocalMediaArtifactObjectStore(rootPath string, maxContentBytes int64) (*LocalMediaArtifactObjectStore, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return nil, errors.New("local media storage path is empty")
	}
	if maxContentBytes <= 0 {
		return nil, errors.New("local media max content bytes must be positive")
	}
	absolute, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve local media storage path: %w", err)
	}
	if err := os.MkdirAll(absolute, localMediaDirectoryMode); err != nil {
		return nil, fmt.Errorf("create local media storage path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat local media storage path: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("local media storage path is not a directory")
	}
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, fmt.Errorf("open local media storage root: %w", err)
	}
	store := &LocalMediaArtifactObjectStore{root: root, rootPath: absolute, maxContentBytes: maxContentBytes}
	if err := store.Check(context.Background()); err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("check local media storage: %w", err)
	}
	return store, nil
}

func (s *LocalMediaArtifactObjectStore) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}

func (s *LocalMediaArtifactObjectStore) Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.root == nil {
		return nil, ErrMediaContentUnavailable
	}
	data, contentType, checksum, err := validateMediaObjectInput(input, s.maxContentBytes)
	if err != nil {
		return nil, err
	}
	objectKey, err := newLocalMediaObjectKey(input.Direction, contentType)
	if err != nil {
		return nil, err
	}
	if err := s.writeAtomic(ctx, objectKey, data); err != nil {
		return nil, err
	}
	return mediaArtifactFromStoredInput(
		input, MediaStorageProviderLocal, objectKey, contentType, checksum, int64(len(data)),
	), nil
}

func (s *LocalMediaArtifactObjectStore) PutStream(
	ctx context.Context,
	input MediaArtifactInput,
	body io.Reader,
) (*MediaArtifact, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.root == nil {
		return nil, ErrMediaContentUnavailable
	}
	stream, contentType, err := newValidatedMediaObjectStream(ctx, input, body, s.maxContentBytes)
	if err != nil {
		return nil, err
	}
	objectKey, err := newLocalMediaObjectKey(input.Direction, contentType)
	if err != nil {
		return nil, err
	}
	size, checksum, err := s.writeAtomicStream(ctx, objectKey, stream)
	if err != nil {
		return nil, err
	}
	return mediaArtifactFromStoredInput(
		input, MediaStorageProviderLocal, objectKey, contentType, checksum, size,
	), nil
}

func (s *LocalMediaArtifactObjectStore) Open(
	ctx context.Context,
	artifact *MediaArtifact,
	byteRange string,
) (*MediaContent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.root == nil || artifact == nil ||
		artifact.StorageProvider != MediaStorageProviderLocal || !isSafeMediaObjectKey(artifact.ObjectKey) {
		return nil, ErrMediaArtifactNotFound
	}
	if byteRange != "" {
		if err := ValidateMediaRange(byteRange); err != nil {
			return nil, err
		}
	}
	objectName := filepath.FromSlash(artifact.ObjectKey)
	info, err := s.root.Lstat(objectName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMediaArtifactNotFound
		}
		return nil, fmt.Errorf("stat local media object: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, ErrMediaArtifactNotFound
	}
	if artifact.SizeBytes > 0 && artifact.SizeBytes != info.Size() {
		return nil, fmt.Errorf("%w: local media object size differs from artifact", ErrMediaStorageIntegrity)
	}
	contentType := normalizeStoredMediaContentType(artifact.ContentType, artifact.MediaType)
	if contentType == "application/octet-stream" {
		return nil, ErrInvalidMediaInput
	}
	file, err := s.root.Open(objectName)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrMediaArtifactNotFound
		}
		return nil, fmt.Errorf("open local media object: %w", err)
	}
	if byteRange == "" {
		return &MediaContent{
			Body: file, StatusCode: http.StatusOK, ContentType: contentType,
			ContentLength: info.Size(), AcceptRanges: "bytes",
		}, nil
	}
	start, end, err := resolveMediaByteRange(byteRange, info.Size())
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	length := end - start + 1
	return &MediaContent{
		Body: &localMediaSectionReadCloser{
			SectionReader: io.NewSectionReader(file, start, length),
			file:          file,
		},
		StatusCode: http.StatusPartialContent, ContentType: contentType, ContentLength: length,
		ContentRange: fmt.Sprintf("bytes %d-%d/%d", start, end, info.Size()), AcceptRanges: "bytes",
	}, nil
}

func (s *LocalMediaArtifactObjectStore) Discard(ctx context.Context, input MediaArtifactInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.root == nil || input.StorageProvider != MediaStorageProviderLocal ||
		!isSafeMediaObjectKey(input.ObjectKey) {
		return ErrMediaArtifactNotFound
	}
	err := s.root.Remove(filepath.FromSlash(input.ObjectKey))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete local media object: %w", err)
	}
	if err := syncLocalMediaDirectory(s.root, filepath.FromSlash(path.Dir(input.ObjectKey))); err != nil {
		return fmt.Errorf("sync local media directory after delete: %w", err)
	}
	return nil
}

func (s *LocalMediaArtifactObjectStore) Check(ctx context.Context) (returnErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.root == nil {
		return ErrMediaContentUnavailable
	}
	if err := s.root.MkdirAll(".health", localMediaDirectoryMode); err != nil {
		return fmt.Errorf("create local media health directory: %w", err)
	}
	token, err := randomMediaStorageToken()
	if err != nil {
		return err
	}
	name := filepath.Join(".health", ".probe-"+token)
	payload := []byte("fluxcode-media-storage-check")
	file, err := s.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, localMediaFileMode)
	if err != nil {
		return fmt.Errorf("create local media health object: %w", err)
	}
	defer func() {
		if removeErr := s.root.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			returnErr = errors.Join(returnErr, fmt.Errorf("delete local media health object: %w", removeErr))
		}
	}()
	if err := writeLocalMediaData(ctx, file, payload); err != nil {
		_ = file.Close()
		return fmt.Errorf("write local media health object: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync local media health object: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close local media health object: %w", err)
	}
	read, err := s.root.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read local media health object: %w", err)
	}
	if !bytes.Equal(read, payload) {
		return ErrMediaStorageIntegrity
	}
	return nil
}

func (s *LocalMediaArtifactObjectStore) writeAtomic(ctx context.Context, objectKey string, data []byte) (returnErr error) {
	if !isSafeMediaObjectKey(objectKey) {
		return ErrInvalidMediaObjectKey
	}
	objectName := filepath.FromSlash(objectKey)
	directory := filepath.Dir(objectName)
	if err := s.root.MkdirAll(directory, localMediaDirectoryMode); err != nil {
		return fmt.Errorf("create local media object directory: %w", err)
	}
	token, err := randomMediaStorageToken()
	if err != nil {
		return err
	}
	temporary := filepath.Join(directory, ".tmp-"+token)
	file, err := s.root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, localMediaFileMode)
	if err != nil {
		return fmt.Errorf("create local media temporary object: %w", err)
	}
	temporaryExists := true
	defer func() {
		if temporaryExists {
			if removeErr := s.root.Remove(temporary); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				returnErr = errors.Join(returnErr, fmt.Errorf("clean local media temporary object: %w", removeErr))
			}
		}
	}()
	if err := writeLocalMediaData(ctx, file, data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write local media object: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync local media object: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close local media object: %w", err)
	}
	if err := s.commitAtomicObject(temporary, objectName, directory); err != nil {
		return err
	}
	temporaryExists = false
	return nil
}

func (s *LocalMediaArtifactObjectStore) writeAtomicStream(
	ctx context.Context,
	objectKey string,
	stream *validatedMediaObjectStream,
) (size int64, checksum string, returnErr error) {
	if !isSafeMediaObjectKey(objectKey) || stream == nil {
		return 0, "", ErrInvalidMediaObjectKey
	}
	objectName := filepath.FromSlash(objectKey)
	directory := filepath.Dir(objectName)
	if err := s.root.MkdirAll(directory, localMediaDirectoryMode); err != nil {
		return 0, "", fmt.Errorf("create local media object directory: %w", err)
	}
	token, err := randomMediaStorageToken()
	if err != nil {
		return 0, "", err
	}
	temporary := filepath.Join(directory, ".tmp-"+token)
	file, err := s.root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, localMediaFileMode)
	if err != nil {
		return 0, "", fmt.Errorf("create local media temporary object: %w", err)
	}
	temporaryExists := true
	defer func() {
		if temporaryExists {
			if removeErr := s.root.Remove(temporary); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				returnErr = errors.Join(returnErr, fmt.Errorf("clean local media temporary object: %w", removeErr))
			}
		}
	}()
	buffer := make([]byte, 128<<10)
	if _, err := io.CopyBuffer(file, stream, buffer); err != nil {
		_ = file.Close()
		return 0, "", fmt.Errorf("stream local media object: %w", err)
	}
	size, checksum, err = stream.Finalize()
	if err != nil {
		_ = file.Close()
		return 0, "", fmt.Errorf("verify local media object stream: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = file.Close()
		return 0, "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return 0, "", fmt.Errorf("sync local media object: %w", err)
	}
	if err := file.Close(); err != nil {
		return 0, "", fmt.Errorf("close local media object: %w", err)
	}
	if err := s.commitAtomicObject(temporary, objectName, directory); err != nil {
		return 0, "", err
	}
	temporaryExists = false
	return size, checksum, nil
}

type localMediaAtomicCommitOperations struct {
	rename        func(string, string) error
	remove        func(string) error
	syncDirectory func(string) error
}

func (s *LocalMediaArtifactObjectStore) commitAtomicObject(temporary, objectName, directory string) error {
	return commitLocalMediaAtomicObject(localMediaAtomicCommitOperations{
		rename: s.root.Rename,
		remove: s.root.Remove,
		syncDirectory: func(directory string) error {
			return syncLocalMediaDirectory(s.root, directory)
		},
	}, temporary, objectName, directory)
}

func commitLocalMediaAtomicObject(
	operations localMediaAtomicCommitOperations,
	temporary string,
	objectName string,
	directory string,
) error {
	if err := operations.rename(temporary, objectName); err != nil {
		return fmt.Errorf("commit local media object: %w", err)
	}
	syncErr := operations.syncDirectory(directory)
	if syncErr == nil {
		return nil
	}

	removeErr := operations.remove(objectName)
	var rollbackSyncErr error
	if removeErr == nil {
		rollbackSyncErr = operations.syncDirectory(directory)
	}
	return errors.Join(
		fmt.Errorf("sync local media object directory: %w", syncErr),
		wrapLocalMediaAtomicCleanupError("remove local media object after directory sync failure", removeErr),
		wrapLocalMediaAtomicCleanupError("sync local media object directory after rollback", rollbackSyncErr),
	)
}

func wrapLocalMediaAtomicCleanupError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func newLocalMediaObjectKey(direction, contentType string) (string, error) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "input" && direction != "output" {
		direction = "staging"
	}
	token, err := randomMediaStorageToken()
	if err != nil {
		return "", err
	}
	key := path.Join("tasks", time.Now().UTC().Format("2006/01/02"), direction, token+mediaObjectExtension(contentType))
	if !isSafeMediaObjectKey(key) {
		return "", ErrInvalidMediaObjectKey
	}
	return key, nil
}

func randomMediaStorageToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate media storage key: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func writeLocalMediaData(ctx context.Context, file *os.File, data []byte) error {
	const chunkSize = 128 << 10
	for offset := 0; offset < len(data); {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		written, err := file.Write(data[offset:end])
		if err != nil {
			return err
		}
		if written <= 0 {
			return io.ErrShortWrite
		}
		offset += written
	}
	return nil
}

func syncLocalMediaDirectory(root *os.Root, directory string) error {
	file, err := root.Open(directory)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func resolveMediaByteRange(value string, total int64) (int64, int64, error) {
	parsed, err := parseMediaByteRange(value)
	if err != nil {
		return 0, 0, err
	}
	if total <= 0 {
		return 0, 0, ErrMediaRangeNotSatisfiable
	}
	if !parsed.hasStart {
		suffix := parsed.end
		if suffix > total {
			suffix = total
		}
		return total - suffix, total - 1, nil
	}
	if parsed.start >= total {
		return 0, 0, ErrMediaRangeNotSatisfiable
	}
	end := total - 1
	if parsed.hasEnd && parsed.end < end {
		end = parsed.end
	}
	return parsed.start, end, nil
}

type localMediaSectionReadCloser struct {
	*io.SectionReader
	file *os.File
}

func (r *localMediaSectionReadCloser) Close() error {
	if r == nil || r.file == nil {
		return nil
	}
	return r.file.Close()
}

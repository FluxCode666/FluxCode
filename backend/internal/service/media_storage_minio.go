package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type mediaS3Client interface {
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
	HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

type MinIOMediaArtifactObjectStore struct {
	client          mediaS3Client
	bucket          string
	prefix          string
	maxContentBytes int64
}

func NewMinIOMediaArtifactObjectStore(
	ctx context.Context,
	cfg MediaMinIOConfig,
	maxContentBytes int64,
) (*MinIOMediaArtifactObjectStore, error) {
	wrapper := MediaStorageConfig{Provider: MediaStorageProviderMinIO, LocalPath: ".", MinIO: cfg}
	if err := normalizeAndValidateMediaStorageConfig(&wrapper, true); err != nil {
		return nil, invalidMediaStorageConfig(err)
	}
	cfg = wrapper.MinIO
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load MinIO client config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(cfg.Endpoint)
		options.UsePathStyle = cfg.ForcePathStyle
		options.APIOptions = append(options.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	return newMinIOMediaArtifactObjectStoreWithClient(client, cfg, maxContentBytes)
}

func newMinIOMediaArtifactObjectStoreWithClient(
	client mediaS3Client,
	cfg MediaMinIOConfig,
	maxContentBytes int64,
) (*MinIOMediaArtifactObjectStore, error) {
	if client == nil {
		return nil, errors.New("MinIO media client is nil")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, errors.New("MinIO media bucket is empty")
	}
	if maxContentBytes <= 0 {
		return nil, errors.New("MinIO media max content bytes must be positive")
	}
	prefix := strings.Trim(strings.TrimSpace(cfg.Prefix), "/")
	if prefix == "" {
		prefix = defaultMediaStoragePrefix
	}
	if !isSafeMediaObjectKey(prefix + "/probe") {
		return nil, errors.New("MinIO media prefix is invalid")
	}
	return &MinIOMediaArtifactObjectStore{
		client: client, bucket: strings.TrimSpace(cfg.Bucket), prefix: prefix, maxContentBytes: maxContentBytes,
	}, nil
}

func (s *MinIOMediaArtifactObjectStore) Put(ctx context.Context, input MediaArtifactInput) (*MediaArtifact, error) {
	if s == nil || s.client == nil {
		return nil, ErrMediaContentUnavailable
	}
	data, contentType, checksum, err := validateMediaObjectInput(input, s.maxContentBytes)
	if err != nil {
		return nil, err
	}
	key, err := s.newObjectKey(input, contentType)
	if err != nil {
		return nil, err
	}
	contentLength := int64(len(data))
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(data),
		ContentLength: &contentLength, ContentType: aws.String(contentType),
		Metadata: map[string]string{"sha256": checksum, "media-type": string(input.MediaType)},
	})
	if err != nil {
		return nil, fmt.Errorf("put MinIO media object: %w", err)
	}
	return mediaArtifactFromStoredInput(input, MediaStorageProviderMinIO, key, contentType, checksum, contentLength), nil
}

func (s *MinIOMediaArtifactObjectStore) PutStream(
	ctx context.Context,
	input MediaArtifactInput,
	body io.Reader,
) (*MediaArtifact, error) {
	if s == nil || s.client == nil {
		return nil, ErrMediaContentUnavailable
	}
	stream, contentType, err := newValidatedMediaObjectStream(ctx, input, body, s.maxContentBytes)
	if err != nil {
		return nil, err
	}
	key, err := s.newObjectKey(input, contentType)
	if err != nil {
		return nil, err
	}
	metadata := map[string]string{"media-type": string(input.MediaType)}
	if checksum := strings.ToLower(strings.TrimSpace(input.ChecksumSHA256)); checksum != "" {
		metadata["sha256"] = checksum
	}
	putInput := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: stream,
		ContentType: aws.String(contentType), Metadata: metadata,
	}
	if input.SizeBytes > 0 {
		contentLength := input.SizeBytes
		putInput.ContentLength = &contentLength
	}
	if _, err := s.client.PutObject(ctx, putInput); err != nil {
		cleanupErr := s.cleanupFailedPut(ctx, key)
		return nil, errors.Join(fmt.Errorf("put MinIO media object stream: %w", err), cleanupErr)
	}
	size, checksum, err := stream.Finalize()
	if err != nil {
		cleanupErr := s.cleanupFailedPut(ctx, key)
		return nil, errors.Join(fmt.Errorf("verify MinIO media object stream: %w", err), cleanupErr)
	}
	return mediaArtifactFromStoredInput(input, MediaStorageProviderMinIO, key, contentType, checksum, size), nil
}

func (s *MinIOMediaArtifactObjectStore) cleanupFailedPut(ctx context.Context, key string) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_, err := s.client.DeleteObject(cleanupCtx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete failed MinIO media object: %w", err)
	}
	return nil
}

func (s *MinIOMediaArtifactObjectStore) Open(ctx context.Context, artifact *MediaArtifact, byteRange string) (*MediaContent, error) {
	if s == nil || s.client == nil || artifact == nil {
		return nil, ErrMediaArtifactNotFound
	}
	if artifact.StorageProvider != MediaStorageProviderMinIO || !s.ownsKey(artifact.ObjectKey) {
		return nil, ErrMediaArtifactNotFound
	}
	if byteRange != "" {
		if err := ValidateMediaRange(byteRange); err != nil {
			return nil, err
		}
	}
	input := &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(artifact.ObjectKey)}
	if byteRange != "" {
		input.Range = aws.String(byteRange)
	}
	result, err := s.client.GetObject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("get MinIO media object: %w", err)
	}
	if result == nil || result.Body == nil {
		return nil, ErrMediaContentUnavailable
	}
	contentLength := aws.ToInt64(result.ContentLength)
	if contentLength < 0 || contentLength > s.maxContentBytes ||
		(artifact.SizeBytes > 0 && contentLength > artifact.SizeBytes) {
		_ = result.Body.Close()
		return nil, ErrMediaContentTooLarge
	}
	fullObject := byteRange == "" && aws.ToString(result.ContentRange) == ""
	if fullObject && artifact.SizeBytes > 0 && contentLength > 0 && contentLength != artifact.SizeBytes {
		_ = result.Body.Close()
		return nil, fmt.Errorf("%w: MinIO media object size differs from artifact", ErrMediaStorageIntegrity)
	}
	if fullObject && artifact.ChecksumSHA256 != "" {
		for key, value := range result.Metadata {
			if strings.EqualFold(key, "sha256") && value != "" && !strings.EqualFold(value, artifact.ChecksumSHA256) {
				_ = result.Body.Close()
				return nil, fmt.Errorf("%w: MinIO media object checksum metadata differs from artifact", ErrMediaStorageIntegrity)
			}
		}
	}
	expectedLength := int64(-1)
	checksum := ""
	if fullObject {
		if artifact.SizeBytes > 0 {
			expectedLength = artifact.SizeBytes
		}
		checksum = artifact.ChecksumSHA256
	} else if contentLength > 0 {
		expectedLength = contentLength
	}
	body := newVerifiedMediaReadCloser(result.Body, s.maxContentBytes, expectedLength, checksum)
	contentType := normalizeStoredMediaContentType(aws.ToString(result.ContentType), artifact.MediaType)
	status := http.StatusOK
	if byteRange != "" || aws.ToString(result.ContentRange) != "" {
		status = http.StatusPartialContent
	}
	if contentLength <= 0 && fullObject && artifact.SizeBytes > 0 {
		contentLength = artifact.SizeBytes
	}
	return &MediaContent{
		Body: body, StatusCode: status, ContentType: contentType,
		ContentLength: contentLength, ContentRange: aws.ToString(result.ContentRange),
		AcceptRanges: aws.ToString(result.AcceptRanges),
	}, nil
}

type verifiedMediaReadCloser struct {
	body            io.ReadCloser
	maximum         int64
	expected        int64
	read            int64
	hasher          hash.Hash
	wantChecksum    string
	verificationErr error
}

func newVerifiedMediaReadCloser(body io.ReadCloser, maximum, expected int64, checksum string) io.ReadCloser {
	reader := &verifiedMediaReadCloser{
		body: body, maximum: maximum, expected: expected,
		wantChecksum: strings.ToLower(strings.TrimSpace(checksum)),
	}
	if reader.wantChecksum != "" {
		reader.hasher = sha256.New()
	}
	return reader
}

func (r *verifiedMediaReadCloser) Read(buffer []byte) (int, error) {
	if r == nil || r.body == nil {
		return 0, ErrMediaContentUnavailable
	}
	if r.verificationErr != nil {
		return 0, r.verificationErr
	}
	limit := r.maximum
	if r.expected >= 0 && r.expected < limit {
		limit = r.expected
	}
	if r.read >= limit {
		var probe [1]byte
		n, err := r.body.Read(probe[:])
		if n > 0 {
			if r.expected >= 0 {
				r.verificationErr = fmt.Errorf("%w: media object exceeds expected size", ErrMediaStorageIntegrity)
			} else {
				r.verificationErr = ErrMediaContentTooLarge
			}
			return 0, r.verificationErr
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		if err == nil {
			return 0, nil
		}
		return 0, r.verifyEOF()
	}
	remaining := limit - r.read
	if int64(len(buffer)) > remaining {
		buffer = buffer[:remaining]
	}
	n, err := r.body.Read(buffer)
	if n > 0 {
		r.read += int64(n)
		if r.hasher != nil {
			_, _ = r.hasher.Write(buffer[:n])
		}
	}
	if errors.Is(err, io.EOF) {
		return n, r.verifyEOF()
	}
	return n, err
}

func (r *verifiedMediaReadCloser) verifyEOF() error {
	if r.expected >= 0 && r.read != r.expected {
		r.verificationErr = fmt.Errorf("%w: media object ended before expected size", ErrMediaStorageIntegrity)
		return r.verificationErr
	}
	if r.hasher != nil && !strings.EqualFold(hex.EncodeToString(r.hasher.Sum(nil)), r.wantChecksum) {
		r.verificationErr = fmt.Errorf("%w: media object checksum differs from artifact", ErrMediaStorageIntegrity)
		return r.verificationErr
	}
	return io.EOF
}

func (r *verifiedMediaReadCloser) Close() error {
	if r == nil || r.body == nil {
		return nil
	}
	return r.body.Close()
}

func (s *MinIOMediaArtifactObjectStore) Discard(ctx context.Context, input MediaArtifactInput) error {
	if s == nil || s.client == nil {
		return ErrMediaContentUnavailable
	}
	if input.StorageProvider != MediaStorageProviderMinIO || !s.ownsKey(input.ObjectKey) {
		return ErrMediaArtifactNotFound
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(input.ObjectKey)})
	if err != nil {
		return fmt.Errorf("delete MinIO media object: %w", err)
	}
	return nil
}

func (s *MinIOMediaArtifactObjectStore) Check(ctx context.Context) (err error) {
	if s == nil || s.client == nil {
		return ErrMediaContentUnavailable
	}
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("check MinIO media bucket: %w", err)
	}
	key, err := s.randomKey("health", ".txt")
	if err != nil {
		return err
	}
	payload := []byte("fluxcode-media-storage-check")
	length := int64(len(payload))
	if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: bytes.NewReader(payload),
		ContentLength: &length, ContentType: aws.String("text/plain"),
	}); err != nil {
		return fmt.Errorf("write MinIO media health object: %w", err)
	}
	defer func() {
		_, deleteErr := s.client.DeleteObject(context.WithoutCancel(ctx), &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
		if deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("delete MinIO media health object: %w", deleteErr))
		}
	}()
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("read MinIO media health object: %w", err)
	}
	if result == nil || result.Body == nil {
		return errors.New("MinIO media health read returned an empty body")
	}
	defer result.Body.Close() //nolint:errcheck
	read, err := io.ReadAll(io.LimitReader(result.Body, int64(len(payload))+1))
	if err != nil {
		return fmt.Errorf("read MinIO media health body: %w", err)
	}
	if !bytes.Equal(read, payload) {
		return errors.New("MinIO media health object content mismatch")
	}
	return nil
}

func (s *MinIOMediaArtifactObjectStore) newObjectKey(input MediaArtifactInput, contentType string) (string, error) {
	direction := strings.ToLower(strings.TrimSpace(input.Direction))
	if direction != "input" && direction != "output" {
		direction = "staging"
	}
	return s.randomKey(direction, mediaObjectExtension(contentType))
}

func (s *MinIOMediaArtifactObjectStore) randomKey(segment, extension string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate media object key: %w", err)
	}
	key := path.Join(s.prefix, time.Now().UTC().Format("2006/01/02"), segment, hex.EncodeToString(random[:])+extension)
	if !s.ownsKey(key) {
		return "", errors.New("generated MinIO media key is invalid")
	}
	return key, nil
}

func (s *MinIOMediaArtifactObjectStore) ownsKey(key string) bool {
	return s != nil && isSafeMediaObjectKey(key) && strings.HasPrefix(key, s.prefix+"/")
}

func validateMediaObjectInput(input MediaArtifactInput, maxBytes int64) ([]byte, string, string, error) {
	if input.MediaType != MediaTypeImage && input.MediaType != MediaTypeVideo {
		return nil, "", "", ErrInvalidMediaInput
	}
	if len(input.Data) == 0 {
		return nil, "", "", ErrInvalidMediaInput
	}
	if maxBytes <= 0 || int64(len(input.Data)) > maxBytes {
		return nil, "", "", ErrMediaContentTooLarge
	}
	if input.SizeBytes > 0 && input.SizeBytes != int64(len(input.Data)) {
		return nil, "", "", ErrInvalidMediaInput
	}
	contentType := normalizeStoredMediaContentType(input.ContentType, input.MediaType)
	if contentType == "application/octet-stream" {
		return nil, "", "", ErrInvalidMediaInput
	}
	detected := detectMediaObjectContentType(input.Data, input.MediaType, contentType)
	if detected == "application/octet-stream" || detected != contentType {
		return nil, "", "", ErrInvalidMediaInput
	}
	sum := sha256.Sum256(input.Data)
	checksum := hex.EncodeToString(sum[:])
	if input.ChecksumSHA256 != "" && !strings.EqualFold(strings.TrimSpace(input.ChecksumSHA256), checksum) {
		return nil, "", "", ErrInvalidMediaInput
	}
	return input.Data, contentType, checksum, nil
}

func detectMediaObjectContentType(data []byte, mediaType MediaType, declared string) string {
	detected := normalizeStoredMediaContentType(http.DetectContentType(data), mediaType)
	if detected != "application/octet-stream" {
		return detected
	}
	// net/http does not recognize ISO Base Media files. Both MP4 and QuickTime
	// use an ftyp box, so the already-normalized declared type disambiguates
	// them after the common container signature has been verified.
	if mediaType == MediaTypeVideo && len(data) >= 12 && string(data[4:8]) == "ftyp" &&
		(declared == "video/mp4" || declared == "video/quicktime") {
		return declared
	}
	return "application/octet-stream"
}

func normalizeStoredMediaContentType(value string, mediaType MediaType) string {
	if mediaType == MediaTypeImage {
		contentType, _ := NormalizeImageContentType(value)
		return contentType
	}
	if mediaType == MediaTypeVideo {
		contentType, _ := NormalizeVideoContentType(value)
		return contentType
	}
	return "application/octet-stream"
}

func mediaObjectExtension(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/quicktime":
		return ".mov"
	default:
		return ".bin"
	}
}

func isSafeMediaObjectKey(key string) bool {
	key = strings.TrimSpace(key)
	return key != "" && !strings.HasPrefix(key, "/") && !strings.Contains(key, "\\") &&
		!strings.ContainsRune(key, '\x00') && path.Clean(key) == key && key != "." && !strings.HasPrefix(key, "../")
}

func mediaArtifactFromStoredInput(
	input MediaArtifactInput,
	provider, objectKey, contentType, checksum string,
	size int64,
) *MediaArtifact {
	artifact := &MediaArtifact{
		Direction: input.Direction, Position: input.Position, MediaType: input.MediaType,
		ContentType: contentType, SizeBytes: size, ChecksumSHA256: checksum,
		StorageProvider: provider, StorageStatus: "stored", ObjectKey: objectKey,
		Resolution: input.Resolution,
	}
	if input.Width > 0 {
		artifact.Width = mediaIntPointer(input.Width)
	}
	if input.Height > 0 {
		artifact.Height = mediaIntPointer(input.Height)
	}
	if input.DurationSeconds > 0 {
		artifact.DurationSeconds = mediaFloatPointer(input.DurationSeconds)
	}
	if input.FPS > 0 {
		artifact.FPS = mediaFloatPointer(input.FPS)
	}
	return artifact
}

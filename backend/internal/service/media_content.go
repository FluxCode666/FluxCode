package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	ErrInvalidMediaInput                = errors.New("invalid media input")
	ErrInvalidMediaRange                = errors.New("invalid media range")
	ErrMediaRangeNotSatisfiable         = errors.New("media range not satisfiable")
	ErrMediaArtifactNotFound            = errors.New("media artifact not found")
	ErrMediaContentUnavailable          = errors.New("media content unavailable")
	ErrMediaContentAccountRequired      = errors.New("media content account required")
	ErrMediaContentTooLarge             = errors.New("media content too large")
	ErrMediaArtifactObjectStoreDisabled = errors.New("media artifact object store disabled")
	ErrMediaVideoObjectStorageRequired  = errors.New("media video object storage required")
)

var singleMediaByteRangePattern = regexp.MustCompile(`^bytes=(?:[0-9]+-[0-9]*|-[0-9]+)$`)

const maxInlineMediaDecodedBytes = 1 << 20

type MediaContentTaskRepository interface {
	GetByPublicIDForUser(ctx context.Context, publicID string, userID int64) (*MediaTask, error)
}

type MediaContentAccountRepository interface {
	GetByID(ctx context.Context, id int64) (*Account, error)
}

type DisabledMediaArtifactObjectStore struct{}

func NewDisabledMediaArtifactObjectStore() *DisabledMediaArtifactObjectStore {
	return &DisabledMediaArtifactObjectStore{}
}

func (*DisabledMediaArtifactObjectStore) Put(context.Context, MediaArtifactInput) (*MediaArtifact, error) {
	return nil, ErrMediaArtifactObjectStoreDisabled
}

func (*DisabledMediaArtifactObjectStore) Open(context.Context, *MediaArtifact, string) (*MediaContent, error) {
	return nil, ErrMediaArtifactObjectStoreDisabled
}

func (*DisabledMediaArtifactObjectStore) Discard(context.Context, MediaArtifactInput) error {
	return ErrMediaArtifactObjectStoreDisabled
}

type MediaContentService struct {
	tasks       MediaContentTaskRepository
	artifacts   MediaArtifactRepository
	settings    MediaSettingsProvider
	accounts    MediaContentAccountRepository
	adapters    *MediaAdapterRegistry
	httpReader  MediaHTTPContentReader
	objectStore MediaArtifactObjectStore
}

func NewMediaContentService(
	tasks MediaContentTaskRepository,
	artifacts MediaArtifactRepository,
	settings MediaSettingsProvider,
	accounts MediaContentAccountRepository,
	adapters *MediaAdapterRegistry,
	httpReader MediaHTTPContentReader,
	objectStore MediaArtifactObjectStore,
) *MediaContentService {
	return &MediaContentService{
		tasks: tasks, artifacts: artifacts, settings: settings, accounts: accounts,
		adapters: adapters, httpReader: httpReader, objectStore: objectStore,
	}
}

func (s *MediaContentService) Stage(ctx context.Context, _ int64, input MediaArtifactInput) (MediaArtifactInput, error) {
	if err := ctx.Err(); err != nil {
		return MediaArtifactInput{}, err
	}
	if input.ExternalURL != "" {
		if s == nil || s.httpReader == nil {
			return MediaArtifactInput{}, ErrInvalidMediaInput
		}
		normalized, err := s.httpReader.ValidateURL(input.ExternalURL)
		if err != nil {
			return MediaArtifactInput{}, fmt.Errorf("validate external media input: %w", err)
		}
		contentType, err := mediaContentTypeFromExternalURL(normalized, input.MediaType)
		if err != nil {
			return MediaArtifactInput{}, err
		}
		input.ExternalURL = normalized
		input.ContentType = contentType
		input.Data = nil
		return input, nil
	}
	if len(input.Data) == 0 {
		return MediaArtifactInput{}, ErrInvalidMediaInput
	}
	if input.SizeBytes <= 0 {
		input.SizeBytes = int64(len(input.Data))
	}
	if input.ChecksumSHA256 == "" {
		sum := sha256.Sum256(input.Data)
		input.ChecksumSHA256 = hex.EncodeToString(sum[:])
	}
	if s == nil || s.objectStore == nil {
		if input.MediaType == MediaTypeVideo {
			return MediaArtifactInput{}, ErrMediaVideoObjectStorageRequired
		}
		return MediaArtifactInput{}, ErrMediaArtifactObjectStoreDisabled
	}
	stored, err := s.objectStore.Put(ctx, input)
	if err != nil {
		if input.MediaType == MediaTypeVideo && errors.Is(err, ErrMediaArtifactObjectStoreDisabled) {
			return MediaArtifactInput{}, ErrMediaVideoObjectStorageRequired
		}
		return MediaArtifactInput{}, fmt.Errorf("stage media input: %w", err)
	}
	if stored == nil || (stored.ObjectKey == "" && stored.UpstreamReference == "") {
		return MediaArtifactInput{}, ErrInvalidMediaInput
	}
	if stored.Direction == "" {
		stored.Direction = input.Direction
	}
	stored.Position = input.Position
	if stored.MediaType == "" {
		stored.MediaType = input.MediaType
	}
	if stored.ContentType == "" {
		stored.ContentType = input.ContentType
	}
	if stored.SizeBytes <= 0 {
		stored.SizeBytes = input.SizeBytes
	}
	if stored.ChecksumSHA256 == "" {
		stored.ChecksumSHA256 = input.ChecksumSHA256
	}
	return mediaArtifactInputFromStored(stored), nil
}

func (s *MediaContentService) Discard(ctx context.Context, _ int64, input MediaArtifactInput) error {
	if input.ExternalURL != "" && input.ObjectKey == "" && input.UpstreamReference == "" {
		return nil
	}
	if input.ObjectKey == "" && input.UpstreamReference == "" {
		return nil
	}
	if s == nil || s.objectStore == nil {
		return ErrMediaArtifactObjectStoreDisabled
	}
	if err := s.objectStore.Discard(ctx, input); err != nil {
		return fmt.Errorf("discard staged media input: %w", err)
	}
	return nil
}

func mediaContentTypeFromExternalURL(raw string, mediaType MediaType) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return "", ErrInvalidMediaInput
	}
	extension := strings.ToLower(path.Ext(parsed.Path))
	var contentType string
	switch extension {
	case ".png":
		contentType = "image/png"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
	case ".webp":
		contentType = "image/webp"
	case ".mp4":
		contentType = "video/mp4"
	case ".webm":
		contentType = "video/webm"
	case ".mov":
		contentType = "video/quicktime"
	default:
		return "", ErrInvalidMediaInput
	}
	if !strings.HasPrefix(contentType, string(mediaType)+"/") {
		return "", ErrInvalidMediaInput
	}
	return contentType, nil
}

func (s *MediaContentService) PersistOutputs(ctx context.Context, task *MediaTask, inputs []MediaArtifactInput) ([]MediaArtifact, error) {
	if s == nil || task == nil || s.artifacts == nil || s.settings == nil {
		return nil, ErrMediaContentUnavailable
	}
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load media content settings: %w", err)
	}
	if settings == nil {
		return nil, ErrMediaContentUnavailable
	}
	stored := make([]MediaArtifact, 0, len(inputs))
	for i := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input := inputs[i]
		input.Direction = "output"
		input.Position = i
		if s.objectStore != nil {
			artifact, putErr := s.objectStore.Put(ctx, input)
			if putErr == nil && artifact != nil && artifact.ObjectKey != "" {
				mergeStoredArtifactMetadata(artifact, input)
				artifact.TaskID = task.ID
				artifact.Direction = "output"
				artifact.Position = i
				artifact.StorageStatus = "stored"
				created, createErr := s.artifacts.Create(ctx, artifact)
				if createErr != nil {
					return nil, fmt.Errorf("persist stored media output: %w", createErr)
				}
				stored = append(stored, *created)
				continue
			}
		}

		if strings.HasPrefix(strings.ToLower(input.UpstreamReference), "data:") {
			_, _, _, inlineErr := decodeMediaDataReferenceBounded(
				input.UpstreamReference, input.ContentType, maxInlineMediaDecodedBytes,
			)
			if inlineErr != nil {
				return nil, inlineErr
			}
		}
		proxyAllowed := input.MediaType == MediaTypeImage || settings.MediaVideoProxyFallbackEnabled
		if !proxyAllowed || (input.UpstreamReference == "" && input.ExternalURL == "") {
			return nil, ErrMediaContentUnavailable
		}
		if input.ExternalURL != "" {
			if s.httpReader == nil {
				return nil, ErrMediaContentUnavailable
			}
			normalized, validateErr := s.httpReader.ValidateURL(input.ExternalURL)
			if validateErr != nil {
				return nil, ErrMediaContentUnavailable
			}
			input.ExternalURL = normalized
		}
		created, createErr := s.artifacts.Create(ctx, artifactFromProxyInput(task.ID, input))
		if createErr != nil {
			return nil, fmt.Errorf("persist proxy media output: %w", createErr)
		}
		stored = append(stored, *created)
	}
	return stored, nil
}

func (s *MediaContentService) OpenVideo(ctx context.Context, publicID string, userID int64, byteRange string) (*MediaContent, error) {
	if s == nil || s.tasks == nil || s.artifacts == nil {
		return nil, ErrMediaContentUnavailable
	}
	if byteRange != "" {
		if err := ValidateMediaRange(byteRange); err != nil {
			return nil, err
		}
	}
	task, err := s.tasks.GetByPublicIDForUser(ctx, strings.TrimSpace(publicID), userID)
	if err != nil || task == nil || task.MediaType != MediaTypeVideo || task.Status != MediaTaskStatusCompleted {
		return nil, ErrMediaTaskNotFound
	}
	artifacts, err := s.artifacts.ListByTaskID(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("list video artifacts: %w", err)
	}
	artifact := firstOutputVideo(artifacts)
	if artifact == nil {
		return nil, ErrMediaArtifactNotFound
	}
	if artifact.ObjectKey != "" && s.objectStore != nil {
		content, openErr := s.objectStore.Open(ctx, artifact, byteRange)
		if openErr == nil && content != nil {
			return content, nil
		}
		switch {
		case errors.Is(openErr, ErrInvalidMediaRange), errors.Is(openErr, ErrMediaRangeNotSatisfiable):
			return nil, openErr
		case openErr == nil, errors.Is(openErr, ErrMediaArtifactObjectStoreDisabled), errors.Is(openErr, ErrMediaContentUnavailable):
		default:
			return nil, fmt.Errorf("open stored media content: %w", openErr)
		}
	}
	if data, contentType, inline, decodeErr := decodeMediaDataReferenceBounded(
		artifact.UpstreamReference, artifact.ContentType, maxInlineMediaDecodedBytes,
	); inline {
		if decodeErr != nil {
			return nil, decodeErr
		}
		if byteRange != "" {
			return sliceMediaContent(data, contentType, byteRange)
		}
		return &MediaContent{
			Body: io.NopCloser(bytes.NewReader(data)), StatusCode: http.StatusOK,
			ContentType: contentType, ContentLength: int64(len(data)), AcceptRanges: "bytes",
		}, nil
	}
	if s.settings == nil {
		return nil, ErrMediaContentUnavailable
	}
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil || settings == nil || !settings.MediaVideoProxyFallbackEnabled || artifact.UpstreamReference == "" {
		return nil, ErrMediaContentUnavailable
	}
	if task.AccountID == nil || s.accounts == nil || s.adapters == nil {
		return nil, ErrMediaContentUnavailable
	}
	account, err := s.accounts.GetByID(ctx, *task.AccountID)
	if err != nil || account == nil {
		return nil, ErrMediaContentUnavailable
	}
	adapter, err := s.adapters.Resolve(task.Adapter)
	if err != nil {
		return nil, ErrMediaContentUnavailable
	}
	fetcher, ok := adapter.(MediaContentFetcher)
	if !ok {
		return nil, ErrMediaContentUnavailable
	}
	content, err := fetcher.OpenContent(ctx, account, artifact, byteRange)
	if err != nil {
		return nil, fmt.Errorf("open proxied media content: %w", err)
	}
	if content == nil {
		return nil, ErrMediaContentUnavailable
	}
	return content, nil
}

func ValidateMediaRange(value string) error {
	if !singleMediaByteRangePattern.MatchString(value) || strings.Contains(value, ",") {
		return ErrInvalidMediaRange
	}
	return nil
}

func sliceMediaContent(data []byte, contentType, byteRange string) (*MediaContent, error) {
	if err := ValidateMediaRange(byteRange); err != nil {
		return nil, err
	}
	spec := strings.TrimPrefix(byteRange, "bytes=")
	parts := strings.SplitN(spec, "-", 2)
	total := int64(len(data))
	if total == 0 {
		return nil, ErrMediaRangeNotSatisfiable
	}
	var start, end int64
	var err error
	if parts[0] == "" {
		var suffix int64
		suffix, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return nil, ErrInvalidMediaRange
		}
		if suffix > total {
			suffix = total
		}
		start, end = total-suffix, total-1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 || start >= total {
			return nil, ErrMediaRangeNotSatisfiable
		}
		if parts[1] == "" {
			end = total - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil || end < start {
				return nil, ErrMediaRangeNotSatisfiable
			}
			if end >= total {
				end = total - 1
			}
		}
	}
	selected := data[start : end+1]
	return &MediaContent{
		Body: io.NopCloser(bytes.NewReader(selected)), StatusCode: http.StatusPartialContent,
		ContentType: contentType, ContentLength: int64(len(selected)),
		ContentRange: fmt.Sprintf("bytes %d-%d/%d", start, end, total), AcceptRanges: "bytes",
	}, nil
}

func mediaArtifactInputFromStored(stored *MediaArtifact) MediaArtifactInput {
	input := MediaArtifactInput{
		Direction: stored.Direction, Position: stored.Position, MediaType: stored.MediaType,
		ContentType: stored.ContentType, SizeBytes: stored.SizeBytes, ChecksumSHA256: stored.ChecksumSHA256,
		ObjectKey: stored.ObjectKey, UpstreamReference: stored.UpstreamReference, Resolution: stored.Resolution,
	}
	if stored.Width != nil {
		input.Width = *stored.Width
	}
	if stored.Height != nil {
		input.Height = *stored.Height
	}
	if stored.DurationSeconds != nil {
		input.DurationSeconds = *stored.DurationSeconds
	}
	if stored.FPS != nil {
		input.FPS = *stored.FPS
	}
	return input
}

func mergeStoredArtifactMetadata(artifact *MediaArtifact, input MediaArtifactInput) {
	if artifact.MediaType == "" {
		artifact.MediaType = input.MediaType
	}
	if artifact.ContentType == "" {
		artifact.ContentType = input.ContentType
	}
	if artifact.SizeBytes <= 0 {
		artifact.SizeBytes = input.SizeBytes
	}
	if artifact.ChecksumSHA256 == "" {
		artifact.ChecksumSHA256 = input.ChecksumSHA256
	}
	if artifact.Resolution == "" {
		artifact.Resolution = input.Resolution
	}
	if artifact.Width == nil && input.Width > 0 {
		artifact.Width = mediaIntPointer(input.Width)
	}
	if artifact.Height == nil && input.Height > 0 {
		artifact.Height = mediaIntPointer(input.Height)
	}
	if artifact.DurationSeconds == nil && input.DurationSeconds > 0 {
		artifact.DurationSeconds = mediaFloatPointer(input.DurationSeconds)
	}
	if artifact.FPS == nil && input.FPS > 0 {
		artifact.FPS = mediaFloatPointer(input.FPS)
	}
}

func artifactFromProxyInput(taskID int64, input MediaArtifactInput) *MediaArtifact {
	reference := input.UpstreamReference
	if reference == "" {
		reference = input.ExternalURL
	}
	artifact := &MediaArtifact{
		TaskID: taskID, Direction: "output", Position: input.Position, MediaType: input.MediaType,
		ContentType: input.ContentType, SizeBytes: input.SizeBytes, ChecksumSHA256: input.ChecksumSHA256,
		StorageStatus: "proxy", UpstreamReference: reference, Resolution: input.Resolution,
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

func firstOutputVideo(artifacts []MediaArtifact) *MediaArtifact {
	for i := range artifacts {
		if artifacts[i].Direction == "output" && artifacts[i].MediaType == MediaTypeVideo {
			copy := artifacts[i]
			return &copy
		}
	}
	return nil
}

func decodeMediaDataReference(reference, fallbackContentType string) ([]byte, string, bool) {
	data, contentType, inline, err := decodeMediaDataReferenceBounded(
		reference, fallbackContentType, maxInlineMediaDecodedBytes,
	)
	if err != nil {
		return nil, "", false
	}
	return data, contentType, inline
}

func decodeMediaDataReferenceBounded(reference, fallbackContentType string, maxBytes int64) ([]byte, string, bool, error) {
	if !strings.HasPrefix(strings.ToLower(reference), "data:") {
		return nil, "", false, nil
	}
	metadata, payload, ok := strings.Cut(reference[5:], ",")
	if !ok || !strings.HasSuffix(strings.ToLower(metadata), ";base64") {
		return nil, "", true, ErrMediaContentUnavailable
	}
	contentType := strings.TrimSuffix(metadata, ";base64")
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = parsed
	} else {
		return nil, "", true, ErrMediaContentUnavailable
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "video/") {
		return nil, "", true, ErrMediaContentUnavailable
	}
	if maxBytes <= 0 {
		return nil, "", true, ErrMediaContentTooLarge
	}
	if int64(base64.StdEncoding.DecodedLen(len(payload))) > maxBytes+2 {
		return nil, "", true, ErrMediaContentTooLarge
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	decoded, err := io.ReadAll(io.LimitReader(decoder, maxBytes+1))
	if err != nil {
		return nil, "", true, ErrMediaContentUnavailable
	}
	if int64(len(decoded)) > maxBytes {
		return nil, "", true, ErrMediaContentTooLarge
	}
	if contentType == "" {
		contentType = fallbackContentType
	}
	return decoded, contentType, true, nil
}

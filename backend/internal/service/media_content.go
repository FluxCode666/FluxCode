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
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidMediaInput                = errors.New("invalid media input")
	ErrInvalidMediaRange                = errors.New("invalid media range")
	ErrMediaRangeNotSatisfiable         = errors.New("media range not satisfiable")
	ErrMediaArtifactNotFound            = errors.New("media artifact not found")
	ErrMediaArtifactConflict            = errors.New("media artifact content identity conflict")
	ErrMediaContentUnavailable          = errors.New("media content unavailable")
	ErrMediaContentAccountRequired      = errors.New("media content account required")
	ErrMediaContentTooLarge             = errors.New("media content too large")
	ErrMediaArtifactObjectStoreDisabled = errors.New("media artifact object store disabled")
	ErrMediaVideoObjectStorageRequired  = errors.New("media video object storage required")
)

const maxInlineMediaDecodedBytes = 1 << 20

const maxMaterializedMediaInputBytes = 64 << 20

type MediaContentTaskRepository interface {
	GetByPublicIDForUser(ctx context.Context, publicID string, userID, apiKeyID int64) (*MediaTask, error)
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
	tasks              MediaContentTaskRepository
	artifacts          MediaArtifactRepository
	settings           MediaSettingsProvider
	accounts           MediaContentAccountRepository
	adapters           *MediaAdapterRegistry
	httpReader         MediaHTTPContentReader
	objectStore        MediaArtifactObjectStore
	storageConsistency MediaStorageConsistencyRepository
}

func NewMediaContentService(
	tasks MediaContentTaskRepository,
	artifacts MediaArtifactRepository,
	settings MediaSettingsProvider,
	accounts MediaContentAccountRepository,
	adapters *MediaAdapterRegistry,
	httpReader MediaHTTPContentReader,
	objectStore MediaArtifactObjectStore,
	storageConsistency ...MediaStorageConsistencyRepository,
) *MediaContentService {
	service := &MediaContentService{
		tasks: tasks, artifacts: artifacts, settings: settings, accounts: accounts,
		adapters: adapters, httpReader: httpReader, objectStore: objectStore,
	}
	if len(storageConsistency) > 0 {
		service.storageConsistency = storageConsistency[0]
	}
	return service
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

func (s *MediaContentService) LoadInputs(
	ctx context.Context,
	task *MediaTask,
	spec MediaSpec,
	account *Account,
) ([]MediaArtifactInput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.artifacts == nil || task == nil || task.ID <= 0 {
		return nil, ErrMediaContentUnavailable
	}
	requested, err := mediaRequestedArtifactInputs(spec)
	if err != nil {
		return nil, err
	}
	if len(requested) == 0 {
		return nil, nil
	}
	artifacts, err := s.artifacts.ListByTaskID(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("list media input artifacts: %w", err)
	}
	byID := make(map[int64]*MediaArtifact, len(artifacts))
	for index := range artifacts {
		artifact := &artifacts[index]
		if artifact.ID > 0 {
			byID[artifact.ID] = artifact
		}
	}
	seen := make(map[int64]struct{}, len(requested))
	inputs := make([]MediaArtifactInput, 0, len(requested))
	for position, ref := range requested {
		if ref.id <= 0 {
			return nil, ErrInvalidMediaInput
		}
		if _, exists := seen[ref.id]; exists {
			return nil, ErrInvalidMediaInput
		}
		seen[ref.id] = struct{}{}
		artifact := byID[ref.id]
		if artifact == nil || artifact.TaskID != task.ID || artifact.Direction != "input" || artifact.MediaType != ref.mediaType {
			return nil, ErrInvalidMediaInput
		}
		input, loadErr := s.loadArtifactInput(ctx, artifact, account)
		if loadErr != nil {
			return nil, fmt.Errorf("load media input artifact %d: %w", ref.id, loadErr)
		}
		input.Position = position
		inputs = append(inputs, input)
	}
	return inputs, nil
}

type mediaRequestedArtifactInput struct {
	id        int64
	mediaType MediaType
}

func mediaRequestedArtifactInputs(spec MediaSpec) ([]mediaRequestedArtifactInput, error) {
	requested := make([]mediaRequestedArtifactInput, 0, MaxMediaReferenceInputs)
	if spec.Image != nil {
		for _, id := range spec.Image.InputArtifactIDs {
			requested = append(requested, mediaRequestedArtifactInput{id: id, mediaType: MediaTypeImage})
		}
	}
	if spec.Video != nil {
		for _, id := range spec.Video.ReferenceArtifactIDs {
			requested = append(requested, mediaRequestedArtifactInput{id: id, mediaType: MediaTypeImage})
		}
		if spec.Video.SourceArtifactID != nil {
			requested = append(requested, mediaRequestedArtifactInput{id: *spec.Video.SourceArtifactID, mediaType: MediaTypeVideo})
		}
	}
	if len(requested) > MaxMediaReferenceInputs {
		return nil, ErrInvalidMediaInput
	}
	return requested, nil
}

func (s *MediaContentService) loadArtifactInput(
	ctx context.Context,
	artifact *MediaArtifact,
	account *Account,
) (MediaArtifactInput, error) {
	if artifact == nil {
		return MediaArtifactInput{}, ErrInvalidMediaInput
	}
	var content *MediaContent
	var err error
	switch {
	case artifact.ObjectKey != "":
		if s.objectStore == nil {
			return MediaArtifactInput{}, ErrMediaArtifactObjectStoreDisabled
		}
		content, err = s.objectStore.Open(ctx, artifact, "")
	case artifact.PublicURL != "":
		content, err = s.openExternalInput(ctx, artifact.PublicURL, artifact.MediaType, account)
	case strings.HasPrefix(strings.ToLower(artifact.UpstreamReference), "data:"):
		data, contentType, _, decodeErr := decodeMediaDataReferenceBounded(
			artifact.UpstreamReference, artifact.ContentType, maxMaterializedMediaInputBytes, artifact.MediaType,
		)
		if decodeErr != nil {
			return MediaArtifactInput{}, decodeErr
		}
		return validatedMaterializedArtifactInput(artifact, data, contentType)
	case artifact.UpstreamReference != "":
		content, err = s.openExternalInput(ctx, artifact.UpstreamReference, artifact.MediaType, account)
	default:
		return MediaArtifactInput{}, ErrMediaContentUnavailable
	}
	if err != nil {
		return MediaArtifactInput{}, err
	}
	if content == nil || content.Body == nil {
		return MediaArtifactInput{}, ErrMediaContentUnavailable
	}
	defer content.Body.Close() //nolint:errcheck
	if content.ContentLength > maxMaterializedMediaInputBytes {
		return MediaArtifactInput{}, ErrMediaContentTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(content.Body, maxMaterializedMediaInputBytes+1))
	if err != nil {
		return MediaArtifactInput{}, err
	}
	if int64(len(data)) > maxMaterializedMediaInputBytes {
		return MediaArtifactInput{}, ErrMediaContentTooLarge
	}
	contentType := content.ContentType
	if normalizeStoredMediaContentType(contentType, artifact.MediaType) == "application/octet-stream" {
		contentType = artifact.ContentType
	}
	return validatedMaterializedArtifactInput(artifact, data, contentType)
}

func (s *MediaContentService) openExternalInput(
	ctx context.Context,
	rawURL string,
	mediaType MediaType,
	account *Account,
) (*MediaContent, error) {
	if s.httpReader == nil || account == nil {
		return nil, ErrMediaContentAccountRequired
	}
	return s.httpReader.Open(ctx, MediaHTTPContentRequest{URL: rawURL, Account: account, MediaType: mediaType})
}

func validatedMaterializedArtifactInput(
	artifact *MediaArtifact,
	data []byte,
	contentType string,
) (MediaArtifactInput, error) {
	input := mediaArtifactInputFromStored(artifact)
	input.Data = data
	input.ExternalURL = ""
	input.UpstreamReference = ""
	validated, normalizedType, checksum, err := validateMediaObjectInput(input, maxMaterializedMediaInputBytes)
	if err != nil {
		return MediaArtifactInput{}, err
	}
	if normalizeStoredMediaContentType(contentType, artifact.MediaType) != normalizedType {
		return MediaArtifactInput{}, ErrInvalidMediaInput
	}
	if artifact.SizeBytes > 0 && artifact.SizeBytes != int64(len(validated)) {
		return MediaArtifactInput{}, ErrMediaStorageIntegrity
	}
	input.ContentType = normalizedType
	input.SizeBytes = int64(len(validated))
	input.ChecksumSHA256 = checksum
	return input, nil
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

func (s *MediaContentService) PersistOutputs(
	ctx context.Context,
	task *MediaTask,
	inputs []MediaArtifactInput,
) (result []MediaArtifact, returnErr error) {
	if s == nil || task == nil || s.artifacts == nil || s.settings == nil {
		return nil, ErrMediaContentUnavailable
	}
	if len(inputs) == 0 || (task.MediaType != MediaTypeImage && task.MediaType != MediaTypeVideo) {
		return nil, ErrInvalidMediaInput
	}
	for i := range inputs {
		if inputs[i].MediaType != task.MediaType {
			return nil, ErrInvalidMediaInput
		}
	}
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("load media content settings: %w", err)
	}
	if settings == nil {
		return nil, ErrMediaContentUnavailable
	}
	var storageDownloadAccount *Account
	if mediaOutputsNeedMaterialization(s.objectStore, inputs) {
		if task.AccountID == nil || s.accounts == nil || s.httpReader == nil {
			return nil, ErrMediaContentAccountRequired
		}
		storageDownloadAccount, err = s.accounts.GetByID(ctx, *task.AccountID)
		if err != nil {
			return nil, fmt.Errorf("load media output download account: %w", err)
		}
		if storageDownloadAccount == nil {
			return nil, ErrMediaContentAccountRequired
		}
	}
	stored := make([]MediaArtifact, 0, len(inputs))
	committed := false
	defer func() {
		if committed || len(stored) == 0 {
			return
		}
		result = nil
		returnErr = errors.Join(returnErr, s.rollbackPersistedOutputs(ctx, stored))
	}()
	for i := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input := inputs[i]
		input.Direction = "output"
		input.Position = i
		if s.objectStore != nil {
			var artifact *MediaArtifact
			var putErr error
			if len(input.Data) == 0 && input.ExternalURL != "" && storageDownloadAccount != nil {
				artifact, putErr = s.storeExternalOutput(ctx, input, storageDownloadAccount)
			} else {
				artifact, putErr = s.objectStore.Put(ctx, input)
			}
			if putErr == nil && artifact != nil && artifact.ObjectKey != "" {
				mergeStoredArtifactMetadata(artifact, input)
				if artifact.MediaType != task.MediaType {
					return nil, errors.Join(ErrInvalidMediaInput, s.discardStoredArtifact(ctx, artifact))
				}
				if artifact.MediaType == MediaTypeImage {
					if deliveryErr := validateStoredImageDelivery(artifact); deliveryErr != nil {
						return nil, errors.Join(deliveryErr, s.discardStoredArtifact(ctx, artifact))
					}
				}
				artifact.TaskID = task.ID
				artifact.Direction = "output"
				artifact.Position = i
				artifact.StorageStatus = "stored"
				created, createErr := commitMediaArtifact(ctx, s.artifacts, s.storageConsistency, artifact)
				if createErr != nil {
					if errors.Is(createErr, ErrMediaStorageCommitOutcomeUnknown) {
						// COMMIT may have succeeded. Retain this object and every prior
						// row in the batch; compensating either could create a broken
						// immutable locator.
						committed = true
						return nil, fmt.Errorf("persist stored media output: %w", createErr)
					}
					return nil, errors.Join(
						fmt.Errorf("persist stored media output: %w", createErr),
						s.discardStoredArtifact(ctx, artifact),
					)
				}
				if created == nil || created.ID <= 0 {
					return nil, errors.Join(
						errors.New("persist stored media output: repository returned an invalid artifact"),
						s.discardStoredArtifact(ctx, artifact),
					)
				}
				stored = append(stored, *created)
				continue
			}
			if putErr != nil && !errors.Is(putErr, ErrMediaArtifactObjectStoreDisabled) {
				return nil, fmt.Errorf("store media output: %w", putErr)
			}
			if putErr == nil {
				return nil, fmt.Errorf("%w: object store returned an incomplete artifact", ErrMediaStorageIntegrity)
			}
		}

		if strings.HasPrefix(strings.ToLower(input.UpstreamReference), "data:") {
			_, contentType, _, inlineErr := decodeMediaDataReferenceBounded(
				input.UpstreamReference, input.ContentType, maxInlineMediaDecodedBytes, input.MediaType,
			)
			if inlineErr != nil {
				return nil, inlineErr
			}
			input.ContentType = contentType
		}
		if input.MediaType == MediaTypeImage {
			var artifact *MediaArtifact
			switch {
			case strings.HasPrefix(strings.ToLower(input.UpstreamReference), "data:"):
				artifact = artifactFromProxyInput(task.ID, input)
			case input.ExternalURL != "":
				if s.httpReader == nil {
					return nil, ErrMediaContentUnavailable
				}
				normalized, validateErr := s.httpReader.ValidateURL(input.ExternalURL)
				if validateErr != nil || !IsSafePublicMediaURL(normalized) {
					return nil, ErrMediaContentUnavailable
				}
				artifact = artifactFromPublicImageInput(task.ID, input, normalized)
			default:
				return nil, ErrMediaContentUnavailable
			}
			created, createErr := commitMediaArtifact(ctx, s.artifacts, s.storageConsistency, artifact)
			if createErr != nil {
				return nil, fmt.Errorf("persist deliverable image output: %w", createErr)
			}
			if created == nil || created.ID <= 0 {
				return nil, errors.New("persist deliverable image output: repository returned an invalid artifact")
			}
			stored = append(stored, *created)
			continue
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
		created, createErr := commitMediaArtifact(
			ctx,
			s.artifacts,
			s.storageConsistency,
			artifactFromProxyInput(task.ID, input),
		)
		if createErr != nil {
			return nil, fmt.Errorf("persist proxy media output: %w", createErr)
		}
		if created == nil || created.ID <= 0 {
			return nil, errors.New("persist proxy media output: repository returned an invalid artifact")
		}
		stored = append(stored, *created)
	}
	committed = true
	return stored, nil
}

func (s *MediaContentService) rollbackPersistedOutputs(ctx context.Context, artifacts []MediaArtifact) error {
	if s == nil || s.artifacts == nil || len(artifacts) == 0 {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	var cleanupErrors []error
	for index := len(artifacts) - 1; index >= 0; index-- {
		artifact := artifacts[index]
		deleted, err := s.artifacts.DeleteExact(cleanupCtx, &artifact)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete media output %d during rollback: %w", artifact.ID, err))
			continue
		}
		if artifact.ObjectKey == "" {
			continue
		}
		if s.objectStore == nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("discard media output %d during rollback: object store is unavailable", artifact.ID))
			if deleted {
				if _, restoreErr := commitMediaArtifact(cleanupCtx, s.artifacts, s.storageConsistency, &artifact); restoreErr != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("restore media output %d locator after rollback failure: %w", artifact.ID, restoreErr))
				}
			}
			continue
		}
		if err := s.objectStore.Discard(cleanupCtx, mediaArtifactInputFromStored(&artifact)); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("discard media output %d during rollback: %w", artifact.ID, err))
			if deleted {
				// Restore the immutable locator if object deletion failed. A retained
				// orphan is recoverable; an unindexed object is not.
				if _, restoreErr := commitMediaArtifact(cleanupCtx, s.artifacts, s.storageConsistency, &artifact); restoreErr != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("restore media output %d locator after rollback failure: %w", artifact.ID, restoreErr))
				}
			}
		}
	}
	return errors.Join(cleanupErrors...)
}

func mediaOutputsNeedMaterialization(store MediaArtifactObjectStore, inputs []MediaArtifactInput) bool {
	if store == nil {
		return false
	}
	if _, disabled := store.(*DisabledMediaArtifactObjectStore); disabled {
		return false
	}
	for i := range inputs {
		if len(inputs[i].Data) == 0 && strings.TrimSpace(inputs[i].ExternalURL) != "" {
			return true
		}
	}
	return false
}

func (s *MediaContentService) storeExternalOutput(
	ctx context.Context,
	input MediaArtifactInput,
	account *Account,
) (artifact *MediaArtifact, returnErr error) {
	streamStore, ok := s.objectStore.(MediaArtifactStreamObjectStore)
	if !ok {
		return nil, fmt.Errorf("%w: configured media store does not support streaming writes", ErrMediaStorageProviderUnavailable)
	}
	content, err := s.httpReader.Open(ctx, MediaHTTPContentRequest{
		URL: input.ExternalURL, Account: account, MediaType: input.MediaType,
	})
	if err != nil {
		return nil, fmt.Errorf("download media output for storage: %w", err)
	}
	if content == nil || content.Body == nil {
		return nil, ErrMediaContentUnavailable
	}
	defer func() {
		closeErr := content.Body.Close()
		if closeErr == nil {
			return
		}
		closeErr = fmt.Errorf("close media output download: %w", closeErr)
		if artifact != nil {
			cleanupErr := s.discardStoredArtifact(ctx, artifact)
			artifact = nil
			returnErr = errors.Join(returnErr, closeErr, cleanupErr)
			return
		}
		returnErr = errors.Join(returnErr, closeErr)
	}()
	if content.StatusCode != http.StatusOK {
		return nil, ErrMediaContentUnavailable
	}
	if content.ContentLength > 0 {
		if input.SizeBytes > 0 && input.SizeBytes != content.ContentLength {
			return nil, fmt.Errorf("%w: upstream media length differs from adapter metadata", ErrMediaStorageIntegrity)
		}
		input.SizeBytes = content.ContentLength
	}
	responseContentType := normalizeStoredMediaContentType(content.ContentType, input.MediaType)
	adapterContentType := normalizeStoredMediaContentType(input.ContentType, input.MediaType)
	if responseContentType != "application/octet-stream" &&
		adapterContentType != "application/octet-stream" &&
		responseContentType != adapterContentType {
		return nil, fmt.Errorf(
			"%w: upstream media content type differs from adapter metadata",
			ErrMediaStorageIntegrity,
		)
	}
	declared := responseContentType
	if declared == "application/octet-stream" {
		declared = adapterContentType
	}
	if declared == "application/octet-stream" {
		return nil, ErrInvalidMediaInput
	}
	input.ContentType = declared
	input.ExternalURL = ""
	artifact, err = streamStore.PutStream(ctx, input, content.Body)
	if err != nil {
		return artifact, fmt.Errorf("store downloaded media output: %w", err)
	}
	if artifact == nil || artifact.ObjectKey == "" {
		return artifact, ErrMediaStorageIntegrity
	}
	return artifact, nil
}

func (s *MediaContentService) discardStoredArtifact(ctx context.Context, artifact *MediaArtifact) error {
	if s == nil || s.objectStore == nil || artifact == nil || artifact.ObjectKey == "" {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.objectStore.Discard(cleanupCtx, mediaArtifactInputFromStored(artifact)); err != nil {
		return fmt.Errorf("discard uncommitted media output: %w", err)
	}
	return nil
}

func validateStoredImageDelivery(artifact *MediaArtifact) error {
	if artifact == nil || artifact.MediaType != MediaTypeImage {
		return ErrMediaContentUnavailable
	}
	if artifact.PublicURL != "" {
		artifact.PublicURL = strings.TrimSpace(artifact.PublicURL)
		if IsSafePublicMediaURL(artifact.PublicURL) {
			return nil
		}
		return ErrMediaContentUnavailable
	}
	if artifact.ObjectKey != "" &&
		(artifact.StorageProvider == MediaStorageProviderLocal || artifact.StorageProvider == MediaStorageProviderMinIO) {
		return nil
	}
	_, contentType, inline, err := decodeMediaDataReferenceBounded(
		artifact.UpstreamReference, artifact.ContentType, maxInlineMediaDecodedBytes, MediaTypeImage,
	)
	if err != nil {
		return err
	}
	if !inline {
		return ErrMediaContentUnavailable
	}
	artifact.ContentType = contentType
	return nil
}

func (s *MediaContentService) OpenImage(ctx context.Context, publicID string, userID, apiKeyID int64, position int, byteRange string) (*MediaContent, error) {
	if position < 0 {
		return nil, ErrMediaArtifactNotFound
	}
	return s.openMediaContent(ctx, publicID, userID, apiKeyID, position, byteRange, MediaTypeImage)
}

func (s *MediaContentService) OpenVideo(ctx context.Context, publicID string, userID, apiKeyID int64, byteRange string) (*MediaContent, error) {
	return s.openMediaContent(ctx, publicID, userID, apiKeyID, 0, byteRange, MediaTypeVideo)
}

func (s *MediaContentService) openMediaContent(
	ctx context.Context,
	publicID string,
	userID int64,
	apiKeyID int64,
	position int,
	byteRange string,
	mediaType MediaType,
) (*MediaContent, error) {
	if s == nil || s.tasks == nil || s.artifacts == nil {
		return nil, ErrMediaContentUnavailable
	}
	if byteRange != "" {
		if err := ValidateMediaRange(byteRange); err != nil {
			return nil, err
		}
	}
	task, err := s.tasks.GetByPublicIDForUser(ctx, strings.TrimSpace(publicID), userID, apiKeyID)
	if err != nil {
		if errors.Is(err, ErrMediaTaskNotFound) {
			return nil, ErrMediaTaskNotFound
		}
		return nil, errors.Join(ErrMediaContentUnavailable, fmt.Errorf("load media task: %w", err))
	}
	if task == nil || task.MediaType != mediaType || task.Status != MediaTaskStatusCompleted ||
		task.BillingStatus != MediaBillingStatusSettled {
		return nil, ErrMediaTaskNotFound
	}
	artifacts, err := s.artifacts.ListByTaskID(ctx, task.ID)
	if err != nil {
		return nil, fmt.Errorf("list media artifacts: %w", err)
	}
	artifact := outputMediaAtPosition(artifacts, mediaType, position)
	if artifact == nil {
		return nil, ErrMediaArtifactNotFound
	}
	var fallbackCauses []error
	if artifact.ObjectKey != "" && s.objectStore != nil {
		content, openErr := s.objectStore.Open(ctx, artifact, byteRange)
		if openErr == nil && content != nil && content.Body != nil {
			return content, nil
		}
		if errors.Is(openErr, ErrInvalidMediaRange) || errors.Is(openErr, ErrMediaRangeNotSatisfiable) {
			return nil, openErr
		}
		if openErr != nil {
			fallbackCauses = append(fallbackCauses, fmt.Errorf("open stored media content: %w", openErr))
		} else if content == nil {
			fallbackCauses = append(fallbackCauses, errors.New("object store returned nil media content"))
		} else {
			fallbackCauses = append(fallbackCauses, errors.New("object store returned media content with nil body"))
		}
	}
	if data, contentType, inline, decodeErr := decodeMediaDataReferenceBounded(
		artifact.UpstreamReference, artifact.ContentType, maxInlineMediaDecodedBytes, mediaType,
	); inline {
		if decodeErr != nil {
			return nil, mediaContentFallbackError(append(fallbackCauses, decodeErr)...)
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
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	settings, err := s.settings.GetAllSettings(ctx)
	if err != nil {
		fallbackCauses = append(fallbackCauses, fmt.Errorf("load media proxy settings: %w", err))
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	if settings == nil || (mediaType == MediaTypeVideo && !settings.MediaVideoProxyFallbackEnabled) || artifact.UpstreamReference == "" {
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	if task.AccountID == nil || s.accounts == nil || s.adapters == nil {
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	account, err := s.accounts.GetByID(ctx, *task.AccountID)
	if err != nil {
		fallbackCauses = append(fallbackCauses, fmt.Errorf("load media content account: %w", err))
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	if account == nil {
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	adapter, err := s.adapters.Resolve(task.Adapter)
	if err != nil {
		fallbackCauses = append(fallbackCauses, fmt.Errorf("resolve media content adapter: %w", err))
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	fetcher, ok := adapter.(MediaContentFetcher)
	if !ok {
		fallbackCauses = append(fallbackCauses, errors.New("media adapter does not support content fetching"))
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	content, err := fetcher.OpenContent(ctx, account, artifact, byteRange)
	if err != nil {
		fallbackCauses = append(fallbackCauses, fmt.Errorf("open proxied media content: %w", err))
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	if content == nil {
		fallbackCauses = append(fallbackCauses, errors.New("media content fetcher returned nil content"))
		return nil, mediaContentFallbackError(fallbackCauses...)
	}
	return content, nil
}

func mediaContentFallbackError(causes ...error) error {
	errorsToJoin := []error{ErrMediaContentUnavailable}
	for _, cause := range causes {
		if cause != nil {
			errorsToJoin = append(errorsToJoin, cause)
		}
	}
	return errors.Join(errorsToJoin...)
}

func ValidateMediaRange(value string) error {
	_, err := parseMediaByteRange(value)
	return err
}

type mediaByteRange struct {
	start    int64
	end      int64
	hasStart bool
	hasEnd   bool
}

func parseMediaByteRange(value string) (mediaByteRange, error) {
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return mediaByteRange{}, ErrInvalidMediaRange
	}
	spec := strings.TrimPrefix(value, "bytes=")
	if strings.Count(spec, "-") != 1 {
		return mediaByteRange{}, ErrInvalidMediaRange
	}
	parts := strings.SplitN(spec, "-", 2)
	if parts[0] == "" && parts[1] == "" {
		return mediaByteRange{}, ErrInvalidMediaRange
	}
	parsed := mediaByteRange{hasStart: parts[0] != "", hasEnd: parts[1] != ""}
	if (parsed.hasStart && !isASCIIDigits(parts[0])) || (parsed.hasEnd && !isASCIIDigits(parts[1])) {
		return mediaByteRange{}, ErrInvalidMediaRange
	}
	var err error
	if parsed.hasStart {
		parsed.start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || parsed.start < 0 {
			return mediaByteRange{}, ErrInvalidMediaRange
		}
	}
	if parsed.hasEnd {
		parsed.end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || parsed.end < 0 {
			return mediaByteRange{}, ErrInvalidMediaRange
		}
	}
	if !parsed.hasStart && parsed.end == 0 {
		return mediaByteRange{}, ErrInvalidMediaRange
	}
	if parsed.hasStart && parsed.hasEnd && parsed.end < parsed.start {
		return mediaByteRange{}, ErrInvalidMediaRange
	}
	return parsed, nil
}

func isASCIIDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func sliceMediaContent(data []byte, contentType, byteRange string) (*MediaContent, error) {
	parsed, err := parseMediaByteRange(byteRange)
	if err != nil {
		return nil, err
	}
	total := int64(len(data))
	if total == 0 {
		return nil, ErrMediaRangeNotSatisfiable
	}
	var start, end int64
	if !parsed.hasStart {
		suffix := parsed.end
		if suffix > total {
			suffix = total
		}
		start, end = total-suffix, total-1
	} else {
		start = parsed.start
		if start >= total {
			return nil, ErrMediaRangeNotSatisfiable
		}
		if !parsed.hasEnd {
			end = total - 1
		} else {
			end = parsed.end
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
		StorageProvider: stored.StorageProvider, StorageRevision: stored.StorageRevision, ObjectKey: stored.ObjectKey,
		UpstreamReference: stored.UpstreamReference, Resolution: stored.Resolution,
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
	if artifact.StorageProvider == "" {
		artifact.StorageProvider = input.StorageProvider
	}
	if artifact.StorageRevision == "" {
		artifact.StorageRevision = input.StorageRevision
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
		StorageStatus: "proxy", StorageProvider: MediaStorageProviderLegacy,
		UpstreamReference: reference, Resolution: input.Resolution,
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

func artifactFromPublicImageInput(taskID int64, input MediaArtifactInput, publicURL string) *MediaArtifact {
	artifact := artifactFromProxyInput(taskID, input)
	artifact.PublicURL = publicURL
	artifact.UpstreamReference = ""
	return artifact
}

func IsSafePublicMediaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed != nil && parsed.Scheme == "https" && parsed.Hostname() != "" &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == ""
}

func NormalizeVideoContentType(value string) (string, bool) {
	value = strings.TrimSpace(strings.SplitN(value, "\r", 2)[0])
	value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	parsed, _, err := mime.ParseMediaType(value)
	if err != nil || !strings.HasPrefix(strings.ToLower(parsed), "video/") {
		return "application/octet-stream", false
	}
	return strings.ToLower(parsed), true
}

// NormalizeImageContentType returns a safe image content type for downstream
// delivery. Invalid or non-image values are downgraded to an attachment-safe
// octet-stream response.
func NormalizeImageContentType(value string) (string, bool) {
	value = strings.TrimSpace(strings.SplitN(value, "\r", 2)[0])
	value = strings.TrimSpace(strings.SplitN(value, "\n", 2)[0])
	parsed, _, err := mime.ParseMediaType(value)
	parsed = strings.ToLower(parsed)
	if err != nil || !strings.HasPrefix(parsed, "image/") || parsed == "image/svg+xml" {
		return "application/octet-stream", false
	}
	return parsed, true
}

func MediaImageB64JSON(artifact MediaArtifact) (string, bool) {
	if artifact.MediaType != MediaTypeImage {
		return "", false
	}
	data, _, inline, err := decodeMediaDataReferenceBounded(
		artifact.UpstreamReference, artifact.ContentType, maxInlineMediaDecodedBytes, MediaTypeImage,
	)
	if err != nil || !inline {
		return "", false
	}
	return base64.StdEncoding.EncodeToString(data), true
}

func outputMediaAtPosition(artifacts []MediaArtifact, mediaType MediaType, position int) *MediaArtifact {
	for i := range artifacts {
		if artifacts[i].Direction == "output" && artifacts[i].MediaType == mediaType && artifacts[i].Position == position {
			copy := artifacts[i]
			return &copy
		}
	}
	return nil
}

func decodeMediaDataReference(reference, fallbackContentType string) ([]byte, string, bool) {
	data, contentType, inline, err := decodeMediaDataReferenceBounded(
		reference, fallbackContentType, maxInlineMediaDecodedBytes, MediaTypeVideo,
	)
	if err != nil {
		return nil, "", false
	}
	return data, contentType, inline
}

func decodeMediaDataReferenceBounded(reference, fallbackContentType string, maxBytes int64, expectedMediaType MediaType) ([]byte, string, bool, error) {
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
	if expectedMediaType != MediaTypeImage && expectedMediaType != MediaTypeVideo {
		return nil, "", true, ErrMediaContentUnavailable
	}
	if !strings.HasPrefix(strings.ToLower(contentType), string(expectedMediaType)+"/") {
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

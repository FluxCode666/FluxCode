package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	maxMediaIdempotencyKeyBytes = 255
	maxMediaUploadFiles         = 16
	defaultMediaRequestMaxBytes = 64 << 20
	mediaStagingCleanupTimeout  = 10 * time.Second
)

type MediaTaskApplication interface {
	Create(ctx context.Context, req service.MediaCreateRequest) (*service.MediaCreateResult, error)
	GetForUser(ctx context.Context, publicID string, userID int64) (*service.MediaTask, []service.MediaArtifact, error)
}

type MediaVideoContentOpener interface {
	OpenVideo(ctx context.Context, publicID string, userID int64, byteRange string) (*service.MediaContent, error)
}

type MediaTaskHandler struct {
	app             MediaTaskApplication
	content         MediaVideoContentOpener
	stager          service.MediaInputLifecycle
	maxRequestBytes int64
}

func NewMediaTaskHandler(
	app MediaTaskApplication,
	content MediaVideoContentOpener,
	stager service.MediaInputLifecycle,
	cfg *config.Config,
) *MediaTaskHandler {
	maxBytes := int64(defaultMediaRequestMaxBytes)
	if cfg != nil && cfg.Server.MaxRequestBodySize > 0 {
		maxBytes = cfg.Server.MaxRequestBodySize
	}
	return &MediaTaskHandler{app: app, content: content, stager: stager, maxRequestBytes: maxBytes}
}

type mediaJSONCreateRequest struct {
	Model              string   `json:"model"`
	Prompt             string   `json:"prompt"`
	Async              *bool    `json:"async"`
	Count              int      `json:"n"`
	Size               string   `json:"size"`
	Quality            string   `json:"quality"`
	OutputFormat       string   `json:"output_format"`
	ResponseFormat     string   `json:"response_format"`
	DurationSeconds    int      `json:"duration_seconds"`
	Resolution         string   `json:"resolution"`
	FPS                int      `json:"fps"`
	ImageURL           string   `json:"image_url"`
	ReferenceImageURLs []string `json:"reference_image_urls"`
	VideoURL           string   `json:"video_url"`
}

type mediaTaskResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	MediaType string          `json:"media_type"`
	Operation string          `json:"operation"`
	Model     string          `json:"model"`
	Status    string          `json:"status"`
	Progress  int             `json:"progress"`
	CreatedAt int64           `json:"created_at"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *mediaTaskError `json:"error,omitempty"`
}

type mediaTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type mediaImageResponse struct {
	Created int64                `json:"created"`
	Data    []mediaImageDataItem `json:"data"`
}

type mediaImageDataItem struct {
	URL     string `json:"url,omitempty"`
	B64JSON string `json:"b64_json,omitempty"`
}

type mediaAPIErrorEnvelope struct {
	Error mediaAPIError `json:"error"`
}

type mediaAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (h *MediaTaskHandler) CreateImageGeneration(c *gin.Context) {
	identity, ok := mediaCreateIdentity(c)
	if !ok {
		writeMediaError(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	idempotencyKey, ok := mediaIdempotencyKey(c)
	if !ok {
		return
	}
	var request mediaJSONCreateRequest
	if !h.decodeJSON(c, &request) {
		return
	}
	count := request.Count
	if count == 0 {
		count = 1
	}
	req := service.MediaCreateRequest{
		UserID: identity.userID, APIKeyID: identity.apiKeyID, GroupID: identity.groupID,
		MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
		RequestedModel: strings.TrimSpace(request.Model), ClientAsync: request.Async != nil && *request.Async,
		IdempotencyKey: idempotencyKey,
		Spec: service.MediaSpec{Image: &service.ImageSpec{
			Prompt: request.Prompt, Count: count, Size: request.Size, Quality: request.Quality,
			OutputFormat: request.OutputFormat, ResponseFormat: request.ResponseFormat,
		}},
	}
	_ = h.create(c, req)
}

func (h *MediaTaskHandler) CreateImageEdit(c *gin.Context) {
	identity, ok := mediaCreateIdentity(c)
	if !ok {
		writeMediaError(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	idempotencyKey, ok := mediaIdempotencyKey(c)
	if !ok {
		return
	}
	form, ok := h.parseMultipart(c)
	if !ok {
		return
	}
	defer form.RemoveAll()
	if !validMultipartFileSet(form, map[string]struct{}{"image": {}}) {
		writeMediaError(c, http.StatusBadRequest, "invalid_image", "A supported image upload is required")
		return
	}
	async, ok := parseMultipartAsync(form.Value["async"])
	if !ok {
		writeMediaError(c, http.StatusBadRequest, "invalid_async", "async must be true, false, or omitted")
		return
	}
	count, ok := parseOptionalPositiveFormInt(firstFormValue(form.Value, "n"), 1)
	if !ok {
		writeMediaError(c, http.StatusBadRequest, "invalid_number", "n must be a positive integer")
		return
	}
	files := form.File["image"]
	if len(files) == 0 || len(files) > maxMediaUploadFiles {
		writeMediaError(c, http.StatusBadRequest, "invalid_image", "A supported image upload is required")
		return
	}
	inputs, err := h.stageUploads(c.Request.Context(), identity.userID, files, service.MediaTypeImage)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	req := service.MediaCreateRequest{
		UserID: identity.userID, APIKeyID: identity.apiKeyID, GroupID: identity.groupID,
		MediaType: service.MediaTypeImage, Operation: service.MediaOperationImageEdit,
		RequestedModel: strings.TrimSpace(firstFormValue(form.Value, "model")), ClientAsync: async,
		IdempotencyKey: idempotencyKey,
		Spec: service.MediaSpec{Image: &service.ImageSpec{
			Prompt: firstFormValue(form.Value, "prompt"), Count: count,
			Size: firstFormValue(form.Value, "size"), Quality: firstFormValue(form.Value, "quality"),
			OutputFormat: firstFormValue(form.Value, "output_format"), ResponseFormat: firstFormValue(form.Value, "response_format"),
		}},
		Inputs: inputs,
	}
	if !h.create(c, req) {
		_ = h.discardStagedInputs(c.Request.Context(), identity.userID, inputs)
	}
}

func (h *MediaTaskHandler) GetImageTask(c *gin.Context) {
	h.getTask(c, service.MediaTypeImage)
}

func (h *MediaTaskHandler) CreateVideo(c *gin.Context) {
	identity, ok := mediaCreateIdentity(c)
	if !ok {
		writeMediaError(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	idempotencyKey, ok := mediaIdempotencyKey(c)
	if !ok {
		return
	}
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		h.createMultipartVideo(c, identity, idempotencyKey)
		return
	}
	var request mediaJSONCreateRequest
	if !h.decodeJSON(c, &request) {
		return
	}
	operation := service.MediaOperationTextToVideo
	inputValues := make([]struct {
		value     string
		mediaType service.MediaType
	}, 0, len(request.ReferenceImageURLs)+2)
	if request.ImageURL != "" {
		operation = service.MediaOperationImageToVideo
		inputValues = append(inputValues, struct {
			value     string
			mediaType service.MediaType
		}{request.ImageURL, service.MediaTypeImage})
	}
	if len(request.ReferenceImageURLs) > 0 {
		operation = service.MediaOperationReferenceVideo
		for _, value := range request.ReferenceImageURLs {
			inputValues = append(inputValues, struct {
				value     string
				mediaType service.MediaType
			}{value, service.MediaTypeImage})
		}
	}
	if request.VideoURL != "" {
		operation = service.MediaOperationVideoRemix
		inputValues = append(inputValues, struct {
			value     string
			mediaType service.MediaType
		}{request.VideoURL, service.MediaTypeVideo})
	}
	pending := make([]service.MediaArtifactInput, 0, len(inputValues))
	for position, item := range inputValues {
		pending = append(pending, service.MediaArtifactInput{
			Direction: "input", Position: position, MediaType: item.mediaType, ExternalURL: item.value,
		})
	}
	inputs, err := h.stageInputs(c.Request.Context(), identity.userID, pending)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	req := service.MediaCreateRequest{
		UserID: identity.userID, APIKeyID: identity.apiKeyID, GroupID: identity.groupID,
		MediaType: service.MediaTypeVideo, Operation: operation, RequestedModel: strings.TrimSpace(request.Model),
		ClientAsync:    request.Async != nil && *request.Async,
		IdempotencyKey: idempotencyKey,
		Spec: service.MediaSpec{Video: &service.VideoSpec{
			Prompt: request.Prompt, DurationSeconds: request.DurationSeconds, Resolution: request.Resolution, FPS: request.FPS,
		}},
		Inputs: inputs,
	}
	if !h.create(c, req) {
		_ = h.discardStagedInputs(c.Request.Context(), identity.userID, inputs)
	}
}

func (h *MediaTaskHandler) GetVideoTask(c *gin.Context) {
	h.getTask(c, service.MediaTypeVideo)
}

func (h *MediaTaskHandler) GetVideoContent(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		writeMediaError(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	byteRange := strings.TrimSpace(c.GetHeader("Range"))
	if byteRange != "" {
		if err := service.ValidateMediaRange(byteRange); err != nil {
			writeMediaError(c, http.StatusRequestedRangeNotSatisfiable, "invalid_range", "The requested range is invalid")
			return
		}
	}
	if h == nil || h.content == nil {
		writeMediaError(c, http.StatusBadGateway, "media_content_unavailable", "Media content is temporarily unavailable")
		return
	}
	content, err := h.content.OpenVideo(c.Request.Context(), c.Param("id"), subject.UserID, byteRange)
	if err != nil {
		h.writeContentError(c, err)
		return
	}
	if content == nil || content.Body == nil {
		writeMediaError(c, http.StatusBadGateway, "media_content_unavailable", "Media content is temporarily unavailable")
		return
	}
	defer content.Body.Close()
	headers := map[string]string{}
	if content.ContentRange != "" {
		headers["Content-Range"] = content.ContentRange
	}
	if content.AcceptRanges != "" {
		headers["Accept-Ranges"] = content.AcceptRanges
	}
	contentType := content.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.DataFromReader(content.StatusCode, content.ContentLength, contentType, content.Body, headers)
}

func (h *MediaTaskHandler) writeContentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrMediaTaskNotFound):
		writeMediaError(c, http.StatusNotFound, "media_task_not_found", "Media task not found")
	case errors.Is(err, service.ErrInvalidMediaRange), errors.Is(err, service.ErrMediaRangeNotSatisfiable):
		writeMediaError(c, http.StatusRequestedRangeNotSatisfiable, "invalid_range", "The requested range is invalid or unsatisfiable")
	default:
		writeMediaError(c, http.StatusBadGateway, "media_content_unavailable", "Media content is temporarily unavailable")
	}
}

type mediaIdentity struct {
	userID, apiKeyID, groupID int64
}

func mediaCreateIdentity(c *gin.Context) (mediaIdentity, bool) {
	apiKey, keyOK := middleware2.GetAPIKeyFromContext(c)
	subject, subjectOK := middleware2.GetAuthSubjectFromContext(c)
	if !keyOK || !subjectOK || apiKey == nil || apiKey.GroupID == nil || *apiKey.GroupID <= 0 ||
		apiKey.UserID <= 0 || subject.UserID != apiKey.UserID {
		return mediaIdentity{}, false
	}
	return mediaIdentity{userID: apiKey.UserID, apiKeyID: apiKey.ID, groupID: *apiKey.GroupID}, true
}

func (h *MediaTaskHandler) decodeJSON(c *gin.Context, target any) bool {
	if h == nil || h.app == nil || h.stager == nil {
		writeMediaError(c, http.StatusServiceUnavailable, "media_unavailable", "Media generation is temporarily unavailable")
		return false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxRequestBytes)
	decoder := json.NewDecoder(c.Request.Body)
	if err := decoder.Decode(target); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeMediaError(c, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
		} else {
			writeMediaError(c, http.StatusBadRequest, "invalid_request", "Request body is invalid")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeMediaError(c, http.StatusBadRequest, "invalid_request", "Request body is invalid")
		return false
	}
	return true
}

func (h *MediaTaskHandler) parseMultipart(c *gin.Context) (*multipart.Form, bool) {
	if h == nil || h.app == nil || h.stager == nil {
		writeMediaError(c, http.StatusServiceUnavailable, "media_unavailable", "Media generation is temporarily unavailable")
		return nil, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxRequestBytes)
	if err := c.Request.ParseMultipartForm(h.maxRequestBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) || strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			writeMediaError(c, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large")
		} else {
			writeMediaError(c, http.StatusBadRequest, "invalid_multipart", "Multipart request is invalid")
		}
		return nil, false
	}
	if c.Request.MultipartForm == nil {
		writeMediaError(c, http.StatusBadRequest, "invalid_multipart", "Multipart request is invalid")
		return nil, false
	}
	return c.Request.MultipartForm, true
}

func (h *MediaTaskHandler) stageUploads(ctx context.Context, userID int64, files []*multipart.FileHeader, mediaType service.MediaType) ([]service.MediaArtifactInput, error) {
	pending := make([]service.MediaArtifactInput, 0, len(files))
	for position, header := range files {
		if header == nil || header.Size <= 0 || header.Size > h.maxRequestBytes || !allowedMediaUploadExtension(header.Filename, mediaType) {
			return nil, service.ErrInvalidMediaInput
		}
		file, err := header.Open()
		if err != nil {
			return nil, service.ErrInvalidMediaInput
		}
		data, readErr := io.ReadAll(io.LimitReader(file, h.maxRequestBytes+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) > h.maxRequestBytes || !allowedDetectedMediaType(data, mediaType) {
			return nil, service.ErrInvalidMediaInput
		}
		pending = append(pending, service.MediaArtifactInput{
			Direction: "input", Position: position, MediaType: mediaType,
			ContentType: http.DetectContentType(data), Data: data, SizeBytes: int64(len(data)),
		})
	}
	return h.stageInputs(ctx, userID, pending)
}

func (h *MediaTaskHandler) stageInputs(ctx context.Context, userID int64, pending []service.MediaArtifactInput) ([]service.MediaArtifactInput, error) {
	stagedInputs := make([]service.MediaArtifactInput, 0, len(pending))
	for i := range pending {
		staged, err := h.stager.Stage(ctx, userID, pending[i])
		if err != nil {
			return nil, errors.Join(err, h.discardStagedInputs(ctx, userID, stagedInputs))
		}
		if len(staged.Data) != 0 || (staged.ObjectKey == "" && staged.UpstreamReference == "" && staged.ExternalURL == "") {
			return nil, errors.Join(service.ErrInvalidMediaInput, h.discardStagedInputs(ctx, userID, append(stagedInputs, staged)))
		}
		stagedInputs = append(stagedInputs, staged)
	}
	return stagedInputs, nil
}

func (h *MediaTaskHandler) discardStagedInputs(ctx context.Context, userID int64, inputs []service.MediaArtifactInput) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mediaStagingCleanupTimeout)
	defer cancel()
	var cleanupErrors []error
	for i := len(inputs) - 1; i >= 0; i-- {
		if err := h.stager.Discard(cleanupCtx, userID, inputs[i]); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("discard staged media input: %w", err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func (h *MediaTaskHandler) createMultipartVideo(c *gin.Context, identity mediaIdentity, idempotencyKey string) {
	form, ok := h.parseMultipart(c)
	if !ok {
		return
	}
	defer form.RemoveAll()
	if !validMultipartFileSet(form, map[string]struct{}{"image": {}, "video": {}}) {
		writeMediaError(c, http.StatusBadRequest, "invalid_media", "A supported media upload is required")
		return
	}
	async, ok := parseMultipartAsync(form.Value["async"])
	if !ok {
		writeMediaError(c, http.StatusBadRequest, "invalid_async", "async must be true, false, or omitted")
		return
	}
	durationSeconds, ok := parseOptionalPositiveFormInt(firstFormValue(form.Value, "duration_seconds"), 0)
	if !ok {
		writeMediaError(c, http.StatusBadRequest, "invalid_number", "duration_seconds must be a positive integer")
		return
	}
	fps, ok := parseOptionalPositiveFormInt(firstFormValue(form.Value, "fps"), 0)
	if !ok {
		writeMediaError(c, http.StatusBadRequest, "invalid_number", "fps must be a positive integer")
		return
	}
	var files []*multipart.FileHeader
	mediaType := service.MediaTypeVideo
	operation := service.MediaOperationVideoRemix
	imageFiles := form.File["image"]
	videoFiles := form.File["video"]
	if len(imageFiles) > 0 && len(videoFiles) > 0 {
		writeMediaError(c, http.StatusBadRequest, "invalid_media", "Provide image uploads or video uploads, not both")
		return
	}
	if len(imageFiles) > 0 {
		files = imageFiles
		mediaType = service.MediaTypeImage
		operation = service.MediaOperationImageToVideo
	} else {
		files = videoFiles
	}
	if len(files) == 0 || len(files) > maxMediaUploadFiles {
		writeMediaError(c, http.StatusBadRequest, "invalid_media", "A supported media upload is required")
		return
	}
	inputs, err := h.stageUploads(c.Request.Context(), identity.userID, files, mediaType)
	if err != nil {
		h.writeServiceError(c, err)
		return
	}
	req := service.MediaCreateRequest{
		UserID: identity.userID, APIKeyID: identity.apiKeyID, GroupID: identity.groupID,
		MediaType: service.MediaTypeVideo, Operation: operation,
		RequestedModel: strings.TrimSpace(firstFormValue(form.Value, "model")), ClientAsync: async,
		IdempotencyKey: idempotencyKey,
		Spec: service.MediaSpec{Video: &service.VideoSpec{
			Prompt:          firstFormValue(form.Value, "prompt"),
			DurationSeconds: durationSeconds,
			Resolution:      firstFormValue(form.Value, "resolution"), FPS: fps,
		}},
		Inputs: inputs,
	}
	if !h.create(c, req) {
		_ = h.discardStagedInputs(c.Request.Context(), identity.userID, inputs)
	}
}

func (h *MediaTaskHandler) create(c *gin.Context, req service.MediaCreateRequest) bool {
	result, err := h.app.Create(c.Request.Context(), req)
	inputsAdopted := result != nil && result.InputsAdopted
	if err != nil {
		h.writeServiceError(c, err)
		return inputsAdopted
	}
	if result == nil || result.Task == nil {
		writeMediaError(c, http.StatusBadGateway, "media_generation_failed", "Media generation failed")
		return inputsAdopted
	}
	switch result.Disposition {
	case service.MediaCreateDispositionAccepted, service.MediaCreateDispositionFallbackAsync:
		c.JSON(http.StatusAccepted, taskResponse(result.Task, result.Artifacts))
		return inputsAdopted
	case service.MediaCreateDispositionCompleted:
		if result.Task.MediaType == service.MediaTypeImage {
			c.JSON(http.StatusOK, imageResponse(result.Task, result.Artifacts))
			return inputsAdopted
		}
		c.JSON(http.StatusOK, taskResponse(result.Task, result.Artifacts))
		return inputsAdopted
	case service.MediaCreateDispositionGatewayTimeout:
		writeMediaError(c, http.StatusGatewayTimeout, "media_gateway_timeout", "Media generation did not complete before the gateway timeout")
		return inputsAdopted
	default:
		writeMediaError(c, http.StatusBadGateway, "media_generation_failed", "Media generation failed")
		return inputsAdopted
	}
}

func (h *MediaTaskHandler) getTask(c *gin.Context, expectedType service.MediaType) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		writeMediaError(c, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
		return
	}
	if h == nil || h.app == nil {
		writeMediaError(c, http.StatusServiceUnavailable, "media_unavailable", "Media generation is temporarily unavailable")
		return
	}
	task, artifacts, err := h.app.GetForUser(c.Request.Context(), c.Param("id"), subject.UserID)
	if err != nil || task == nil || task.MediaType != expectedType {
		if err == nil || errors.Is(err, service.ErrMediaTaskNotFound) {
			writeMediaError(c, http.StatusNotFound, "media_task_not_found", "Media task not found")
			return
		}
		h.writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, taskResponse(task, artifacts))
}

func taskResponse(task *service.MediaTask, artifacts []service.MediaArtifact) mediaTaskResponse {
	response := mediaTaskResponse{
		ID: task.PublicID, Object: "media.task", MediaType: string(task.MediaType), Operation: string(task.Operation),
		Model: task.RequestedModel, Status: publicMediaStatus(task.Status), Progress: task.Progress, CreatedAt: task.CreatedAt.Unix(),
	}
	if task.Status == service.MediaTaskStatusCompleted {
		if task.MediaType == service.MediaTypeVideo {
			result, _ := json.Marshal(map[string]string{"content_url": "/v1/videos/" + url.PathEscape(task.PublicID) + "/content"})
			response.Result = result
		} else {
			result, _ := json.Marshal(map[string]any{"data": imageData(artifacts)})
			response.Result = result
		}
	}
	if task.Status == service.MediaTaskStatusFailed {
		response.Error = safeMediaTaskError(task.ErrorCode)
	}
	return response
}

func imageResponse(task *service.MediaTask, artifacts []service.MediaArtifact) mediaImageResponse {
	created := time.Now().Unix()
	if task != nil && !task.CreatedAt.IsZero() {
		created = task.CreatedAt.Unix()
	}
	return mediaImageResponse{Created: created, Data: imageData(artifacts)}
}

func imageData(artifacts []service.MediaArtifact) []mediaImageDataItem {
	data := make([]mediaImageDataItem, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.Direction != "output" || artifact.MediaType != service.MediaTypeImage || !safePublicMediaURL(artifact.PublicURL) {
			continue
		}
		data = append(data, mediaImageDataItem{URL: artifact.PublicURL})
	}
	return data
}

func safePublicMediaURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return true
}

func publicMediaStatus(status service.MediaTaskStatus) string {
	switch status {
	case service.MediaTaskStatusQueued, service.MediaTaskStatusInProgress, service.MediaTaskStatusCompleted, service.MediaTaskStatusFailed:
		return string(status)
	default:
		return string(service.MediaTaskStatusFailed)
	}
}

func safeMediaTaskError(code string) *mediaTaskError {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "content_policy_violation":
		return &mediaTaskError{Code: "content_policy_violation", Message: "The request was rejected by the content policy"}
	case "storage_unavailable":
		return &mediaTaskError{Code: "storage_unavailable", Message: "Media storage is temporarily unavailable"}
	case "sync_timeout", "timeout":
		return &mediaTaskError{Code: "timeout", Message: "Media generation timed out"}
	default:
		return &mediaTaskError{Code: "generation_failed", Message: "Media generation failed"}
	}
}

func mediaIdempotencyKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if len([]byte(key)) > maxMediaIdempotencyKeyBytes {
		writeMediaError(c, http.StatusBadRequest, "invalid_idempotency_key", "Idempotency-Key must not exceed 255 bytes")
		return "", false
	}
	return key, true
}

func parseMultipartAsync(values []string) (bool, bool) {
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" || strings.TrimSpace(values[0]) == "false" {
		return false, true
	}
	if strings.TrimSpace(values[0]) == "true" {
		return true, true
	}
	return false, false
}

func firstFormValue(values map[string][]string, key string) string {
	if len(values[key]) == 0 {
		return ""
	}
	return values[key][0]
}

func validMultipartFileSet(form *multipart.Form, allowed map[string]struct{}) bool {
	if form == nil {
		return false
	}
	count := 0
	for field, files := range form.File {
		if _, ok := allowed[field]; !ok && len(files) > 0 {
			return false
		}
		count += len(files)
		if count > maxMediaUploadFiles {
			return false
		}
	}
	return true
}

func parseOptionalPositiveFormInt(raw string, fallback int) (int, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fallback, true
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil || value <= 0 {
		return 0, false
	}
	return value, true
}

func allowedMediaUploadExtension(name string, mediaType service.MediaType) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if mediaType == service.MediaTypeImage {
		return ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".webp"
	}
	return ext == ".mp4" || ext == ".webm" || ext == ".mov"
}

func allowedDetectedMediaType(data []byte, mediaType service.MediaType) bool {
	detected := strings.ToLower(strings.SplitN(http.DetectContentType(data), ";", 2)[0])
	if mediaType == service.MediaTypeImage {
		return detected == "image/png" || detected == "image/jpeg" || detected == "image/webp"
	}
	return detected == "video/mp4" || detected == "video/webm" || detected == "video/quicktime"
}

func (h *MediaTaskHandler) writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrMediaTaskNotFound):
		writeMediaError(c, http.StatusNotFound, "media_task_not_found", "Media task not found")
	case errors.Is(err, service.ErrGroupNotFound):
		writeMediaError(c, http.StatusNotFound, "group_not_found", "Media group not found")
	case errors.Is(err, service.ErrMediaIdempotencyConflict):
		writeMediaError(c, http.StatusConflict, "idempotency_conflict", "Idempotency-Key conflicts with an earlier request")
	case errors.Is(err, service.ErrMediaTaskInitializing):
		writeMediaError(c, http.StatusConflict, "media_task_initializing", "Media task initialization is still in progress")
	case errors.Is(err, service.ErrMediaContentRejected):
		writeMediaError(c, http.StatusForbidden, "content_policy_violation", "The media request was rejected by content policy")
	case errors.Is(err, service.ErrMediaGenerationNotAllowed):
		writeMediaError(c, http.StatusForbidden, "media_generation_not_allowed", "Media generation is not allowed for this group")
	case errors.Is(err, service.ErrMediaInputNotRecoverable):
		writeMediaError(c, http.StatusBadRequest, "invalid_media_input", "The media input is invalid")
	case errors.Is(err, service.ErrInvalidMediaRange), errors.Is(err, service.ErrMediaRangeNotSatisfiable):
		writeMediaError(c, http.StatusRequestedRangeNotSatisfiable, "invalid_range", "The requested range is invalid or unsatisfiable")
	case errors.Is(err, service.ErrMediaVideoObjectStorageRequired):
		writeMediaError(c, http.StatusServiceUnavailable, "video_storage_required", "Video uploads require configured object storage")
	case errors.Is(err, service.ErrMediaArtifactNotFound), errors.Is(err, service.ErrMediaContentUnavailable),
		errors.Is(err, service.ErrMediaArtifactObjectStoreDisabled), errors.Is(err, service.ErrMediaContentTooLarge):
		writeMediaError(c, http.StatusBadGateway, "media_content_unavailable", "Media content is temporarily unavailable")
	case errors.Is(err, service.ErrInvalidMediaInput), errors.Is(err, service.ErrInvalidMediaSpec),
		errors.Is(err, service.ErrMediaModelNotFound), errors.Is(err, service.ErrMediaOperationUnsupported),
		errors.Is(err, service.ErrMediaSpecOutsideModelConstraints):
		writeMediaError(c, http.StatusBadRequest, "invalid_request", "The media request is invalid")
	default:
		writeMediaError(c, http.StatusInternalServerError, "internal_error", "The media request could not be completed")
	}
}

func writeMediaError(c *gin.Context, status int, code, message string) {
	c.JSON(status, mediaAPIErrorEnvelope{Error: mediaAPIError{Code: code, Message: message, Type: "invalid_request_error"}})
}

var (
	_ MediaTaskApplication        = (*service.MediaOrchestrator)(nil)
	_ MediaVideoContentOpener     = (*service.MediaContentService)(nil)
	_ service.MediaInputLifecycle = (*service.MediaContentService)(nil)
)

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mediaTaskApplicationStub struct {
	lastCreate                service.MediaCreateRequest
	createResult              *service.MediaCreateResult
	createErr                 error
	createCalls               int
	getTask                   *service.MediaTask
	getArtifacts              []service.MediaArtifact
	getErr                    error
	content                   *service.MediaContent
	contentErr                error
	contentCalls              int
	lastRange                 string
	stageErr                  error
	stageErrAt                int
	stageCalls                int
	discarded                 []service.MediaArtifactInput
	requireLiveDiscardContext bool
	discardHasDeadline        bool
}

type handlerMediaHTTPReader struct{}

func (handlerMediaHTTPReader) ValidateURL(raw string) (string, error) { return raw, nil }
func (handlerMediaHTTPReader) Open(context.Context, service.MediaHTTPContentRequest) (*service.MediaContent, error) {
	return nil, service.ErrMediaContentUnavailable
}

func (s *mediaTaskApplicationStub) Create(_ context.Context, req service.MediaCreateRequest) (*service.MediaCreateResult, error) {
	s.createCalls++
	s.lastCreate = req
	return s.createResult, s.createErr
}

func (s *mediaTaskApplicationStub) GetForUser(_ context.Context, publicID string, userID int64) (*service.MediaTask, []service.MediaArtifact, error) {
	if publicID != "task_public" || userID != 42 {
		return nil, nil, service.ErrMediaTaskNotFound
	}
	if s.getErr != nil {
		return nil, nil, s.getErr
	}
	if s.getTask != nil {
		return s.getTask, s.getArtifacts, nil
	}
	return &service.MediaTask{
		PublicID: "task_public", MediaType: service.MediaTypeVideo, Operation: service.MediaOperationTextToVideo,
		RequestedModel: "fake-video", Status: service.MediaTaskStatusQueued, CreatedAt: time.Unix(1784112000, 0),
	}, nil, nil
}

func (s *mediaTaskApplicationStub) OpenVideo(_ context.Context, _ string, _ int64, byteRange string) (*service.MediaContent, error) {
	s.contentCalls++
	s.lastRange = byteRange
	return s.content, s.contentErr
}

func (s *mediaTaskApplicationStub) Stage(_ context.Context, _ int64, input service.MediaArtifactInput) (service.MediaArtifactInput, error) {
	s.stageCalls++
	if s.stageErr != nil && (s.stageErrAt == 0 || s.stageCalls == s.stageErrAt) {
		return service.MediaArtifactInput{}, s.stageErr
	}
	input.Data = nil
	input.ObjectKey = fmt.Sprintf("staged/input-%d", s.stageCalls)
	return input, nil
}

func (s *mediaTaskApplicationStub) Discard(ctx context.Context, _ int64, input service.MediaArtifactInput) error {
	if _, ok := ctx.Deadline(); ok {
		s.discardHasDeadline = true
	}
	if s.requireLiveDiscardContext && ctx.Err() != nil {
		return ctx.Err()
	}
	s.discarded = append(s.discarded, input)
	return nil
}

func imageEditRequest(t *testing.T, async string) (*http.Request, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "fake-edit"))
	require.NoError(t, w.WriteField("prompt", "add a hat"))
	if async != "<omitted>" {
		require.NoError(t, w.WriteField("async", async))
	}
	part, err := w.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, "/v1/images/edits", &body, 42)
	contentType := w.FormDataContentType()
	req.Header.Set("Content-Type", contentType)
	return req, contentType
}

func multiImageEditRequest(t *testing.T, files map[string][]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "fake-edit"))
	require.NoError(t, w.WriteField("prompt", "edit"))
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data := files[name]
		part, err := w.CreateFormFile("image", name)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, "/v1/images/edits", &body, 42)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func videoUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "fake-video"))
	require.NoError(t, w.WriteField("prompt", "remix"))
	part, err := w.CreateFormFile("video", "input.mp4")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00mp42isom"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, "/v1/videos", &body, 42)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func mediaMultipartRequestWithFields(
	t *testing.T,
	path string,
	fileField string,
	fileName string,
	fileData []byte,
	fields map[string]string,
) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range fields {
		require.NoError(t, w.WriteField(key, value))
	}
	part, err := w.CreateFormFile(fileField, fileName)
	require.NoError(t, err)
	_, err = part.Write(fileData)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, path, &body, 42)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func disguisedVideoUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "fake-video"))
	require.NoError(t, w.WriteField("prompt", "remix"))
	part, err := w.CreateFormFile("video", "not-really-video.mp4")
	require.NoError(t, err)
	_, err = part.Write([]byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07})
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, "/v1/videos", &body, 42)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func mixedVideoUploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	require.NoError(t, w.WriteField("model", "fake-video"))
	require.NoError(t, w.WriteField("prompt", "ambiguous"))
	image, err := w.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = image.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	video, err := w.CreateFormFile("video", "input.mp4")
	require.NoError(t, err)
	_, err = video.Write([]byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00mp42isom"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, "/v1/videos", &body, 42)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func newStandaloneMediaRouter(t *testing.T) (*gin.Engine, *mediaTaskApplicationStub) {
	return newStandaloneMediaRouterWithStager(t, nil)
}

func newStandaloneMediaRouterWithStager(t *testing.T, stager service.MediaInputLifecycle) (*gin.Engine, *mediaTaskApplicationStub) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	app := &mediaTaskApplicationStub{createResult: &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeVideo, Operation: service.MediaOperationTextToVideo,
			RequestedModel: "fake-video", Status: service.MediaTaskStatusQueued, CreatedAt: time.Unix(1784112000, 0),
		},
		Disposition: service.MediaCreateDispositionAccepted, InputsAdopted: true,
	}}
	if stager == nil {
		stager = app
	}
	h := NewMediaTaskHandler(app, app, stager, &config.Config{Server: config.ServerConfig{MaxRequestBodySize: 1 << 20}})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID, _ := strconv.ParseInt(c.GetHeader("X-Test-User-ID"), 10, 64)
		c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID})
		if c.GetHeader("X-Test-API-Key") != "" {
			apiKeyUserID := userID
			if raw := c.GetHeader("X-Test-API-Key-User-ID"); raw != "" {
				apiKeyUserID, _ = strconv.ParseInt(raw, 10, 64)
			}
			groupID := int64(7)
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: 8, UserID: apiKeyUserID, GroupID: &groupID})
		}
		c.Next()
	})
	router.POST("/v1/images/generations", h.CreateImageGeneration)
	router.POST("/v1/images/edits", h.CreateImageEdit)
	router.GET("/v1/images/:id", h.GetImageTask)
	router.POST("/v1/videos", h.CreateVideo)
	router.GET("/v1/videos/:id", h.GetVideoTask)
	router.GET("/v1/videos/:id/content", h.GetVideoContent)
	return router, app
}

func TestMediaTaskHandlerExternalURLUsesRealStagerContentTypeContract(t *testing.T) {
	stager := service.NewMediaContentService(nil, nil, nil, nil, nil, handlerMediaHTTPReader{}, service.NewDisabledMediaArtifactObjectStore())

	t.Run("valid image URL reaches create", func(t *testing.T) {
		router, app := newStandaloneMediaRouterWithStager(t, stager)
		rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{
			"model":"fake-video","prompt":"sunset","image_url":"https://media.example/input.png?token=internal"
		}`, 42)
		require.Equal(t, http.StatusAccepted, rec.Code)
		require.Equal(t, 1, app.createCalls)
		require.Equal(t, "image/png", app.lastCreate.Inputs[0].ContentType)
	})

	for _, raw := range []string{
		"https://media.example/input.mp4",
		"https://media.example/no-extension",
	} {
		t.Run(raw, func(t *testing.T) {
			router, app := newStandaloneMediaRouterWithStager(t, stager)
			rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{
				"model":"fake-video","prompt":"sunset","image_url":"`+raw+`"
			}`, 42)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, app.createCalls)
		})
	}
}

func newAPIKeyRequest(method, path string, body io.Reader, userID int64) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User-ID", strconv.FormatInt(userID, 10))
	return req
}

func performRequest(router *gin.Engine, req *http.Request, userID int64, apiKey bool) *httptest.ResponseRecorder {
	if apiKey {
		req.Header.Set("X-Test-API-Key", "set")
	}
	req.Header.Set("X-Test-User-ID", strconv.FormatInt(userID, 10))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func performAPIKeyRequest(router *gin.Engine, method, path, body string, userID int64) *httptest.ResponseRecorder {
	return performRequest(router, newAPIKeyRequest(method, path, bytes.NewBufferString(body), userID), userID, true)
}

func performAuthenticatedRequest(router *gin.Engine, method, path, body string, userID int64) *httptest.ResponseRecorder {
	return performRequest(router, newAPIKeyRequest(method, path, bytes.NewBufferString(body), userID), userID, false)
}

func TestMediaTaskHandlerAsyncVideoReturns202(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{"model":"fake-video","prompt":"sunset","async":true}`, 42)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.JSONEq(t, `{
		"id":"task_public","object":"media.task","media_type":"video",
		"operation":"text_to_video","model":"fake-video","status":"queued",
		"progress":0,"created_at":1784112000
	}`, rec.Body.String())
	require.True(t, app.lastCreate.ClientAsync)
}

func TestMediaTaskHandlerRejectsMismatchedAuthSubjectAndAPIKeyUser(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	req := newAPIKeyRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"fake-video","prompt":"sunset"}`), 42)
	req.Header.Set("X-Test-API-Key-User-ID", "99")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Zero(t, app.createCalls)
}

func TestMediaTaskHandlerSyncImageKeepsOpenAIResponseShape(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task:        &service.MediaTask{PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage, RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0)},
		Artifacts:   []service.MediaArtifact{{Direction: "output", MediaType: service.MediaTypeImage, PublicURL: "https://cdn.example/image.png"}},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"created":1784112000,"data":[{"url":"https://cdn.example/image.png"}]}`, rec.Body.String())
}

func TestMediaTaskHandlerImageEditMultipartAsync(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "fake-edit", "prompt": "add a hat", "async": "true"} {
		require.NoError(t, w.WriteField(key, value))
	}
	part, err := w.CreateFormFile("image", "input.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("\x89PNG\r\n\x1a\n"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	req := newAPIKeyRequest(http.MethodPost, "/v1/images/edits", &body, 42)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Equal(t, service.MediaOperationImageEdit, app.lastCreate.Operation)
	require.True(t, app.lastCreate.ClientAsync)
	require.NotEmpty(t, app.lastCreate.Inputs)
	require.Nil(t, app.lastCreate.Inputs[0].Data)
}

func TestMediaTaskHandlerValidatesEveryUploadBeforeStaging(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	req := multiImageEditRequest(t, map[string][]byte{
		"first.png":  []byte("\x89PNG\r\n\x1a\n"),
		"second.png": {0x00, 0x01, 0x02, 0x03},
	})
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, app.stageCalls)
	require.Zero(t, app.createCalls)
}

func TestMediaTaskHandlerCleansStagedInputsOnPartialStageFailure(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.stageErr = service.ErrInvalidMediaInput
	app.stageErrAt = 2
	req := multiImageEditRequest(t, map[string][]byte{
		"first.png":  []byte("\x89PNG\r\n\x1a\n"),
		"second.png": []byte("\x89PNG\r\n\x1a\n"),
	})
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, 2, app.stageCalls)
	require.Len(t, app.discarded, 1)
	require.Equal(t, "staged/input-1", app.discarded[0].ObjectKey)
	require.Zero(t, app.createCalls)
}

func TestMediaTaskHandlerCleansStagedInputsWhenApplicationRejects(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = nil
	app.createErr = service.ErrMediaInputNotRecoverable
	req, _ := imageEditRequest(t, "false")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, app.discarded, 1)
	require.Equal(t, "staged/input-1", app.discarded[0].ObjectKey)
}

func TestMediaTaskHandlerKeepsStagedInputsWhenApplicationAdoptedBeforeError(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task:          &service.MediaTask{PublicID: "task_public", MediaType: service.MediaTypeImage},
		InputsAdopted: true,
	}
	app.createErr = context.Canceled
	req, _ := imageEditRequest(t, "false")

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Empty(t, app.discarded)
}

func TestMediaTaskHandlerCleansStagedInputsWhenApplicationExplicitlyRejectsOwnership(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task:          &service.MediaTask{PublicID: "task_public", MediaType: service.MediaTypeImage},
		InputsAdopted: false,
	}
	app.createErr = service.ErrMediaInputNotRecoverable
	req, _ := imageEditRequest(t, "false")

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, app.discarded, 1)
}

func TestMediaTaskHandlerCleansMultipleStagedInputsInReverseOrderWhenApplicationRejects(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = nil
	app.createErr = service.ErrMediaInputNotRecoverable
	req := multiImageEditRequest(t, map[string][]byte{
		"first.png":  []byte("\x89PNG\r\n\x1a\n"),
		"second.png": []byte("\x89PNG\r\n\x1a\n"),
	})

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, app.discarded, 2)
	require.Equal(t, "staged/input-2", app.discarded[0].ObjectKey)
	require.Equal(t, "staged/input-1", app.discarded[1].ObjectKey)
}

func TestMediaTaskHandlerKeepsStagedInputsWhenApplicationAccepts(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	req, _ := imageEditRequest(t, "true")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.Empty(t, app.discarded)
}

func TestMediaTaskHandlerKeepsStagedInputsForEveryAdoptedSuccessDisposition(t *testing.T) {
	for _, tt := range []struct {
		name        string
		disposition service.MediaCreateDisposition
		status      int
	}{
		{name: "fallback async", disposition: service.MediaCreateDispositionFallbackAsync, status: http.StatusAccepted},
		{name: "completed", disposition: service.MediaCreateDispositionCompleted, status: http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			app.createResult = &service.MediaCreateResult{
				Task: &service.MediaTask{
					PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationImageEdit,
					Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
				},
				Disposition: tt.disposition, InputsAdopted: true,
			}
			req, _ := imageEditRequest(t, "false")

			rec := performRequest(router, req, 42, true)
			require.Equal(t, tt.status, rec.Code)
			require.Empty(t, app.discarded)
		})
	}
}

func TestMediaTaskHandlerGatewayTimeoutDoesNotExposeTaskID(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task:          &service.MediaTask{PublicID: "task_secret", MediaType: service.MediaTypeVideo},
		Disposition:   service.MediaCreateDispositionGatewayTimeout,
		InputsAdopted: true,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{"model":"fake-video","prompt":"sunset"}`, 42)
	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
	require.NotContains(t, rec.Body.String(), "task_secret")
}

func TestMediaTaskHandlerKeepsStagedInputOnGatewayTimeoutAfterAdoption(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task:          &service.MediaTask{PublicID: "task_secret", MediaType: service.MediaTypeVideo},
		Disposition:   service.MediaCreateDispositionGatewayTimeout,
		InputsAdopted: true,
	}

	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{
		"model":"fake-video","prompt":"sunset","image_url":"https://media.example/input.png"
	}`, 42)
	require.Equal(t, http.StatusGatewayTimeout, rec.Code)
	require.Empty(t, app.discarded)
}

func TestMediaTaskHandlerCleansStagedInputAfterRequestCancellation(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = nil
	app.createErr = service.ErrMediaInputNotRecoverable
	app.requireLiveDiscardContext = true
	req := newAPIKeyRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"fake-video","prompt":"sunset","image_url":"https://media.example/input.png"
	}`), 42)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Len(t, app.discarded, 1)
	require.Equal(t, "staged/input-1", app.discarded[0].ObjectKey)
	require.True(t, app.discardHasDeadline)
}

func TestMediaTaskHandlerMapsStableServiceErrorsWithoutLeak(t *testing.T) {
	for _, tt := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "idempotency conflict", err: service.ErrMediaIdempotencyConflict, status: http.StatusConflict, code: "idempotency_conflict"},
		{name: "content rejected", err: service.ErrMediaContentRejected, status: http.StatusForbidden, code: "content_policy_violation"},
		{name: "generation disabled", err: service.ErrMediaGenerationNotAllowed, status: http.StatusForbidden, code: "media_generation_not_allowed"},
		{name: "input not recoverable", err: service.ErrMediaInputNotRecoverable, status: http.StatusBadRequest, code: "invalid_media_input"},
		{name: "task initializing", err: service.ErrMediaTaskInitializing, status: http.StatusConflict, code: "media_task_initializing"},
		{name: "group missing", err: service.ErrGroupNotFound, status: http.StatusNotFound, code: "group_not_found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			app.createErr = fmt.Errorf("internal upstream Authorization secret: %w", tt.err)
			rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{
				"model":"fake-image","prompt":"cat"
			}`, 42)
			require.Equal(t, tt.status, rec.Code)
			require.Contains(t, rec.Body.String(), `"code":"`+tt.code+`"`)
			require.NotContains(t, rec.Body.String(), "upstream")
			require.NotContains(t, rec.Body.String(), "Authorization")
			require.NotContains(t, rec.Body.String(), "secret")
		})
	}
}

func TestMediaTaskHandlerHidesTaskFromDifferentUser(t *testing.T) {
	router, _ := newStandaloneMediaRouter(t)
	rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/videos/task_other", "", 99)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "owner")
}

func TestMediaTaskHandlerVideoContentForwardsRangeWithoutLeakingUpstream(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.content = &service.MediaContent{
		Body: io.NopCloser(bytes.NewBufferString("2345")), StatusCode: http.StatusPartialContent,
		ContentType: "video/mp4", ContentLength: 4, ContentRange: "bytes 2-5/10", AcceptRanges: "bytes",
	}
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public/content", nil, 42)
	req.Header.Set("Range", "bytes=2-5")
	rec := performRequest(router, req, 42, false)
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, "bytes 2-5/10", rec.Header().Get("Content-Range"))
	require.Equal(t, "2345", rec.Body.String())
	require.NotContains(t, rec.Header().Get("Location"), "upstream")
}

func TestMediaRouterDoesNotExposeCancel(t *testing.T) {
	router, _ := newStandaloneMediaRouter(t)
	rec := performAuthenticatedRequest(router, http.MethodDelete, "/v1/videos/task_public", "", 42)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMediaTaskHandlerJSONAsyncThreeStates(t *testing.T) {
	for _, tt := range []struct {
		name, field string
		want        bool
	}{
		{name: "omitted", field: "", want: false},
		{name: "false", field: `,"async":false`, want: false},
		{name: "true", field: `,"async":true`, want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			body := `{"model":"fake-video","prompt":"sunset"` + tt.field + `}`
			rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", body, 42)
			require.Equal(t, http.StatusAccepted, rec.Code)
			require.Equal(t, tt.want, app.lastCreate.ClientAsync)
		})
	}
}

func TestMediaTaskHandlerMultipartAsyncThreeStates(t *testing.T) {
	for _, tt := range []struct {
		name, value string
		want        bool
		status      int
	}{
		{name: "omitted", value: "<omitted>", want: false, status: http.StatusAccepted},
		{name: "empty", value: "", want: false, status: http.StatusAccepted},
		{name: "false", value: "false", want: false, status: http.StatusAccepted},
		{name: "true", value: "true", want: true, status: http.StatusAccepted},
		{name: "uppercase rejected", value: "TRUE", status: http.StatusBadRequest},
		{name: "invalid", value: "1", status: http.StatusBadRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			req, _ := imageEditRequest(t, tt.value)
			rec := performRequest(router, req, 42, true)
			require.Equal(t, tt.status, rec.Code)
			if tt.status == http.StatusAccepted {
				require.Equal(t, tt.want, app.lastCreate.ClientAsync)
			} else {
				require.Zero(t, app.createCalls)
			}
		})
	}
}

func TestMediaTaskHandlerRejectsInvalidMultipartNumbersBeforeStaging(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n")
	mp4 := []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00mp42isom")
	for _, tt := range []struct {
		name, path, fileField, fileName string
		data                            []byte
		fields                          map[string]string
	}{
		{name: "negative image count", path: "/v1/images/edits", fileField: "image", fileName: "input.png", data: png, fields: map[string]string{"model": "fake", "prompt": "edit", "n": "-1"}},
		{name: "invalid image count", path: "/v1/images/edits", fileField: "image", fileName: "input.png", data: png, fields: map[string]string{"model": "fake", "prompt": "edit", "n": "many"}},
		{name: "negative duration", path: "/v1/videos", fileField: "video", fileName: "input.mp4", data: mp4, fields: map[string]string{"model": "fake", "prompt": "edit", "duration_seconds": "-1"}},
		{name: "invalid fps", path: "/v1/videos", fileField: "video", fileName: "input.mp4", data: mp4, fields: map[string]string{"model": "fake", "prompt": "edit", "fps": "fast"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			req := mediaMultipartRequestWithFields(t, tt.path, tt.fileField, tt.fileName, tt.data, tt.fields)
			rec := performRequest(router, req, 42, true)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, app.stageCalls)
			require.Zero(t, app.createCalls)
		})
	}
}

func TestMediaTaskHandlerIdempotencyKeyByteLimit(t *testing.T) {
	t.Run("255 bytes", func(t *testing.T) {
		router, app := newStandaloneMediaRouter(t)
		req := newAPIKeyRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"fake-video","prompt":"sunset"}`), 42)
		req.Header.Set("Idempotency-Key", strings.Repeat("a", 255))
		rec := performRequest(router, req, 42, true)
		require.Equal(t, http.StatusAccepted, rec.Code)
		require.Len(t, app.lastCreate.IdempotencyKey, 255)
	})
	t.Run("256 bytes", func(t *testing.T) {
		router, app := newStandaloneMediaRouter(t)
		req := newAPIKeyRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"fake-video","prompt":"sunset"}`), 42)
		req.Header.Set("Idempotency-Key", strings.Repeat("a", 256))
		rec := performRequest(router, req, 42, true)
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Zero(t, app.createCalls)
	})
}

func TestMediaTaskHandlerRejectsLongIdempotencyKeyBeforeStagingExternalInput(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	req := newAPIKeyRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{
		"model":"fake-video","prompt":"sunset","image_url":"https://media.example/input.png"
	}`), 42)
	req.Header.Set("Idempotency-Key", strings.Repeat("a", 256))
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, app.stageCalls)
	require.Zero(t, app.createCalls)
}

func TestMediaTaskHandlerVideoUploadWithoutStorageDoesNotCreateTask(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.stageErr = service.ErrMediaVideoObjectStorageRequired
	rec := performRequest(router, videoUploadRequest(t), 42, true)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Zero(t, app.createCalls)
	require.NotContains(t, rec.Body.String(), "input.mp4")
}

func TestMediaTaskHandlerRejectsVideoWhoseDetectedTypeDoesNotMatchExtension(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	rec := performRequest(router, disguisedVideoUploadRequest(t), 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, app.createCalls)
}

func TestMediaTaskHandlerRejectsAmbiguousImageAndVideoUpload(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	rec := performRequest(router, mixedVideoUploadRequest(t), 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Zero(t, app.stageCalls)
	require.Zero(t, app.createCalls)
}

func TestMediaTaskHandlerPublicDTOHidesInternalReferencesAndSignedURLs(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	accountID := int64(9)
	app.getTask = &service.MediaTask{
		PublicID: "task_public", AccountID: &accountID, UpstreamTaskID: "upstream_task_secret",
		PollMetadata: json.RawMessage(`{"authorization":"secret"}`), MediaType: service.MediaTypeVideo,
		Operation: service.MediaOperationTextToVideo, RequestedModel: "fake-video", Status: service.MediaTaskStatusCompleted,
		CreatedAt: time.Unix(1784112000, 0),
	}
	app.getArtifacts = []service.MediaArtifact{{
		Direction: "output", MediaType: service.MediaTypeVideo, ObjectKey: "private/key",
		UpstreamReference: "https://upstream.example/video?token=secret",
		PublicURL:         "https://cdn.example/video?X-Amz-Signature=secret",
	}}
	rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/videos/task_public", "", 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"content_url":"/v1/videos/task_public/content"`)
	for _, secret := range []string{"upstream_task_secret", "authorization", "private/key", "upstream.example", "X-Amz-Signature", "secret"} {
		require.NotContains(t, rec.Body.String(), secret)
	}
}

func TestMediaTaskHandlerFailedDTOHidesInternalErrorMessage(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.getTask = &service.MediaTask{
		PublicID: "task_public", MediaType: service.MediaTypeVideo, Operation: service.MediaOperationTextToVideo,
		RequestedModel: "fake-video", Status: service.MediaTaskStatusFailed, ErrorCode: "upstream_401",
		ErrorMessage: "Authorization Bearer secret at https://upstream.example", CreatedAt: time.Unix(1784112000, 0),
	}
	rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/videos/task_public", "", 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"generation_failed"`)
	require.NotContains(t, rec.Body.String(), "Authorization")
	require.NotContains(t, rec.Body.String(), "upstream.example")
}

func TestMediaTaskHandlerImageDTOOmitsSignedAndInternalURLs(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
			RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		},
		Artifacts: []service.MediaArtifact{
			{Direction: "output", MediaType: service.MediaTypeImage, PublicURL: "https://cdn.example/signed?sig=secret", UpstreamReference: "https://upstream.example/private"},
			{Direction: "output", MediaType: service.MediaTypeImage, PublicURL: "https://cdn.example/public.png"},
		},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "https://cdn.example/public.png")
	for _, secret := range []string{"token", "secret", "upstream.example"} {
		require.NotContains(t, rec.Body.String(), secret)
	}
}

func TestMediaTaskHandlerCompletedImageQueryReturnsTaskWithOpenAIDataResult(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.getTask = &service.MediaTask{
		PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
		RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
	}
	app.getArtifacts = []service.MediaArtifact{{
		Direction: "output", MediaType: service.MediaTypeImage, PublicURL: "https://cdn.example/public.png",
	}}
	rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/images/task_public", "", 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"object":"media.task"`)
	require.Contains(t, rec.Body.String(), `"result":{"data":[{"url":"https://cdn.example/public.png"}]}`)
}

func TestMediaTaskHandlerInvalidRangeReturns416WithoutOpeningContent(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public/content", nil, 42)
	req.Header.Set("Range", "bytes=0-1,4-5")
	rec := performRequest(router, req, 42, false)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, rec.Code)
	require.Zero(t, app.contentCalls)
}

func TestMediaTaskHandlerContentOpenerRangeErrorReturns416(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.contentErr = fmt.Errorf("object store range: %w", service.ErrMediaRangeNotSatisfiable)
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public/content", nil, 42)
	req.Header.Set("Range", "bytes=99-100")

	rec := performRequest(router, req, 42, false)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, rec.Code)
	require.Equal(t, 1, app.contentCalls)
	require.Contains(t, rec.Body.String(), `"code":"invalid_range"`)
	require.NotContains(t, rec.Body.String(), "object store")
}

func TestMediaTaskHandlerContentOpenerFailuresReturn502WithoutCauseLeak(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "secure proxy unsupported", err: service.ErrSecureHTTPUpstreamProxyUnsupported},
		{name: "secure upstream required", err: service.ErrMediaSecureUpstreamRequired},
		{name: "network failure", err: errors.New("dial tcp internal-upstream: secret network failure")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			app.contentErr = tt.err

			rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/videos/task_public/content", "", 42)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.Contains(t, rec.Body.String(), `"code":"media_content_unavailable"`)
			require.NotContains(t, rec.Body.String(), "internal-upstream")
			require.NotContains(t, rec.Body.String(), "secret")
		})
	}
}

func TestMediaTaskHandlerMissingCompletedArtifactReturns502WithoutLeak(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.contentErr = service.ErrMediaArtifactNotFound
	rec := performAuthenticatedRequest(router, http.MethodGet, "/v1/videos/task_public/content", "", 42)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.Contains(t, rec.Body.String(), "media_content_unavailable")
	require.NotContains(t, rec.Body.String(), "artifact")
}

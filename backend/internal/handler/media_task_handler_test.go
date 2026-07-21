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
	"net/url"
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
	lastImagePosition         int
	stageErr                  error
	stageErrAt                int
	invalidStageAt            int
	stageCalls                int
	discarded                 []service.MediaArtifactInput
	discardErr                error
	requireLiveDiscardContext bool
	discardHasDeadline        bool
}

type mediaInputCleanupObserverStub struct {
	calls          int
	operation      service.MediaOperation
	inputCount     int
	classification string
}

func (s *mediaInputCleanupObserverStub) ObserveMediaInputCleanupFailure(
	_ context.Context,
	operation service.MediaOperation,
	inputCount int,
	classification string,
) {
	s.calls++
	s.operation = operation
	s.inputCount = inputCount
	s.classification = classification
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

func (s *mediaTaskApplicationStub) GetForUser(_ context.Context, publicID string, userID, apiKeyID int64) (*service.MediaTask, []service.MediaArtifact, error) {
	if publicID != "task_public" || userID != 42 || apiKeyID != 8 {
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

func (s *mediaTaskApplicationStub) OpenVideo(_ context.Context, _ string, _ int64, _ int64, byteRange string) (*service.MediaContent, error) {
	s.contentCalls++
	s.lastRange = byteRange
	return s.content, s.contentErr
}

func (s *mediaTaskApplicationStub) OpenImage(_ context.Context, _ string, _ int64, _ int64, position int, byteRange string) (*service.MediaContent, error) {
	s.contentCalls++
	s.lastImagePosition = position
	s.lastRange = byteRange
	return s.content, s.contentErr
}

func (s *mediaTaskApplicationStub) Stage(_ context.Context, _ int64, input service.MediaArtifactInput) (service.MediaArtifactInput, error) {
	s.stageCalls++
	if s.stageErr != nil && (s.stageErrAt == 0 || s.stageCalls == s.stageErrAt) {
		return service.MediaArtifactInput{}, s.stageErr
	}
	if s.invalidStageAt > 0 && s.stageCalls == s.invalidStageAt {
		return service.MediaArtifactInput{}, nil
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
	return s.discardErr
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
	return newStandaloneMediaRouterWithStagerAndCleanupObserver(t, nil, nil)
}

func newStandaloneMediaRouterWithStager(t *testing.T, stager service.MediaInputLifecycle) (*gin.Engine, *mediaTaskApplicationStub) {
	return newStandaloneMediaRouterWithStagerAndCleanupObserver(t, stager, nil)
}

func newStandaloneMediaRouterWithStagerAndCleanupObserver(
	t *testing.T,
	stager service.MediaInputLifecycle,
	observer MediaInputCleanupObserver,
) (*gin.Engine, *mediaTaskApplicationStub) {
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
	if observer != nil {
		h.SetInputCleanupObserver(observer)
	}
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
			apiKeyID := int64(8)
			if raw := c.GetHeader("X-Test-API-Key-ID"); raw != "" {
				apiKeyID, _ = strconv.ParseInt(raw, 10, 64)
			}
			c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{ID: apiKeyID, UserID: apiKeyUserID, GroupID: &groupID})
		}
		c.Next()
	})
	router.POST("/v1/images/generations", h.CreateImageGeneration)
	router.POST("/v1/images/edits", h.CreateImageEdit)
	router.GET("/v1/images/:id/content", h.GetImageContent)
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

func TestMediaTaskHandlerAcceptsStrictQuickTimeMOVUpload(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{name: "major brand", data: []byte{
			0x00, 0x00, 0x00, 0x14, 'f', 't', 'y', 'p',
			'q', 't', ' ', ' ', 0x00, 0x00, 0x00, 0x00,
			'q', 't', ' ', ' ',
		}},
		{name: "compatible brand", data: []byte{
			0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p',
			'i', 's', 'o', 'm', 0x00, 0x00, 0x00, 0x00,
			'm', 'p', '4', '2', 'q', 't', ' ', ' ',
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			req := mediaMultipartRequestWithFields(
				t, "/v1/videos", "video", "input.mov", tt.data,
				map[string]string{"model": "fake-video", "prompt": "remix"},
			)

			rec := performRequest(router, req, 42, true)
			require.Equal(t, http.StatusAccepted, rec.Code)
			require.Equal(t, 1, app.stageCalls)
			require.Equal(t, 1, app.createCalls)
			require.Len(t, app.lastCreate.Inputs, 1)
			require.Equal(t, "video/quicktime", app.lastCreate.Inputs[0].ContentType)
			require.Nil(t, app.lastCreate.Inputs[0].Data)
		})
	}
}

func TestMediaTaskHandlerRejectsMalformedOrNonQuickTimeMOVUpload(t *testing.T) {
	oversizedFTYP := make([]byte, quickTimeBoxScanBytes+4)
	copy(oversizedFTYP, []byte{
		0x00, 0x00, 0x10, 0x04, 'f', 't', 'y', 'p',
		'q', 't', ' ', ' ', 0x00, 0x00, 0x00, 0x00,
	})
	for _, tt := range []struct {
		name string
		data []byte
	}{
		{name: "arbitrary ftyp substring", data: []byte("not-a-box-ftypqt  ")},
		{name: "ftyp exceeds bounded scan", data: oversizedFTYP},
		{name: "truncated box", data: []byte{
			0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p',
			'q', 't', ' ', ' ', 0x00, 0x00, 0x00, 0x00,
			'q', 't', ' ', ' ',
		}},
		{name: "wrong brand", data: []byte{
			0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p',
			'i', 's', 'o', 'm', 0x00, 0x00, 0x00, 0x00,
			'm', 'p', '4', '2', 'i', 's', 'o', 'm',
		}},
		{name: "undersized ftyp", data: []byte{
			0x00, 0x00, 0x00, 0x0c, 'f', 't', 'y', 'p',
			'q', 't', ' ', ' ',
		}},
		{name: "misaligned compatible brands", data: []byte{
			0x00, 0x00, 0x00, 0x12, 'f', 't', 'y', 'p',
			'q', 't', ' ', ' ', 0x00, 0x00, 0x00, 0x00, 'q', 't',
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			req := mediaMultipartRequestWithFields(
				t, "/v1/videos", "video", "input.mov", tt.data,
				map[string]string{"model": "fake-video", "prompt": "remix"},
			)

			rec := performRequest(router, req, 42, true)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, app.stageCalls)
			require.Zero(t, app.createCalls)
		})
	}
}

func TestMediaTaskHandlerKeepsMP4AndWebMUploadDetection(t *testing.T) {
	for _, tt := range []struct {
		name        string
		filename    string
		data        []byte
		contentType string
	}{
		{
			name: "mp4", filename: "input.mp4", contentType: "video/mp4",
			data: []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00mp42isom"),
		},
		{
			name: "webm", filename: "input.webm", contentType: "video/webm",
			data: []byte("\x1a\x45\xdf\xa3\x9f\x42\x86\x81\x01"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			req := mediaMultipartRequestWithFields(
				t, "/v1/videos", "video", tt.filename, tt.data,
				map[string]string{"model": "fake-video", "prompt": "remix"},
			)

			rec := performRequest(router, req, 42, true)
			require.Equal(t, http.StatusAccepted, rec.Code)
			require.Len(t, app.lastCreate.Inputs, 1)
			require.Equal(t, tt.contentType, app.lastCreate.Inputs[0].ContentType)
		})
	}
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

func TestMediaTaskHandlerCleanupFailurePreservesBusinessErrorAndReportsSafeMetadata(t *testing.T) {
	observer := &mediaInputCleanupObserverStub{}
	router, app := newStandaloneMediaRouterWithStagerAndCleanupObserver(t, nil, observer)
	app.createResult = nil
	app.createErr = service.ErrMediaInputNotRecoverable
	app.discardErr = errors.New("discard staged/input-1 https://private.example Authorization=secret failed")
	req, _ := imageEditRequest(t, "false")

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"invalid_media_input"`)
	for _, secret := range []string{"staged/input-1", "private.example", "Authorization", "secret", "discard"} {
		require.NotContains(t, rec.Body.String(), secret)
	}
	require.Equal(t, 1, observer.calls)
	require.Equal(t, service.MediaOperationImageEdit, observer.operation)
	require.Equal(t, 1, observer.inputCount)
	require.Equal(t, "discard_failed", observer.classification)
}

func TestMediaTaskHandlerCleanupFailureCannotOverrideCreateErrorClassification(t *testing.T) {
	primaryErrors := []struct {
		name string
		err  error
		code string
	}{
		{name: "model not found", err: service.ErrMediaModelNotFound, code: "invalid_request"},
		{name: "invalid spec", err: service.ErrInvalidMediaSpec, code: "invalid_request"},
		{name: "input not recoverable", err: service.ErrMediaInputNotRecoverable, code: "invalid_media_input"},
	}
	cleanupErrors := []struct {
		name string
		err  error
	}{
		{name: "object store disabled", err: service.ErrMediaArtifactObjectStoreDisabled},
		{name: "content unavailable", err: service.ErrMediaContentUnavailable},
	}
	for _, primary := range primaryErrors {
		for _, cleanup := range cleanupErrors {
			t.Run(primary.name+"/"+cleanup.name, func(t *testing.T) {
				observer := &mediaInputCleanupObserverStub{}
				router, app := newStandaloneMediaRouterWithStagerAndCleanupObserver(t, nil, observer)
				app.createResult = nil
				app.createErr = primary.err
				app.discardErr = fmt.Errorf("cleanup private-object https://private.example Authorization=secret: %w", cleanup.err)
				req, _ := imageEditRequest(t, "false")

				rec := performRequest(router, req, 42, true)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), `"code":"`+primary.code+`"`)
				for _, secret := range []string{"private-object", "private.example", "Authorization", "secret", "media_content_unavailable"} {
					require.NotContains(t, rec.Body.String(), secret)
				}
				require.Equal(t, 1, observer.calls)
				require.Equal(t, service.MediaOperationImageEdit, observer.operation)
				require.Equal(t, 1, observer.inputCount)
				require.Equal(t, "discard_failed", observer.classification)
			})
		}
	}
}

func TestMediaTaskHandlerCleanupFailureCannotOverridePartialStageErrorClassification(t *testing.T) {
	primaryErrors := []struct {
		name string
		err  error
		code string
	}{
		{name: "model not found", err: service.ErrMediaModelNotFound, code: "invalid_request"},
		{name: "invalid spec", err: service.ErrInvalidMediaSpec, code: "invalid_request"},
		{name: "input not recoverable", err: service.ErrMediaInputNotRecoverable, code: "invalid_media_input"},
	}
	cleanupErrors := []struct {
		name string
		err  error
	}{
		{name: "object store disabled", err: service.ErrMediaArtifactObjectStoreDisabled},
		{name: "content unavailable", err: service.ErrMediaContentUnavailable},
	}
	for _, primary := range primaryErrors {
		for _, cleanup := range cleanupErrors {
			t.Run(primary.name+"/"+cleanup.name, func(t *testing.T) {
				observer := &mediaInputCleanupObserverStub{}
				router, app := newStandaloneMediaRouterWithStagerAndCleanupObserver(t, nil, observer)
				app.stageErr = primary.err
				app.stageErrAt = 2
				app.discardErr = fmt.Errorf("cleanup staged/input-1 https://private.example: %w", cleanup.err)
				req := multiImageEditRequest(t, map[string][]byte{
					"first.png":  []byte("\x89PNG\r\n\x1a\n"),
					"second.png": []byte("\x89PNG\r\n\x1a\n"),
				})

				rec := performRequest(router, req, 42, true)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				require.Contains(t, rec.Body.String(), `"code":"`+primary.code+`"`)
				require.NotContains(t, rec.Body.String(), "private.example")
				require.NotContains(t, rec.Body.String(), "staged/input-1")
				require.Zero(t, app.createCalls)
				require.Equal(t, 1, observer.calls)
				require.Equal(t, service.MediaOperationImageEdit, observer.operation)
				require.Equal(t, 1, observer.inputCount)
				require.Equal(t, "discard_failed", observer.classification)
			})
		}
	}
}

func TestMediaTaskHandlerInvalidStagedResultReportsCleanupFailureOnce(t *testing.T) {
	observer := &mediaInputCleanupObserverStub{}
	router, app := newStandaloneMediaRouterWithStagerAndCleanupObserver(t, nil, observer)
	app.invalidStageAt = 2
	app.discardErr = errors.New("cleanup staged/input-1 private-object failed")
	req := multiImageEditRequest(t, map[string][]byte{
		"first.png":  []byte("\x89PNG\r\n\x1a\n"),
		"second.png": []byte("\x89PNG\r\n\x1a\n"),
	})

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"invalid_request"`)
	require.NotContains(t, rec.Body.String(), "private-object")
	require.NotContains(t, rec.Body.String(), "staged/input-1")
	require.Zero(t, app.createCalls)
	require.Equal(t, 1, observer.calls)
	require.Equal(t, service.MediaOperationImageEdit, observer.operation)
	require.Equal(t, 2, observer.inputCount)
	require.Equal(t, "discard_failed", observer.classification)
}

func TestMediaTaskHandlerCleanupFailureAfterWrittenResponseIsObserved(t *testing.T) {
	observer := &mediaInputCleanupObserverStub{}
	router, app := newStandaloneMediaRouterWithStagerAndCleanupObserver(t, nil, observer)
	app.createResult.InputsAdopted = false
	app.discardErr = errors.New("discard private-object-key failed")
	req, _ := imageEditRequest(t, "true")

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NotContains(t, rec.Body.String(), "private-object-key")
	require.NotContains(t, rec.Body.String(), "discard")
	require.Equal(t, 1, observer.calls)
	require.Equal(t, service.MediaOperationImageEdit, observer.operation)
	require.Equal(t, 1, observer.inputCount)
	require.Equal(t, "discard_failed", observer.classification)
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

func TestMediaTaskHandlerMapsBillingEligibilityErrorsLikeTextGateways(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "insufficient balance", err: service.ErrInsufficientBalance},
		{name: "api key expired", err: service.ErrAPIKeyExpired},
		{name: "api key quota exhausted", err: service.ErrAPIKeyQuotaExhausted},
		{name: "api key 5h rate limit", err: service.ErrAPIKeyRateLimit5hExceeded},
		{name: "api key 1d rate limit", err: service.ErrAPIKeyRateLimit1dExceeded},
		{name: "api key 7d rate limit", err: service.ErrAPIKeyRateLimit7dExceeded},
		{name: "subscription invalid", err: service.ErrSubscriptionInvalid},
		{name: "subscription expired", err: service.ErrSubscriptionExpired},
		{name: "subscription suspended", err: service.ErrSubscriptionSuspended},
		{name: "subscription missing", err: service.ErrSubscriptionNotFound},
		{name: "subscription daily limit", err: service.ErrDailyLimitExceeded},
		{name: "subscription weekly limit", err: service.ErrWeeklyLimitExceeded},
		{name: "subscription monthly limit", err: service.ErrMonthlyLimitExceeded},
		{name: "billing unavailable", err: service.ErrBillingServiceUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			app.createResult = nil
			app.createErr = fmt.Errorf("precharge internal Authorization secret: %w", tt.err)
			wantStatus, wantCode, wantMessage := billingErrorDetails(app.createErr)

			recorder := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{
				"model":"fake-image","prompt":"cat","async":true
			}`, 42)

			require.Equal(t, wantStatus, recorder.Code)
			var response mediaAPIErrorEnvelope
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, wantCode, response.Error.Code)
			require.Equal(t, wantCode, response.Error.Type)
			require.Equal(t, wantMessage, response.Error.Message)
			require.NotContains(t, recorder.Body.String(), "Authorization")
			require.NotContains(t, recorder.Body.String(), "secret")
		})
	}
}

func TestMediaTaskHandlerReturnsUnavailableForTombstone(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = nil
	app.createErr = service.ErrMediaModelAdapterUnavailable.WithMetadata(map[string]string{
		"model_id": "grok-2-image", "resolution_status": "unresolved", "reason_code": "MEDIA_ADAPTER_UNRESOLVED",
	})

	recorder := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"grok-2-image","prompt":"cat"}`, 42)

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"MEDIA_MODEL_ADAPTER_UNAVAILABLE","message":"The media model adapter is temporarily unavailable","type":"server_error"}}`, recorder.Body.String())
}

func TestMediaTaskHandlerHidesTaskFromDifferentUser(t *testing.T) {
	router, _ := newStandaloneMediaRouter(t)
	rec := performAPIKeyRequest(router, http.MethodGet, "/v1/videos/task_other", "", 99)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "owner")
}

func TestMediaTaskHandlerHidesTaskFromDifferentAPIKeyOfSameUser(t *testing.T) {
	router, _ := newStandaloneMediaRouter(t)
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public", nil, 42)
	req.Header.Set("X-Test-API-Key-ID", "9")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMediaTaskHandlerVideoContentForwardsRangeWithoutLeakingUpstream(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.content = &service.MediaContent{
		Body: io.NopCloser(bytes.NewBufferString("2345")), StatusCode: http.StatusPartialContent,
		ContentType: "video/mp4", ContentLength: 4, ContentRange: "bytes 2-5/10", AcceptRanges: "bytes",
	}
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public/content", nil, 42)
	req.Header.Set("Range", "bytes=2-5")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, "bytes 2-5/10", rec.Header().Get("Content-Range"))
	require.Equal(t, "2345", rec.Body.String())
	require.NotContains(t, rec.Header().Get("Location"), "upstream")
}

func TestMediaTaskHandlerVideoContentDowngradesUnsafeContentType(t *testing.T) {
	for _, contentType := range []string{"", "text/html", "application/javascript"} {
		t.Run(contentType, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			app.content = &service.MediaContent{
				Body: io.NopCloser(strings.NewReader("payload")), StatusCode: http.StatusOK,
				ContentType: contentType, ContentLength: 7, AcceptRanges: "bytes",
			}
			rec := performAPIKeyRequest(router, http.MethodGet, "/v1/videos/task_public/content", "", 42)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
			require.Equal(t, "attachment", rec.Header().Get("Content-Disposition"))
			require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
			require.Equal(t, "payload", rec.Body.String())
		})
	}
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

func TestMediaTaskHandlerVideoDurationAndFPSMayBeOmitted(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		router, app := newStandaloneMediaRouter(t)
		rec := performAPIKeyRequest(router, http.MethodPost, "/v1/videos", `{"model":"fake-video","prompt":"sunset"}`, 42)
		require.Equal(t, http.StatusAccepted, rec.Code)
		require.Zero(t, app.lastCreate.Spec.Video.DurationSeconds)
		require.Zero(t, app.lastCreate.Spec.Video.FPS)
	})

	t.Run("multipart", func(t *testing.T) {
		router, app := newStandaloneMediaRouter(t)
		rec := performRequest(router, videoUploadRequest(t), 42, true)
		require.Equal(t, http.StatusAccepted, rec.Code)
		require.Zero(t, app.lastCreate.Spec.Video.DurationSeconds)
		require.Zero(t, app.lastCreate.Spec.Video.FPS)
	})
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
	rec := performAPIKeyRequest(router, http.MethodGet, "/v1/videos/task_public", "", 42)
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
	rec := performAPIKeyRequest(router, http.MethodGet, "/v1/videos/task_public", "", 42)
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

func TestMediaTaskHandlerImageDTOConvertsBoundedImageDataToB64JSON(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
			RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		},
		Artifacts: []service.MediaArtifact{{
			Direction: "output", MediaType: service.MediaTypeImage, ContentType: "image/png",
			UpstreamReference: "data:image/png;base64,aW1hZ2U=",
		}},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat","response_format":"b64_json"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"b64_json":"aW1hZ2U="`)
	require.NotContains(t, rec.Body.String(), `"url"`)
	require.NotContains(t, rec.Body.String(), "data:image")
}

func TestMediaTaskHandlerImageDTOUsesURLContractForInlineOutputByDefault(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
			RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		},
		Artifacts: []service.MediaArtifact{{
			Direction: "output", MediaType: service.MediaTypeImage, ContentType: "image/png",
			UpstreamReference: "data:image/png;base64,aW1hZ2U=",
		}},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"url":"/v1/images/task_public/content?index=0"`)
	require.NotContains(t, rec.Body.String(), "b64_json")
}

func TestMediaTaskHandlerImageDTOReadsStoredOutputForB64JSON(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
			RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		},
		Artifacts: []service.MediaArtifact{{
			Direction: "output", Position: 2, MediaType: service.MediaTypeImage, ObjectKey: "private/image.png", ContentType: "image/png",
		}},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	app.content = &service.MediaContent{
		Body: io.NopCloser(strings.NewReader("image")), StatusCode: http.StatusOK,
		ContentType: "image/png", ContentLength: int64(len("image")),
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat","response_format":"b64_json"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"b64_json":"aW1hZ2U="`)
	require.NotContains(t, rec.Body.String(), `"url"`)
	require.Equal(t, 2, app.lastImagePosition)
}

func TestMediaTaskHandlerImageDTOUsesAuthenticatedContentURLForStoredOutput(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
			RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		},
		Artifacts: []service.MediaArtifact{{
			Direction: "output", MediaType: service.MediaTypeImage, ObjectKey: "private/image.png", ContentType: "image/png",
		}},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cat"}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"url":"/v1/images/task_public/content?index=0"`)
	require.NotContains(t, rec.Body.String(), "private/image.png")
}

func TestMediaTaskHandlerImageDTOUsesDistinctContentURLsForStoredOutputs(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.createResult = &service.MediaCreateResult{
		Task: &service.MediaTask{
			PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
			RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		},
		Artifacts: []service.MediaArtifact{
			{Direction: "output", Position: 0, MediaType: service.MediaTypeImage, ObjectKey: "private/first.png", ContentType: "image/png"},
			{Direction: "output", Position: 1, MediaType: service.MediaTypeImage, ObjectKey: "private/second.png", ContentType: "image/png"},
		},
		Disposition: service.MediaCreateDispositionCompleted,
	}
	rec := performAPIKeyRequest(router, http.MethodPost, "/v1/images/generations", `{"model":"fake-image","prompt":"cats","n":2}`, 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"url":"/v1/images/task_public/content?index=0"`)
	require.Contains(t, rec.Body.String(), `"url":"/v1/images/task_public/content?index=1"`)
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
	rec := performAPIKeyRequest(router, http.MethodGet, "/v1/images/task_public", "", 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"object":"media.task"`)
	require.Contains(t, rec.Body.String(), `"result":{"data":[{"url":"https://cdn.example/public.png"}]}`)
}

func TestMediaTaskHandlerCompletedImageQueryPreservesB64JSONFromSelectedRequest(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.getTask = &service.MediaTask{
		PublicID: "task_public", MediaType: service.MediaTypeImage, Operation: service.MediaOperationTextToImage,
		RequestedModel: "fake-image", Status: service.MediaTaskStatusCompleted, CreatedAt: time.Unix(1784112000, 0),
		ImageResponseFormat: "b64_json",
	}
	app.getArtifacts = []service.MediaArtifact{{
		Direction: "output", MediaType: service.MediaTypeImage, ObjectKey: "private/image.png", ContentType: "image/png",
	}}
	app.content = &service.MediaContent{
		Body: io.NopCloser(strings.NewReader("image")), StatusCode: http.StatusOK,
		ContentType: "image/png", ContentLength: int64(len("image")),
	}
	rec := performAPIKeyRequest(router, http.MethodGet, "/v1/images/task_public", "", 42)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"result":{"data":[{"b64_json":"aW1hZ2U="}]}`)
	require.NotContains(t, rec.Body.String(), `"url"`)
}

func TestMediaTaskHandlerInvalidRangeReturns416WithoutOpeningContent(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public/content", nil, 42)
	req.Header.Set("Range", "bytes=+0-+1")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, rec.Code)
	require.Zero(t, app.contentCalls)
}

func TestMediaTaskHandlerContentOpenerRangeErrorReturns416(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.contentErr = fmt.Errorf("object store range: %w", service.ErrMediaRangeNotSatisfiable)
	req := newAPIKeyRequest(http.MethodGet, "/v1/videos/task_public/content", nil, 42)
	req.Header.Set("Range", "bytes=99-100")

	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, rec.Code)
	require.Equal(t, 1, app.contentCalls)
	require.Contains(t, rec.Body.String(), `"code":"invalid_range"`)
	require.NotContains(t, rec.Body.String(), "object store")
}

func TestMediaTaskHandlerImageContentReturnsImageWithRange(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.content = &service.MediaContent{
		Body: io.NopCloser(strings.NewReader("123")), StatusCode: http.StatusPartialContent,
		ContentType: "image/png", ContentLength: 3, ContentRange: "bytes 1-3/6", AcceptRanges: "bytes",
	}
	req := newAPIKeyRequest(http.MethodGet, "/v1/images/task_public/content?index=3", nil, 42)
	req.Header.Set("Range", "bytes=1-3")
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusPartialContent, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "bytes", rec.Header().Get("Accept-Ranges"))
	require.Equal(t, "bytes 1-3/6", rec.Header().Get("Content-Range"))
	require.Equal(t, "123", rec.Body.String())
	require.Equal(t, 3, app.lastImagePosition)
}

func TestMediaTaskHandlerImageContentDefaultsToFirstOutput(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.content = &service.MediaContent{
		Body: io.NopCloser(strings.NewReader("image")), StatusCode: http.StatusOK,
		ContentType: "image/png", ContentLength: 5,
	}
	req := newAPIKeyRequest(http.MethodGet, "/v1/images/task_public/content", nil, 42)
	rec := performRequest(router, req, 42, true)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 0, app.lastImagePosition)
}

func TestMediaTaskHandlerImageContentRejectsInvalidIndexBeforeOpeningContent(t *testing.T) {
	for _, index := range []string{"-1", "+1", "1.0", "4294967296"} {
		t.Run(index, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			req := newAPIKeyRequest(http.MethodGet, "/v1/images/task_public/content?index="+url.QueryEscape(index), nil, 42)
			rec := performRequest(router, req, 42, true)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Zero(t, app.contentCalls)
			require.Contains(t, rec.Body.String(), `"code":"invalid_index"`)
		})
	}
}

func TestMediaTaskHandlerContentOpenerFailuresReturn502WithoutCauseLeak(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{name: "secure proxy unsupported", err: service.ErrSecureHTTPUpstreamProxyUnsupported},
		{name: "secure upstream required", err: service.ErrMediaSecureUpstreamRequired},
		{name: "network failure", err: errors.New("dial tcp internal-upstream: secret network failure")},
		{name: "task repository database failure", err: errors.Join(service.ErrMediaContentUnavailable, errors.New("postgres private-dsn credential failure"))},
		{name: "task repository deadline", err: errors.Join(service.ErrMediaContentUnavailable, context.DeadlineExceeded)},
		{name: "task repository cancellation", err: errors.Join(service.ErrMediaContentUnavailable, context.Canceled)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			router, app := newStandaloneMediaRouter(t)
			app.contentErr = tt.err

			rec := performAPIKeyRequest(router, http.MethodGet, "/v1/videos/task_public/content", "", 42)
			require.Equal(t, http.StatusBadGateway, rec.Code)
			require.Contains(t, rec.Body.String(), `"code":"media_content_unavailable"`)
			require.NotContains(t, rec.Body.String(), "internal-upstream")
			require.NotContains(t, rec.Body.String(), "secret")
			require.NotContains(t, rec.Body.String(), "private-dsn")
			require.NotContains(t, rec.Body.String(), "credential")
			require.NotContains(t, rec.Body.String(), "deadline")
			require.NotContains(t, rec.Body.String(), "canceled")
		})
	}
}

func TestMediaTaskHandlerMissingCompletedArtifactReturns404WithoutLeak(t *testing.T) {
	router, app := newStandaloneMediaRouter(t)
	app.contentErr = service.ErrMediaArtifactNotFound
	rec := performAPIKeyRequest(router, http.MethodGet, "/v1/videos/task_public/content", "", 42)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "media_content_not_found")
	require.NotContains(t, rec.Body.String(), "artifact")
}

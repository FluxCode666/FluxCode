package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIImagesReadErrorBody struct {
	err error
}

func (b *openAIImagesReadErrorBody) Read([]byte) (int, error) { return 0, b.err }
func (b *openAIImagesReadErrorBody) Close() error             { return nil }

func TestParseOpenAIImagesRequestClassifiesOfficialAndArbitrarySizes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		size string
		want string
	}{
		{size: "1024x1024", want: "1K"},
		{size: "1536x1024", want: "2K"},
		{size: "2048x1152", want: "2K"},
		{size: "2048x2048", want: "2K"},
		{size: "3840x2160", want: "4K"},
		{size: "2160x3840", want: "4K"},
		{size: "512x512", want: "1K"},
		{size: "1280x768", want: "2K"},
		{size: "2560x1440", want: "4K"},
		{size: "auto", want: "2K"},
		{size: "invalid", want: "2K"},
	}

	svc := &OpenAIGatewayService{}
	for _, tt := range tests {
		t.Run(tt.size, func(t *testing.T) {
			body := []byte(fmt.Sprintf(`{"model":"gpt-image-2","prompt":"draw a cat","size":%q}`, tt.size))
			req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = req

			parsed, err := svc.ParseOpenAIImagesRequest(c, body)

			require.NoError(t, err)
			require.Equal(t, tt.size, parsed.Size)
			require.Equal(t, tt.want, parsed.SizeTier)
		})
	}
}

func TestParseOpenAIImagesRequestResponseFormatURLStaysBasicAndInvalidRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &OpenAIGatewayService{}

	body := []byte(`{"prompt":"draw a cat","response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	parsed, err := svc.ParseOpenAIImagesRequest(c, body)

	require.NoError(t, err)
	require.Equal(t, "url", parsed.ResponseFormat)
	require.Equal(t, OpenAIImagesCapabilityBasic, parsed.RequiredCapability)

	invalidBody := []byte(`{"prompt":"draw a cat","response_format":"json"}`)
	invalidReq := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(invalidBody))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	invalidCtx, _ := gin.CreateTestContext(invalidRec)
	invalidCtx.Request = invalidReq

	_, err = svc.ParseOpenAIImagesRequest(invalidCtx, invalidBody)

	require.Error(t, err)
	require.Contains(t, err.Error(), "response_format")
}

func TestRewriteOpenAIImagesModelStripsLocalResponseFormatJSONAndMultipart(t *testing.T) {
	jsonBody := []byte(`{"model":"gpt-image-1","prompt":"draw a cat","response_format":"url"}`)
	rewrittenJSON, _, err := rewriteOpenAIImagesModel(jsonBody, "application/json", "gpt-image-2")

	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", gjson.GetBytes(rewrittenJSON, "model").String())
	require.False(t, gjson.GetBytes(rewrittenJSON, "response_format").Exists())

	var multipartBody bytes.Buffer
	writer := multipart.NewWriter(&multipartBody)
	require.NoError(t, writer.WriteField("model", "gpt-image-1"))
	require.NoError(t, writer.WriteField("prompt", "draw a cat"))
	require.NoError(t, writer.WriteField("response_format", "url"))
	part, err := writer.CreateFormFile("image", "cat.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-image"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	rewrittenMultipart, rewrittenType, err := rewriteOpenAIImagesModel(multipartBody.Bytes(), writer.FormDataContentType(), "gpt-image-2")
	require.NoError(t, err)

	_, params, err := mime.ParseMediaType(rewrittenType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(rewrittenMultipart), params["boundary"])
	fields := map[string]string{}
	fileSeen := false
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		data, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FileName() != "" {
			fileSeen = true
			require.Equal(t, "image", part.FormName())
			require.Equal(t, "fake-image", string(data))
			continue
		}
		fields[part.FormName()] = string(data)
	}
	require.True(t, fileSeen)
	require.Equal(t, "gpt-image-2", fields["model"])
	require.Equal(t, "draw a cat", fields["prompt"])
	require.NotContains(t, fields, "response_format")
}

func TestBuildOpenAIImagesResponsesRequestUsesOfficialToolParameters(t *testing.T) {
	outputCompression := 80
	partialImages := 2
	parsed := &OpenAIImagesRequest{
		Endpoint:          openAIImagesEditsEndpoint,
		Model:             "gpt-image-2",
		Prompt:            "preserve the face and replace the background",
		N:                 3,
		Size:              "1536x1024",
		InputFidelity:     "high",
		OutputCompression: &outputCompression,
		PartialImages:     &partialImages,
		InputImageURLs:    []string{"data:image/png;base64,aGVsbG8="},
	}

	body, err := buildOpenAIImagesResponsesRequest(parsed, "gpt-image-2")

	require.NoError(t, err)
	tool := gjson.GetBytes(body, "tools.0")
	require.Equal(t, "edit", tool.Get("action").String())
	require.Equal(t, "high", tool.Get("input_fidelity").String())
	require.Equal(t, int64(80), tool.Get("output_compression").Int())
	require.Equal(t, int64(2), tool.Get("partial_images").Int())
	require.False(t, tool.Get("n").Exists())
}

func TestForwardOpenAIImagesAPIKeyConvertsUpstreamURLToDefaultB64JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("png-bytes")
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/generated.png", r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer imageServer.Close()

	body := []byte(`{"prompt":"draw a cat"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"created":1710000000,"data":[{"url":%q,"revised_prompt":"draw a cat"}]}`,
				imageServer.URL+"/generated.png",
			))),
		},
	}
	svc := newOpenAIImagesForwardTestService(upstream)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)
	require.Equal(t, "b64_json", parsed.ResponseFormat)

	result, err := svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyTestAccount(), body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "response_format").Exists())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, base64.StdEncoding.EncodeToString(imageBytes), gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").String())
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "data.0.url").Exists())
}

func TestForwardOpenAIImagesAPIKeyReturnsDataURLModeWhenURLRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("proxied-png-bytes")
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/generated.png", r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer imageServer.Close()

	body := []byte(`{"prompt":"draw a cat","response_format":"url"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"created":1710000000,"data":[{"url":%q,"revised_prompt":"draw a cat"}]}`,
				imageServer.URL+"/generated.png",
			))),
		},
	}
	svc := newOpenAIImagesForwardTestService(upstream)
	account := newOpenAIImagesAPIKeyTestAccount()
	account.Extra = map[string]any{
		OpenAIImageResponseURLModeExtraKey: OpenAIImageResponseURLModeBase64URL,
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "response_format").Exists())
	require.Equal(
		t,
		"data:image/png;base64,"+base64.StdEncoding.EncodeToString(imageBytes),
		gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String(),
	)
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").Exists())
}

func TestForwardOpenAIImagesAPIKeyReturnsHTTPURLByDefaultFromB64JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"prompt":"draw a cat","response_format":"url"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","output_format":"png"}]}`,
			)),
		},
	}
	cache := newOpenAIImagesFakeImageCache()
	svc := newOpenAIImagesForwardTestService(upstream)
	svc.SetOpenAIImageCache(cache)
	account := newOpenAIImagesAPIKeyTestAccount()
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "response_format").Exists())
	proxyURL := gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String()
	require.Contains(t, proxyURL, "proxy.example.test/v1/images/proxy/")
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").Exists())
	require.Len(t, cache.sets, 1)
	require.Equal(t, []byte("hello"), cache.sets[0].data)
	require.Equal(t, "image/png", cache.sets[0].contentType)
	require.Equal(t, 72*time.Hour, cache.sets[0].ttl)

	token := proxyURL[strings.LastIndex(proxyURL, "/")+1:]
	proxyRec := httptest.NewRecorder()
	proxyCtx, _ := gin.CreateTestContext(proxyRec)
	proxyCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/images/proxy/"+token, nil)

	err = svc.ProxyOpenAIImagesURL(proxyCtx, token)

	require.NoError(t, err)
	require.Equal(t, http.StatusOK, proxyRec.Code)
	require.Equal(t, "image/png", proxyRec.Header().Get("Content-Type"))
	require.Equal(t, []byte("hello"), proxyRec.Body.Bytes())
}

func TestForwardOpenAIImagesAPIKeyReturnsObjectStoreURLWhenHTTPURLRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"prompt":"draw a cat","response_format":"url"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"created":1710000000,"data":[{"b64_json":"aGVsbG8=","output_format":"png"}]}`,
			)),
		},
	}
	cache := newOpenAIImagesFakeImageCache()
	objectStore := &generatedImageObjectStoreStub{url: "https://cdn.example.com/openai/generated.png"}
	svc := newOpenAIImagesForwardTestService(upstream)
	svc.SetOpenAIImageCache(cache)
	svc.SetGeneratedImageObjectStore(objectStore)
	account := newOpenAIImagesAPIKeyTestAccount()
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://cdn.example.com/openai/generated.png", gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String())
	require.False(t, gjson.GetBytes(rec.Body.Bytes(), "data.0.b64_json").Exists())
	require.Len(t, objectStore.uploads, 1)
	require.Equal(t, []byte("hello"), objectStore.uploads[0].Data)
	require.Equal(t, "image/png", objectStore.uploads[0].ContentType)
	require.Empty(t, cache.sets)
}

func TestForwardOpenAIImagesAPIKeyReturnsHTTPURLModeFromUpstreamURLWithConfiguredTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("downloaded-png-bytes")
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/generated.png", r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer imageServer.Close()

	body := []byte(`{"prompt":"draw a cat","response_format":"url"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"created":1710000000,"data":[{"url":%q}]}`,
				imageServer.URL+"/generated.png",
			))),
		},
	}
	cache := newOpenAIImagesFakeImageCache()
	svc := newOpenAIImagesForwardTestService(upstream)
	svc.SetOpenAIImageCache(cache)
	svc.SetSettingService(NewSettingService(&openAIImagesSettingRepoStub{values: map[string]string{
		SettingKeyOpenAIImageURLCacheTTLHours: "12",
	}}, &config.Config{}))
	account := newOpenAIImagesAPIKeyTestAccount()
	account.Extra = map[string]any{
		OpenAIImageResponseURLModeExtraKey: OpenAIImageResponseURLModeHTTPURL,
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	proxyURL := gjson.GetBytes(rec.Body.Bytes(), "data.0.url").String()
	require.Contains(t, proxyURL, "proxy.example.test/v1/images/proxy/")
	require.NotContains(t, proxyURL, imageServer.URL)
	require.Len(t, cache.sets, 1)
	require.Equal(t, imageBytes, cache.sets[0].data)
	require.Equal(t, "image/png", cache.sets[0].contentType)
	require.Equal(t, 12*time.Hour, cache.sets[0].ttl)
}

func TestForwardOpenAIImagesAPIKeyStreamingConvertsB64JSONToURLWhenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"prompt":"draw a cat","stream":true,"response_format":"url"}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"event: image_generation.completed\n" +
					"data: {\"type\":\"image_generation.completed\",\"b64_json\":\"aGVsbG8=\",\"output_format\":\"png\"}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}
	svc := newOpenAIImagesForwardTestService(upstream)
	account := newOpenAIImagesAPIKeyTestAccount()
	account.Extra = map[string]any{
		OpenAIImageResponseURLModeExtraKey: OpenAIImageResponseURLModeBase64URL,
	}
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, account, body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, gjson.GetBytes(upstream.lastBody, "response_format").Exists())
	require.Contains(t, rec.Body.String(), `"url":"data:image/png;base64,aGVsbG8="`)
	require.NotContains(t, rec.Body.String(), `"b64_json"`)
}

func TestForwardOpenAIImagesAPIKeyStreamingConvertsUpstreamURLToDefaultB64JSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	imageBytes := []byte("stream-png-bytes")
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/generated.png", r.URL.Path)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer imageServer.Close()

	body := []byte(`{"prompt":"draw a cat","stream":true}`)
	c, rec := newOpenAIImagesForwardTestContext(body)
	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				"event: image_generation.completed\n"+
					"data: {\"type\":\"image_generation.completed\",\"url\":%q}\n\n"+
					"data: [DONE]\n\n",
				imageServer.URL+"/generated.png",
			))),
		},
	}
	svc := newOpenAIImagesForwardTestService(upstream)
	parsed, err := svc.ParseOpenAIImagesRequest(c, body)
	require.NoError(t, err)

	result, err := svc.ForwardImages(context.Background(), c, newOpenAIImagesAPIKeyTestAccount(), body, parsed, "")

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), `"b64_json":"`+base64.StdEncoding.EncodeToString(imageBytes)+`"`)
	require.NotContains(t, rec.Body.String(), imageServer.URL)
}

func newOpenAIImagesForwardTestContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "http://proxy.example.test/v1/images/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func newOpenAIImagesForwardTestService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	return &OpenAIGatewayService{
		cfg: &config.Config{
			JWT: config.JWTConfig{Secret: strings.Repeat("x", 32)},
			Security: config.SecurityConfig{
				URLAllowlist: config.URLAllowlistConfig{
					AllowInsecureHTTP: true,
					AllowPrivateHosts: true,
				},
			},
		},
		httpUpstream: upstream,
	}
}

func newOpenAIImagesAPIKeyTestAccount() *Account {
	return &Account{
		ID:          991,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key": "sk-test",
		},
	}
}

func TestOpenAIImageResponseURLModeDefaultsAndNormalizes(t *testing.T) {
	require.Equal(t, OpenAIImageResponseURLModeHTTPURL, newOpenAIImagesAPIKeyTestAccount().GetOpenAIImageResponseURLMode())

	account := newOpenAIImagesAPIKeyTestAccount()
	account.Extra = map[string]any{
		OpenAIImageResponseURLModeExtraKey: OpenAIImageResponseURLModeHTTPURL,
	}

	require.Equal(t, OpenAIImageResponseURLModeHTTPURL, account.GetOpenAIImageResponseURLMode())

	account.Extra[OpenAIImageResponseURLModeExtraKey] = "invalid"

	require.Equal(t, OpenAIImageResponseURLModeHTTPURL, account.GetOpenAIImageResponseURLMode())
}

func TestSettingServiceOpenAIImageURLCacheTTLHoursDefaultsAndUpdates(t *testing.T) {
	repo := &openAIImagesSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	settings := svc.parseSettings(repo.values)
	require.Equal(t, 72, settings.OpenAIImageURLCacheTTLHours)
	require.Equal(t, 72*time.Hour, svc.GetOpenAIImageURLCacheTTL(context.Background()))

	settings.OpenAIImageURLCacheTTLHours = 24
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))

	require.Equal(t, "24", repo.values[SettingKeyOpenAIImageURLCacheTTLHours])
}

type openAIImagesFakeImageCache struct {
	sets   []openAIImagesFakeImageCacheSet
	values map[string]openAIImagesFakeImageCacheSet
}

type openAIImagesFakeImageCacheSet struct {
	data        []byte
	contentType string
	ttl         time.Duration
}

func newOpenAIImagesFakeImageCache() *openAIImagesFakeImageCache {
	return &openAIImagesFakeImageCache{values: make(map[string]openAIImagesFakeImageCacheSet)}
}

func (c *openAIImagesFakeImageCache) SetImage(ctx context.Context, id string, data []byte, contentType string, ttl time.Duration) error {
	copied := append([]byte(nil), data...)
	set := openAIImagesFakeImageCacheSet{
		data:        copied,
		contentType: contentType,
		ttl:         ttl,
	}
	c.sets = append(c.sets, set)
	c.values[id] = set
	return nil
}

func (c *openAIImagesFakeImageCache) GetImage(ctx context.Context, id string) ([]byte, string, error) {
	set, ok := c.values[id]
	if !ok {
		return nil, "", ErrOpenAIImageCacheNotFound
	}
	return append([]byte(nil), set.data...), set.contentType, nil
}

type openAIImagesSettingRepoStub struct {
	values map[string]string
}

func (r *openAIImagesSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if value, ok := r.values[key]; ok {
		return &Setting{Key: key, Value: value}, nil
	}
	return nil, ErrSettingNotFound
}

func (r *openAIImagesSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}

func (r *openAIImagesSettingRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *openAIImagesSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		out[key] = r.values[key]
	}
	return out, nil
}

func (r *openAIImagesSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *openAIImagesSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *openAIImagesSettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestCollectOpenAIImagesFromResponsesBodyHandlesFullResponsesJSON(t *testing.T) {
	body := []byte(`{
		"id": "resp_123",
		"created_at": 1710000006,
		"status": "completed",
		"output": [
			{
				"id": "ig_123",
				"type": "image_generation_call",
				"result": "aGVsbG8=",
				"revised_prompt": "draw a cat",
				"output_format": "png"
			}
		],
		"usage": {"input_tokens": 1, "output_tokens": 1}
	}`)

	results, createdAt, usageRaw, firstMeta, foundFinal, err := collectOpenAIImagesFromResponsesBody(body)

	require.NoError(t, err)
	require.True(t, foundFinal)
	require.Equal(t, int64(1710000006), createdAt)
	require.JSONEq(t, `{"input_tokens":1,"output_tokens":1}`, string(usageRaw))
	require.Len(t, results, 1)
	require.Equal(t, "aGVsbG8=", results[0].Result)
	require.Equal(t, "draw a cat", firstMeta.RevisedPrompt)
	require.Equal(t, "png", firstMeta.OutputFormat)
}

func TestCollectOpenAIImagesFromResponsesBodyHandlesResponseDoneEvent(t *testing.T) {
	body := []byte("data: {\"type\":\"response.done\",\"response\":{\"created_at\":1710000007,\"output\":[{\"id\":\"ig_456\",\"type\":\"image_generation_call\",\"result\":\"aW1hZ2U=\",\"output_format\":\"webp\"}],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")

	results, createdAt, usageRaw, firstMeta, foundFinal, err := collectOpenAIImagesFromResponsesBody(body)

	require.NoError(t, err)
	require.True(t, foundFinal)
	require.Equal(t, int64(1710000007), createdAt)
	require.JSONEq(t, `{"input_tokens":2,"output_tokens":1}`, string(usageRaw))
	require.Len(t, results, 1)
	require.Equal(t, "aW1hZ2U=", results[0].Result)
	require.Equal(t, "webp", firstMeta.OutputFormat)
}

func TestExtractOpenAIImagesUpstreamErrorClassifiesIncompleteResponses(t *testing.T) {
	body := []byte("data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp_incomplete\",\"incomplete_details\":{\"reason\":\"max_output_tokens\"}}}\n\n")

	upstreamErr := extractOpenAIImagesUpstreamError(body)

	require.NotNil(t, upstreamErr)
	require.Equal(t, http.StatusBadGateway, upstreamErr.StatusCode)
	require.True(t, IsOpenAIImagesRetryableUpstreamError(upstreamErr))
	require.Equal(t, "response_incomplete", upstreamErr.Code)
	require.Contains(t, upstreamErr.Message, "max_output_tokens")
}

func TestExtractOpenAIImagesUpstreamErrorDoesNotRetryContentFilterIncomplete(t *testing.T) {
	body := []byte("data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp_blocked\",\"incomplete_details\":{\"reason\":\"content_filter\"}}}\n\n")

	upstreamErr := extractOpenAIImagesUpstreamError(body)

	require.NotNil(t, upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.False(t, IsOpenAIImagesRetryableUpstreamError(upstreamErr))
	require.Equal(t, "image_generation_user_error", upstreamErr.ErrorType)
	require.Equal(t, "response_incomplete", upstreamErr.Code)
}

func TestHandleOpenAIImagesOAuthNonStreamingResponseReturnsFailoverOnEmptyOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(bytes.NewReader([]byte(
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_empty\",\"status\":\"completed\",\"output\":[]}}\n\n" +
				"data: [DONE]\n\n",
		))),
	}
	svc := &OpenAIGatewayService{}

	_, imageCount, err := svc.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, "b64_json", "gpt-image-2", GeneratedImageRecordContext{})

	require.Zero(t, imageCount)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.False(t, c.Writer.Written())
}

func TestOpenAIImagesOAuthBodyReadTransportErrorFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"X-Request-Id": []string{"req_h2_read_failure"},
			"X-Upstream":   []string{"preserved"},
		},
		Body: &openAIImagesReadErrorBody{err: errors.New("stream error: stream ID 11; INTERNAL_ERROR; received from peer")},
	}
	account := &Account{ID: 5400, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	svc := &OpenAIGatewayService{}

	_, _, readErr := svc.handleOpenAIImagesOAuthNonStreamingResponse(resp, c, "b64_json", "gpt-image-2", GeneratedImageRecordContext{})
	require.Error(t, readErr)
	err := svc.handleOpenAIImagesOAuthStreamReadError(context.Background(), c, account, "https://api.openai.com/v1/responses", resp, c.Writer.Size(), readErr)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.JSONEq(t, `{"error":{"type":"upstream_error","code":"upstream_http2_stream_error","message":"Upstream HTTP/2 stream failed"}}`, string(failoverErr.ResponseBody))
	require.Equal(t, "req_h2_read_failure", failoverErr.ResponseHeaders.Get("x-request-id"))
	require.Equal(t, "preserved", failoverErr.ResponseHeaders.Get("x-upstream"))
	resp.Header.Set("X-Upstream", "mutated")
	require.Equal(t, "preserved", failoverErr.ResponseHeaders.Get("x-upstream"))

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, "req_h2_read_failure", events[0].UpstreamRequestID)
	require.Equal(t, "Upstream HTTP/2 stream failed", events[0].Message)
}

func TestOpenAIImagesOAuthBodyReadErrorsNotMisclassified(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "context canceled", err: context.Canceled},
		{name: "response too large", err: fmt.Errorf("%w: limit=1", ErrUpstreamResponseBodyTooLarge)},
		{name: "semantic error", err: &OpenAIImagesUpstreamError{StatusCode: http.StatusBadRequest, ErrorType: "invalid_request_error", Code: "invalid_value", Message: "bad image request"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
			err := tt.err
			if tt.name != "semantic error" && shouldClassifyOpenAIUpstreamStreamReadError(err, c.Request.Context()) {
				err = newOpenAIUpstreamStreamReadError(err)
			}

			got := (&OpenAIGatewayService{}).handleOpenAIImagesOAuthStreamReadError(context.Background(), c, &Account{Platform: PlatformOpenAI}, "", resp, c.Writer.Size(), err)
			var failoverErr *UpstreamFailoverError
			require.False(t, errors.As(got, &failoverErr))
			require.ErrorIs(t, got, tt.err)
		})
	}
}

func TestOpenAIImagesOAuthTransportErrorAfterDownstreamWriteDoesNotFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	before := c.Writer.Size()
	_, writeErr := c.Writer.Write([]byte("downstream image bytes"))
	require.NoError(t, writeErr)
	classifiedErr := newOpenAIUpstreamStreamReadError(errors.New("unexpected EOF"))
	account := &Account{ID: 5401, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	resp := &http.Response{Header: http.Header{"X-Request-Id": []string{"req_after_write"}}}

	err := (&OpenAIGatewayService{}).handleOpenAIImagesOAuthStreamReadError(context.Background(), c, account, "", resp, before, classifiedErr)

	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
	require.ErrorIs(t, err, classifiedErr)
	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "retry_exhausted_failover", events[0].Kind)
}

func TestShouldClassifyOpenAIUpstreamStreamReadErrorTransportStrings(t *testing.T) {
	for _, message := range []string{"unexpected EOF", "connection reset by peer", "broken pipe", "use of closed network connection"} {
		t.Run(message, func(t *testing.T) {
			require.True(t, shouldClassifyOpenAIUpstreamStreamReadError(errors.New(message)))
		})
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, shouldClassifyOpenAIUpstreamStreamReadError(errors.New("unexpected EOF"), canceledCtx))
}

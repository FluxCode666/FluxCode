package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type nanoBananaHTTPUpstreamStub struct {
	do    func(*http.Request, string, int64, int) (*http.Response, error)
	calls int
}

func (s *nanoBananaHTTPUpstreamStub) Do(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
) (*http.Response, error) {
	s.calls++
	if s.do == nil {
		return nil, errors.New("unexpected nano banana request")
	}
	return s.do(req, proxyURL, accountID, accountConcurrency)
}

func (s *nanoBananaHTTPUpstreamStub) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestNanoBananaMediaAdapterGeneratesWithGeminiProtocol(t *testing.T) {
	t.Parallel()
	image := nanoBananaTestPNG(t)
	upstream := &nanoBananaHTTPUpstreamStub{do: func(
		req *http.Request,
		proxyURL string,
		accountID int64,
		accountConcurrency int,
	) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "https://gemini.example.test/custom/v1beta/models/gemini-3.1-flash-image-preview:generateContent", req.URL.String())
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
		require.Equal(t, "google-secret", req.Header.Get("x-goog-api-key"))
		require.Empty(t, req.Header.Get("Authorization"))
		require.Equal(t, "http://proxy.example.test:8080", proxyURL)
		require.Equal(t, int64(42), accountID)
		require.Equal(t, 7, accountConcurrency)

		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		contents := payload["contents"].([]any)
		content := contents[0].(map[string]any)
		require.Equal(t, "user", content["role"])
		parts := content["parts"].([]any)
		require.Equal(t, "draw a cat", parts[0].(map[string]any)["text"])
		generationConfig := payload["generationConfig"].(map[string]any)
		require.Equal(t, []any{"TEXT", "IMAGE"}, generationConfig["responseModalities"])
		require.Equal(t, map[string]any{
			"aspectRatio": "2:3",
			"imageSize":   "2K",
		}, generationConfig["imageConfig"])

		return nanoBananaTestResponse(http.StatusOK, `{
			"candidates":[{"content":{"parts":[
				{"text":"done"},
				{"inlineData":{"mimeType":"image/png","data":"`+base64.StdEncoding.EncodeToString(image)+`"}}
			]}}],
			"usageMetadata":{
				"candidatesTokenCount":42,
				"candidatesTokensDetails":[{"modality":"IMAGE","tokenCount":37}]
			}
		}`), nil
	}}
	adapter := NewNanoBananaMediaAdapter(upstream)

	result, err := adapter.Generate(context.Background(), nanoBananaGenerationRequest())
	require.NoError(t, err)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, "output", result.Artifacts[0].Direction)
	require.Equal(t, MediaTypeImage, result.Artifacts[0].MediaType)
	require.Equal(t, "image/png", result.Artifacts[0].ContentType)
	require.Equal(t, image, result.Artifacts[0].Data)
	require.Equal(t, int64(len(image)), result.Artifacts[0].SizeBytes)
	require.Equal(t, 1, result.Usage.ImageCount)
	require.Equal(t, "1024x1536", result.Usage.ImageSize)
	require.Equal(t, 42, result.Usage.OutputTokens)
}

func TestNanoBananaMediaAdapterBuildsOrderedImageEditAndReadsSnakeCase(t *testing.T) {
	t.Parallel()
	first := nanoBananaTestPNG(t)
	second := nanoBananaTestJPEG(t)
	output := nanoBananaTestWebP()
	upstream := &nanoBananaHTTPUpstreamStub{do: func(
		req *http.Request,
		_ string,
		_ int64,
		_ int,
	) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		contents := payload["contents"].([]any)
		parts := contents[0].(map[string]any)["parts"].([]any)
		require.Len(t, parts, 3)
		require.Equal(t, "edit both images", parts[0].(map[string]any)["text"])
		firstInline := parts[1].(map[string]any)["inlineData"].(map[string]any)
		secondInline := parts[2].(map[string]any)["inlineData"].(map[string]any)
		require.Equal(t, "image/png", firstInline["mimeType"])
		require.Equal(t, base64.StdEncoding.EncodeToString(first), firstInline["data"])
		require.Equal(t, "image/jpeg", secondInline["mimeType"])
		require.Equal(t, base64.StdEncoding.EncodeToString(second), secondInline["data"])
		require.NotContains(t, firstInline, "mime_type")

		return nanoBananaTestResponse(http.StatusOK, `{
			"candidates":[{"content":{"parts":[{"inline_data":{
				"mime_type":"image/webp","data":"`+base64.StdEncoding.EncodeToString(output)+`"
			}}]}}],
			"usage_metadata":{"candidates_tokens_details":[{"modality":"IMAGE","token_count":19}]}
		}`), nil
	}}
	request := nanoBananaGenerationRequest()
	request.Task.Operation = MediaOperationImageEdit
	request.Spec.Image.Prompt = "edit both images"
	request.Spec.Image.Size = ""
	request.Spec.Image.InputArtifactIDs = []int64{10, 11}
	request.ResolvedRequest = json.RawMessage(`{"image":{"prompt":"edit both images","n":1,"input_artifact_ids":[10,11]}}`)
	request.Inputs = []MediaArtifactInput{
		{Direction: "input", Position: 9, MediaType: MediaTypeImage, ContentType: "image/jpeg", Data: second},
		{Direction: "input", Position: 3, MediaType: MediaTypeImage, ContentType: "image/png", Data: first},
	}

	result, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, "image/webp", result.Artifacts[0].ContentType)
	require.Equal(t, output, result.Artifacts[0].Data)
	require.Equal(t, 19, result.Usage.OutputTokens)
}

func TestNanoBananaMediaAdapterMergesProviderExtensionsWithoutOverridingCore(t *testing.T) {
	t.Parallel()
	image := nanoBananaTestPNG(t)
	upstream := &nanoBananaHTTPUpstreamStub{do: func(
		req *http.Request,
		_ string,
		_ int64,
		_ int,
	) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		generationConfig := payload["generationConfig"].(map[string]any)
		require.Equal(t, []any{"TEXT", "IMAGE"}, generationConfig["responseModalities"])
		require.Equal(t, float64(0.25), generationConfig["temperature"])
		require.Equal(t, map[string]any{
			"aspectRatio": "16:9",
			"imageSize":   "4K",
		}, generationConfig["imageConfig"])
		require.Equal(t, []any{map[string]any{"category": "HARM_CATEGORY_DANGEROUS_CONTENT", "threshold": "BLOCK_ONLY_HIGH"}}, payload["safetySettings"])
		require.NotContains(t, payload, "prompt")
		require.NotContains(t, payload, "size")

		return nanoBananaTestResponse(http.StatusOK, `{"candidates":[{"content":{"parts":[{
			"inlineData":{"mimeType":"image/png","data":"`+base64.StdEncoding.EncodeToString(image)+`"}
		}]}}]}`), nil
	}}
	request := nanoBananaGenerationRequest()
	request.ResolvedRequest = json.RawMessage(`{"image":{
		"prompt":"draw a cat",
		"n":1,
		"generationConfig":{"temperature":0.25,"imageConfig":{"aspectRatio":"16:9","imageSize":"4K"}},
		"safetySettings":[{"category":"HARM_CATEGORY_DANGEROUS_CONTENT","threshold":"BLOCK_ONLY_HIGH"}]
	}}`)

	_, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), request)
	require.NoError(t, err)
}

func TestNanoBananaMediaAdapterNormalizesSnakeCaseGenerationConfigExtension(t *testing.T) {
	t.Parallel()
	image := nanoBananaTestPNG(t)
	upstream := &nanoBananaHTTPUpstreamStub{do: func(
		req *http.Request,
		_ string,
		_ int64,
		_ int,
	) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(body, &payload))
		require.NotContains(t, payload, "generation_config")
		generationConfig := payload["generationConfig"].(map[string]any)
		require.Equal(t, []any{"TEXT", "IMAGE"}, generationConfig["responseModalities"])
		require.Equal(t, float64(0.4), generationConfig["temperature"])

		return nanoBananaTestResponse(http.StatusOK, `{"candidates":[{"content":{"parts":[{
			"inlineData":{"mimeType":"image/png","data":"`+base64.StdEncoding.EncodeToString(image)+`"}
		}]}}]}`), nil
	}}
	request := nanoBananaGenerationRequest()
	request.ResolvedRequest = json.RawMessage(`{"image":{
		"prompt":"draw a cat",
		"n":1,
		"generation_config":{"temperature":0.4}
	}}`)

	_, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), request)
	require.NoError(t, err)
}

func TestNanoBananaMediaAdapterRejectsProtectedResolvedOverrides(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		resolved string
	}{
		{name: "contents", resolved: `{"image":{"prompt":"cat","n":1,"contents":[]}}`},
		{name: "model", resolved: `{"image":{"prompt":"cat","n":1,"model":"other"}}`},
		{name: "api key", resolved: `{"image":{"prompt":"cat","n":1,"api_key":"other"}}`},
		{name: "authorization", resolved: `{"image":{"prompt":"cat","n":1,"authorization":"other"}}`},
		{name: "headers", resolved: `{"image":{"prompt":"cat","n":1,"headers":{}}}`},
		{name: "url", resolved: `{"image":{"prompt":"cat","n":1,"url":"https://other.test"}}`},
		{name: "base url", resolved: `{"image":{"prompt":"cat","n":1,"base_url":"https://other.test"}}`},
		{name: "response modalities", resolved: `{"image":{"prompt":"cat","n":1,"generationConfig":{"responseModalities":["TEXT"]}}}`},
		{name: "snake response modalities", resolved: `{"image":{"prompt":"cat","n":1,"generationConfig":{"response_modalities":["TEXT"]}}}`},
		{name: "snake config camel response modalities", resolved: `{"image":{"prompt":"cat","n":1,"generation_config":{"responseModalities":["TEXT"]}}}`},
		{name: "snake config snake response modalities", resolved: `{"image":{"prompt":"cat","n":1,"generation_config":{"response_modalities":["TEXT"]}}}`},
		{name: "hyphen aliases", resolved: `{"image":{"prompt":"cat","n":1,"generation-config":{"response-modalities":["TEXT"]}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			upstream := &nanoBananaHTTPUpstreamStub{}
			request := nanoBananaGenerationRequest()
			request.ResolvedRequest = json.RawMessage(tt.resolved)

			_, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), request)
			requireNanoBananaAdapterError(t, err, "media_adapter_mapping_forbidden", false, true)
			require.Zero(t, upstream.calls)
		})
	}
}

func TestNanoBananaMediaAdapterRejectsAmbiguousGenerationConfigAliases(t *testing.T) {
	t.Parallel()
	upstream := &nanoBananaHTTPUpstreamStub{}
	request := nanoBananaGenerationRequest()
	request.ResolvedRequest = json.RawMessage(`{"image":{
		"prompt":"cat",
		"n":1,
		"generationConfig":{"temperature":0.2},
		"generation_config":{"temperature":0.4}
	}}`)

	_, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), request)
	requireNanoBananaAdapterError(t, err, "media_adapter_mapping_invalid", false, true)
	require.Zero(t, upstream.calls)
}

func TestNanoBananaMediaAdapterClassifiesHTTPAndTransportFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		status        int
		body          string
		transportErr  error
		wantCode      string
		wantRetryable bool
		wantUnknown   bool
	}{
		{name: "bad request", status: http.StatusBadRequest, body: `{"error":{"message":"invalid argument"}}`, wantCode: "upstream_bad_request"},
		{name: "content policy", status: http.StatusBadRequest, body: `{"error":{"message":"blocked by safety policy"}}`, wantCode: "content_policy_violation"},
		{name: "unauthorized", status: http.StatusUnauthorized, wantCode: "upstream_authentication_failed", wantRetryable: true},
		{name: "forbidden", status: http.StatusForbidden, wantCode: "upstream_permission_denied", wantRetryable: true},
		{name: "rate limited", status: http.StatusTooManyRequests, wantCode: "upstream_rate_limited", wantRetryable: true},
		{name: "server error", status: http.StatusBadGateway, wantCode: "upstream_unavailable", wantRetryable: true},
		{name: "timeout", transportErr: nanoBananaTimeoutError{}, wantCode: "upstream_timeout", wantRetryable: true, wantUnknown: true},
		{name: "transport", transportErr: errors.New("connection reset"), wantCode: "upstream_unavailable", wantRetryable: true, wantUnknown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			upstream := &nanoBananaHTTPUpstreamStub{do: func(
				*http.Request, string, int64, int,
			) (*http.Response, error) {
				if tt.transportErr != nil {
					return nil, tt.transportErr
				}
				return nanoBananaTestResponse(tt.status, tt.body), nil
			}}

			_, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), nanoBananaGenerationRequest())
			adapterError := requireNanoBananaAdapterError(t, err, tt.wantCode, tt.wantRetryable, false)
			require.Equal(t, tt.wantUnknown, adapterError.SubmissionUnknown)
			require.NotContains(t, err.Error(), "invalid argument")
			require.NotContains(t, err.Error(), "google-secret")
		})
	}
}

func TestNanoBananaMediaAdapterClosesResponseBodyReturnedWithTransportError(t *testing.T) {
	t.Parallel()
	body := &nanoBananaCloseObserver{Reader: strings.NewReader("partial response")}
	upstream := &nanoBananaHTTPUpstreamStub{do: func(
		*http.Request, string, int64, int,
	) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadGateway, Body: body}, errors.New("connection reset")
	}}

	_, err := NewNanoBananaMediaAdapter(upstream).Generate(context.Background(), nanoBananaGenerationRequest())
	requireNanoBananaAdapterError(t, err, "upstream_unavailable", true, false)
	require.True(t, body.closed)
}

func TestDecodeNanoBananaResponseRejectsUnsafeOrMissingImages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		body          string
		wantCode      string
		wantRetryable bool
		wantUnknown   bool
	}{
		{
			name:     "safety block",
			body:     `{"promptFeedback":{"blockReason":"SAFETY"},"candidates":[]}`,
			wantCode: "content_policy_violation",
		},
		{
			name:     "candidate safety block",
			body:     `{"candidates":[{"finishReason":"IMAGE_SAFETY","content":{"parts":[]}}]}`,
			wantCode: "content_policy_violation",
		},
		{
			name:     "no image",
			body:     `{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"no image"}]}}]}`,
			wantCode: "upstream_invalid_response", wantRetryable: true, wantUnknown: true,
		},
		{
			name:     "invalid base64",
			body:     `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"not-base64"}}]}}]}`,
			wantCode: "upstream_invalid_response", wantRetryable: true, wantUnknown: true,
		},
		{
			name:     "unsafe mime",
			body:     `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/svg+xml","data":"PHN2Zz4="}}]}}]}`,
			wantCode: "upstream_invalid_response", wantRetryable: true, wantUnknown: true,
		},
		{
			name:     "content type mismatch",
			body:     `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"cGxhaW4gdGV4dA=="}}]}}]}`,
			wantCode: "upstream_invalid_response", wantRetryable: true, wantUnknown: true,
		},
		{
			name: "more images than requested",
			body: `{"candidates":[{"content":{"parts":[
				{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}},
				{"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}}
			]}}]}`,
			wantCode: "upstream_invalid_response", wantRetryable: true, wantUnknown: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := decodeNanoBananaResponse([]byte(tt.body), 1)
			adapterError := requireNanoBananaAdapterError(t, err, tt.wantCode, tt.wantRetryable, false)
			require.Equal(t, tt.wantUnknown, adapterError.SubmissionUnknown)
		})
	}
}

func TestNanoBananaMediaAdapterRegistrationUsesExactKnownModelsOnly(t *testing.T) {
	t.Parallel()
	adapter := NewNanoBananaMediaAdapter(&nanoBananaHTTPUpstreamStub{})
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.RegisterDefinition(nanoBananaMediaAdapterRegistration(adapter)))
	resolver := NewMediaAdapterResolver(registry)

	knownModels := []string{
		"nano-banana",
		"nano-banana-pro",
		"nano-banana-pro-preview",
		"nano-banana-2",
		"gemini-2.0-flash-exp",
		"gemini-2.0-flash-exp-image-generation",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-3-pro-image",
		"gemini-3-pro-image-preview",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
	}
	require.ElementsMatch(t, knownModels, nanoBananaCanonicalModelIDs)
	for _, modelID := range knownModels {
		resolution := resolver.Resolve("google", modelID, []MediaOperation{MediaOperationTextToImage})
		require.True(t, resolution.IsReady(), modelID)
		require.Equal(t, nanoBananaMediaAdapterKey, resolution.ResolvedAdapter)
		require.Equal(t, MediaAdapterMatchedExact, resolution.MatchedBy)
		require.True(t, resolution.Capabilities.SyncUpstream)
		require.False(t, resolution.Capabilities.NativeAsyncUpstream)
	}
	require.Equal(
		t,
		MediaAdapterResolutionUnresolved,
		resolver.Resolve("google", "nano-banana-future", []MediaOperation{MediaOperationTextToImage}).Status,
	)
	require.Equal(
		t,
		MediaAdapterResolutionCapabilityMismatch,
		resolver.Resolve("google", "nano-banana", []MediaOperation{MediaOperationImageToImage}).Status,
	)
	_, asyncSubmitter := any(adapter).(MediaAsyncSubmitter)
	require.False(t, asyncSubmitter)
}

func nanoBananaGenerationRequest() MediaExecutionRequest {
	return MediaExecutionRequest{
		Task: &MediaTask{Operation: MediaOperationTextToImage},
		Account: &Account{
			ID:          42,
			Concurrency: 7,
			Credentials: map[string]any{
				"api_key":  "google-secret",
				"base_url": "https://gemini.example.test/custom",
			},
			Proxy: &Proxy{Protocol: "http", Host: "proxy.example.test", Port: 8080, Status: StatusActive},
		},
		Spec: MediaSpec{Image: &ImageSpec{
			Prompt: "draw a cat",
			Size:   "1024x1536",
			Count:  1,
		}},
		ResolvedRequest: json.RawMessage(`{"image":{"prompt":"draw a cat","size":"1024x1536","n":1}}`),
		UpstreamModel:   "gemini-3.1-flash-image-preview",
	}
}

func nanoBananaTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func nanoBananaTestPNG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	testImage := image.NewRGBA(image.Rect(0, 0, 1, 1))
	testImage.Set(0, 0, color.RGBA{R: 255, A: 255})
	err := png.Encode(&output, testImage)
	require.NoError(t, err)
	return output.Bytes()
}

func nanoBananaTestJPEG(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	testImage := image.NewRGBA(image.Rect(0, 0, 1, 1))
	testImage.Set(0, 0, color.RGBA{G: 255, A: 255})
	err := jpeg.Encode(&output, testImage, nil)
	require.NoError(t, err)
	return output.Bytes()
}

func nanoBananaTestWebP() []byte {
	return []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 'V', 'P', '8', ' '}
}

func requireNanoBananaAdapterError(
	t *testing.T,
	err error,
	wantCode string,
	wantRetryable bool,
	wantSystemFailure bool,
) *MediaAdapterError {
	t.Helper()
	require.Error(t, err)
	var adapterError *MediaAdapterError
	require.ErrorAs(t, err, &adapterError)
	require.Equal(t, wantCode, adapterError.Code)
	require.Equal(t, wantRetryable, adapterError.Retryable)
	require.Equal(t, wantSystemFailure, adapterError.SystemFailure)
	return adapterError
}

type nanoBananaTimeoutError struct{}

func (nanoBananaTimeoutError) Error() string   { return "timeout" }
func (nanoBananaTimeoutError) Timeout() bool   { return true }
func (nanoBananaTimeoutError) Temporary() bool { return true }

type nanoBananaCloseObserver struct {
	io.Reader
	closed bool
}

func (r *nanoBananaCloseObserver) Close() error {
	r.closed = true
	return nil
}

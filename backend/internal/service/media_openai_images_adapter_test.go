package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type openAIImagesMediaUpstreamStub struct {
	do func(*http.Request, string, int64, int) (*http.Response, error)
}

func (s *openAIImagesMediaUpstreamStub) Do(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
) (*http.Response, error) {
	if s == nil || s.do == nil {
		return nil, errors.New("unexpected OpenAI Images request")
	}
	return s.do(req, proxyURL, accountID, accountConcurrency)
}

func (s *openAIImagesMediaUpstreamStub) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func TestOpenAIImagesMediaAdapterGeneratesFromResolvedRequest(t *testing.T) {
	t.Parallel()
	png := openAIImagesTestPNG()
	upstream := &openAIImagesMediaUpstreamStub{do: func(
		req *http.Request,
		proxyURL string,
		accountID int64,
		accountConcurrency int,
	) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "https://images.example.test/v1/images/generations", req.URL.String())
		require.Equal(t, "Bearer secret-key", req.Header.Get("Authorization"))
		require.Equal(t, "application/json", req.Header.Get("Content-Type"))
		require.Equal(t, "http://proxy.local:8080", proxyURL)
		require.Equal(t, int64(42), accountID)
		require.Equal(t, 7, accountConcurrency)

		requestBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(requestBody, &body))
		require.Equal(t, "provider-gpt-image", body["model"])
		require.Equal(t, "a cat", body["prompt"])
		require.Equal(t, "square", body["chicun"])
		require.Equal(t, map[string]any{"strength": float64(0.5)}, body["provider_options"])
		require.NotContains(t, body, "input_artifact_ids")
		require.NotContains(t, body, "response_format")

		return openAIImagesTestResponse(http.StatusOK, `{
			"data":[{"b64_json":"`+base64.StdEncoding.EncodeToString(png)+`","output_format":"png"}],
			"usage":{"output_tokens":23}
		}`), nil
	}}
	adapter := NewOpenAIImagesMediaAdapter(upstream)
	account := openAIImagesTestAccount()
	account.Credentials["base_url"] = "https://images.example.test/v1"

	result, err := adapter.Generate(context.Background(), MediaExecutionRequest{
		Task:          &MediaTask{Operation: MediaOperationTextToImage},
		Account:       account,
		Spec:          MediaSpec{Image: &ImageSpec{Prompt: "a cat", Size: "1024x1024", OutputFormat: "png", Count: 1}},
		UpstreamModel: "provider-gpt-image",
		ResolvedRequest: json.RawMessage(`{"image":{
			"prompt":"a cat",
			"n":1,
			"chicun":"square",
			"provider_options":{"strength":0.5},
			"input_artifact_ids":[91],
			"response_format":"url"
		}}`),
	})
	require.NoError(t, err)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, png, result.Artifacts[0].Data)
	require.Equal(t, "image/png", result.Artifacts[0].ContentType)
	require.Equal(t, int64(len(png)), result.Artifacts[0].SizeBytes)
	require.Equal(t, 1, result.Usage.ImageCount)
	require.Equal(t, "1024x1024", result.Usage.ImageSize)
	require.Equal(t, 23, result.Usage.OutputTokens)
}

func TestOpenAIImagesMediaAdapterBuildsMultipleImageEditMultipart(t *testing.T) {
	t.Parallel()
	first := openAIImagesTestPNG()
	second := openAIImagesTestJPEG()
	upstream := &openAIImagesMediaUpstreamStub{do: func(
		req *http.Request,
		_ string,
		_ int64,
		_ int,
	) (*http.Response, error) {
		require.Equal(t, "https://images.example.test/custom/v1/images/edits", req.URL.String())
		mediaType, parameters, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		require.NoError(t, err)
		require.Equal(t, "multipart/form-data", mediaType)
		reader := multipart.NewReader(req.Body, parameters["boundary"])
		fields := make(map[string]string)
		var fileFields []string
		var files [][]byte
		for {
			part, nextErr := reader.NextPart()
			if errors.Is(nextErr, io.EOF) {
				break
			}
			require.NoError(t, nextErr)
			data, readErr := io.ReadAll(part)
			require.NoError(t, readErr)
			if part.FileName() == "" {
				fields[part.FormName()] = string(data)
				continue
			}
			fileFields = append(fileFields, part.FormName())
			files = append(files, data)
		}
		require.Equal(t, "provider-edit-model", fields["model"])
		require.Equal(t, "replace the sky", fields["prompt"])
		require.Equal(t, "provider-high", fields["provider_quality"])
		require.Equal(t, `{"seed":7}`, fields["provider_options"])
		require.NotContains(t, fields, "input_artifact_ids")
		require.NotContains(t, fields, "response_format")
		require.Equal(t, []string{"image[]", "image[]"}, fileFields)
		require.Equal(t, [][]byte{first, second}, files)

		return openAIImagesTestResponse(http.StatusOK, `{
			"data":[{"url":"https://cdn.example.test/generated/output.webp"}],
			"output_format":"webp"
		}`), nil
	}}
	adapter := NewOpenAIImagesMediaAdapter(upstream)

	result, err := adapter.Generate(context.Background(), MediaExecutionRequest{
		Task:          &MediaTask{Operation: MediaOperationImageEdit},
		Account:       openAIImagesTestAccount(),
		Spec:          MediaSpec{Image: &ImageSpec{Prompt: "replace the sky", Count: 1}},
		UpstreamModel: "provider-edit-model",
		ResolvedRequest: json.RawMessage(`{"image":{
			"prompt":"replace the sky",
			"provider_quality":"provider-high",
			"provider_options":{"seed":7},
			"input_artifact_ids":[1,2],
			"response_format":"b64_json"
		}}`),
		Inputs: []MediaArtifactInput{
			{MediaType: MediaTypeImage, ContentType: "image/png", Data: first},
			{MediaType: MediaTypeImage, ContentType: "image/jpeg", Data: second},
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Artifacts, 1)
	require.Equal(t, "https://cdn.example.test/generated/output.webp", result.Artifacts[0].ExternalURL)
	require.Equal(t, "image/webp", result.Artifacts[0].ContentType)
}

func TestOpenAIImagesMediaAdapterSingleImageEditUsesImageField(t *testing.T) {
	t.Parallel()
	encoded, contentType, err := encodeOpenAIImagesEditRequest(
		map[string]any{"model": "gpt-image-2", "prompt": "edit"},
		[]MediaArtifactInput{{MediaType: MediaTypeImage, ContentType: "image/png", Data: openAIImagesTestPNG()}},
	)
	require.NoError(t, err)
	_, parameters, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(encoded), parameters["boundary"])
	var fileField string
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		require.NoError(t, nextErr)
		if part.FileName() != "" {
			fileField = part.FormName()
		}
	}
	require.Equal(t, "image", fileField)
}

func TestOpenAIImagesMediaAdapterClassifiesHTTPAndTransportFailures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		status            int
		transportErr      error
		wantCode          string
		wantRetryable     bool
		wantUnknown       bool
		wantSystemFailure bool
	}{
		{name: "bad request", status: http.StatusBadRequest, wantCode: "upstream_request_rejected"},
		{name: "unauthorized", status: http.StatusUnauthorized, wantCode: "upstream_auth_failed"},
		{name: "rate limited", status: http.StatusTooManyRequests, wantCode: "upstream_rate_limited", wantRetryable: true},
		{name: "server error", status: http.StatusBadGateway, wantCode: "upstream_unavailable", wantRetryable: true},
		{
			name: "transport outcome unknown", transportErr: errors.New("connection reset"),
			wantCode: "upstream_transport_error", wantRetryable: true, wantUnknown: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewOpenAIImagesMediaAdapter(&openAIImagesMediaUpstreamStub{do: func(
				*http.Request, string, int64, int,
			) (*http.Response, error) {
				if tt.transportErr != nil {
					return nil, tt.transportErr
				}
				return openAIImagesTestResponse(tt.status, `{"error":{"message":"secret provider detail"}}`), nil
			}})
			_, err := adapter.Generate(context.Background(), openAIImagesGenerationRequest())
			require.Error(t, err)
			var adapterErr *MediaAdapterError
			require.ErrorAs(t, err, &adapterErr)
			require.Equal(t, tt.wantCode, adapterErr.Code)
			require.Equal(t, tt.wantRetryable, adapterErr.Retryable)
			require.Equal(t, tt.wantUnknown, adapterErr.SubmissionUnknown)
			require.Equal(t, tt.wantSystemFailure, adapterErr.SystemFailure)
			require.NotContains(t, adapterErr.Error(), "secret provider detail")
			require.NotContains(t, adapterErr.Error(), "secret-key")
		})
	}
}

func TestOpenAIImagesMediaAdapterRejectsInvalidSuccessfulResponse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
	}{
		{name: "empty data", body: `{"data":[]}`},
		{name: "missing payload", body: `{"data":[{}]}`},
		{name: "invalid base64", body: `{"data":[{"b64_json":"not-base64"}]}`},
		{name: "base64 is not image", body: `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString([]byte("plain text")) + `","output_format":"png"}]}`},
		{name: "unsafe url", body: `{"data":[{"url":"https://user:password@example.test/image.png"}]}`},
		{name: "more images than requested", body: `{"data":[{"url":"https://cdn.example.test/first.png"},{"url":"https://cdn.example.test/second.png"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			adapter := NewOpenAIImagesMediaAdapter(&openAIImagesMediaUpstreamStub{do: func(
				*http.Request, string, int64, int,
			) (*http.Response, error) {
				return openAIImagesTestResponse(http.StatusOK, tt.body), nil
			}})
			_, err := adapter.Generate(context.Background(), openAIImagesGenerationRequest())
			require.Error(t, err)
			var adapterErr *MediaAdapterError
			require.ErrorAs(t, err, &adapterErr)
			require.Equal(t, "system_upstream_response", adapterErr.Code)
			require.True(t, adapterErr.SystemFailure)
			require.False(t, adapterErr.Retryable)
		})
	}
}

func TestOpenAIImagesMediaAdapterRejectsProtectedResolvedOverrides(t *testing.T) {
	t.Parallel()
	tests := []string{"model", "api_key", "authorization", "headers", "url", "base_url"}
	for _, field := range tests {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			calls := 0
			adapter := NewOpenAIImagesMediaAdapter(&openAIImagesMediaUpstreamStub{do: func(
				*http.Request, string, int64, int,
			) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected request")
			}})
			request := openAIImagesGenerationRequest()
			request.ResolvedRequest = json.RawMessage(`{"image":{"prompt":"cat","n":1,"` + field + `":"override"}}`)

			_, err := adapter.Generate(context.Background(), request)
			require.Error(t, err)
			var adapterErr *MediaAdapterError
			require.ErrorAs(t, err, &adapterErr)
			require.Equal(t, "media_adapter_mapping_forbidden", adapterErr.Code)
			require.True(t, adapterErr.SystemFailure)
			require.Zero(t, calls)
		})
	}
}

func TestOpenAIImagesMediaAdapterRegistrationResolvesSupportedModels(t *testing.T) {
	t.Parallel()
	adapter := NewOpenAIImagesMediaAdapter(&openAIImagesMediaUpstreamStub{})
	registry := NewMediaAdapterRegistry()
	require.NoError(t, registry.RegisterDefinition(NewOpenAIImagesMediaAdapterRegistration(adapter)))
	resolver := NewMediaAdapterResolver(registry)

	for _, modelID := range []string{"gpt-image-1", "gpt-image-1.5", "gpt-image-2"} {
		resolution := resolver.Resolve("openai", modelID, []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit})
		require.True(t, resolution.IsReady(), modelID)
		require.Equal(t, OpenAIImagesMediaAdapterKey, resolution.ResolvedAdapter)
		require.True(t, resolution.Capabilities.SyncUpstream)
		require.False(t, resolution.Capabilities.NativeAsyncUpstream)
	}
	exact := resolver.Resolve("openai", "chatgpt-image-latest", []MediaOperation{MediaOperationTextToImage})
	require.True(t, exact.IsReady())
	require.Equal(t, MediaAdapterMatchedExact, exact.MatchedBy)

	unknown := resolver.Resolve("openai", "dall-e-3", []MediaOperation{MediaOperationTextToImage})
	require.Equal(t, MediaAdapterResolutionUnresolved, unknown.Status)
}

func openAIImagesGenerationRequest() MediaExecutionRequest {
	return MediaExecutionRequest{
		Task:            &MediaTask{Operation: MediaOperationTextToImage},
		Account:         openAIImagesTestAccount(),
		Spec:            MediaSpec{Image: &ImageSpec{Prompt: "cat", Count: 1}},
		UpstreamModel:   "provider-gpt-image",
		ResolvedRequest: json.RawMessage(`{"image":{"prompt":"cat","n":1}}`),
	}
}

func openAIImagesTestAccount() *Account {
	return &Account{
		ID:          42,
		Concurrency: 7,
		Credentials: map[string]any{
			"api_key":  "secret-key",
			"base_url": "https://images.example.test/custom/v1",
		},
		Proxy: &Proxy{
			Protocol: "http",
			Host:     "proxy.local",
			Port:     8080,
			Status:   StatusActive,
		},
	}
}

func openAIImagesTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func openAIImagesTestPNG() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 16)...)
}

func openAIImagesTestJPEG() []byte {
	return append([]byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0}, 16)...)
}

package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	OpenAIImagesMediaAdapterKey = "openai-images"

	openAIImagesMediaGenerationsEndpoint = "/v1/images/generations"
	openAIImagesMediaEditsEndpoint       = "/v1/images/edits"
	openAIImagesMediaMaxResponseBytes    = 128 << 20
	openAIImagesMediaMaxErrorBytes       = 64 << 10
	openAIImagesMediaMaxInputBytes       = 64 << 20
)

var errOpenAIImagesMediaBodyTooLarge = errors.New("OpenAI Images response body is too large")

// OpenAIImagesMediaAdapter executes the official OpenAI Images protocol for
// the isolated media task runtime. Downstream asynchronous requests still use
// the media Worker; this adapter intentionally exposes only synchronous
// upstream generation.
type OpenAIImagesMediaAdapter struct {
	upstream HTTPUpstream
}

var (
	_ MediaAdapter       = (*OpenAIImagesMediaAdapter)(nil)
	_ MediaSyncGenerator = (*OpenAIImagesMediaAdapter)(nil)
)

func NewOpenAIImagesMediaAdapter(upstream HTTPUpstream) *OpenAIImagesMediaAdapter {
	return &OpenAIImagesMediaAdapter{upstream: upstream}
}

func (*OpenAIImagesMediaAdapter) Name() string {
	return OpenAIImagesMediaAdapterKey
}

// NewOpenAIImagesMediaAdapterRegistration returns the code-owned routing
// rules. Model definitions and account bindings never select this key
// directly.
func NewOpenAIImagesMediaAdapterRegistration(adapter *OpenAIImagesMediaAdapter) MediaAdapterRegistration {
	operations := []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit}
	capabilities := MediaAdapterRuleCapabilities{
		Operations:   append([]MediaOperation(nil), operations...),
		SyncUpstream: true,
	}
	return MediaAdapterRegistration{
		Key:                 OpenAIImagesMediaAdapterKey,
		Adapter:             adapter,
		SupportedOperations: operations,
		ExactRules: []MediaAdapterExactRule{{
			Vendor:       "openai",
			ModelID:      "chatgpt-image-latest",
			Capabilities: capabilities,
		}},
		FamilyRules: []MediaAdapterFamilyRule{{
			Vendor:   "openai",
			FamilyID: "gpt-image",
			Match: func(modelID string) bool {
				modelID = strings.ToLower(strings.TrimSpace(modelID))
				return strings.HasPrefix(modelID, "gpt-image-") && len(modelID) > len("gpt-image-")
			},
			Capabilities: capabilities,
		}},
	}
}

func (a *OpenAIImagesMediaAdapter) Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.upstream == nil {
		return nil, openAIImagesSystemError("system_adapter", "OpenAI 图片 Adapter 未配置", nil)
	}
	if req.Account == nil {
		return nil, openAIImagesSystemError("system_account", "媒体账号不可用", nil)
	}
	if req.Spec.Image == nil || req.Spec.Validate(MediaTypeImage) != nil {
		return nil, openAIImagesSystemError("system_request", "图片请求无效", ErrInvalidMediaSpec)
	}

	apiKey := strings.TrimSpace(req.Account.GetCredential("api_key"))
	baseURL := strings.TrimSpace(req.Account.GetCredential("base_url"))
	model := strings.TrimSpace(req.UpstreamModel)
	if apiKey == "" || baseURL == "" || model == "" {
		return nil, openAIImagesSystemError("system_account_config", "媒体账号配置不完整", nil)
	}

	body, err := decodeOpenAIImagesResolvedBody(req.ResolvedRequest)
	if err != nil {
		return nil, openAIImagesSystemError("system_request", "映射后的图片请求无效", err)
	}
	if err := validateOpenAIImagesResolvedBodyProtection(body); err != nil {
		return nil, openAIImagesSystemError("media_adapter_mapping_forbidden", "映射后的图片请求覆盖了受保护字段", err)
	}
	delete(body, "input_artifact_ids")
	delete(body, "response_format")
	body["model"] = model

	operation, err := openAIImagesExecutionOperation(req)
	if err != nil {
		return nil, openAIImagesSystemError("system_request", "图片操作不受支持", err)
	}

	var requestBody io.Reader
	contentType := "application/json"
	endpoint := openAIImagesMediaGenerationsEndpoint
	switch operation {
	case MediaOperationTextToImage:
		if len(req.Inputs) != 0 {
			return nil, openAIImagesSystemError("system_request", "文生图请求不能包含输入图片", nil)
		}
		encoded, encodeErr := json.Marshal(body)
		if encodeErr != nil {
			return nil, openAIImagesSystemError("system_request", "图片请求无法编码", encodeErr)
		}
		requestBody = bytes.NewReader(encoded)
	case MediaOperationImageEdit:
		if len(req.Inputs) == 0 {
			return nil, openAIImagesSystemError("system_request", "图片编辑缺少输入图片", nil)
		}
		encoded, multipartContentType, encodeErr := encodeOpenAIImagesEditRequest(body, req.Inputs)
		if encodeErr != nil {
			return nil, openAIImagesSystemError("system_input_artifact", "图片编辑输入无效", encodeErr)
		}
		endpoint = openAIImagesMediaEditsEndpoint
		contentType = multipartContentType
		requestBody = bytes.NewReader(encoded)
	}

	targetURL, err := buildOpenAIImagesMediaURL(baseURL, endpoint)
	if err != nil {
		return nil, openAIImagesSystemError("system_account_config", "媒体账号 Base URL 无效", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, requestBody)
	if err != nil {
		return nil, openAIImagesSystemError("system_request", "无法创建上游图片请求", err)
	}
	httpRequest.Header.Set("Authorization", "Bearer "+apiKey)
	httpRequest.Header.Set("Content-Type", contentType)
	httpRequest.Header.Set("Accept", "application/json")

	response, err := a.upstream.Do(
		httpRequest,
		req.Account.EffectiveProxyURL(),
		req.Account.ID,
		req.Account.Concurrency,
	)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		return nil, &MediaAdapterError{
			Code:              "upstream_transport_error",
			Message:           "上游图片服务连接失败",
			Retryable:         true,
			SubmissionUnknown: true,
			SystemFailure:     false,
			Cause:             err,
		}
	}
	if response == nil || response.Body == nil {
		return nil, openAIImagesSystemError("system_upstream_response", "上游图片服务返回无效响应", nil)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		// Consume only a bounded prefix on a best-effort basis. The body is
		// intentionally excluded from caller-safe errors and is never read
		// without a hard limit.
		_, _ = readOpenAIImagesMediaBody(response.Body, openAIImagesMediaMaxErrorBytes)
		return nil, classifyOpenAIImagesHTTPError(response.StatusCode)
	}
	responseBody, err := readOpenAIImagesMediaBody(response.Body, openAIImagesMediaMaxResponseBytes)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, contextErr
		}
		if errors.Is(err, errOpenAIImagesMediaBodyTooLarge) {
			return nil, openAIImagesSystemError("system_upstream_response", "上游图片响应过大", err)
		}
		return nil, &MediaAdapterError{
			Code:              "upstream_response_read_failed",
			Message:           "读取上游图片响应失败",
			Retryable:         true,
			SubmissionUnknown: true,
			SystemFailure:     false,
			Cause:             err,
		}
	}
	return decodeOpenAIImagesGenerateResult(responseBody, req)
}

func openAIImagesExecutionOperation(req MediaExecutionRequest) (MediaOperation, error) {
	if req.Task == nil {
		return "", errors.New("media task is nil")
	}
	switch req.Task.Operation {
	case MediaOperationTextToImage, MediaOperationImageEdit:
		return req.Task.Operation, nil
	default:
		return "", fmt.Errorf("unsupported operation %q", req.Task.Operation)
	}
}

func decodeOpenAIImagesResolvedBody(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, errors.New("resolved request is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("resolved request contains multiple JSON values")
		}
		return nil, err
	}
	body, ok := envelope[string(MediaTypeImage)].(map[string]any)
	if !ok || body == nil {
		return nil, errors.New("resolved request image body is missing")
	}
	return body, nil
}

func validateOpenAIImagesResolvedBodyProtection(body map[string]any) error {
	for key := range body {
		normalized := strings.ToLower(strings.TrimSpace(key))
		normalized = strings.NewReplacer("_", "", "-", "").Replace(normalized)
		switch normalized {
		case "model", "apikey", "authorization", "headers", "url", "baseurl":
			return fmt.Errorf("protected field %q", key)
		}
	}
	return nil
}

func encodeOpenAIImagesEditRequest(fields map[string]any, inputs []MediaArtifactInput) ([]byte, string, error) {
	var output bytes.Buffer
	writer := multipart.NewWriter(&output)
	failed := true
	defer func() {
		if failed {
			_ = writer.Close()
		}
	}()

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, err := openAIImagesMultipartFieldValue(fields[key])
		if err != nil {
			return nil, "", fmt.Errorf("encode multipart field %s: %w", key, err)
		}
		if value == nil {
			continue
		}
		if err := writer.WriteField(key, *value); err != nil {
			return nil, "", fmt.Errorf("write multipart field %s: %w", key, err)
		}
	}

	fieldName := "image"
	if len(inputs) > 1 {
		fieldName = "image[]"
	}
	for index := range inputs {
		input := inputs[index]
		contentType, extension, err := validateOpenAIImagesInput(input)
		if err != nil {
			return nil, "", fmt.Errorf("input image %d: %w", index, err)
		}
		header := make(textproto.MIMEHeader)
		header.Set(
			"Content-Disposition",
			fmt.Sprintf(`form-data; name=%q; filename=%q`, fieldName, fmt.Sprintf("image-%d%s", index+1, extension)),
		)
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			return nil, "", fmt.Errorf("create image part %d: %w", index, err)
		}
		if _, err := part.Write(input.Data); err != nil {
			return nil, "", fmt.Errorf("write image part %d: %w", index, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart request: %w", err)
	}
	failed = false
	return output.Bytes(), writer.FormDataContentType(), nil
}

func openAIImagesMultipartFieldValue(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	var encoded string
	switch value := value.(type) {
	case string:
		encoded = value
	case json.Number:
		encoded = value.String()
	case bool:
		encoded = strconv.FormatBool(value)
	case float64:
		encoded = strconv.FormatFloat(value, 'g', -1, 64)
	case float32:
		encoded = strconv.FormatFloat(float64(value), 'g', -1, 32)
	case int:
		encoded = strconv.Itoa(value)
	case int64:
		encoded = strconv.FormatInt(value, 10)
	case int32:
		encoded = strconv.FormatInt(int64(value), 10)
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		encoded = string(data)
	}
	return &encoded, nil
}

func validateOpenAIImagesInput(input MediaArtifactInput) (string, string, error) {
	if input.MediaType != "" && input.MediaType != MediaTypeImage {
		return "", "", errors.New("artifact is not an image")
	}
	if len(input.Data) == 0 {
		return "", "", errors.New("artifact data is empty")
	}
	if len(input.Data) > openAIImagesMediaMaxInputBytes {
		return "", "", errors.New("artifact is too large")
	}
	contentType := strings.ToLower(strings.TrimSpace(input.ContentType))
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		contentType = parsed
	}
	detectedContentType := strings.ToLower(http.DetectContentType(input.Data))
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detectedContentType
	}
	if detectedContentType != contentType {
		return "", "", fmt.Errorf(
			"declared image content type %q does not match detected type %q",
			contentType,
			detectedContentType,
		)
	}
	switch contentType {
	case "image/png":
		return contentType, ".png", nil
	case "image/jpeg":
		return contentType, ".jpg", nil
	case "image/webp":
		return contentType, ".webp", nil
	default:
		return "", "", fmt.Errorf("unsupported image content type %q", contentType)
	}
}

func buildOpenAIImagesMediaURL(baseURL, endpoint string) (string, error) {
	target := buildOpenAIImagesURL(baseURL, endpoint)
	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("base URL must use http or https")
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("base URL is not a valid upstream endpoint")
	}
	return parsed.String(), nil
}

func readOpenAIImagesMediaBody(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil || limit <= 0 {
		return nil, errors.New("response body is unavailable")
	}
	limited := io.LimitReader(reader, limit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errOpenAIImagesMediaBodyTooLarge
	}
	return body, nil
}

type openAIImagesMediaResponse struct {
	Data []struct {
		B64JSON      string `json:"b64_json"`
		URL          string `json:"url"`
		OutputFormat string `json:"output_format"`
	} `json:"data"`
	OutputFormat string `json:"output_format"`
	Usage        struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func decodeOpenAIImagesGenerateResult(raw []byte, req MediaExecutionRequest) (*MediaGenerateResult, error) {
	var response openAIImagesMediaResponse
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&response); err != nil {
		return nil, openAIImagesSystemError("system_upstream_response", "上游图片响应格式无效", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("upstream response contains multiple JSON values")
		}
		return nil, openAIImagesSystemError("system_upstream_response", "上游图片响应格式无效", err)
	}
	if len(response.Data) == 0 {
		return nil, openAIImagesSystemError("system_upstream_response", "上游图片响应未包含图片", nil)
	}
	if len(response.Data) > MaxMediaImageCount || len(response.Data) > req.Spec.Image.Count {
		return nil, openAIImagesSystemError("system_upstream_response", "上游图片响应包含过多图片", nil)
	}

	requestFormat := ""
	imageSize := ""
	if req.Spec.Image != nil {
		requestFormat = req.Spec.Image.OutputFormat
		imageSize = req.Spec.Image.Size
	}
	artifacts := make([]MediaArtifactInput, 0, len(response.Data))
	for index := range response.Data {
		item := response.Data[index]
		format := firstNonEmptyMediaImageString(item.OutputFormat, response.OutputFormat, requestFormat)
		contentType := openAIImagesMediaOutputMIMEType(format)
		artifact := MediaArtifactInput{
			Direction:   "output",
			Position:    index,
			MediaType:   MediaTypeImage,
			ContentType: contentType,
		}
		switch {
		case strings.TrimSpace(item.B64JSON) != "":
			data, dataContentType, err := decodeOpenAIImagesMediaBase64(item.B64JSON)
			if err != nil {
				return nil, openAIImagesSystemError("system_upstream_response", "上游图片数据无效", err)
			}
			detectedContentType := strings.ToLower(http.DetectContentType(data))
			if !strings.HasPrefix(detectedContentType, "image/") {
				return nil, openAIImagesSystemError("system_upstream_response", "上游返回的内容不是图片", nil)
			}
			if dataContentType != "" && dataContentType != detectedContentType {
				return nil, openAIImagesSystemError("system_upstream_response", "上游图片类型与内容不一致", nil)
			}
			artifact.ContentType = detectedContentType
			artifact.Data = data
			artifact.SizeBytes = int64(len(data))
		case strings.TrimSpace(item.URL) != "":
			normalized, err := validateOpenAIImagesMediaResultURL(item.URL)
			if err != nil {
				return nil, openAIImagesSystemError("system_upstream_response", "上游图片地址无效", err)
			}
			if artifact.ContentType == "" {
				artifact.ContentType = "image/png"
			}
			artifact.ExternalURL = normalized
		default:
			return nil, openAIImagesSystemError("system_upstream_response", "上游图片响应缺少图片数据", nil)
		}
		artifacts = append(artifacts, artifact)
	}
	return &MediaGenerateResult{
		Artifacts: artifacts,
		Usage: MediaUsage{
			ImageCount:   len(artifacts),
			ImageSize:    imageSize,
			OutputTokens: response.Usage.OutputTokens,
		},
	}, nil
}

func decodeOpenAIImagesMediaBase64(raw string) ([]byte, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", errors.New("base64 image is empty")
	}
	contentType := ""
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		comma := strings.IndexByte(raw, ',')
		if comma <= len("data:") || comma == len(raw)-1 {
			return nil, "", errors.New("data URL is invalid")
		}
		header := raw[len("data:"):comma]
		parts := strings.Split(header, ";")
		contentType = strings.ToLower(strings.TrimSpace(parts[0]))
		if !strings.HasPrefix(contentType, "image/") {
			return nil, "", errors.New("data URL is not an image")
		}
		isBase64 := false
		for _, part := range parts[1:] {
			if strings.EqualFold(strings.TrimSpace(part), "base64") {
				isBase64 = true
				break
			}
		}
		if !isBase64 {
			return nil, "", errors.New("image data URL is not base64 encoded")
		}
		raw = raw[comma+1:]
	}
	raw = strings.TrimSpace(raw)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(raw, "="))
	}
	if err != nil || len(decoded) == 0 {
		return nil, "", errors.New("base64 image cannot be decoded")
	}
	if len(decoded) > openAIImagesMediaMaxInputBytes {
		return nil, "", errors.New("decoded image is too large")
	}
	if contentType != "" {
		parsed, _, err := mime.ParseMediaType(contentType)
		if err != nil || !strings.HasPrefix(parsed, "image/") {
			return nil, "", errors.New("image content type is invalid")
		}
		contentType = parsed
	}
	return decoded, contentType, nil
}

func validateOpenAIImagesMediaResultURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("image URL must be an http or https URL without userinfo")
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}

func openAIImagesMediaOutputMIMEType(format string) string {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func firstNonEmptyMediaImageString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func classifyOpenAIImagesHTTPError(statusCode int) *MediaAdapterError {
	classified := &MediaAdapterError{
		Code:          "upstream_request_rejected",
		Message:       "上游图片请求被拒绝",
		SystemFailure: false,
		Cause:         fmt.Errorf("upstream images API returned HTTP %d", statusCode),
	}
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		classified.Code = "upstream_auth_failed"
		classified.Message = "上游图片服务认证失败"
	case http.StatusRequestTimeout:
		classified.Code = "upstream_timeout"
		classified.Message = "上游图片服务请求超时"
		classified.Retryable = true
	case http.StatusTooManyRequests:
		classified.Code = "upstream_rate_limited"
		classified.Message = "上游图片服务请求过于频繁"
		classified.Retryable = true
	default:
		if statusCode == http.StatusTooEarly || statusCode >= http.StatusInternalServerError {
			classified.Code = "upstream_unavailable"
			classified.Message = "上游图片服务暂时不可用"
			classified.Retryable = true
		}
	}
	return classified
}

func openAIImagesSystemError(code, message string, cause error) *MediaAdapterError {
	return &MediaAdapterError{
		Code:          code,
		Message:       message,
		SystemFailure: true,
		Cause:         cause,
	}
}

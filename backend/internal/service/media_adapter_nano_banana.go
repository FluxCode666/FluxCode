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
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	nanoBananaMediaAdapterKey     = "nano-banana"
	nanoBananaMaxInlineInputBytes = 14 << 20
	nanoBananaMaxResponseBytes    = 128 << 20
	nanoBananaMaxErrorBodyBytes   = 1 << 20
)

var nanoBananaCanonicalModelIDs = []string{
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

// NanoBananaMediaAdapter translates the canonical media image request into the
// Gemini generateContent image protocol. Gemini image models do not expose a
// native task API, so downstream asynchronous requests still execute this
// synchronous call through the local media queue.
type NanoBananaMediaAdapter struct {
	httpUpstream HTTPUpstream
}

var (
	_ MediaAdapter       = (*NanoBananaMediaAdapter)(nil)
	_ MediaSyncGenerator = (*NanoBananaMediaAdapter)(nil)
)

func NewNanoBananaMediaAdapter(httpUpstream HTTPUpstream) *NanoBananaMediaAdapter {
	return &NanoBananaMediaAdapter{httpUpstream: httpUpstream}
}

func (a *NanoBananaMediaAdapter) Name() string {
	return nanoBananaMediaAdapterKey
}

func (a *NanoBananaMediaAdapter) Generate(ctx context.Context, req MediaExecutionRequest) (*MediaGenerateResult, error) {
	if ctx == nil {
		return nil, nanoBananaSystemError("media_adapter_invalid_request", "media generation context is unavailable", nil)
	}
	if a == nil || a.httpUpstream == nil {
		return nil, nanoBananaSystemError("media_adapter_unavailable", "nano banana adapter is unavailable", nil)
	}

	payload, err := buildNanoBananaRequest(req)
	if err != nil {
		return nil, err
	}
	endpoint, apiKey, err := nanoBananaEndpoint(req)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nanoBananaSystemError("media_adapter_request_encode_failed", "nano banana request cannot be encoded", err)
	}
	upstreamRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, nanoBananaSystemError("media_adapter_request_build_failed", "nano banana request cannot be built", err)
	}
	upstreamRequest.Header.Set("Content-Type", "application/json")
	upstreamRequest.Header.Set("x-goog-api-key", apiKey)

	response, err := a.httpUpstream.Do(
		upstreamRequest,
		req.Account.EffectiveProxyURL(),
		req.Account.ID,
		req.Account.Concurrency,
	)
	if err != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, classifyNanoBananaTransportError(err)
	}
	if response == nil || response.Body == nil {
		return nil, nanoBananaUnknownUpstreamError(
			"upstream_invalid_response",
			"nano banana upstream returned an invalid response",
			true,
			nil,
		)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, readErr := readNanoBananaBody(response.Body, nanoBananaMaxErrorBodyBytes)
		if readErr != nil {
			return nil, nanoBananaUpstreamError(
				"upstream_invalid_response",
				"nano banana upstream error response cannot be read",
				true,
				readErr,
			)
		}
		return nil, classifyNanoBananaHTTPError(response.StatusCode, body)
	}

	body, err := readNanoBananaBody(response.Body, nanoBananaMaxResponseBytes)
	if err != nil {
		return nil, nanoBananaUnknownUpstreamError(
			"upstream_invalid_response",
			"nano banana upstream response cannot be read",
			true,
			err,
		)
	}
	result, err := decodeNanoBananaResponse(body, req.Spec.Image.Count)
	if err != nil {
		return nil, err
	}
	if result.Usage.ImageSize == "" && req.Spec.Image != nil {
		result.Usage.ImageSize = strings.TrimSpace(req.Spec.Image.Size)
	}
	return result, nil
}

func buildNanoBananaRequest(req MediaExecutionRequest) (map[string]any, error) {
	if req.Task == nil || req.Account == nil || req.Spec.Image == nil {
		return nil, nanoBananaSystemError("media_adapter_invalid_request", "nano banana request is incomplete", nil)
	}
	if err := req.Spec.Validate(MediaTypeImage); err != nil {
		return nil, nanoBananaSystemError("media_adapter_invalid_request", "nano banana image request is invalid", err)
	}
	if req.Spec.Image.Count != 1 {
		return nil, nanoBananaSystemError(
			"media_adapter_invalid_request",
			"nano banana currently supports exactly one generated image",
			nil,
		)
	}

	parts := []any{map[string]any{"text": strings.TrimSpace(req.Spec.Image.Prompt)}}
	switch req.Task.Operation {
	case MediaOperationTextToImage:
		if len(req.Inputs) != 0 || len(req.Spec.Image.InputArtifactIDs) != 0 {
			return nil, nanoBananaSystemError("media_adapter_invalid_request", "text-to-image does not accept input images", nil)
		}
	case MediaOperationImageEdit:
		if len(req.Spec.Image.InputArtifactIDs) != len(req.Inputs) {
			return nil, nanoBananaSystemError("media_adapter_invalid_input", "nano banana image edit inputs are incomplete", nil)
		}
		inputParts, err := buildNanoBananaInputParts(req.Inputs)
		if err != nil {
			return nil, err
		}
		parts = append(parts, inputParts...)
	default:
		return nil, nanoBananaSystemError(
			"media_adapter_unsupported_operation",
			"nano banana does not support the requested media operation",
			nil,
		)
	}

	imageConfig, err := nanoBananaImageConfig(req.Spec.Image.Size)
	if err != nil {
		return nil, nanoBananaSystemError("media_adapter_invalid_request", "nano banana image size is invalid", err)
	}
	generationConfig := map[string]any{
		"responseModalities": []any{"TEXT", "IMAGE"},
	}
	if len(imageConfig) != 0 {
		generationConfig["imageConfig"] = imageConfig
	}
	payload := map[string]any{
		"contents": []any{map[string]any{
			"role":  "user",
			"parts": parts,
		}},
		"generationConfig": generationConfig,
	}

	extensions, err := nanoBananaResolvedExtensions(req.ResolvedRequest)
	if err != nil {
		return nil, err
	}
	if err := mergeNanoBananaExtensions(payload, extensions); err != nil {
		return nil, err
	}
	return payload, nil
}

func buildNanoBananaInputParts(inputs []MediaArtifactInput) ([]any, error) {
	if len(inputs) == 0 || len(inputs) > MaxMediaReferenceInputs {
		return nil, nanoBananaSystemError("media_adapter_invalid_input", "nano banana image edit inputs are invalid", nil)
	}
	ordered := append([]MediaArtifactInput(nil), inputs...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	parts := make([]any, 0, len(ordered))
	totalBytes := 0
	previousPosition := -1
	for index := range ordered {
		input := ordered[index]
		if input.Position < 0 || input.Position == previousPosition ||
			(input.Direction != "" && input.Direction != "input") || input.MediaType != MediaTypeImage || len(input.Data) == 0 {
			return nil, nanoBananaSystemError("media_adapter_invalid_input", "nano banana image edit input is invalid", nil)
		}
		previousPosition = input.Position
		contentType, ok := normalizeNanoBananaImageContentType(input.ContentType)
		if !ok {
			return nil, nanoBananaSystemError("media_adapter_invalid_input", "nano banana image edit input type is unsupported", nil)
		}
		if !nanoBananaImageDataMatchesContentType(input.Data, contentType) {
			return nil, nanoBananaSystemError("media_adapter_invalid_input", "nano banana image edit input content does not match its type", nil)
		}
		totalBytes += len(input.Data)
		if totalBytes > nanoBananaMaxInlineInputBytes {
			return nil, nanoBananaSystemError("media_adapter_input_too_large", "nano banana image edit inputs exceed the upstream inline limit", nil)
		}
		parts = append(parts, map[string]any{
			"inlineData": map[string]any{
				"mimeType": contentType,
				"data":     base64.StdEncoding.EncodeToString(input.Data),
			},
		})
	}
	return parts, nil
}

func normalizeNanoBananaImageContentType(value string) (string, bool) {
	parsed, _, err := mime.ParseMediaType(strings.TrimSpace(value))
	if err != nil {
		return "", false
	}
	parsed = strings.ToLower(parsed)
	if parsed == "image/jpg" {
		parsed = "image/jpeg"
	}
	switch parsed {
	case "image/png", "image/jpeg", "image/webp":
		return parsed, true
	default:
		return "", false
	}
}

func nanoBananaImageDataMatchesContentType(data []byte, contentType string) bool {
	detected := strings.ToLower(strings.TrimSpace(strings.SplitN(http.DetectContentType(data), ";", 2)[0]))
	if detected == "image/jpg" {
		detected = "image/jpeg"
	}
	return detected == contentType
}

func nanoBananaImageConfig(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "auto") {
		return nil, nil
	}
	if imageSize, ok := normalizeNanoBananaImageSize(raw); ok {
		return map[string]any{"imageSize": imageSize}, nil
	}
	if aspectRatio, ok := normalizeNanoBananaAspectRatio(raw); ok {
		return map[string]any{"aspectRatio": aspectRatio}, nil
	}

	dimensions := strings.Split(strings.ToLower(raw), "x")
	if len(dimensions) != 2 {
		return nil, fmt.Errorf("unsupported size %q", raw)
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(dimensions[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(dimensions[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 || width > 4096 || height > 4096 {
		return nil, fmt.Errorf("unsupported size %q", raw)
	}
	divisor := nanoBananaGreatestCommonDivisor(width, height)
	aspectRatio, ok := normalizeNanoBananaAspectRatio(fmt.Sprintf("%d:%d", width/divisor, height/divisor))
	if !ok {
		return nil, fmt.Errorf("unsupported aspect ratio for size %q", raw)
	}
	imageSize := "1K"
	longest := max(width, height)
	if longest > 2048 {
		imageSize = "4K"
	} else if longest > 1024 {
		imageSize = "2K"
	}
	return map[string]any{"aspectRatio": aspectRatio, "imageSize": imageSize}, nil
}

func normalizeNanoBananaImageSize(raw string) (string, bool) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	switch value {
	case "1K", "2K", "4K":
		return value, true
	default:
		return "", false
	}
}

func normalizeNanoBananaAspectRatio(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	allowed := map[string]struct{}{
		"1:1": {}, "1:4": {}, "1:8": {}, "2:3": {}, "3:2": {}, "3:4": {}, "4:1": {},
		"4:3": {}, "4:5": {}, "5:4": {}, "8:1": {}, "9:16": {}, "16:9": {}, "21:9": {},
	}
	_, ok := allowed[value]
	return value, ok
}

func nanoBananaGreatestCommonDivisor(left, right int) int {
	for right != 0 {
		left, right = right, left%right
	}
	return left
}

func nanoBananaResolvedExtensions(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var envelope map[string]any
	if err := decoder.Decode(&envelope); err != nil {
		return nil, nanoBananaSystemError("media_adapter_mapping_invalid", "resolved nano banana request is invalid", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple top-level JSON values")
		}
		return nil, nanoBananaSystemError("media_adapter_mapping_invalid", "resolved nano banana request is invalid", err)
	}
	value, exists := envelope["image"]
	if !exists {
		return nil, nanoBananaSystemError("media_adapter_mapping_invalid", "resolved nano banana image request is missing", nil)
	}
	image, ok := value.(map[string]any)
	if !ok {
		return nil, nanoBananaSystemError("media_adapter_mapping_invalid", "resolved nano banana image request is invalid", nil)
	}
	extensions := cloneMediaMappingObject(image)
	for _, canonical := range []string{
		"prompt", "size", "quality", "output_format", "response_format", "n", "input_artifact_ids",
	} {
		delete(extensions, canonical)
	}
	return extensions, nil
}

func mergeNanoBananaExtensions(payload, extensions map[string]any) error {
	if len(extensions) == 0 {
		return nil
	}
	var generationConfigKeys []string
	for key := range extensions {
		normalizedKey := normalizeNanoBananaProtectedKey(key)
		switch normalizedKey {
		case "contents", "model", "apikey", "authorization", "headers", "url", "baseurl":
			return nanoBananaSystemError("media_adapter_mapping_forbidden", "resolved nano banana request overrides a protected field", nil)
		}
		if normalizedKey == "generationconfig" {
			generationConfigKeys = append(generationConfigKeys, key)
		}
	}
	if len(generationConfigKeys) > 1 {
		return nanoBananaSystemError("media_adapter_mapping_invalid", "resolved nano banana request contains ambiguous generationConfig extensions", nil)
	}
	if len(generationConfigKeys) == 1 {
		generationConfigKey := generationConfigKeys[0]
		generationConfig, ok := extensions[generationConfigKey].(map[string]any)
		if !ok {
			return nanoBananaSystemError("media_adapter_mapping_invalid", "nano banana generationConfig extension must be an object", nil)
		}
		for key := range generationConfig {
			if normalizeNanoBananaProtectedKey(key) == "responsemodalities" {
				return nanoBananaSystemError("media_adapter_mapping_forbidden", "resolved nano banana request overrides response modalities", nil)
			}
		}
		delete(extensions, generationConfigKey)
		targetGenerationConfig, ok := payload["generationConfig"].(map[string]any)
		if !ok {
			return nanoBananaSystemError("media_adapter_mapping_invalid", "nano banana generationConfig payload is invalid", nil)
		}
		deepMergeNanoBananaObject(targetGenerationConfig, generationConfig)
	}
	deepMergeNanoBananaObject(payload, extensions)
	return nil
}

func normalizeNanoBananaProtectedKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "")
	return replacer.Replace(value)
}

func deepMergeNanoBananaObject(target, source map[string]any) {
	for key, sourceValue := range source {
		sourceObject, sourceIsObject := sourceValue.(map[string]any)
		targetObject, targetIsObject := target[key].(map[string]any)
		if sourceIsObject && targetIsObject {
			deepMergeNanoBananaObject(targetObject, sourceObject)
			continue
		}
		target[key] = cloneMediaMappingValue(sourceValue)
	}
}

func nanoBananaEndpoint(req MediaExecutionRequest) (string, string, error) {
	if req.Account == nil {
		return "", "", nanoBananaSystemError("media_adapter_invalid_request", "nano banana account is unavailable", nil)
	}
	apiKey := strings.TrimSpace(req.Account.GetCredential("api_key"))
	baseURL := strings.TrimSpace(req.Account.GetCredential("base_url"))
	upstreamModel := strings.TrimSpace(req.UpstreamModel)
	if apiKey == "" || baseURL == "" || upstreamModel == "" {
		return "", "", nanoBananaUpstreamError(
			"upstream_account_invalid",
			"nano banana upstream account is incomplete",
			true,
			nil,
		)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed == nil || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", nanoBananaUpstreamError(
			"upstream_account_invalid",
			"nano banana upstream base URL is invalid",
			true,
			err,
		)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/v1beta/models/" + url.PathEscape(upstreamModel) + ":generateContent"
	return endpoint, apiKey, nil
}

type nanoBananaInlineData struct {
	MimeType      string `json:"mimeType"`
	MimeTypeSnake string `json:"mime_type"`
	Data          string `json:"data"`
}

type nanoBananaResponsePart struct {
	InlineData      *nanoBananaInlineData `json:"inlineData"`
	InlineDataSnake *nanoBananaInlineData `json:"inline_data"`
}

type nanoBananaTokenDetail struct {
	Modality        string `json:"modality"`
	TokenCount      int    `json:"tokenCount"`
	TokenCountSnake int    `json:"token_count"`
}

type nanoBananaResponse struct {
	Candidates []struct {
		Content struct {
			Parts []nanoBananaResponsePart `json:"parts"`
		} `json:"content"`
		FinishReason      string `json:"finishReason"`
		FinishReasonSnake string `json:"finish_reason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason      string `json:"blockReason"`
		BlockReasonSnake string `json:"block_reason"`
	} `json:"promptFeedback"`
	PromptFeedbackSnake struct {
		BlockReason      string `json:"blockReason"`
		BlockReasonSnake string `json:"block_reason"`
	} `json:"prompt_feedback"`
	UsageMetadata struct {
		CandidatesTokenCount       int                     `json:"candidatesTokenCount"`
		CandidatesTokenCountSnake  int                     `json:"candidates_token_count"`
		CandidatesTokensDetails    []nanoBananaTokenDetail `json:"candidatesTokensDetails"`
		CandidatesTokensDetailsAlt []nanoBananaTokenDetail `json:"candidates_tokens_details"`
	} `json:"usageMetadata"`
	UsageMetadataSnake struct {
		CandidatesTokenCount       int                     `json:"candidatesTokenCount"`
		CandidatesTokenCountSnake  int                     `json:"candidates_token_count"`
		CandidatesTokensDetails    []nanoBananaTokenDetail `json:"candidatesTokensDetails"`
		CandidatesTokensDetailsAlt []nanoBananaTokenDetail `json:"candidates_tokens_details"`
	} `json:"usage_metadata"`
}

func decodeNanoBananaResponse(body []byte, maxOutputs int) (*MediaGenerateResult, error) {
	if maxOutputs <= 0 || maxOutputs > MaxMediaImageCount {
		return nil, nanoBananaSystemError(
			"media_adapter_invalid_request",
			"nano banana requested image count is invalid",
			nil,
		)
	}
	var decoded nanoBananaResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, nanoBananaUnknownUpstreamError(
			"upstream_invalid_response",
			"nano banana upstream returned malformed JSON",
			true,
			err,
		)
	}

	artifacts := make([]MediaArtifactInput, 0, 1)
	blockedReason := firstNonEmpty(
		decoded.PromptFeedback.BlockReason,
		decoded.PromptFeedback.BlockReasonSnake,
		decoded.PromptFeedbackSnake.BlockReason,
		decoded.PromptFeedbackSnake.BlockReasonSnake,
	)
	for candidateIndex := range decoded.Candidates {
		candidate := decoded.Candidates[candidateIndex]
		finishReason := firstNonEmpty(candidate.FinishReason, candidate.FinishReasonSnake)
		if isNanoBananaBlockedReason(finishReason) {
			blockedReason = finishReason
		}
		for partIndex := range candidate.Content.Parts {
			part := candidate.Content.Parts[partIndex]
			inlineData := part.InlineData
			if inlineData == nil {
				inlineData = part.InlineDataSnake
			}
			if inlineData == nil {
				continue
			}
			if len(artifacts) >= maxOutputs {
				return nil, nanoBananaUnknownUpstreamError(
					"upstream_invalid_response",
					"nano banana upstream returned too many images",
					true,
					nil,
				)
			}
			contentType, ok := normalizeNanoBananaImageContentType(firstNonEmpty(inlineData.MimeType, inlineData.MimeTypeSnake))
			if !ok {
				return nil, nanoBananaUnknownUpstreamError(
					"upstream_invalid_response",
					"nano banana upstream returned an unsupported image type",
					true,
					nil,
				)
			}
			data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(inlineData.Data))
			if err != nil || len(data) == 0 {
				return nil, nanoBananaUnknownUpstreamError(
					"upstream_invalid_response",
					"nano banana upstream returned invalid image data",
					true,
					err,
				)
			}
			if !nanoBananaImageDataMatchesContentType(data, contentType) {
				return nil, nanoBananaUnknownUpstreamError(
					"upstream_invalid_response",
					"nano banana upstream image content does not match its type",
					true,
					nil,
				)
			}
			artifacts = append(artifacts, MediaArtifactInput{
				Direction:   "output",
				Position:    len(artifacts),
				MediaType:   MediaTypeImage,
				ContentType: contentType,
				Data:        data,
				SizeBytes:   int64(len(data)),
			})
		}
	}
	if len(artifacts) == 0 {
		if blockedReason != "" {
			return nil, nanoBananaUpstreamError(
				"content_policy_violation",
				"nano banana rejected the request because of its content policy",
				false,
				nil,
			)
		}
		return nil, nanoBananaUnknownUpstreamError(
			"upstream_invalid_response",
			"nano banana upstream returned no image",
			true,
			nil,
		)
	}

	outputTokens := decoded.UsageMetadata.CandidatesTokenCount
	if outputTokens == 0 {
		outputTokens = decoded.UsageMetadata.CandidatesTokenCountSnake
	}
	usageDetails := decoded.UsageMetadata.CandidatesTokensDetails
	if len(usageDetails) == 0 {
		usageDetails = decoded.UsageMetadata.CandidatesTokensDetailsAlt
	}
	if outputTokens == 0 && len(usageDetails) == 0 {
		outputTokens = decoded.UsageMetadataSnake.CandidatesTokenCount
		if outputTokens == 0 {
			outputTokens = decoded.UsageMetadataSnake.CandidatesTokenCountSnake
		}
		usageDetails = decoded.UsageMetadataSnake.CandidatesTokensDetails
		if len(usageDetails) == 0 {
			usageDetails = decoded.UsageMetadataSnake.CandidatesTokensDetailsAlt
		}
	}
	if outputTokens == 0 {
		for _, detail := range usageDetails {
			if strings.EqualFold(strings.TrimSpace(detail.Modality), "IMAGE") {
				count := detail.TokenCount
				if count == 0 {
					count = detail.TokenCountSnake
				}
				outputTokens += count
			}
		}
	}

	return &MediaGenerateResult{
		Artifacts: artifacts,
		Usage: MediaUsage{
			ImageCount:   len(artifacts),
			OutputTokens: outputTokens,
		},
	}, nil
}

func isNanoBananaBlockedReason(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SAFETY", "IMAGE_SAFETY", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "RECITATION":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readNanoBananaBody(reader io.Reader, limit int64) ([]byte, error) {
	if reader == nil || limit <= 0 {
		return nil, errors.New("response body is unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("response body exceeds limit")
	}
	return body, nil
}

func classifyNanoBananaTransportError(err error) error {
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return nanoBananaUnknownUpstreamError("upstream_timeout", "nano banana upstream request timed out", true, err)
	}
	return nanoBananaUnknownUpstreamError("upstream_unavailable", "nano banana upstream request failed", true, err)
}

func classifyNanoBananaHTTPError(status int, body []byte) error {
	message := strings.ToLower(nanoBananaProviderErrorMessage(body))
	if strings.Contains(message, "safety") || strings.Contains(message, "content policy") ||
		strings.Contains(message, "prohibited content") || strings.Contains(message, "blocked") {
		return nanoBananaUpstreamError(
			"content_policy_violation",
			"nano banana rejected the request because of its content policy",
			false,
			nil,
		)
	}
	switch status {
	case http.StatusUnauthorized:
		return nanoBananaUpstreamError("upstream_authentication_failed", "nano banana upstream authentication failed", true, nil)
	case http.StatusForbidden:
		return nanoBananaUpstreamError("upstream_permission_denied", "nano banana upstream denied access", true, nil)
	case http.StatusRequestTimeout:
		return nanoBananaUpstreamError("upstream_timeout", "nano banana upstream request timed out", true, nil)
	case http.StatusTooManyRequests:
		return nanoBananaUpstreamError("upstream_rate_limited", "nano banana upstream is temporarily unavailable", true, nil)
	default:
		if status >= http.StatusInternalServerError {
			return nanoBananaUpstreamError("upstream_unavailable", "nano banana upstream is temporarily unavailable", true, nil)
		}
		return nanoBananaUpstreamError("upstream_bad_request", "nano banana upstream rejected the request", false, nil)
	}
}

func nanoBananaProviderErrorMessage(body []byte) string {
	var response struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &response) != nil {
		return ""
	}
	return firstNonEmpty(response.Error.Message, response.Message)
}

func nanoBananaSystemError(code, message string, cause error) error {
	return &MediaAdapterError{
		Code:          code,
		Message:       message,
		SystemFailure: true,
		Cause:         cause,
	}
}

func nanoBananaUpstreamError(code, message string, retryable bool, cause error) error {
	return &MediaAdapterError{
		Code:      code,
		Message:   message,
		Retryable: retryable,
		Cause:     cause,
	}
}

func nanoBananaUnknownUpstreamError(code, message string, retryable bool, cause error) error {
	return &MediaAdapterError{
		Code:              code,
		Message:           message,
		Retryable:         retryable,
		SubmissionUnknown: true,
		Cause:             cause,
	}
}

func nanoBananaMediaAdapterRegistration(adapter MediaAdapter) MediaAdapterRegistration {
	operations := []MediaOperation{MediaOperationTextToImage, MediaOperationImageEdit}
	rules := make([]MediaAdapterExactRule, 0, len(nanoBananaCanonicalModelIDs))
	for _, modelID := range nanoBananaCanonicalModelIDs {
		rules = append(rules, MediaAdapterExactRule{
			Vendor:  "google",
			ModelID: modelID,
			Capabilities: MediaAdapterRuleCapabilities{
				Operations:   append([]MediaOperation(nil), operations...),
				SyncUpstream: true,
			},
		})
	}
	return MediaAdapterRegistration{
		Key:                 nanoBananaMediaAdapterKey,
		Adapter:             adapter,
		SupportedOperations: operations,
		ExactRules:          rules,
	}
}

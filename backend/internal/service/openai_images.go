package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	openAIImagesGenerationsEndpoint = "/v1/images/generations"
	openAIImagesEditsEndpoint       = "/v1/images/edits"
	openAIImagesProxyEndpoint       = "/v1/images/proxy"

	openAIImagesGenerationsURL = "https://api.openai.com/v1/images/generations"
	openAIImagesEditsURL       = "https://api.openai.com/v1/images/edits"

	openAIChatGPTStartURL          = "https://chatgpt.com/"
	openAIChatGPTFilesURL          = "https://chatgpt.com/backend-api/files"
	openAIImageBackendUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	openAIImageMaxDownloadBytes    = 20 << 20 // 20MB per image download
	openAIImageMaxUploadPartSize   = 20 << 20 // 20MB per multipart upload part
	openAIImagesResponsesMainModel = "gpt-5.4-mini"
)

var ErrOpenAIImageCacheNotFound = errors.New("openai image cache not found")

type OpenAIImageCache interface {
	SetImage(ctx context.Context, id string, data []byte, contentType string, ttl time.Duration) error
	GetImage(ctx context.Context, id string) ([]byte, string, error)
}

type OpenAIImagesCapability string

const (
	OpenAIImagesCapabilityBasic  OpenAIImagesCapability = "images-basic"
	OpenAIImagesCapabilityNative OpenAIImagesCapability = "images-native"
)

type OpenAIImagesUpload struct {
	FieldName   string
	FileName    string
	ContentType string
	Data        []byte
	Width       int
	Height      int
}

type OpenAIImagesRequest struct {
	Endpoint           string
	ContentType        string
	Multipart          bool
	Model              string
	ExplicitModel      bool
	Prompt             string
	Stream             bool
	N                  int
	Size               string
	ExplicitSize       bool
	SizeTier           string
	ResponseFormat     string
	Quality            string
	Background         string
	OutputFormat       string
	Moderation         string
	InputFidelity      string
	Style              string
	OutputCompression  *int
	PartialImages      *int
	HasMask            bool
	HasNativeOptions   bool
	RequiredCapability OpenAIImagesCapability
	InputImageURLs     []string
	MaskImageURL       string
	Uploads            []OpenAIImagesUpload
	MaskUpload         *OpenAIImagesUpload
	Body               []byte
	bodyHash           string
}

func (r *OpenAIImagesRequest) IsEdits() bool {
	return r != nil && r.Endpoint == openAIImagesEditsEndpoint
}

func (r *OpenAIImagesRequest) StickySessionSeed() string {
	if r == nil {
		return ""
	}
	parts := []string{
		"openai-images",
		strings.TrimSpace(r.Endpoint),
		strings.TrimSpace(r.Model),
		strings.TrimSpace(r.Size),
		strings.TrimSpace(r.Prompt),
	}
	seed := strings.Join(parts, "|")
	if strings.TrimSpace(r.Prompt) == "" && r.bodyHash != "" {
		seed += "|body=" + r.bodyHash
	}
	return seed
}

func (s *OpenAIGatewayService) ParseOpenAIImagesRequest(c *gin.Context, body []byte) (*OpenAIImagesRequest, error) {
	if c == nil || c.Request == nil {
		return nil, fmt.Errorf("missing request context")
	}
	endpoint := normalizeOpenAIImagesEndpointPath(c.Request.URL.Path)
	if endpoint == "" {
		return nil, fmt.Errorf("unsupported images endpoint")
	}

	contentType := strings.TrimSpace(c.GetHeader("Content-Type"))
	req := &OpenAIImagesRequest{
		Endpoint:    endpoint,
		ContentType: contentType,
		N:           1,
		Body:        body,
	}
	if len(body) > 0 {
		sum := sha256.Sum256(body)
		req.bodyHash = hex.EncodeToString(sum[:8])
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		req.Multipart = true
		if parseErr := parseOpenAIImagesMultipartRequest(body, contentType, req); parseErr != nil {
			return nil, parseErr
		}
	} else {
		if len(body) == 0 {
			return nil, fmt.Errorf("request body is empty")
		}
		if !gjson.ValidBytes(body) {
			return nil, fmt.Errorf("failed to parse request body")
		}
		if parseErr := parseOpenAIImagesJSONRequest(body, req); parseErr != nil {
			return nil, parseErr
		}
	}

	responseFormat, err := normalizeOpenAIImagesResponseFormat(req.ResponseFormat)
	if err != nil {
		return nil, err
	}
	req.ResponseFormat = responseFormat
	applyOpenAIImagesDefaults(req)
	if err := validateOpenAIImagesModel(req.Model); err != nil {
		return nil, err
	}
	req.SizeTier = normalizeOpenAIImageSizeTier(req.Size)
	req.RequiredCapability = classifyOpenAIImagesCapability(req)
	return req, nil
}

func parseOpenAIImagesJSONRequest(body []byte, req *OpenAIImagesRequest) error {
	if modelResult := gjson.GetBytes(body, "model"); modelResult.Exists() {
		req.Model = strings.TrimSpace(modelResult.String())
		req.ExplicitModel = req.Model != ""
	}
	req.Prompt = strings.TrimSpace(gjson.GetBytes(body, "prompt").String())

	if streamResult := gjson.GetBytes(body, "stream"); streamResult.Exists() {
		if streamResult.Type != gjson.True && streamResult.Type != gjson.False {
			return fmt.Errorf("invalid stream field type")
		}
		req.Stream = streamResult.Bool()
	}

	if nResult := gjson.GetBytes(body, "n"); nResult.Exists() {
		if nResult.Type != gjson.Number {
			return fmt.Errorf("invalid n field type")
		}
		req.N = int(nResult.Int())
		if req.N <= 0 {
			return fmt.Errorf("n must be greater than 0")
		}
	}

	if sizeResult := gjson.GetBytes(body, "size"); sizeResult.Exists() {
		req.Size = strings.TrimSpace(sizeResult.String())
		req.ExplicitSize = req.Size != ""
	}
	req.ResponseFormat = strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "response_format").String()))
	req.Quality = strings.TrimSpace(gjson.GetBytes(body, "quality").String())
	req.Background = strings.TrimSpace(gjson.GetBytes(body, "background").String())
	req.OutputFormat = strings.TrimSpace(gjson.GetBytes(body, "output_format").String())
	req.Moderation = strings.TrimSpace(gjson.GetBytes(body, "moderation").String())
	req.InputFidelity = strings.TrimSpace(gjson.GetBytes(body, "input_fidelity").String())
	req.Style = strings.TrimSpace(gjson.GetBytes(body, "style").String())
	req.HasMask = gjson.GetBytes(body, "mask").Exists()
	if outputCompression := gjson.GetBytes(body, "output_compression"); outputCompression.Exists() {
		if outputCompression.Type != gjson.Number {
			return fmt.Errorf("invalid output_compression field type")
		}
		v := int(outputCompression.Int())
		req.OutputCompression = &v
	}
	if partialImages := gjson.GetBytes(body, "partial_images"); partialImages.Exists() {
		if partialImages.Type != gjson.Number {
			return fmt.Errorf("invalid partial_images field type")
		}
		v := int(partialImages.Int())
		req.PartialImages = &v
	}
	if req.IsEdits() {
		images := gjson.GetBytes(body, "images")
		if images.Exists() {
			if !images.IsArray() {
				return fmt.Errorf("invalid images field type")
			}
			for _, item := range images.Array() {
				if imageURL := strings.TrimSpace(item.Get("image_url").String()); imageURL != "" {
					req.InputImageURLs = append(req.InputImageURLs, imageURL)
					continue
				}
				if item.Get("file_id").Exists() {
					return fmt.Errorf("images[].file_id is not supported (use images[].image_url instead)")
				}
			}
		}
		if maskImageURL := strings.TrimSpace(gjson.GetBytes(body, "mask.image_url").String()); maskImageURL != "" {
			req.MaskImageURL = maskImageURL
			req.HasMask = true
		}
		if gjson.GetBytes(body, "mask.file_id").Exists() {
			return fmt.Errorf("mask.file_id is not supported (use mask.image_url instead)")
		}
		if len(req.InputImageURLs) == 0 {
			return fmt.Errorf("images[].image_url is required")
		}
	}
	req.HasNativeOptions = hasOpenAINativeImageOptions(func(path string) bool {
		return gjson.GetBytes(body, path).Exists()
	})
	return nil
}

func parseOpenAIImagesMultipartRequest(body []byte, contentType string, req *OpenAIImagesRequest) error {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read multipart body: %w", err)
		}
		name := strings.TrimSpace(part.FormName())
		if name == "" {
			_ = part.Close()
			continue
		}

		data, err := io.ReadAll(io.LimitReader(part, openAIImageMaxUploadPartSize))
		_ = part.Close()
		if err != nil {
			return fmt.Errorf("read multipart field %s: %w", name, err)
		}

		fileName := strings.TrimSpace(part.FileName())
		if fileName != "" {
			partContentType := strings.TrimSpace(part.Header.Get("Content-Type"))
			if name == "mask" && len(data) > 0 {
				req.HasMask = true
				width, height := parseOpenAIImageDimensions(part.Header)
				maskUpload := OpenAIImagesUpload{
					FieldName:   name,
					FileName:    fileName,
					ContentType: partContentType,
					Data:        data,
					Width:       width,
					Height:      height,
				}
				req.MaskUpload = &maskUpload
			}
			if name == "image" || strings.HasPrefix(name, "image[") {
				width, height := parseOpenAIImageDimensions(part.Header)
				req.Uploads = append(req.Uploads, OpenAIImagesUpload{
					FieldName:   name,
					FileName:    fileName,
					ContentType: partContentType,
					Data:        data,
					Width:       width,
					Height:      height,
				})
			}
			continue
		}

		value := strings.TrimSpace(string(data))
		switch name {
		case "model":
			req.Model = value
			req.ExplicitModel = value != ""
		case "prompt":
			req.Prompt = value
		case "size":
			req.Size = value
			req.ExplicitSize = value != ""
		case "response_format":
			req.ResponseFormat = strings.ToLower(value)
		case "stream":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("invalid stream field value")
			}
			req.Stream = parsed
		case "n":
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return fmt.Errorf("n must be a positive integer")
			}
			req.N = n
		case "quality":
			req.Quality = value
			req.HasNativeOptions = true
		case "background":
			req.Background = value
			req.HasNativeOptions = true
		case "output_format":
			req.OutputFormat = value
			req.HasNativeOptions = true
		case "moderation":
			req.Moderation = value
			req.HasNativeOptions = true
		case "input_fidelity":
			req.InputFidelity = value
			req.HasNativeOptions = true
		case "style":
			req.Style = value
			req.HasNativeOptions = true
		case "output_compression":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid output_compression field value")
			}
			req.OutputCompression = &n
			req.HasNativeOptions = true
		case "partial_images":
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid partial_images field value")
			}
			req.PartialImages = &n
			req.HasNativeOptions = true
		default:
			if isOpenAINativeImageOption(name) && value != "" {
				req.HasNativeOptions = true
			}
		}
	}

	if len(req.Uploads) == 0 && req.IsEdits() {
		return fmt.Errorf("image file is required")
	}
	return nil
}

func parseOpenAIImageDimensions(_ textproto.MIMEHeader) (int, int) {
	return 0, 0
}

func applyOpenAIImagesDefaults(req *OpenAIImagesRequest) {
	if req == nil {
		return
	}
	if req.N <= 0 {
		req.N = 1
	}
	if strings.TrimSpace(req.Model) != "" {
		req.Model = strings.TrimSpace(req.Model)
		return
	}
	req.Model = "gpt-image-2"
}

func isOpenAIImageGenerationModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "gpt-image-")
}

func validateOpenAIImagesModel(model string) error {
	model = strings.TrimSpace(model)
	if isOpenAIImageGenerationModel(model) {
		return nil
	}
	if model == "" {
		return fmt.Errorf("images endpoint requires an image model")
	}
	return fmt.Errorf("images endpoint requires an image model, got %q", model)
}

func normalizeOpenAIImagesEndpointPath(path string) string {
	trimmed := strings.TrimSpace(path)
	switch {
	case strings.Contains(trimmed, "/images/generations"):
		return openAIImagesGenerationsEndpoint
	case strings.Contains(trimmed, "/images/edits"):
		return openAIImagesEditsEndpoint
	default:
		return ""
	}
}

func classifyOpenAIImagesCapability(req *OpenAIImagesRequest) OpenAIImagesCapability {
	if req == nil {
		return OpenAIImagesCapabilityNative
	}
	if req.ExplicitModel || req.ExplicitSize {
		return OpenAIImagesCapabilityNative
	}
	model := strings.ToLower(strings.TrimSpace(req.Model))
	if !strings.HasPrefix(model, "gpt-image-") {
		return OpenAIImagesCapabilityNative
	}
	if req.Stream || req.N != 1 || req.HasMask || req.HasNativeOptions {
		return OpenAIImagesCapabilityNative
	}
	if req.IsEdits() && !req.Multipart {
		return OpenAIImagesCapabilityNative
	}
	return OpenAIImagesCapabilityBasic
}

func normalizeOpenAIImagesResponseFormat(format string) (string, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "b64_json", nil
	}
	switch format {
	case "b64_json", "url":
		return format, nil
	default:
		return "", fmt.Errorf("response_format must be one of: b64_json, url")
	}
}

func hasOpenAINativeImageOptions(exists func(path string) bool) bool {
	for _, path := range []string{
		"background",
		"quality",
		"style",
		"output_format",
		"output_compression",
		"moderation",
		"input_fidelity",
		"partial_images",
	} {
		if exists(path) {
			return true
		}
	}
	return false
}

func isOpenAINativeImageOption(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "background", "quality", "style", "output_format", "output_compression", "moderation", "input_fidelity", "partial_images":
		return true
	default:
		return false
	}
}

func normalizeOpenAIImageSizeTier(size string) string {
	trimmed := strings.TrimSpace(size)
	switch strings.ToLower(trimmed) {
	case "1k":
		return "1K"
	case "2k":
		return "2K"
	case "4k":
		return "4K"
	case "", "auto":
		return "2K"
	}

	width, height, ok := parseOpenAIImageSize(trimmed)
	if !ok {
		return "2K"
	}
	maxEdge := width
	if height > maxEdge {
		maxEdge = height
	}
	switch {
	case maxEdge <= 1024:
		return "1K"
	case maxEdge <= 2048:
		return "2K"
	default:
		return "4K"
	}
}

func parseOpenAIImageSize(size string) (int, int, bool) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(size)), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func (s *OpenAIGatewayService) ForwardImages(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
	recordMeta ...*GeneratedImageRecordContext,
) (*OpenAIForwardResult, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parsed images request is required")
	}
	switch account.Type {
	case AccountTypeAPIKey:
		return s.forwardOpenAIImagesAPIKey(ctx, c, account, body, parsed, channelMappedModel, firstGeneratedImageRecordContext(recordMeta))
	case AccountTypeOAuth:
		return s.forwardOpenAIImagesOAuth(ctx, c, account, parsed, channelMappedModel, firstGeneratedImageRecordContext(recordMeta))
	default:
		return nil, fmt.Errorf("unsupported account type: %s", account.Type)
	}
}

func (s *OpenAIGatewayService) forwardOpenAIImagesAPIKey(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
	recordMeta *GeneratedImageRecordContext,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if err := validateOpenAIImagesModel(requestModel); err != nil {
		return nil, err
	}
	upstreamModel := account.GetMappedModel(requestModel)
	if err := validateOpenAIImagesModel(upstreamModel); err != nil {
		return nil, err
	}
	logger.LegacyPrintf(
		"service.openai_gateway",
		"[OpenAI] Images request routing request_model=%s upstream_model=%s endpoint=%s account_type=%s",
		strings.TrimSpace(parsed.Model),
		upstreamModel,
		parsed.Endpoint,
		account.Type,
	)
	forwardBody, forwardContentType, err := rewriteOpenAIImagesModel(body, parsed.ContentType, upstreamModel)
	if err != nil {
		return nil, err
	}
	if !parsed.Multipart {
		setOpsUpstreamRequestBody(c, forwardBody)
	}

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	upstreamReq, err := s.buildOpenAIImagesRequest(ctx, c, account, forwardBody, forwardContentType, token, parsed.Endpoint)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstreamStart := time.Now()
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
			Kind:               "request_error",
			Message:            safeErr,
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				UpstreamURL:        safeUpstreamURL(upstreamReq.URL.String()),
				Kind:               "failover",
				Message:            upstreamMsg,
			})
			s.handleFailoverSideEffects(ctx, resp, account, respBody)
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && isPoolModeRetryableStatus(resp.StatusCode),
			}
		}
		return s.handleErrorResponse(ctx, resp, c, account, forwardBody)
	}
	defer func() { _ = resp.Body.Close() }()

	var usage OpenAIUsage
	imageCount := parsed.N
	var firstTokenMs *int
	responseURLMode := account.GetOpenAIImageResponseURLMode()
	recordCtx := normalizeGeneratedImageRecordContext(ctx, recordMeta, account, parsed, requestModel, resp.Header.Get("x-request-id"))
	if parsed.Stream {
		streamUsage, streamCount, ttft, err := s.handleOpenAIImagesStreamingResponse(resp, c, startTime, parsed.ResponseFormat, responseURLMode, recordCtx)
		if err != nil {
			return nil, err
		}
		usage = streamUsage
		imageCount = streamCount
		firstTokenMs = ttft
	} else {
		nonStreamUsage, nonStreamCount, err := s.handleOpenAIImagesNonStreamingResponse(resp, c, parsed.ResponseFormat, responseURLMode, recordCtx)
		if err != nil {
			return nil, err
		}
		usage = nonStreamUsage
		if nonStreamCount > 0 {
			imageCount = nonStreamCount
		}
	}
	return &OpenAIForwardResult{
		RequestID:       resp.Header.Get("x-request-id"),
		Usage:           usage,
		Model:           requestModel,
		UpstreamModel:   upstreamModel,
		Stream:          parsed.Stream,
		ResponseHeaders: resp.Header.Clone(),
		Duration:        time.Since(startTime),
		FirstTokenMs:    firstTokenMs,
		ImageCount:      imageCount,
		ImageSize:       parsed.SizeTier,
	}, nil
}

func (s *OpenAIGatewayService) buildOpenAIImagesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	contentType string,
	token string,
	endpoint string,
) (*http.Request, error) {
	targetURL := openAIImagesGenerationsURL
	if endpoint == openAIImagesEditsEndpoint {
		targetURL = openAIImagesEditsURL
	}
	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" && account.Platform == PlatformCodex2API {
		return nil, errors.New("base_url is required for codex2api accounts")
	}
	if baseURL != "" {
		validatedURL, err := s.validateUpstreamBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
		targetURL = buildOpenAIImagesURL(validatedURL, endpoint)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	for key, values := range c.Request.Header {
		if !openaiPassthroughAllowedHeaders[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("User-Agent", customUA)
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func buildOpenAIImagesURL(base string, endpoint string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	relative := strings.TrimPrefix(strings.TrimSpace(endpoint), "/v1")
	if strings.HasSuffix(normalized, endpoint) || strings.HasSuffix(normalized, relative) {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + relative
	}
	return normalized + endpoint
}

func rewriteOpenAIImagesModel(body []byte, contentType string, model string) ([]byte, string, error) {
	model = strings.TrimSpace(model)
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && strings.EqualFold(mediaType, "multipart/form-data") {
		rewrittenBody, rewrittenType, rewriteErr := rewriteOpenAIImagesMultipartModel(body, contentType, model)
		return rewrittenBody, rewrittenType, rewriteErr
	}
	rewritten := body
	if gjson.GetBytes(rewritten, "response_format").Exists() {
		rewritten, err = sjson.DeleteBytes(rewritten, "response_format")
		if err != nil {
			return nil, "", fmt.Errorf("remove image response_format: %w", err)
		}
	}
	if model != "" {
		rewritten, err = sjson.SetBytes(rewritten, "model", model)
		if err != nil {
			return nil, "", fmt.Errorf("rewrite image request model: %w", err)
		}
	}
	return rewritten, contentType, nil
}

func rewriteOpenAIImagesMultipartModel(body []byte, contentType string, model string) ([]byte, string, error) {
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", fmt.Errorf("parse multipart content-type: %w", err)
	}
	boundary := strings.TrimSpace(params["boundary"])
	if boundary == "" {
		return nil, "", fmt.Errorf("multipart boundary is required")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	modelWritten := false

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("read multipart body: %w", err)
		}

		formName := strings.TrimSpace(part.FormName())
		if formName == "response_format" && part.FileName() == "" {
			_ = part.Close()
			continue
		}
		partHeader := cloneMultipartHeader(part.Header)
		target, err := writer.CreatePart(partHeader)
		if err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("create multipart part: %w", err)
		}

		if formName == "model" && part.FileName() == "" && model != "" {
			if _, err := target.Write([]byte(model)); err != nil {
				_ = part.Close()
				return nil, "", fmt.Errorf("rewrite multipart model: %w", err)
			}
			modelWritten = true
			_ = part.Close()
			continue
		}
		if _, err := io.Copy(target, part); err != nil {
			_ = part.Close()
			return nil, "", fmt.Errorf("copy multipart part: %w", err)
		}
		_ = part.Close()
	}

	if !modelWritten && model != "" {
		if err := writer.WriteField("model", model); err != nil {
			return nil, "", fmt.Errorf("append multipart model field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("finalize multipart body: %w", err)
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

func cloneMultipartHeader(src textproto.MIMEHeader) textproto.MIMEHeader {
	dst := make(textproto.MIMEHeader, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func (s *OpenAIGatewayService) handleOpenAIImagesNonStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	responseFormat string,
	responseURLMode string,
	recordCtx GeneratedImageRecordContext,
) (OpenAIUsage, int, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	body, err = s.transformOpenAIImagesAPIKeyResponse(c.Request.Context(), c, body, responseFormat, responseURLMode, recordCtx)
	if err != nil {
		return OpenAIUsage{}, 0, err
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := "application/json"
	if s.cfg != nil && !s.cfg.Security.ResponseHeaders.Enabled {
		if upstreamType := resp.Header.Get("Content-Type"); upstreamType != "" {
			contentType = upstreamType
		}
	}
	c.Data(resp.StatusCode, contentType, body)

	usage, _ := extractOpenAIUsageFromJSONBytes(body)
	return usage, extractOpenAIImageCountFromJSONBytes(body), nil
}

func (s *OpenAIGatewayService) transformOpenAIImagesAPIKeyResponse(
	ctx context.Context,
	c *gin.Context,
	body []byte,
	responseFormat string,
	responseURLMode string,
	recordCtx GeneratedImageRecordContext,
) ([]byte, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, nil
	}
	format, err := normalizeOpenAIImagesResponseFormat(responseFormat)
	if err != nil {
		return nil, err
	}
	data := gjson.GetBytes(body, "data")
	if !data.Exists() || !data.IsArray() {
		return body, nil
	}

	out := body
	rootOutputFormat := strings.TrimSpace(gjson.GetBytes(body, "output_format").String())
	rootModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	for i, item := range data.Array() {
		basePath := fmt.Sprintf("data.%d", i)
		b64Value := normalizeOpenAIImageBase64(item.Get("b64_json").String())
		urlValue := strings.TrimSpace(item.Get("url").String())
		outputFormat := strings.TrimSpace(item.Get("output_format").String())
		if outputFormat == "" {
			outputFormat = rootOutputFormat
		}
		itemRecordCtx := recordCtx
		if itemModel := strings.TrimSpace(item.Get("model").String()); itemModel != "" {
			itemRecordCtx.Model = itemModel
		} else if rootModel != "" {
			itemRecordCtx.Model = rootModel
		}
		revisedPrompt := strings.TrimSpace(item.Get("revised_prompt").String())

		switch format {
		case "url":
			if urlValue != "" {
				imageBytes, resolvedContentType, err := s.resolveOpenAIImagesResponseImageBytes(ctx, c, urlValue, "")
				if err != nil {
					return nil, err
				}
				renderedURL, objectUploadHandled, err := s.renderOpenAIImagesResolvedResponseURL(ctx, c, imageBytes, resolvedContentType, responseURLMode)
				if err != nil {
					return nil, err
				}
				s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
					Meta:                itemRecordCtx,
					ImageData:           imageBytes,
					ContentType:         resolvedContentType,
					OutputFormat:        outputFormat,
					Source:              GeneratedImageSourceUpstreamURL,
					RevisedPrompt:       revisedPrompt,
					ObjectUploadHandled: objectUploadHandled,
				})
				out, _ = sjson.SetBytes(out, basePath+".url", renderedURL)
				out, _ = sjson.DeleteBytes(out, basePath+".b64_json")
				continue
			}
			if b64Value != "" {
				contentType := openAIImageOutputMIMEType(outputFormat)
				imageBytes, resolvedContentType, err := s.resolveOpenAIImagesResponseImageBytes(ctx, c, b64Value, contentType)
				if err != nil {
					return nil, err
				}
				renderedURL, objectUploadHandled, err := s.renderOpenAIImagesResolvedResponseURL(ctx, c, imageBytes, resolvedContentType, responseURLMode)
				if err != nil {
					return nil, err
				}
				s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
					Meta:                itemRecordCtx,
					ImageData:           imageBytes,
					ContentType:         resolvedContentType,
					OutputFormat:        outputFormat,
					Source:              GeneratedImageSourceB64JSON,
					RevisedPrompt:       revisedPrompt,
					ObjectUploadHandled: objectUploadHandled,
				})
				out, _ = sjson.SetBytes(out, basePath+".url", renderedURL)
				out, _ = sjson.DeleteBytes(out, basePath+".b64_json")
			}
		default:
			if b64Value != "" {
				s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
					Meta:          itemRecordCtx,
					Value:         b64Value,
					ContentType:   openAIImageOutputMIMEType(outputFormat),
					OutputFormat:  outputFormat,
					Source:        GeneratedImageSourceB64JSON,
					RevisedPrompt: revisedPrompt,
				})
				out, _ = sjson.SetBytes(out, basePath+".b64_json", b64Value)
				out, _ = sjson.DeleteBytes(out, basePath+".url")
				continue
			}
			if urlValue == "" {
				continue
			}
			imageBytes, contentType, err := s.downloadOpenAIImagesExternalURL(ctx, c.Request.Header, urlValue)
			if err != nil {
				return nil, fmt.Errorf("download upstream image url: %w", err)
			}
			s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
				Meta:          itemRecordCtx,
				ImageData:     imageBytes,
				ContentType:   contentType,
				OutputFormat:  outputFormat,
				Source:        GeneratedImageSourceUpstreamURL,
				RevisedPrompt: revisedPrompt,
			})
			out, _ = sjson.SetBytes(out, basePath+".b64_json", base64.StdEncoding.EncodeToString(imageBytes))
			out, _ = sjson.DeleteBytes(out, basePath+".url")
		}
	}
	return out, nil
}

func (s *OpenAIGatewayService) handleOpenAIImagesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	startTime time.Time,
	responseFormat string,
	responseURLMode string,
	recordCtx GeneratedImageRecordContext,
) (OpenAIUsage, int, *int, error) {
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "text/event-stream"
	}
	c.Status(resp.StatusCode)
	c.Header("Content-Type", contentType)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return OpenAIUsage{}, 0, nil, fmt.Errorf("streaming is not supported by response writer")
	}

	reader := bufio.NewReader(resp.Body)
	usage := OpenAIUsage{}
	imageCount := 0
	var firstTokenMs *int

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineToWrite := line
			usageLine := line
			if data, ok := extractOpenAISSEDataLine(strings.TrimRight(string(line), "\r\n")); ok && data != "" && data != "[DONE]" {
				transformedData, transformErr := s.transformOpenAIImagesAPIKeyStreamPayload(c.Request.Context(), c, []byte(data), responseFormat, responseURLMode, recordCtx)
				if transformErr != nil {
					return OpenAIUsage{}, 0, firstTokenMs, transformErr
				}
				if !bytes.Equal(transformedData, []byte(data)) {
					lineToWrite = []byte("data: " + string(transformedData) + openAISSELineEnding(line))
					usageLine = lineToWrite
				}
			}
			if firstTokenMs == nil {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			if _, writeErr := c.Writer.Write(lineToWrite); writeErr != nil {
				return OpenAIUsage{}, 0, firstTokenMs, writeErr
			}
			flusher.Flush()

			if data, ok := extractOpenAISSEDataLine(strings.TrimRight(string(usageLine), "\r\n")); ok && data != "" && data != "[DONE]" {
				dataBytes := []byte(data)
				mergeOpenAIUsage(&usage, dataBytes)
				if count := extractOpenAIImageCountFromJSONBytes(dataBytes); count > imageCount {
					imageCount = count
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return OpenAIUsage{}, 0, firstTokenMs, err
		}
	}
	return usage, imageCount, firstTokenMs, nil
}

func openAISSELineEnding(line []byte) string {
	switch {
	case bytes.HasSuffix(line, []byte("\r\n")):
		return "\r\n"
	case bytes.HasSuffix(line, []byte("\n")):
		return "\n"
	default:
		return "\n"
	}
}

func (s *OpenAIGatewayService) transformOpenAIImagesAPIKeyStreamPayload(
	ctx context.Context,
	c *gin.Context,
	payload []byte,
	responseFormat string,
	responseURLMode string,
	recordCtx GeneratedImageRecordContext,
) ([]byte, error) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload, nil
	}
	if data := gjson.GetBytes(payload, "data"); data.Exists() && data.IsArray() {
		return s.transformOpenAIImagesAPIKeyResponse(ctx, c, payload, responseFormat, responseURLMode, recordCtx)
	}
	format, err := normalizeOpenAIImagesResponseFormat(responseFormat)
	if err != nil {
		return nil, err
	}
	b64Value := normalizeOpenAIImageBase64(gjson.GetBytes(payload, "b64_json").String())
	urlValue := strings.TrimSpace(gjson.GetBytes(payload, "url").String())
	if b64Value == "" && urlValue == "" {
		return payload, nil
	}

	out := payload
	outputFormat := strings.TrimSpace(gjson.GetBytes(payload, "output_format").String())
	revisedPrompt := strings.TrimSpace(gjson.GetBytes(payload, "revised_prompt").String())
	if itemModel := strings.TrimSpace(gjson.GetBytes(payload, "model").String()); itemModel != "" {
		recordCtx.Model = itemModel
	}
	switch format {
	case "url":
		if urlValue != "" {
			imageBytes, resolvedContentType, err := s.resolveOpenAIImagesResponseImageBytes(ctx, c, urlValue, "")
			if err != nil {
				return nil, err
			}
			renderedURL, objectUploadHandled, err := s.renderOpenAIImagesResolvedResponseURL(ctx, c, imageBytes, resolvedContentType, responseURLMode)
			if err != nil {
				return nil, err
			}
			s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
				Meta:                recordCtx,
				ImageData:           imageBytes,
				ContentType:         resolvedContentType,
				OutputFormat:        outputFormat,
				Source:              GeneratedImageSourceUpstreamURL,
				RevisedPrompt:       revisedPrompt,
				ObjectUploadHandled: objectUploadHandled,
			})
			out, _ = sjson.SetBytes(out, "url", renderedURL)
			out, _ = sjson.DeleteBytes(out, "b64_json")
			return out, nil
		}
		contentType := openAIImageOutputMIMEType(outputFormat)
		imageBytes, resolvedContentType, err := s.resolveOpenAIImagesResponseImageBytes(ctx, c, b64Value, contentType)
		if err != nil {
			return nil, err
		}
		renderedURL, objectUploadHandled, err := s.renderOpenAIImagesResolvedResponseURL(ctx, c, imageBytes, resolvedContentType, responseURLMode)
		if err != nil {
			return nil, err
		}
		s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
			Meta:                recordCtx,
			ImageData:           imageBytes,
			ContentType:         resolvedContentType,
			OutputFormat:        outputFormat,
			Source:              GeneratedImageSourceB64JSON,
			RevisedPrompt:       revisedPrompt,
			ObjectUploadHandled: objectUploadHandled,
		})
		out, _ = sjson.SetBytes(out, "url", renderedURL)
		out, _ = sjson.DeleteBytes(out, "b64_json")
	default:
		if b64Value != "" {
			s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
				Meta:          recordCtx,
				Value:         b64Value,
				ContentType:   openAIImageOutputMIMEType(outputFormat),
				OutputFormat:  outputFormat,
				Source:        GeneratedImageSourceB64JSON,
				RevisedPrompt: revisedPrompt,
			})
			out, _ = sjson.SetBytes(out, "b64_json", b64Value)
			out, _ = sjson.DeleteBytes(out, "url")
			return out, nil
		}
		imageBytes, contentType, err := s.downloadOpenAIImagesExternalURL(ctx, c.Request.Header, urlValue)
		if err != nil {
			return nil, fmt.Errorf("download upstream image url: %w", err)
		}
		s.recordGeneratedImageBestEffort(ctx, GeneratedImageRecordInput{
			Meta:          recordCtx,
			ImageData:     imageBytes,
			ContentType:   contentType,
			OutputFormat:  outputFormat,
			Source:        GeneratedImageSourceUpstreamURL,
			RevisedPrompt: revisedPrompt,
		})
		out, _ = sjson.SetBytes(out, "b64_json", base64.StdEncoding.EncodeToString(imageBytes))
		out, _ = sjson.DeleteBytes(out, "url")
	}
	return out, nil
}

func (s *OpenAIGatewayService) renderOpenAIImagesResponseURL(
	ctx context.Context,
	c *gin.Context,
	value string,
	contentType string,
	responseURLMode string,
) (string, bool, error) {
	imageBytes, resolvedContentType, err := s.resolveOpenAIImagesResponseImageBytes(ctx, c, value, contentType)
	if err != nil {
		return "", false, err
	}
	return s.renderOpenAIImagesResolvedResponseURL(ctx, c, imageBytes, resolvedContentType, responseURLMode)
}

func (s *OpenAIGatewayService) renderOpenAIImagesResolvedResponseURL(
	ctx context.Context,
	c *gin.Context,
	imageBytes []byte,
	resolvedContentType string,
	responseURLMode string,
) (string, bool, error) {
	mode := normalizeOpenAIImageResponseURLMode(responseURLMode)
	if resolvedContentType == "" {
		resolvedContentType = "image/png"
	}
	if mode == OpenAIImageResponseURLModeHTTPURL {
		if url, objectUploadHandled, ok := s.uploadGeneratedImageObjectBestEffort(ctx, imageBytes, resolvedContentType); ok {
			return url, objectUploadHandled, nil
		} else if objectUploadHandled {
			url, err := s.cacheOpenAIImagesResponseURL(ctx, c, imageBytes, resolvedContentType)
			return url, true, err
		}
		url, err := s.cacheOpenAIImagesResponseURL(ctx, c, imageBytes, resolvedContentType)
		return url, false, err
	}
	return "data:" + resolvedContentType + ";base64," + base64.StdEncoding.EncodeToString(imageBytes), false, nil
}

func (s *OpenAIGatewayService) resolveOpenAIImagesResponseImageBytes(
	ctx context.Context,
	c *gin.Context,
	value string,
	contentType string,
) ([]byte, string, error) {
	value = strings.TrimSpace(value)
	contentType = strings.TrimSpace(contentType)
	if normalized := normalizeOpenAIImageBase64(value); normalized != "" {
		imageBytes, err := base64.StdEncoding.DecodeString(normalized)
		if err != nil {
			return nil, "", err
		}
		if detected := dataURLContentType(value); detected != "" {
			contentType = detected
		}
		return imageBytes, contentType, nil
	}
	if value == "" {
		return nil, "", fmt.Errorf("image response is missing url and b64_json")
	}
	headers := http.Header{}
	if c != nil && c.Request != nil {
		headers = c.Request.Header
	}
	imageBytes, downloadedContentType, err := s.downloadOpenAIImagesExternalURL(ctx, headers, value)
	if err != nil {
		return nil, "", fmt.Errorf("download upstream image url: %w", err)
	}
	if strings.TrimSpace(downloadedContentType) != "" {
		contentType = strings.TrimSpace(downloadedContentType)
	}
	return imageBytes, contentType, nil
}

func (s *OpenAIGatewayService) cacheOpenAIImagesResponseURL(
	ctx context.Context,
	c *gin.Context,
	imageBytes []byte,
	contentType string,
) (string, error) {
	if s == nil || s.openAIImageCache == nil {
		return "", fmt.Errorf("openai image cache is not configured")
	}
	imageID, err := newOpenAIImagesCacheID()
	if err != nil {
		return "", err
	}
	ttl := s.openAIImagesURLCacheTTL(ctx)
	if err := s.openAIImageCache.SetImage(ctx, imageID, imageBytes, contentType, ttl); err != nil {
		return "", err
	}
	return s.buildOpenAIImagesProxyURL(c, imageID, ttl)
}

func (s *OpenAIGatewayService) openAIImagesURLCacheTTL(ctx context.Context) time.Duration {
	if s != nil && s.settingService != nil {
		return s.settingService.GetOpenAIImageURLCacheTTL(ctx)
	}
	return time.Duration(DefaultOpenAIImageURLCacheTTLHours) * time.Hour
}

func newOpenAIImagesCacheID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate image cache id: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func mergeOpenAIUsage(dst *OpenAIUsage, body []byte) {
	if dst == nil {
		return
	}
	if parsed, ok := extractOpenAIUsageFromJSONBytes(body); ok {
		if parsed.InputTokens > 0 {
			dst.InputTokens = parsed.InputTokens
		}
		if parsed.OutputTokens > 0 {
			dst.OutputTokens = parsed.OutputTokens
		}
		if parsed.CacheReadInputTokens > 0 {
			dst.CacheReadInputTokens = parsed.CacheReadInputTokens
		}
		if parsed.ImageOutputTokens > 0 {
			dst.ImageOutputTokens = parsed.ImageOutputTokens
		}
	}
}

func extractOpenAIImageCountFromJSONBytes(body []byte) int {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return 0
	}
	data := gjson.GetBytes(body, "data")
	if data.Exists() && data.IsArray() {
		return len(data.Array())
	}
	return 0
}

type openAIImagePointerInfo struct {
	Pointer     string
	DownloadURL string
	B64JSON     string
	MimeType    string
	Prompt      string
}

func collectOpenAIImagePointers(body []byte) []openAIImagePointerInfo {
	if len(body) == 0 {
		return nil
	}
	prompt := ""
	for _, path := range []string{
		"message.metadata.dalle.prompt",
		"metadata.dalle.prompt",
		"revised_prompt",
	} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			prompt = value
			break
		}
	}
	matches := openAIImagePointerMatches(body)
	out := make([]openAIImagePointerInfo, 0, len(matches))
	for _, pointer := range matches {
		out = append(out, openAIImagePointerInfo{Pointer: pointer, Prompt: prompt})
	}
	return mergeOpenAIImagePointerInfos(out, collectOpenAIImageInlineAssets(body, prompt))
}

func openAIImagePointerMatches(body []byte) []string {
	raw := string(body)
	matches := make([]string, 0, 4)
	for _, prefix := range []string{"file-service://", "sediment://"} {
		start := 0
		for {
			idx := strings.Index(raw[start:], prefix)
			if idx < 0 {
				break
			}
			idx += start
			end := idx + len(prefix)
			for end < len(raw) {
				ch := raw[end]
				if ch != '-' && ch != '_' &&
					(ch < '0' || ch > '9') &&
					(ch < 'a' || ch > 'z') &&
					(ch < 'A' || ch > 'Z') {
					break
				}
				end++
			}
			matches = append(matches, raw[idx:end])
			start = end
		}
	}
	return dedupeStrings(matches)
}

func mergeOpenAIImagePointerInfos(existing []openAIImagePointerInfo, next []openAIImagePointerInfo) []openAIImagePointerInfo {
	if len(next) == 0 {
		return existing
	}
	seen := make(map[string]openAIImagePointerInfo, len(existing)+len(next))
	out := make([]openAIImagePointerInfo, 0, len(existing)+len(next))
	for _, item := range existing {
		if key := item.identityKey(); key != "" {
			seen[key] = item
		}
		out = append(out, item)
	}
	for _, item := range next {
		key := item.identityKey()
		if key == "" {
			continue
		}
		if existingItem, ok := seen[key]; ok {
			merged := mergeOpenAIImagePointerInfo(existingItem, item)
			if merged != existingItem {
				for i := range out {
					if out[i].identityKey() == key {
						out[i] = merged
						break
					}
				}
				seen[key] = merged
			}
			continue
		}
		seen[key] = item
		out = append(out, item)
	}
	return out
}

func (i openAIImagePointerInfo) identityKey() string {
	switch {
	case strings.TrimSpace(i.Pointer) != "":
		return "pointer:" + strings.TrimSpace(i.Pointer)
	case strings.TrimSpace(i.DownloadURL) != "":
		return "download:" + strings.TrimSpace(i.DownloadURL)
	case strings.TrimSpace(i.B64JSON) != "":
		b64 := strings.TrimSpace(i.B64JSON)
		if len(b64) > 64 {
			b64 = b64[:64]
		}
		return "b64:" + b64
	default:
		return ""
	}
}

func mergeOpenAIImagePointerInfo(existing, next openAIImagePointerInfo) openAIImagePointerInfo {
	merged := existing
	if strings.TrimSpace(merged.Pointer) == "" {
		merged.Pointer = next.Pointer
	}
	if strings.TrimSpace(merged.DownloadURL) == "" {
		merged.DownloadURL = next.DownloadURL
	}
	if strings.TrimSpace(merged.B64JSON) == "" {
		merged.B64JSON = next.B64JSON
	}
	if strings.TrimSpace(merged.MimeType) == "" {
		merged.MimeType = next.MimeType
	}
	if strings.TrimSpace(merged.Prompt) == "" {
		merged.Prompt = next.Prompt
	}
	return merged
}

func resolveOpenAIImageBytes(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer openAIImagePointerInfo,
) ([]byte, error) {
	if normalized := normalizeOpenAIImageBase64(pointer.B64JSON); normalized != "" {
		return base64.StdEncoding.DecodeString(normalized)
	}
	if downloadURL := strings.TrimSpace(pointer.DownloadURL); downloadURL != "" {
		return downloadOpenAIImageBytes(ctx, client, headers, downloadURL)
	}
	if strings.TrimSpace(pointer.Pointer) == "" {
		return nil, fmt.Errorf("image asset is missing pointer, url, and base64 data")
	}
	downloadURL, err := fetchOpenAIImageDownloadURL(ctx, client, headers, conversationID, pointer.Pointer)
	if err != nil {
		return nil, err
	}
	return downloadOpenAIImageBytes(ctx, client, headers, downloadURL)
}

func normalizeOpenAIImageBase64(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		if idx := strings.Index(raw, ","); idx >= 0 && idx+1 < len(raw) {
			raw = raw[idx+1:]
		}
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "=")
	raw += strings.Repeat("=", (4-len(raw)%4)%4)
	if raw == "" {
		return ""
	}
	if _, err := base64.StdEncoding.DecodeString(raw); err != nil {
		return ""
	}
	return raw
}

func collectOpenAIImageInlineAssets(body []byte, fallbackPrompt string) []openAIImagePointerInfo {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil
	}
	var out []openAIImagePointerInfo
	walkOpenAIImageInlineAssets(decoded, strings.TrimSpace(fallbackPrompt), &out)
	return out
}

func walkOpenAIImageInlineAssets(node any, prompt string, out *[]openAIImagePointerInfo) {
	switch value := node.(type) {
	case map[string]any:
		localPrompt := prompt
		for _, key := range []string{"revised_prompt", "image_gen_title", "prompt"} {
			if v, ok := value[key].(string); ok && strings.TrimSpace(v) != "" {
				localPrompt = strings.TrimSpace(v)
				break
			}
		}
		item := openAIImagePointerInfo{
			Prompt:      localPrompt,
			Pointer:     firstNonEmptyString(value["asset_pointer"], value["pointer"]),
			DownloadURL: firstNonEmptyString(value["download_url"], value["url"], value["image_url"]),
			B64JSON:     firstNonEmptyString(value["b64_json"], value["base64"], value["image_base64"]),
			MimeType:    firstNonEmptyString(value["mime_type"], value["mimeType"], value["content_type"]),
		}
		switch {
		case strings.HasPrefix(strings.TrimSpace(item.Pointer), "file-service://"),
			strings.HasPrefix(strings.TrimSpace(item.Pointer), "sediment://"),
			isLikelyOpenAIImageDownloadURL(item.DownloadURL),
			normalizeOpenAIImageBase64(item.B64JSON) != "":
			*out = append(*out, item)
		}
		for _, child := range value {
			walkOpenAIImageInlineAssets(child, localPrompt, out)
		}
	case []any:
		for _, child := range value {
			walkOpenAIImageInlineAssets(child, prompt, out)
		}
	}
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func isLikelyOpenAIImageDownloadURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(raw), "data:image/") {
		return true
	}
	if !strings.HasPrefix(strings.ToLower(raw), "http://") && !strings.HasPrefix(strings.ToLower(raw), "https://") {
		return false
	}
	lower := strings.ToLower(raw)
	return strings.Contains(lower, "/download") ||
		strings.Contains(lower, ".png") ||
		strings.Contains(lower, ".jpg") ||
		strings.Contains(lower, ".jpeg") ||
		strings.Contains(lower, ".webp")
}

func fetchOpenAIImageDownloadURL(
	ctx context.Context,
	client *req.Client,
	headers http.Header,
	conversationID string,
	pointer string,
) (string, error) {
	url := ""
	allowConversationRetry := false
	switch {
	case strings.HasPrefix(pointer, "file-service://"):
		fileID := strings.TrimPrefix(pointer, "file-service://")
		url = fmt.Sprintf("%s/%s/download", openAIChatGPTFilesURL, fileID)
	case strings.HasPrefix(pointer, "sediment://"):
		attachmentID := strings.TrimPrefix(pointer, "sediment://")
		url = fmt.Sprintf("https://chatgpt.com/backend-api/conversation/%s/attachment/%s/download", conversationID, attachmentID)
		allowConversationRetry = true
	default:
		return "", fmt.Errorf("unsupported image pointer: %s", pointer)
	}

	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		var result struct {
			DownloadURL string `json:"download_url"`
		}
		resp, err := client.R().
			SetContext(ctx).
			SetHeaders(headerToMap(headers)).
			SetSuccessResult(&result).
			Get(url)
		if err != nil {
			lastErr = err
		} else if resp.IsSuccessState() && strings.TrimSpace(result.DownloadURL) != "" {
			return strings.TrimSpace(result.DownloadURL), nil
		} else {
			statusErr := newOpenAIImageStatusError(resp, "fetch image download url failed")
			if !allowConversationRetry || !isOpenAIImageTransientConversationNotFoundError(statusErr) {
				return "", statusErr
			}
			lastErr = statusErr
		}
		if attempt == 7 {
			break
		}
		timer := time.NewTimer(750 * time.Millisecond)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return "", ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("fetch image download url failed")
	}
	return "", lastErr
}

func downloadOpenAIImageBytes(ctx context.Context, client *req.Client, headers http.Header, downloadURL string) ([]byte, error) {
	request := client.R().
		SetContext(ctx).
		DisableAutoReadResponse()

	if strings.HasPrefix(downloadURL, openAIChatGPTStartURL) {
		downloadHeaders := cloneHTTPHeader(headers)
		downloadHeaders.Set("Accept", "image/*,*/*;q=0.8")
		downloadHeaders.Del("Content-Type")
		request.SetHeaders(headerToMap(downloadHeaders))
	} else {
		userAgent := strings.TrimSpace(headers.Get("User-Agent"))
		if userAgent == "" {
			userAgent = openAIImageBackendUserAgent
		}
		request.SetHeader("User-Agent", userAgent)
	}

	resp, err := request.Get(downloadURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newOpenAIImageStatusError(resp, "download image bytes failed")
	}
	return io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
}

type openAIImagesProxyTokenPayload struct {
	CacheID string `json:"cache_id,omitempty"`
	URL     string `json:"url,omitempty"`
	Exp     int64  `json:"exp"`
}

func (s *OpenAIGatewayService) ProxyOpenAIImagesURL(c *gin.Context, token string) error {
	if c == nil || c.Request == nil {
		return fmt.Errorf("missing request context")
	}
	payload, err := s.parseOpenAIImagesProxyToken(token, time.Now())
	if err != nil {
		return err
	}
	var imageBytes []byte
	contentType := ""
	if payload.CacheID != "" {
		if s.openAIImageCache == nil {
			return fmt.Errorf("openai image cache is not configured")
		}
		imageBytes, contentType, err = s.openAIImageCache.GetImage(c.Request.Context(), payload.CacheID)
		if err != nil {
			return err
		}
	} else {
		imageBytes, contentType, err = s.downloadOpenAIImagesExternalURL(c.Request.Context(), c.Request.Header, payload.URL)
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Data(http.StatusOK, contentType, imageBytes)
	return nil
}

func (s *OpenAIGatewayService) buildOpenAIImagesProxyURL(c *gin.Context, cacheID string, ttl time.Duration) (string, error) {
	token, err := s.newOpenAIImagesProxyToken(cacheID, time.Now(), ttl)
	if err != nil {
		return "", err
	}
	if c == nil || c.Request == nil {
		return "", fmt.Errorf("missing request context")
	}
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	if forwardedProto := firstForwardedHeaderValue(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		switch strings.ToLower(forwardedProto) {
		case "http", "https":
			scheme = strings.ToLower(forwardedProto)
		}
	}
	host := strings.TrimSpace(c.Request.Host)
	if forwardedHost := firstForwardedHeaderValue(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		return "", fmt.Errorf("missing request host")
	}
	return (&neturl.URL{
		Scheme: scheme,
		Host:   host,
		Path:   openAIImagesProxyEndpoint + "/" + token,
	}).String(), nil
}

func firstForwardedHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.Index(value, ","); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value)
}

func (s *OpenAIGatewayService) newOpenAIImagesProxyToken(cacheID string, now time.Time, ttl time.Duration) (string, error) {
	cacheID = strings.TrimSpace(cacheID)
	if cacheID == "" {
		return "", fmt.Errorf("image cache id is required")
	}
	if ttl <= 0 {
		ttl = time.Duration(DefaultOpenAIImageURLCacheTTLHours) * time.Hour
	}
	secret, err := s.openAIImagesProxySecret()
	if err != nil {
		return "", err
	}
	payload := openAIImagesProxyTokenPayload{
		CacheID: cacheID,
		Exp:     now.Add(ttl).Unix(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payloadBytes)
	sigPart := signOpenAIImagesProxyPayload(payloadPart, secret)
	return payloadPart + "." + sigPart, nil
}

func (s *OpenAIGatewayService) parseOpenAIImagesProxyToken(token string, now time.Time) (openAIImagesProxyTokenPayload, error) {
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return openAIImagesProxyTokenPayload{}, fmt.Errorf("invalid image proxy token")
	}
	secret, err := s.openAIImagesProxySecret()
	if err != nil {
		return openAIImagesProxyTokenPayload{}, err
	}
	if !hmac.Equal([]byte(parts[1]), []byte(signOpenAIImagesProxyPayload(parts[0], secret))) {
		return openAIImagesProxyTokenPayload{}, fmt.Errorf("invalid image proxy token signature")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return openAIImagesProxyTokenPayload{}, fmt.Errorf("invalid image proxy token payload")
	}
	var payload openAIImagesProxyTokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return openAIImagesProxyTokenPayload{}, fmt.Errorf("invalid image proxy token payload")
	}
	if payload.Exp <= now.Unix() {
		return openAIImagesProxyTokenPayload{}, fmt.Errorf("image proxy token expired")
	}
	payload.CacheID = strings.TrimSpace(payload.CacheID)
	if payload.CacheID != "" {
		return payload, nil
	}
	normalizedURL, err := s.validateOpenAIImagesExternalImageURL(payload.URL)
	if err != nil {
		return openAIImagesProxyTokenPayload{}, err
	}
	payload.URL = normalizedURL
	return payload, nil
}

func signOpenAIImagesProxyPayload(payloadPart string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payloadPart))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (s *OpenAIGatewayService) openAIImagesProxySecret() ([]byte, error) {
	if s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.JWT.Secret) != "" {
		return []byte(strings.TrimSpace(s.cfg.JWT.Secret)), nil
	}
	return nil, fmt.Errorf("jwt.secret is required for image proxy urls")
}

func (s *OpenAIGatewayService) validateOpenAIImagesExternalImageURL(raw string) (string, error) {
	allowInsecureHTTP := false
	allowPrivateHosts := false
	if s != nil && s.cfg != nil {
		allowInsecureHTTP = s.cfg.Security.URLAllowlist.AllowInsecureHTTP
		allowPrivateHosts = s.cfg.Security.URLAllowlist.AllowPrivateHosts
	}
	return urlvalidator.ValidateHTTPURL(raw, allowInsecureHTTP, urlvalidator.ValidationOptions{
		AllowPrivate: allowPrivateHosts,
	})
}

func (s *OpenAIGatewayService) downloadOpenAIImagesExternalURL(ctx context.Context, headers http.Header, rawURL string) ([]byte, string, error) {
	if normalized := normalizeOpenAIImageBase64(rawURL); normalized != "" {
		decoded, err := base64.StdEncoding.DecodeString(normalized)
		return decoded, dataURLContentType(rawURL), err
	}
	downloadURL, err := s.validateOpenAIImagesExternalImageURL(rawURL)
	if err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", "image/*,*/*;q=0.8")
	userAgent := strings.TrimSpace(headers.Get("User-Agent"))
	if userAgent == "" {
		userAgent = openAIImageBackendUserAgent
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = fmt.Sprintf("download image bytes failed: status %d", resp.StatusCode)
		}
		return nil, "", fmt.Errorf("%s", sanitizeUpstreamErrorMessage(message))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
	if err != nil {
		return nil, "", err
	}
	return body, strings.TrimSpace(resp.Header.Get("Content-Type")), nil
}

func dataURLContentType(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "data:") {
		return ""
	}
	withoutPrefix := raw[len("data:"):]
	if idx := strings.Index(withoutPrefix, ";"); idx >= 0 {
		return strings.TrimSpace(withoutPrefix[:idx])
	}
	if idx := strings.Index(withoutPrefix, ","); idx >= 0 {
		return strings.TrimSpace(withoutPrefix[:idx])
	}
	return ""
}

type openAIImageStatusError struct {
	StatusCode      int
	Message         string
	ResponseBody    []byte
	ResponseHeaders http.Header
	RequestID       string
	URL             string
}

func (e *openAIImageStatusError) Error() string {
	if e == nil {
		return "openai image backend request failed"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("openai image backend request failed: status %d", e.StatusCode)
	}
	return "openai image backend request failed"
}

func newOpenAIImageStatusError(resp *req.Response, fallback string) error {
	if resp == nil {
		if strings.TrimSpace(fallback) == "" {
			fallback = "openai image backend request failed"
		}
		return fmt.Errorf("%s", fallback)
	}

	statusCode := resp.StatusCode
	headers := http.Header(nil)
	requestID := ""
	requestURL := ""
	body := []byte(nil)

	if resp.Response != nil {
		headers = resp.Header.Clone()
		requestID = strings.TrimSpace(resp.Header.Get("x-request-id"))
		if resp.Request != nil && resp.Request.URL != nil {
			requestURL = resp.Request.URL.String()
		}
		if resp.Body != nil {
			body, _ = io.ReadAll(io.LimitReader(resp.Body, 2<<20))
			_ = resp.Body.Close()
		}
	}

	message := sanitizeUpstreamErrorMessage(extractUpstreamErrorMessage(body))
	if message == "" {
		prefix := strings.TrimSpace(fallback)
		if prefix == "" {
			prefix = "openai image backend request failed"
		}
		message = fmt.Sprintf("%s: status %d", prefix, statusCode)
	}

	return &openAIImageStatusError{
		StatusCode:      statusCode,
		Message:         message,
		ResponseBody:    body,
		ResponseHeaders: headers,
		RequestID:       requestID,
		URL:             requestURL,
	}
}

func isOpenAIImageTransientConversationNotFoundError(err error) bool {
	statusErr, ok := err.(*openAIImageStatusError)
	if !ok || statusErr == nil || statusErr.StatusCode != http.StatusNotFound {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(statusErr.Message))
	if strings.Contains(msg, "conversation_not_found") {
		return true
	}
	if strings.Contains(msg, "conversation") && strings.Contains(msg, "not found") {
		return true
	}
	bodyMsg := strings.ToLower(strings.TrimSpace(extractUpstreamErrorMessage(statusErr.ResponseBody)))
	if strings.Contains(bodyMsg, "conversation_not_found") {
		return true
	}
	return strings.Contains(bodyMsg, "conversation") && strings.Contains(bodyMsg, "not found")
}

func cloneHTTPHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for key, values := range src {
		copied := make([]string, len(values))
		copy(copied, values)
		dst[key] = copied
	}
	return dst
}

func headerToMap(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	result := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) == 0 {
			continue
		}
		result[key] = values[0]
	}
	return result
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

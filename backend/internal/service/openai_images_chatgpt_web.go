package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptweb"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	"github.com/tidwall/sjson"
)

// chatGPTWebImageRef holds metadata for an uploaded image reference (used in edits).
type chatGPTWebImageRef struct {
	FileID   string
	FileName string
	FileSize int
	MimeType string
	Width    int
	Height   int
}

// chatGPTWebRequirements holds sentinel tokens needed for ChatGPT Web API.
type chatGPTWebRequirements struct {
	Token          string
	ProofToken     string
	TurnstileToken string
	SOToken        string
}

// chatGPTWebSSEState collects parsed data from ChatGPT Web SSE stream.
type chatGPTWebSSEState struct {
	Text           string
	ConversationID string
	FileIDs        []string
	SedimentIDs    []string
	Blocked        bool
	ToolInvoked    *bool
	TurnUseCase    string
}

func chatGPTWebImageModelSlug(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	if model == "gpt-image-2" {
		return "gpt-5-3"
	}
	return "auto"
}

func isOpenAIFreeAccount(account *Account) bool {
	planType := strings.ToLower(strings.TrimSpace(account.GetCredential("plan_type")))
	return planType == "" || planType == "free"
}

func buildChatGPTWebPreparePayload(prompt, model string) map[string]any {
	return map[string]any{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     uuid.New().String(),
		"model":                 chatGPTWebImageModelSlug(model),
		"client_prepare_state":  "success",
		"timezone_offset_min":   -480,
		"timezone":              "Asia/Shanghai",
		"conversation_mode":     map[string]any{"kind": "primary_assistant"},
		"system_hints":          []string{"picture_v2"},
		"partial_query": map[string]any{
			"id":     uuid.New().String(),
			"author": map[string]any{"role": "user"},
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{prompt},
			},
		},
		"supports_buffering":     true,
		"supported_encodings":    []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
	}
}

func buildChatGPTWebGeneratePayload(prompt, model string, refs []chatGPTWebImageRef) map[string]any {
	var content map[string]any
	if len(refs) > 0 {
		parts := make([]any, 0, len(refs)+1)
		for _, ref := range refs {
			parts = append(parts, map[string]any{
				"content_type":  "image_asset_pointer",
				"asset_pointer": "file-service://" + ref.FileID,
				"width":         ref.Width,
				"height":        ref.Height,
				"size_bytes":    ref.FileSize,
			})
		}
		parts = append(parts, prompt)
		content = map[string]any{"content_type": "multimodal_text", "parts": parts}
	} else {
		content = map[string]any{"content_type": "text", "parts": []string{prompt}}
	}

	metadata := map[string]any{
		"developer_mode_connector_ids": []any{},
		"selected_github_repos":        []any{},
		"selected_all_github_repos":    false,
		"system_hints":                 []string{"picture_v2"},
		"serialization_metadata":       map[string]any{"custom_symbol_offsets": []any{}},
	}
	if len(refs) > 0 {
		attachments := make([]map[string]any, 0, len(refs))
		for _, ref := range refs {
			attachments = append(attachments, map[string]any{
				"id":       ref.FileID,
				"mimeType": ref.MimeType,
				"name":     ref.FileName,
				"size":     ref.FileSize,
				"width":    ref.Width,
				"height":   ref.Height,
			})
		}
		metadata["attachments"] = attachments
	}

	return map[string]any{
		"action": "next",
		"messages": []map[string]any{{
			"id":          uuid.New().String(),
			"author":      map[string]any{"role": "user"},
			"create_time": float64(time.Now().Unix()),
			"content":     content,
			"metadata":    metadata,
		}},
		"parent_message_id":        uuid.New().String(),
		"model":                    chatGPTWebImageModelSlug(model),
		"client_prepare_state":     "sent",
		"timezone_offset_min":      -480,
		"timezone":                 "Asia/Shanghai",
		"conversation_mode":        map[string]any{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":             []string{"picture_v2"},
		"supports_buffering":       true,
		"supported_encodings":      []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 1200,
			"page_height":       1072,
			"page_width":        1724,
			"pixel_ratio":       1.2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
}

// --------------- SSE Stream Parsing ---------------

var (
	fileIDPattern     = regexp.MustCompile(`file[-_]([A-Za-z0-9]+)`)
	sedimentIDPattern = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
)

func parseChatGPTWebSSEStream(r io.Reader) chatGPTWebSSEState {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	state := chatGPTWebSSEState{}
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}
		updateChatGPTWebSSEState(&state, payload)
	}
	return state
}

func updateChatGPTWebSSEState(state *chatGPTWebSSEState, payload string) {
	// Extract conversation_id via regex (fast path)
	if state.ConversationID == "" {
		if idx := strings.Index(payload, `"conversation_id"`); idx >= 0 {
			// Try to extract value
			rest := payload[idx:]
			if start := strings.Index(rest, `":"`); start >= 0 {
				rest = rest[start+3:]
				if end := strings.Index(rest, `"`); end >= 0 {
					state.ConversationID = rest[:end]
				}
			}
		}
	}

	var event map[string]any
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return
	}

	// Update conversation_id from parsed event
	if cid, ok := event["conversation_id"].(string); ok && cid != "" {
		state.ConversationID = cid
	}

	// Check moderation blocked
	if eventType, _ := event["type"].(string); eventType == "moderation" {
		if mr, ok := event["moderation_response"].(map[string]any); ok {
			if blocked, ok := mr["blocked"].(bool); ok && blocked {
				state.Blocked = true
			}
		}
	}

	// Check if this is an image tool event
	if isImageToolEvent(event) {
		// Extract file_ids and sediment_ids from the payload string
		for _, match := range fileIDPattern.FindAllString(payload, -1) {
			addUnique(&state.FileIDs, match)
		}
		for _, matches := range sedimentIDPattern.FindAllStringSubmatch(payload, -1) {
			if len(matches) >= 2 {
				addUnique(&state.SedimentIDs, matches[1])
			}
		}
	}

	// Check server metadata
	if eventType, _ := event["type"].(string); eventType == "server_ste_metadata" {
		if meta, ok := event["metadata"].(map[string]any); ok {
			if ti, ok := meta["tool_invoked"].(bool); ok {
				state.ToolInvoked = &ti
			}
			if tuc, ok := meta["turn_use_case"].(string); ok && tuc != "" {
				state.TurnUseCase = tuc
			}
		}
	}
}

func isImageToolEvent(event map[string]any) bool {
	// Check event.message or event.v.message
	for _, candidate := range []any{event, event["v"]} {
		m, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		message, ok := m["message"].(map[string]any)
		if !ok {
			continue
		}
		author, _ := message["author"].(map[string]any)
		metadata, _ := message["metadata"].(map[string]any)
		if author["role"] == "tool" && metadata["async_task_type"] == "image_gen" {
			return true
		}
	}
	return false
}

func addUnique(slice *[]string, value string) {
	for _, v := range *slice {
		if v == value {
			return
		}
	}
	*slice = append(*slice, value)
}

// --------------- Full Pipeline ---------------

const (
	chatGPTWebBaseURL            = "https://chatgpt.com"
	chatGPTWebDefaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	chatGPTWebDefaultClientVer   = "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad"
	chatGPTWebDefaultBuildNumber = "5955942"
)

// chatGPTWebClient wraps a req.Client with ChatGPT Web specific state.
type chatGPTWebClient struct {
	client        *req.Client
	accessToken   string
	userAgent     string
	deviceID      string
	sessionID     string
	clientVersion string
	buildNumber   string
	powScriptSrcs []string
	powDataBuild  string
}

func newChatGPTWebClient(accessToken, proxyURL string) *chatGPTWebClient {
	deviceID := uuid.New().String()
	sessionID := uuid.New().String()
	userAgent := chatGPTWebDefaultUserAgent

	client := req.C().
		SetTimeout(300*time.Second).
		SetBaseURL(chatGPTWebBaseURL).
		SetCommonHeader("User-Agent", userAgent).
		SetCommonHeader("Origin", chatGPTWebBaseURL).
		SetCommonHeader("Referer", chatGPTWebBaseURL+"/").
		SetCommonHeader("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8,en-US;q=0.7").
		SetCommonHeader("Cache-Control", "no-cache").
		SetCommonHeader("Pragma", "no-cache").
		SetCommonHeader("Priority", "u=1, i").
		SetCommonHeader("Sec-Ch-Ua", `"Microsoft Edge";v="143", "Chromium";v="143", "Not A(Brand";v="24"`).
		SetCommonHeader("Sec-Ch-Ua-Arch", `"x86"`).
		SetCommonHeader("Sec-Ch-Ua-Bitness", `"64"`).
		SetCommonHeader("Sec-Ch-Ua-Full-Version", `"143.0.3650.96"`).
		SetCommonHeader("Sec-Ch-Ua-Full-Version-List", `"Microsoft Edge";v="143.0.3650.96", "Chromium";v="143.0.7499.147", "Not A(Brand";v="24.0.0.0"`).
		SetCommonHeader("Sec-Ch-Ua-Mobile", "?0").
		SetCommonHeader("Sec-Ch-Ua-Model", `""`).
		SetCommonHeader("Sec-Ch-Ua-Platform", `"Windows"`).
		SetCommonHeader("Sec-Ch-Ua-Platform-Version", `"19.0.0"`).
		SetCommonHeader("Sec-Fetch-Dest", "empty").
		SetCommonHeader("Sec-Fetch-Mode", "cors").
		SetCommonHeader("Sec-Fetch-Site", "same-origin").
		SetCommonHeader("OAI-Device-Id", deviceID).
		SetCommonHeader("OAI-Session-Id", sessionID).
		SetCommonHeader("OAI-Language", "zh-CN").
		SetCommonHeader("OAI-Client-Version", chatGPTWebDefaultClientVer).
		SetCommonHeader("OAI-Client-Build-Number", chatGPTWebDefaultBuildNumber)

	if accessToken != "" {
		client.SetCommonHeader("Authorization", "Bearer "+accessToken)
	}
	if proxyURL != "" {
		client.SetProxyURL(proxyURL)
	}

	return &chatGPTWebClient{
		client:        client,
		accessToken:   accessToken,
		userAgent:     userAgent,
		deviceID:      deviceID,
		sessionID:     sessionID,
		clientVersion: chatGPTWebDefaultClientVer,
		buildNumber:   chatGPTWebDefaultBuildNumber,
	}
}

func (c *chatGPTWebClient) pathHeaders(path string) map[string]string {
	return map[string]string{
		"X-OpenAI-Target-Path":  path,
		"X-OpenAI-Target-Route": path,
	}
}

func (c *chatGPTWebClient) imageHeaders(path string, requirements *chatGPTWebRequirements, conduitToken, accept string) map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       accept,
		"OpenAI-Sentinel-Chat-Requirements-Token": requirements.Token,
		"X-OpenAI-Target-Path":                    path,
		"X-OpenAI-Target-Route":                   path,
	}
	if requirements.ProofToken != "" {
		headers["OpenAI-Sentinel-Proof-Token"] = requirements.ProofToken
	}
	if requirements.TurnstileToken != "" {
		headers["OpenAI-Sentinel-Turnstile-Token"] = requirements.TurnstileToken
	}
	if requirements.SOToken != "" {
		headers["OpenAI-Sentinel-SO-Token"] = requirements.SOToken
	}
	if conduitToken != "" {
		headers["X-Conduit-Token"] = conduitToken
	}
	if accept == "text/event-stream" {
		headers["X-Oai-Turn-Trace-Id"] = uuid.New().String()
	}
	return headers
}

func (c *chatGPTWebClient) bootstrap(ctx context.Context) error {
	resp, err := c.client.R().
		SetContext(ctx).
		SetHeaders(map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
			"Sec-Fetch-Dest":            "document",
			"Sec-Fetch-Mode":            "navigate",
			"Sec-Fetch-Site":            "none",
			"Sec-Fetch-User":            "?1",
			"Upgrade-Insecure-Requests": "1",
		}).
		Get("/")
	if err != nil {
		return fmt.Errorf("bootstrap request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("bootstrap failed: status %d", resp.StatusCode)
	}
	c.powScriptSrcs, c.powDataBuild = chatgptweb.ParsePowResources(resp.String())
	return nil
}

func (c *chatGPTWebClient) sentinel(ctx context.Context) (*chatGPTWebRequirements, error) {
	p := chatgptweb.BuildLegacyRequirementsToken(c.userAgent, c.powScriptSrcs, c.powDataBuild)
	path := "/backend-api/sentinel/chat-requirements"

	var result map[string]any
	resp, err := c.client.R().
		SetContext(ctx).
		SetHeaders(c.pathHeaders(path)).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]string{"p": p}).
		SetSuccessResult(&result).
		Post(path)
	if err != nil {
		return nil, fmt.Errorf("sentinel request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("sentinel failed: status %d", resp.StatusCode)
	}

	token, _ := result["token"].(string)
	if token == "" {
		return nil, fmt.Errorf("sentinel returned empty token")
	}

	// Build proof token if required
	proofToken := ""
	if powInfo, ok := result["proofofwork"].(map[string]any); ok {
		if required, _ := powInfo["required"].(bool); required {
			seed, _ := powInfo["seed"].(string)
			difficulty, _ := powInfo["difficulty"].(string)
			pt, err := chatgptweb.BuildProofToken(seed, difficulty, c.userAgent, c.powScriptSrcs, c.powDataBuild)
			if err != nil {
				return nil, fmt.Errorf("proof token generation failed: %w", err)
			}
			proofToken = pt
		}
	}

	// Solve turnstile if required
	turnstileToken := ""
	if tsInfo, ok := result["turnstile"].(map[string]any); ok {
		if required, _ := tsInfo["required"].(bool); required {
			if dx, ok := tsInfo["dx"].(string); ok && dx != "" {
				turnstileToken = chatgptweb.SolveTurnstileToken(dx, p)
			}
		}
	}

	soToken, _ := result["so_token"].(string)

	return &chatGPTWebRequirements{
		Token:          token,
		ProofToken:     proofToken,
		TurnstileToken: turnstileToken,
		SOToken:        soToken,
	}, nil
}

func (c *chatGPTWebClient) prepare(ctx context.Context, prompt, model string, requirements *chatGPTWebRequirements) (string, error) {
	path := "/backend-api/f/conversation/prepare"
	payload := buildChatGPTWebPreparePayload(prompt, model)

	var result map[string]any
	resp, err := c.client.R().
		SetContext(ctx).
		SetHeaders(c.imageHeaders(path, requirements, "", "*/*")).
		SetBody(payload).
		SetSuccessResult(&result).
		Post(path)
	if err != nil {
		return "", fmt.Errorf("prepare request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("prepare failed: status %d", resp.StatusCode)
	}

	conduitToken, _ := result["conduit_token"].(string)
	if conduitToken == "" {
		return "", fmt.Errorf("prepare returned empty conduit_token")
	}
	return conduitToken, nil
}

func (c *chatGPTWebClient) generate(ctx context.Context, prompt, model string, requirements *chatGPTWebRequirements, conduitToken string, refs []chatGPTWebImageRef) (*req.Response, error) {
	path := "/backend-api/f/conversation"
	payload := buildChatGPTWebGeneratePayload(prompt, model, refs)

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeaders(c.imageHeaders(path, requirements, conduitToken, "text/event-stream")).
		SetBody(payload).
		DisableAutoReadResponse().
		Post(path)
	if err != nil {
		return nil, fmt.Errorf("generate request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("generate failed: status %d, body=%s", resp.StatusCode, string(body))
	}
	return resp, nil
}

func (c *chatGPTWebClient) uploadImage(ctx context.Context, upload OpenAIImagesUpload) (chatGPTWebImageRef, error) {
	data := upload.Data
	width := upload.Width
	height := upload.Height
	fileName := upload.FileName
	if fileName == "" {
		fileName = "image.png"
	}
	mimeType := upload.ContentType
	if mimeType == "" {
		mimeType = "image/png"
	}

	// Step 1: Create file
	path := "/backend-api/files"
	var createResult map[string]any
	resp, err := c.client.R().
		SetContext(ctx).
		SetHeaders(c.pathHeaders(path)).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetBody(map[string]any{
			"file_name": fileName,
			"file_size": len(data),
			"use_case":  "multimodal",
			"width":     width,
			"height":    height,
		}).
		SetSuccessResult(&createResult).
		Post(path)
	if err != nil {
		return chatGPTWebImageRef{}, fmt.Errorf("file create failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		return chatGPTWebImageRef{}, fmt.Errorf("file create failed: status %d", resp.StatusCode)
	}

	fileID, _ := createResult["file_id"].(string)
	uploadURL, _ := createResult["upload_url"].(string)
	if fileID == "" || uploadURL == "" {
		return chatGPTWebImageRef{}, fmt.Errorf("file create returned empty file_id or upload_url")
	}

	// Step 2: Upload to Azure Blob
	time.Sleep(500 * time.Millisecond)
	uploadResp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", mimeType).
		SetHeader("x-ms-blob-type", "BlockBlob").
		SetHeader("x-ms-version", "2020-04-08").
		SetHeader("Accept", "application/json, text/plain, */*").
		SetBody(data).
		Put(uploadURL)
	if err != nil {
		return chatGPTWebImageRef{}, fmt.Errorf("file upload failed: %w", err)
	}
	if uploadResp.StatusCode >= 400 {
		return chatGPTWebImageRef{}, fmt.Errorf("file upload failed: status %d", uploadResp.StatusCode)
	}

	// Step 3: Confirm upload
	confirmPath := fmt.Sprintf("/backend-api/files/%s/uploaded", fileID)
	confirmResp, err := c.client.R().
		SetContext(ctx).
		SetHeaders(c.pathHeaders(confirmPath)).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetBody("{}").
		Post(confirmPath)
	if err != nil {
		return chatGPTWebImageRef{}, fmt.Errorf("file confirm failed: %w", err)
	}
	if confirmResp.StatusCode >= 400 {
		return chatGPTWebImageRef{}, fmt.Errorf("file confirm failed: status %d", confirmResp.StatusCode)
	}

	return chatGPTWebImageRef{
		FileID:   fileID,
		FileName: fileName,
		FileSize: len(data),
		MimeType: mimeType,
		Width:    width,
		Height:   height,
	}, nil
}

func (c *chatGPTWebClient) resolveImageURLs(ctx context.Context, state chatGPTWebSSEState) ([]string, error) {
	var urls []string

	// Try file-service IDs first
	for _, fid := range state.FileIDs {
		if fid == "file_upload" {
			continue
		}
		path := fmt.Sprintf("/backend-api/files/%s/download", fid)
		var result map[string]any
		resp, err := c.client.R().
			SetContext(ctx).
			SetHeaders(c.pathHeaders(path)).
			SetHeader("Accept", "application/json").
			SetSuccessResult(&result).
			Get(path)
		if err != nil {
			slog.Warn("chatgpt_web_file_download_url_failed", "file_id", fid, "error", err.Error())
			continue
		}
		if resp.StatusCode >= 400 {
			slog.Warn("chatgpt_web_file_download_url_failed", "file_id", fid, "status", resp.StatusCode)
			continue
		}
		u := firstNonEmptyStr(result["download_url"], result["url"])
		if u != "" {
			urls = append(urls, u)
		}
	}

	if len(urls) > 0 || state.ConversationID == "" {
		return urls, nil
	}

	// Fallback to sediment attachment download
	for _, sid := range state.SedimentIDs {
		path := fmt.Sprintf("/backend-api/conversation/%s/attachment/%s/download", state.ConversationID, sid)
		var result map[string]any
		resp, err := c.client.R().
			SetContext(ctx).
			SetHeaders(c.pathHeaders(path)).
			SetHeader("Accept", "application/json").
			SetSuccessResult(&result).
			Get(path)
		if err != nil {
			slog.Warn("chatgpt_web_sediment_download_url_failed", "sediment_id", sid, "error", err.Error())
			continue
		}
		if resp.StatusCode >= 400 {
			slog.Warn("chatgpt_web_sediment_download_url_failed", "sediment_id", sid, "status", resp.StatusCode)
			continue
		}
		u := firstNonEmptyStr(result["download_url"], result["url"])
		if u != "" {
			urls = append(urls, u)
		}
	}
	return urls, nil
}

func firstNonEmptyStr(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func (c *chatGPTWebClient) downloadImages(ctx context.Context, urls []string) ([][]byte, error) {
	var images [][]byte
	for _, u := range urls {
		resp, err := c.client.R().
			SetContext(ctx).
			DisableAutoReadResponse().
			Get(u)
		if err != nil {
			return nil, fmt.Errorf("image download failed: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, openAIImageMaxDownloadBytes))
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("image download read failed: %w", err)
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("image download failed: status %d", resp.StatusCode)
		}
		images = append(images, data)
	}
	return images, nil
}

// --------------- Main Entry Point ---------------

func (s *OpenAIGatewayService) forwardOpenAIImagesChatGPTWeb(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := strings.TrimSpace(parsed.Model)
	if mapped := strings.TrimSpace(channelMappedModel); mapped != "" {
		requestModel = mapped
	}
	if requestModel == "" {
		requestModel = "gpt-image-2"
	}

	slog.Info("chatgpt_web_image_start",
		"model", requestModel,
		"endpoint", parsed.Endpoint,
		"uploads", len(parsed.Uploads),
		"account_id", account.ID,
	)

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	webClient := newChatGPTWebClient(token, proxyURL)

	// 1. Bootstrap
	if err := webClient.bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("chatgpt web bootstrap: %w", err)
	}

	// 2. Sentinel
	requirements, err := webClient.sentinel(ctx)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web sentinel: %w", err)
	}

	// 3. Upload images (for edits)
	var refs []chatGPTWebImageRef
	for i, upload := range parsed.Uploads {
		ref, err := webClient.uploadImage(ctx, upload)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web image upload[%d]: %w", i, err)
		}
		refs = append(refs, ref)
	}

	// 4. Prepare → conduit_token
	conduitToken, err := webClient.prepare(ctx, parsed.Prompt, requestModel, requirements)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web prepare: %w", err)
	}

	// 5. Generate (SSE)
	genResp, err := webClient.generate(ctx, parsed.Prompt, requestModel, requirements, conduitToken, refs)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web generate: %w", err)
	}
	defer func() {
		if genResp != nil && genResp.Body != nil {
			_ = genResp.Body.Close()
		}
	}()

	// 6. Parse SSE → collect image pointers
	sseState := parseChatGPTWebSSEStream(genResp.Body)

	slog.Info("chatgpt_web_image_sse_done",
		"conversation_id", sseState.ConversationID,
		"file_ids", sseState.FileIDs,
		"sediment_ids", sseState.SedimentIDs,
		"blocked", sseState.Blocked,
		"text", truncateChatGPTWebText(sseState.Text, 200),
	)

	if sseState.Blocked {
		errMsg := sseState.Text
		if errMsg == "" {
			errMsg = "Image generation blocked by content policy"
		}
		return nil, &openAIImageStatusError{StatusCode: http.StatusBadRequest, Message: errMsg}
	}

	if len(sseState.FileIDs) == 0 && len(sseState.SedimentIDs) == 0 {
		if sseState.Text != "" {
			return nil, fmt.Errorf("no image output, upstream message: %s", truncateChatGPTWebText(sseState.Text, 300))
		}
		return nil, fmt.Errorf("no image pointers in ChatGPT Web response")
	}

	// 7. Resolve download URLs
	imageURLs, err := webClient.resolveImageURLs(ctx, sseState)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web resolve images: %w", err)
	}
	if len(imageURLs) == 0 {
		return nil, fmt.Errorf("failed to resolve any image download URLs")
	}

	// 8. Download images
	imageDataList, err := webClient.downloadImages(ctx, imageURLs)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web download images: %w", err)
	}

	// 9. Build response
	return s.buildChatGPTWebImagesResponse(c, imageDataList, parsed, requestModel, startTime)
}

func (s *OpenAIGatewayService) buildChatGPTWebImagesResponse(
	c *gin.Context,
	imageDataList [][]byte,
	parsed *OpenAIImagesRequest,
	requestModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	createdAt := time.Now().Unix()
	format := strings.ToLower(strings.TrimSpace(parsed.ResponseFormat))
	if format == "" {
		format = "b64_json"
	}

	out := []byte(`{"created":0,"data":[]}`)
	out, _ = sjson.SetBytes(out, "created", createdAt)

	for _, imgData := range imageDataList {
		item := []byte(`{}`)
		b64 := base64.StdEncoding.EncodeToString(imgData)
		if format == "url" {
			item, _ = sjson.SetBytes(item, "url", "data:image/png;base64,"+b64)
		} else {
			item, _ = sjson.SetBytes(item, "b64_json", b64)
		}
		item, _ = sjson.SetBytes(item, "revised_prompt", parsed.Prompt)
		out, _ = sjson.SetRawBytes(out, "data.-1", item)
	}
	out, _ = sjson.SetBytes(out, "model", requestModel)

	if parsed.Stream {
		return s.writeChatGPTWebStreamResponse(c, out, parsed, requestModel, startTime)
	}

	c.Data(http.StatusOK, "application/json; charset=utf-8", out)
	return &OpenAIForwardResult{
		Usage:      OpenAIUsage{},
		Model:      requestModel,
		Stream:     false,
		Duration:   time.Since(startTime),
		ImageCount: len(imageDataList),
		ImageSize:  parsed.SizeTier,
	}, nil
}

func (s *OpenAIGatewayService) writeChatGPTWebStreamResponse(
	c *gin.Context,
	responseBody []byte,
	parsed *OpenAIImagesRequest,
	requestModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming is not supported by response writer")
	}

	prefix := openAIImagesStreamPrefix(parsed)
	eventName := prefix + ".completed"

	// Write all images as completed events
	results := make([]openAIResponsesImageResult, 0)
	format := strings.ToLower(strings.TrimSpace(parsed.ResponseFormat))
	if format == "" {
		format = "b64_json"
	}

	// Parse response body to extract image data
	var respObj struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(responseBody, &respObj); err == nil {
		for _, d := range respObj.Data {
			result := d.B64JSON
			if result == "" && d.URL != "" {
				// Extract base64 from data URL
				if idx := strings.Index(d.URL, ";base64,"); idx >= 0 {
					result = d.URL[idx+8:]
				}
			}
			results = append(results, openAIResponsesImageResult{
				Result:        result,
				RevisedPrompt: d.RevisedPrompt,
				Model:         requestModel,
			})
		}
	}

	ms := int(time.Since(startTime).Milliseconds())
	for _, img := range results {
		payload := buildOpenAIImagesStreamCompletedPayload(eventName, img, format, time.Now().Unix(), nil)
		if err := s.writeOpenAIImagesStreamEvent(c, flusher, eventName, payload); err != nil {
			return nil, err
		}
	}

	return &OpenAIForwardResult{
		Usage:        OpenAIUsage{},
		Model:        requestModel,
		Stream:       true,
		Duration:     time.Since(startTime),
		FirstTokenMs: &ms,
		ImageCount:   len(results),
		ImageSize:    parsed.SizeTier,
	}, nil
}

func truncateChatGPTWebText(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

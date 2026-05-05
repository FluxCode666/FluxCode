# OpenAI OAuth 免费账号图片生成 — ChatGPT Web 链路实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 OAuth 免费账号（plan_type 为 free 或空值）的文生图与图生图请求切到 ChatGPT Web 图片专用链路，解决 gpt-image-2 模型生图失败问题。

**Architecture:** 在 `forwardOpenAIImagesOAuth` 入口按 plan_type 分支；免费账号走新建的 `forwardOpenAIImagesChatGPTWeb`，依次调用 Bootstrap → Sentinel → Prepare → Generate（SSE），收集图片指针后复用已有下载能力转换为 Images API 响应。PoW 和 Turnstile solver 从参考项目 Python 实现移植到 Go。

**Tech Stack:** Go, SHA3-512 (`golang.org/x/crypto/sha3`), base64, JSON SSE, `imroc/req/v3`（复用 privacyClientFactory）

---

## 文件结构

### 新建文件

| 文件 | 职责 |
|------|------|
| `backend/internal/pkg/chatgptweb/pow.go` | PoW proof token 计算：`BuildPowConfig`, `PowGenerate`, `BuildProofToken`, `BuildLegacyRequirementsToken` |
| `backend/internal/pkg/chatgptweb/pow_test.go` | PoW 单元测试 |
| `backend/internal/pkg/chatgptweb/turnstile.go` | Turnstile token solver：`SolveTurnstileToken` |
| `backend/internal/pkg/chatgptweb/turnstile_test.go` | Turnstile 单元测试 |
| `backend/internal/pkg/chatgptweb/bootstrap.go` | Bootstrap HTML 解析：`ParsePowResources` |
| `backend/internal/pkg/chatgptweb/bootstrap_test.go` | Bootstrap 解析测试 |
| `backend/internal/service/openai_images_chatgpt_web.go` | ChatGPT Web 图片链路主逻辑 |
| `backend/internal/service/openai_images_chatgpt_web_test.go` | 主逻辑单元测试 |

### 修改文件

| 文件 | 改动 |
|------|------|
| `backend/internal/service/openai_images_responses.go` | 在 `forwardOpenAIImagesOAuth` 顶部加 `isOpenAIFreeAccount` 分支（~10 行） |
| `backend/go.mod` | 可能需要添加 `golang.org/x/crypto` 依赖（SHA3-512） |

---

## Task 1: PoW 基础设施

**Files:**
- Create: `backend/internal/pkg/chatgptweb/pow.go`
- Create: `backend/internal/pkg/chatgptweb/pow_test.go`

- [ ] **Step 1: Write failing test for BuildPowConfig**

```go
// backend/internal/pkg/chatgptweb/pow_test.go
package chatgptweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildPowConfig_ReturnsCorrectLength(t *testing.T) {
	cfg := BuildPowConfig("Mozilla/5.0 Test", []string{"https://chatgpt.com/backend-api/sentinel/sdk.js"}, "")
	require.Len(t, cfg, 18)
	require.Equal(t, "Mozilla/5.0 Test", cfg[4])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestBuildPowConfig -v`
Expected: FAIL — package/function not found

- [ ] **Step 3: Implement BuildPowConfig**

```go
// backend/internal/pkg/chatgptweb/pow.go
package chatgptweb

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	mrand "math/rand/v2"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/sha3"
	pybase64 "encoding/base64"
)

const DefaultPowScript = "https://chatgpt.com/backend-api/sentinel/sdk.js"

var (
	cores        = []int{8, 16, 24, 32}
	documentKeys = []string{"_reactListeningo743lnnpvdg", "location"}
)

// navigatorKeys and windowKeys omitted for brevity — see reference project utils/pow.py lines 17-141
// Full lists copied verbatim.

func BuildPowConfig(userAgent string, scriptSources []string, dataBuild string) []any {
	if len(scriptSources) == 0 {
		scriptSources = []string{DefaultPowScript}
	}
	return []any{
		randChoice([]int{3000, 4000, 5000}),
		legacyParseTime(),
		4294705152,
		0,
		userAgent,
		scriptSources[mrand.IntN(len(scriptSources))],
		dataBuild,
		"en-US",
		"en-US,es-US,en,es",
		0,
		randNavigatorKey(),
		documentKeys[mrand.IntN(len(documentKeys))],
		randWindowKey(),
		float64(time.Now().UnixMilli()),
		uuid.New().String(),
		"",
		cores[mrand.IntN(len(cores))],
		float64(time.Now().UnixMilli()) - float64(time.Now().UnixMilli()),
	}
}
```

(Full navigator/window key lists, `legacyParseTime`, `randChoice`, `randNavigatorKey`, `randWindowKey` helpers all implemented in the same file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestBuildPowConfig -v`
Expected: PASS

- [ ] **Step 5: Write failing test for PowGenerate**

```go
func TestPowGenerate_SolvesEasyDifficulty(t *testing.T) {
	cfg := BuildPowConfig("Mozilla/5.0 Test", nil, "")
	answer, solved := PowGenerate("test-seed", "0fffff", cfg, 100000)
	require.True(t, solved)
	require.NotEmpty(t, answer)
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestPowGenerate -v`
Expected: FAIL — function not found

- [ ] **Step 7: Implement PowGenerate, BuildProofToken, BuildLegacyRequirementsToken**

```go
func PowGenerate(seed, difficulty string, config []any, limit int) (string, bool) {
	target, _ := hex.DecodeString(difficulty)
	diffLen := len(difficulty) / 2
	seedBytes := []byte(seed)

	// Build static JSON parts (indices 0-2, 4-8, 10-17)
	static1, _ := json.Marshal(config[:3])  // trim trailing "]", append ","
	static2 := ...  // config[4:9]
	static3 := ...  // config[10:]

	for i := 0; i < limit; i++ {
		finalJSON := fmt.Sprintf("%s%d%s%d%s", static1[:len(static1)-1]+",", i, static2, i>>1, static3)
		encoded := pybase64.StdEncoding.EncodeToString([]byte(finalJSON))
		h := sha3.New512()
		h.Write(seedBytes)
		h.Write([]byte(encoded))
		digest := h.Sum(nil)
		if bytes.Compare(digest[:diffLen], target) <= 0 {
			return encoded, true
		}
	}
	fallback := "wQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + pybase64.StdEncoding.EncodeToString([]byte(`"`+seed+`"`))
	return fallback, false
}

func BuildProofToken(seed, difficulty, userAgent string, scriptSources []string, dataBuild string) (string, error) {
	cfg := BuildPowConfig(userAgent, scriptSources, dataBuild)
	answer, solved := PowGenerate(seed, difficulty, cfg, 500000)
	if !solved {
		return "", fmt.Errorf("failed to solve proof token: difficulty=%s", difficulty)
	}
	return "gAAAAAB" + answer, nil
}

func BuildLegacyRequirementsToken(userAgent string, scriptSources []string, dataBuild string) string {
	seed := fmt.Sprintf("%f", mrand.Float64())
	cfg := BuildPowConfig(userAgent, scriptSources, dataBuild)
	answer, _ := PowGenerate(seed, "0fffff", cfg, 500000)
	return "gAAAAAC" + answer
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -v`
Expected: ALL PASS

- [ ] **Step 9: Commit**

```bash
git add backend/internal/pkg/chatgptweb/pow.go backend/internal/pkg/chatgptweb/pow_test.go
git commit -m "feat: add ChatGPT Web PoW proof token infrastructure"
```

---

## Task 2: Turnstile Solver

**Files:**
- Create: `backend/internal/pkg/chatgptweb/turnstile.go`
- Create: `backend/internal/pkg/chatgptweb/turnstile_test.go`

- [ ] **Step 1: Write failing test**

```go
// backend/internal/pkg/chatgptweb/turnstile_test.go
package chatgptweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSolveTurnstileToken_EmptyDx(t *testing.T) {
	result := SolveTurnstileToken("", "test-p")
	require.Empty(t, result, "empty dx should produce empty token")
}

func TestSolveTurnstileToken_InvalidBase64(t *testing.T) {
	result := SolveTurnstileToken("not-valid-base64!!!", "test-p")
	require.Empty(t, result, "invalid base64 dx should produce empty token")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestSolveTurnstile -v`
Expected: FAIL

- [ ] **Step 3: Implement SolveTurnstileToken**

Port `utils/turnstile.py` → `turnstile.go`. Key functions:
- `SolveTurnstileToken(dx, p string) string`
- Internal: `turnstileToStr`, `xorString`, `OrderedMap`, process function dispatch table

The process VM interprets a token list with opcodes (1,2,3,5,6,7,8,14,15,17,18,19,20,21,23,24) exactly as the Python reference.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestSolveTurnstile -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/pkg/chatgptweb/turnstile.go backend/internal/pkg/chatgptweb/turnstile_test.go
git commit -m "feat: add ChatGPT Web Turnstile token solver"
```

---

## Task 3: Bootstrap HTML Parser

**Files:**
- Create: `backend/internal/pkg/chatgptweb/bootstrap.go`
- Create: `backend/internal/pkg/chatgptweb/bootstrap_test.go`

- [ ] **Step 1: Write failing test**

```go
// backend/internal/pkg/chatgptweb/bootstrap_test.go
package chatgptweb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePowResources_ExtractsScripts(t *testing.T) {
	html := `<html data-build="c/abc123/_"><head>
<script src="https://cdn.chatgpt.com/a.js"></script>
<script src="https://cdn.chatgpt.com/b.js"></script>
</head></html>`
	sources, dataBuild := ParsePowResources(html)
	require.Equal(t, []string{"https://cdn.chatgpt.com/a.js", "https://cdn.chatgpt.com/b.js"}, sources)
	require.Equal(t, "c/abc123/_", dataBuild)
}

func TestParsePowResources_EmptyHTML(t *testing.T) {
	sources, dataBuild := ParsePowResources("")
	require.Equal(t, []string{DefaultPowScript}, sources)
	require.Empty(t, dataBuild)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestParsePowResources -v`
Expected: FAIL

- [ ] **Step 3: Implement ParsePowResources**

```go
// backend/internal/pkg/chatgptweb/bootstrap.go
package chatgptweb

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

func ParsePowResources(htmlContent string) (scriptSources []string, dataBuild string) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return []string{DefaultPowScript}, ""
	}
	// Walk DOM, collect <script src="..."> values
	// Extract data-build from <html> tag or from script src matching c/[^/]*/_
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			for _, a := range n.Attr {
				if a.Key == "src" && a.Val != "" {
					scriptSources = append(scriptSources, a.Val)
					if m := regexp.MustCompile(`c/[^/]*/_`).FindString(a.Val); m != "" && dataBuild == "" {
						dataBuild = m
					}
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "html" {
			for _, a := range n.Attr {
				if a.Key == "data-build" && a.Val != "" && dataBuild == "" {
					dataBuild = a.Val
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if len(scriptSources) == 0 {
		scriptSources = []string{DefaultPowScript}
	}
	return
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ -run TestParsePowResources -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/pkg/chatgptweb/bootstrap.go backend/internal/pkg/chatgptweb/bootstrap_test.go
git commit -m "feat: add ChatGPT Web Bootstrap HTML parser"
```

---

## Task 4: ChatGPT Web 图片链路主逻辑 — 模型映射 & Payload 构造

**Files:**
- Create: `backend/internal/service/openai_images_chatgpt_web.go`
- Create: `backend/internal/service/openai_images_chatgpt_web_test.go`

- [ ] **Step 1: Write failing tests for model mapping and payload builders**

```go
// backend/internal/service/openai_images_chatgpt_web_test.go
package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImageModelSlug(t *testing.T) {
	require.Equal(t, "gpt-5-3", chatGPTWebImageModelSlug("gpt-image-2"))
	require.Equal(t, "auto", chatGPTWebImageModelSlug(""))
	require.Equal(t, "auto", chatGPTWebImageModelSlug("unknown-model"))
}

func TestIsOpenAIFreeAccount(t *testing.T) {
	free := &Account{Credentials: map[string]any{"plan_type": "free"}}
	require.True(t, isOpenAIFreeAccount(free))

	empty := &Account{Credentials: map[string]any{}}
	require.True(t, isOpenAIFreeAccount(empty))

	plus := &Account{Credentials: map[string]any{"plan_type": "plus"}}
	require.False(t, isOpenAIFreeAccount(plus))
}

func TestBuildPreparePayload(t *testing.T) {
	body := buildChatGPTWebPreparePayload("draw a cat", "gpt-image-2")
	require.Equal(t, "gpt-5-3", body["model"])
	require.Equal(t, []string{"picture_v2"}, body["system_hints"])
	require.Equal(t, "success", body["client_prepare_state"])
}

func TestBuildGeneratePayload_TextOnly(t *testing.T) {
	body := buildChatGPTWebGeneratePayload("draw a cat", "gpt-image-2", nil)
	require.Equal(t, "gpt-5-3", body["model"])
	msgs := body["messages"].([]map[string]any)
	require.Len(t, msgs, 1)
	content := msgs[0]["content"].(map[string]any)
	require.Equal(t, "text", content["content_type"])
}

func TestBuildGeneratePayload_WithReferences(t *testing.T) {
	refs := []chatGPTWebImageRef{{FileID: "file-abc", Width: 100, Height: 100, FileSize: 1024, MimeType: "image/png", FileName: "test.png"}}
	body := buildChatGPTWebGeneratePayload("edit this", "gpt-image-2", refs)
	msgs := body["messages"].([]map[string]any)
	content := msgs[0]["content"].(map[string]any)
	require.Equal(t, "multimodal_text", content["content_type"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service/ -run "TestImageModelSlug|TestIsOpenAIFreeAccount|TestBuildPreparePayload|TestBuildGeneratePayload" -v`
Expected: FAIL

- [ ] **Step 3: Implement model mapping, account check, and payload builders**

```go
// backend/internal/service/openai_images_chatgpt_web.go
package service

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type chatGPTWebImageRef struct {
	FileID   string
	FileName string
	FileSize int
	MimeType string
	Width    int
	Height   int
}

func chatGPTWebImageModelSlug(model string) string {
	model = strings.TrimSpace(model)
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
		"action":               "next",
		"fork_from_shared_post": false,
		"parent_message_id":    uuid.New().String(),
		"model":                chatGPTWebImageModelSlug(model),
		"client_prepare_state": "success",
		"timezone_offset_min":  -480,
		"timezone":             "Asia/Shanghai",
		"conversation_mode":    map[string]any{"kind": "primary_assistant"},
		"system_hints":         []string{"picture_v2"},
		"partial_query": map[string]any{
			"id":     uuid.New().String(),
			"author": map[string]any{"role": "user"},
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{prompt},
			},
		},
		"supports_buffering":    true,
		"supported_encodings":   []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
	}
}

func buildChatGPTWebGeneratePayload(prompt, model string, refs []chatGPTWebImageRef) map[string]any {
	// Build content
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

	// Build metadata
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
				"id": ref.FileID, "mimeType": ref.MimeType,
				"name": ref.FileName, "size": ref.FileSize,
				"width": ref.Width, "height": ref.Height,
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
		"parent_message_id":                   uuid.New().String(),
		"model":                               chatGPTWebImageModelSlug(model),
		"client_prepare_state":                "sent",
		"timezone_offset_min":                 -480,
		"timezone":                            "Asia/Shanghai",
		"conversation_mode":                   map[string]any{"kind": "primary_assistant"},
		"enable_message_followups":            true,
		"system_hints":                        []string{"picture_v2"},
		"supports_buffering":                  true,
		"supported_encodings":                 []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode": false, "time_since_loaded": 1200,
			"page_height": 1072, "page_width": 1724, "pixel_ratio": 1.2,
			"screen_height": 1440, "screen_width": 2560, "app_name": "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":                "auto",
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service/ -run "TestImageModelSlug|TestIsOpenAIFreeAccount|TestBuildPreparePayload|TestBuildGeneratePayload" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openai_images_chatgpt_web.go backend/internal/service/openai_images_chatgpt_web_test.go
git commit -m "feat: add ChatGPT Web image model mapping and payload builders"
```

---

## Task 5: SSE 事件解析 & 图片指针提取

**Files:**
- Modify: `backend/internal/service/openai_images_chatgpt_web.go`
- Modify: `backend/internal/service/openai_images_chatgpt_web_test.go`

- [ ] **Step 1: Write failing test for SSE parsing**

```go
func TestParseChatGPTWebSSE_ExtractsFileIDs(t *testing.T) {
	sseData := `data: {"type":"moderation","moderation_response":{"blocked":false}}

data: {"message":{"id":"msg-1","author":{"role":"tool"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://file-abc123"}]}},"conversation_id":"conv-xyz"}

data: [DONE]
`
	state := parseChatGPTWebSSEStream(strings.NewReader(sseData))
	require.Equal(t, "conv-xyz", state.ConversationID)
	require.Equal(t, []string{"file-abc123"}, state.FileIDs)
	require.False(t, state.Blocked)
}

func TestParseChatGPTWebSSE_DetectsBlocked(t *testing.T) {
	sseData := `data: {"type":"moderation","moderation_response":{"blocked":true}}

data: [DONE]
`
	state := parseChatGPTWebSSEStream(strings.NewReader(sseData))
	require.True(t, state.Blocked)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service/ -run TestParseChatGPTWebSSE -v`
Expected: FAIL

- [ ] **Step 3: Implement SSE parser**

```go
type chatGPTWebSSEState struct {
	Text           string
	ConversationID string
	FileIDs        []string
	SedimentIDs    []string
	Blocked        bool
	ToolInvoked    *bool
	TurnUseCase    string
}

func parseChatGPTWebSSEStream(r io.Reader) chatGPTWebSSEState {
	scanner := bufio.NewScanner(r)
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
		// Parse JSON event, extract conversation_id, file_ids, sediment_ids
		// Detect moderation blocked, tool invoked, etc.
		updateChatGPTWebSSEState(&state, payload)
	}
	return state
}
```

(Full `updateChatGPTWebSSEState` implementation using `gjson` to extract fields from SSE events, matching reference project's `update_conversation_state` logic.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service/ -run TestParseChatGPTWebSSE -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openai_images_chatgpt_web.go backend/internal/service/openai_images_chatgpt_web_test.go
git commit -m "feat: add ChatGPT Web SSE event parser for image pointers"
```

---

## Task 6: 完整链路集成 — forwardOpenAIImagesChatGPTWeb

**Files:**
- Modify: `backend/internal/service/openai_images_chatgpt_web.go`

- [ ] **Step 1: Implement the full pipeline function**

```go
func (s *OpenAIGatewayService) forwardOpenAIImagesChatGPTWeb(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	parsed *OpenAIImagesRequest,
	channelMappedModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()
	requestModel := resolveRequestModel(parsed, channelMappedModel)

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	// 1. Create HTTP client (reuse privacyClientFactory pattern or direct req.Client)
	client := buildChatGPTWebClient(token, proxyURL)

	// 2. Bootstrap
	powScripts, dataBuild, err := chatGPTWebBootstrap(client)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web bootstrap failed: %w", err)
	}

	// 3. Sentinel chat-requirements
	requirements, err := chatGPTWebSentinel(client, powScripts, dataBuild)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web sentinel failed: %w", err)
	}

	// 4. Upload images (for edits)
	var refs []chatGPTWebImageRef
	if len(parsed.Uploads) > 0 {
		refs, err = chatGPTWebUploadImages(client, parsed.Uploads)
		if err != nil {
			return nil, fmt.Errorf("chatgpt web image upload failed: %w", err)
		}
	}

	// 5. Prepare → conduit_token
	conduitToken, err := chatGPTWebPrepare(client, parsed.Prompt, requestModel, requirements)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web prepare failed: %w", err)
	}

	// 6. Generate (SSE)
	sseResp, err := chatGPTWebGenerate(client, parsed.Prompt, requestModel, requirements, conduitToken, refs)
	if err != nil {
		return nil, fmt.Errorf("chatgpt web generate failed: %w", err)
	}
	defer sseResp.Body.Close()

	// 7. Parse SSE → collect image pointers
	sseState := parseChatGPTWebSSEStream(sseResp.Body)

	if sseState.Blocked {
		return nil, &openAIImageStatusError{StatusCode: 400, Message: "Image generation blocked by content policy"}
	}

	// 8. Resolve & download images (reuse existing functions)
	if len(sseState.FileIDs) == 0 && len(sseState.SedimentIDs) == 0 {
		return nil, fmt.Errorf("no image pointers in response, text: %s", sseState.Text)
	}

	imageBytes, err := s.resolveAndDownloadChatGPTWebImages(ctx, client, sseState, account, token)
	if err != nil {
		return nil, err
	}

	// 9. Build Images API response
	return s.buildChatGPTWebImagesResponse(c, imageBytes, parsed, requestModel, startTime)
}
```

This function wires together all the pieces from Tasks 1-5. Each sub-function (`chatGPTWebBootstrap`, `chatGPTWebSentinel`, `chatGPTWebPrepare`, `chatGPTWebGenerate`, `chatGPTWebUploadImages`) uses the HTTP client to call the respective endpoints with proper headers.

- [ ] **Step 2: Implement HTTP helper functions**

Each helper constructs the correct URL, headers (including `OpenAI-Sentinel-Chat-Requirements-Token`, `OpenAI-Sentinel-Proof-Token`, `X-Conduit-Token`, `X-Oai-Turn-Trace-Id`), and payload, then executes the request.

Key header construction (matching reference project `_image_headers`):

```go
func chatGPTWebImageHeaders(requirements *chatGPTWebRequirements, conduitToken, accept string) map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       accept,
		"OpenAI-Sentinel-Chat-Requirements-Token": requirements.Token,
	}
	if requirements.ProofToken != "" {
		headers["OpenAI-Sentinel-Proof-Token"] = requirements.ProofToken
	}
	if conduitToken != "" {
		headers["X-Conduit-Token"] = conduitToken
	}
	if accept == "text/event-stream" {
		headers["X-Oai-Turn-Trace-Id"] = uuid.New().String()
	}
	return headers
}
```

- [ ] **Step 3: Implement image resolution and response building**

`resolveAndDownloadChatGPTWebImages` uses existing `fetchOpenAIImageDownloadURL` and `downloadOpenAIImageBytes` functions where possible, falling back to direct client calls for the ChatGPT Web download URL API.

`buildChatGPTWebImagesResponse` formats the final response as `{created, data: [{b64_json, revised_prompt}]}` for non-streaming, or writes SSE events for streaming.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/service/openai_images_chatgpt_web.go
git commit -m "feat: implement full ChatGPT Web image generation pipeline"
```

---

## Task 7: 接入分流 — 修改 forwardOpenAIImagesOAuth

**Files:**
- Modify: `backend/internal/service/openai_images_responses.go:719-725`

- [ ] **Step 1: Add free account branch**

At line ~726 of `forwardOpenAIImagesOAuth`, after `startTime := time.Now()`, before `requestModel := ...`:

```go
// ChatGPT Web image path for free accounts
if isOpenAIFreeAccount(account) {
	return s.forwardOpenAIImagesChatGPTWeb(ctx, c, account, parsed, channelMappedModel)
}
```

- [ ] **Step 2: Run existing image tests to verify no regression**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/service/ -run "Image" -v -count=1`
Expected: ALL PASS (existing tests should not be affected as they use non-free accounts)

- [ ] **Step 3: Commit**

```bash
git add backend/internal/service/openai_images_responses.go
git commit -m "feat: route free OAuth accounts to ChatGPT Web image pipeline"
```

---

## Task 8: 依赖管理 & 最终验证

**Files:**
- Modify: `backend/go.mod` (if needed)

- [ ] **Step 1: Add golang.org/x/crypto dependency if not present**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go mod tidy`

- [ ] **Step 2: Run all new tests**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./internal/pkg/chatgptweb/ ./internal/service/ -run "TestBuildPowConfig|TestPowGenerate|TestSolveTurnstile|TestParsePowResources|TestImageModelSlug|TestIsOpenAIFreeAccount|TestBuildPreparePayload|TestBuildGeneratePayload|TestParseChatGPTWebSSE" -v`
Expected: ALL PASS

- [ ] **Step 3: Run full backend test suite**

Run: `cd /Volumes/T7/project/new/FluxCode/backend && go test ./... -count=1 -timeout 120s 2>&1 | tail -30`
Expected: No new failures

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "chore: tidy dependencies for ChatGPT Web image pipeline"
```

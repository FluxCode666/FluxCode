package service

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
)

// subUsageAccountingCacheTTL 会话历史缓存有效期
const subUsageAccountingCacheTTL = time.Hour

// subUsageHistoryEntry 记录某个 session 上一次的 cache_pool_tokens
type subUsageHistoryEntry struct {
	CachePoolTokens int
	UpdatedAt       time.Time
}

// subUsageAccountingStore 全局会话历史缓存（进程内）
var subUsageAccountingStore = struct {
	mu    sync.Mutex
	items map[string]*subUsageHistoryEntry
}{
	items: make(map[string]*subUsageHistoryEntry),
}

// subUsageAccountingResult 本地估算结果
type subUsageAccountingResult struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	CacheCreation5mTokens    int
	CacheCreation1hTokens    int
}

// applySubUsageAccounting 检查并应用 Sub 层 Token 统计。
// 当 account 启用了 anthropic_sub_usage_accounting_enabled 时，
// 用本地估算值覆盖 result.Usage 中的 token 字段。
// 返回 true 表示已应用覆盖。
func (s *GatewayService) applySubUsageAccounting(
	result *ForwardResult,
	account *Account,
	apiKey *APIKey,
	parsed *ParsedRequest,
) bool {
	if result == nil || account == nil || !account.IsAnthropicSubUsageAccountingEnabled() {
		return false
	}
	if parsed == nil || len(parsed.Body) == 0 {
		return false
	}

	est := s.estimateSubUsageAccounting(result, account, apiKey, parsed)

	// 覆盖 result.Usage
	result.Usage.InputTokens = est.InputTokens
	result.Usage.OutputTokens = est.OutputTokens
	result.Usage.CacheCreationInputTokens = est.CacheCreationInputTokens
	result.Usage.CacheReadInputTokens = est.CacheReadInputTokens
	result.Usage.CacheCreation5mTokens = est.CacheCreation5mTokens
	result.Usage.CacheCreation1hTokens = est.CacheCreation1hTokens

	// 日志
	logger.LegacyPrintf("service.gateway",
		"anthropic_sub_usage_accounting: account=%d model=%s input=%d output=%d cache_creation=%d cache_read=%d",
		account.ID, result.Model, est.InputTokens, est.OutputTokens,
		est.CacheCreationInputTokens, est.CacheReadInputTokens)

	return true
}

// estimateSubUsageAccounting 执行本地 token 估算核心逻辑
func (s *GatewayService) estimateSubUsageAccounting(
	result *ForwardResult,
	account *Account,
	apiKey *APIKey,
	parsed *ParsedRequest,
) subUsageAccountingResult {
	body := parsed.Body

	// 1. 估算 input_tokens：最后一条用户消息
	inputTokens := estimateLastUserMessageTokens(parsed)

	// 2. 估算 total_prompt_tokens：system + messages + tools + tool_choice + mcp_servers
	totalPromptTokens := estimateTotalPromptTokens(body, parsed)

	// 3. 计算 cache_pool_tokens
	cachePoolTokens := totalPromptTokens - inputTokens
	if cachePoolTokens < 0 {
		cachePoolTokens = 0
	}

	// 4. output_tokens：使用上游已有值
	outputTokens := result.Usage.OutputTokens
	if outputTokens < 0 {
		outputTokens = 0
	}

	// 5. 拆分 cache_pool 为 read / creation
	var apiKeyID int64
	if apiKey != nil {
		apiKeyID = apiKey.ID
	}
	sessionKey := buildSubUsageSessionKey(account.ID, apiKeyID, result.Model, parsed)
	cacheRead, cacheCreation := splitCachePoolTokens(sessionKey, cachePoolTokens)

	// 6. 更新历史状态
	updateSubUsageHistory(sessionKey, cachePoolTokens)

	// 7. 判断 cache_creation TTL 归类
	cache5m, cache1h := classifyCacheCreationTTL(body, cacheCreation)

	return subUsageAccountingResult{
		InputTokens:              inputTokens,
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: cacheCreation,
		CacheReadInputTokens:     cacheRead,
		CacheCreation5mTokens:    cache5m,
		CacheCreation1hTokens:    cache1h,
	}
}

// estimateLastUserMessageTokens 估算最后一条用户消息的 token 数
func estimateLastUserMessageTokens(parsed *ParsedRequest) int {
	if parsed == nil || len(parsed.Messages) == 0 {
		return 0
	}

	// 倒序查找最后一条 role=user 或 role 为空的消息
	for i := len(parsed.Messages) - 1; i >= 0; i-- {
		msg, ok := parsed.Messages[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := msg["role"].(string)
		if role != "user" && role != "" {
			continue
		}
		content, exists := msg["content"]
		if !exists {
			continue
		}
		return estimateContentTokens(content)
	}
	return 0
}

// estimateContentTokens 估算 Anthropic content 字段的 token 数
// content 可以是 string 或 []content_block
func estimateContentTokens(content any) int {
	switch v := content.(type) {
	case string:
		return subEstimateTokensForText(v)
	case []any:
		total := 0
		for _, block := range v {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			blockType, _ := blockMap["type"].(string)
			switch blockType {
			case "text":
				text, _ := blockMap["text"].(string)
				total += subEstimateTokensForText(text)
			case "tool_use":
				// tool_use block: name + input JSON
				name, _ := blockMap["name"].(string)
				total += subEstimateTokensForText(name)
				if input, ok := blockMap["input"]; ok {
					total += estimateAnyJSONTokens(input)
				}
			case "tool_result":
				if innerContent, ok := blockMap["content"]; ok {
					total += estimateContentTokens(innerContent)
				}
			case "image":
				// 图片按固定 token 估算（Anthropic: ~1600 tokens per image tile）
				total += 1600
			default:
				// thinking, redacted_thinking 等其他类型
				if text, ok := blockMap["text"].(string); ok {
					total += subEstimateTokensForText(text)
				}
				if thinking, ok := blockMap["thinking"].(string); ok {
					total += subEstimateTokensForText(thinking)
				}
			}
		}
		return total
	}
	return 0
}

// estimateTotalPromptTokens 估算整个请求的输入上下文总 token 数
func estimateTotalPromptTokens(body []byte, parsed *ParsedRequest) int {
	total := 0

	// system
	if parsed.System != nil {
		total += estimateSystemTokens(parsed.System)
	}

	// messages
	for _, msg := range parsed.Messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if content, exists := msgMap["content"]; exists {
			total += estimateContentTokens(content)
		}
		// role + overhead per message
		total += 4
	}

	// tools
	toolsResult := gjson.GetBytes(body, "tools")
	if toolsResult.Exists() && toolsResult.IsArray() {
		total += estimateJSONTokens(toolsResult.Raw)
	}

	// tool_choice
	tcResult := gjson.GetBytes(body, "tool_choice")
	if tcResult.Exists() {
		total += estimateJSONTokens(tcResult.Raw)
	}

	// mcp_servers
	mcpResult := gjson.GetBytes(body, "mcp_servers")
	if mcpResult.Exists() && mcpResult.IsArray() {
		total += estimateJSONTokens(mcpResult.Raw)
	}

	return total
}

// estimateSystemTokens 估算 system 字段的 token 数
func estimateSystemTokens(system any) int {
	switch v := system.(type) {
	case string:
		return subEstimateTokensForText(v)
	case []any:
		total := 0
		for _, block := range v {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := blockMap["text"].(string); ok {
				total += subEstimateTokensForText(text)
			}
		}
		return total
	}
	return 0
}

// buildSubUsageSessionKey 构建会话历史 key: account_id:api_key_id:model:scope
func buildSubUsageSessionKey(accountID, apiKeyID int64, model string, parsed *ParsedRequest) string {
	scope := resolveSubUsageScope(parsed)
	return fmt.Sprintf("%d:%d:%s:%s", accountID, apiKeyID, model, scope)
}

// resolveSubUsageScope 按优先级解析 session scope
func resolveSubUsageScope(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
	}

	// 优先级 1: metadata.user_id → 解析出 session_id
	if parsed.MetadataUserID != "" {
		if uid := ParseMetadataUserID(parsed.MetadataUserID); uid != nil && uid.SessionID != "" {
			return "sid:" + uid.SessionID
		}
		// 优先级 2: metadata.user_id 存在但无法解析 → hash 原始值
		h := xxhash.Sum64String(parsed.MetadataUserID)
		return "muid:" + strconv.FormatUint(h, 36)
	}

	// 优先级 3: 可缓存内容 hash（system + 部分 messages）
	contentHash := hashSubUsageCacheableContent(parsed)
	if contentHash != "" {
		return "ch:" + contentHash
	}

	// 优先级 4: 无法识别
	return ""
}

// hashSubUsageCacheableContent 对 system + 前几条 messages 做 hash，用于识别同一会话
func hashSubUsageCacheableContent(parsed *ParsedRequest) string {
	if parsed == nil {
		return ""
	}

	var b strings.Builder
	if parsed.System != nil {
		switch v := parsed.System.(type) {
		case string:
			b.WriteString(v)
		case []any:
			for _, block := range v {
				if blockMap, ok := block.(map[string]any); ok {
					if text, ok := blockMap["text"].(string); ok {
						b.WriteString(text)
					}
				}
			}
		}
	}

	// 取前 3 条消息作为 session 指纹（避免太长）
	limit := 3
	if len(parsed.Messages) < limit {
		limit = len(parsed.Messages)
	}
	for i := 0; i < limit; i++ {
		if msgMap, ok := parsed.Messages[i].(map[string]any); ok {
			if content, exists := msgMap["content"]; exists {
				switch v := content.(type) {
				case string:
					if len(v) > 200 {
						b.WriteString(v[:200])
					} else {
						b.WriteString(v)
					}
				case []any:
					for _, block := range v {
						if bm, ok := block.(map[string]any); ok {
							if text, ok := bm["text"].(string); ok {
								if len(text) > 200 {
									b.WriteString(text[:200])
								} else {
									b.WriteString(text)
								}
							}
						}
					}
				}
			}
		}
	}

	if b.Len() == 0 {
		return ""
	}
	h := xxhash.Sum64String(b.String())
	return strconv.FormatUint(h, 36)
}

// splitCachePoolTokens 根据历史状态拆分 cache_pool 为 read 和 creation
func splitCachePoolTokens(sessionKey string, cachePoolTokens int) (cacheRead, cacheCreation int) {
	if cachePoolTokens <= 0 {
		return 0, 0
	}

	// 无法识别 session → 兜底 85/15
	if sessionKey == "" || strings.HasSuffix(sessionKey, ":") {
		cacheRead = cachePoolTokens * 85 / 100
		cacheCreation = cachePoolTokens - cacheRead
		return
	}

	// 查找历史状态
	prev := getSubUsageHistory(sessionKey)
	if prev == nil {
		// 首次请求，无历史 → 兜底 85/15
		cacheRead = cachePoolTokens * 85 / 100
		cacheCreation = cachePoolTokens - cacheRead
		return
	}

	// 有历史状态: cache_read = min(当前, 上次)
	cacheRead = prev.CachePoolTokens
	if cacheRead > cachePoolTokens {
		cacheRead = cachePoolTokens
	}
	cacheCreation = cachePoolTokens - cacheRead
	return
}

// classifyCacheCreationTTL 根据请求中是否包含 cache_control.ttl="1h" 决定归类
func classifyCacheCreationTTL(body []byte, cacheCreation int) (cache5m, cache1h int) {
	if cacheCreation <= 0 {
		return 0, 0
	}

	// 检查请求体中是否有 cache_control.ttl = "1h"
	bodyStr := string(body)
	if strings.Contains(bodyStr, `"ttl"`) {
		// 使用 gjson 精确检测
		hasTTL1h := false
		// 检查 system 中的 cache_control
		gjson.GetBytes(body, "system").ForEach(func(_, item gjson.Result) bool {
			if item.Get("cache_control.ttl").String() == "1h" {
				hasTTL1h = true
				return false
			}
			return true
		})
		if !hasTTL1h {
			// 检查 messages 中的 cache_control
			gjson.GetBytes(body, "messages").ForEach(func(_, msg gjson.Result) bool {
				msg.Get("content").ForEach(func(_, block gjson.Result) bool {
					if block.Get("cache_control.ttl").String() == "1h" {
						hasTTL1h = true
						return false
					}
					return true
				})
				return !hasTTL1h
			})
		}
		if hasTTL1h {
			return 0, cacheCreation
		}
	}
	return cacheCreation, 0
}

// getSubUsageHistory 从缓存获取历史状态
func getSubUsageHistory(key string) *subUsageHistoryEntry {
	subUsageAccountingStore.mu.Lock()
	defer subUsageAccountingStore.mu.Unlock()

	entry, exists := subUsageAccountingStore.items[key]
	if !exists {
		return nil
	}
	// 检查 TTL
	if time.Since(entry.UpdatedAt) > subUsageAccountingCacheTTL {
		delete(subUsageAccountingStore.items, key)
		return nil
	}
	return entry
}

// updateSubUsageHistory 更新缓存中的历史状态
func updateSubUsageHistory(key string, cachePoolTokens int) {
	if key == "" || strings.HasSuffix(key, ":") {
		return
	}

	subUsageAccountingStore.mu.Lock()
	defer subUsageAccountingStore.mu.Unlock()

	subUsageAccountingStore.items[key] = &subUsageHistoryEntry{
		CachePoolTokens: cachePoolTokens,
		UpdatedAt:       time.Now(),
	}
}

// cleanupSubUsageHistory 清理过期缓存条目（可由定时器调用）
func cleanupSubUsageHistory() {
	subUsageAccountingStore.mu.Lock()
	defer subUsageAccountingStore.mu.Unlock()

	now := time.Now()
	for key, entry := range subUsageAccountingStore.items {
		if now.Sub(entry.UpdatedAt) > subUsageAccountingCacheTTL {
			delete(subUsageAccountingStore.items, key)
		}
	}
}

// subEstimateTokensForText 估算文本 token 数（与 Gemini 兼容服务的逻辑一致）
func subEstimateTokensForText(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}
	ascii := 0
	for _, r := range runes {
		if r <= 0x7f {
			ascii++
		}
	}
	asciiRatio := float64(ascii) / float64(len(runes))
	if asciiRatio >= 0.8 {
		// 英文为主：约 4 字符/token
		return (len(runes) + 3) / 4
	}
	// CJK 为主：约 1 字符/token
	return len(runes)
}

// estimateJSONTokens 估算 JSON 字符串的 token 数
func estimateJSONTokens(jsonStr string) int {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return 0
	}
	// JSON 结构化内容：约 3-4 chars/token（含标点和 key）
	return (len(jsonStr) + 3) / 4
}

// estimateAnyJSONTokens 估算任意 JSON 值的 token 数
func estimateAnyJSONTokens(v any) int {
	switch val := v.(type) {
	case string:
		return subEstimateTokensForText(val)
	case map[string]any:
		total := 2 // {} overhead
		for k, child := range val {
			total += subEstimateTokensForText(k) + 1 // key + colon
			total += estimateAnyJSONTokens(child)
		}
		return total
	case []any:
		total := 2 // [] overhead
		for _, child := range val {
			total += estimateAnyJSONTokens(child)
		}
		return total
	case float64:
		return 1
	case bool:
		return 1
	case nil:
		return 1
	default:
		return 1
	}
}

func init() {
	// 启动后台定期清理过期的 sub-usage history 缓存
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			cleanupSubUsageHistory()
		}
	}()
}

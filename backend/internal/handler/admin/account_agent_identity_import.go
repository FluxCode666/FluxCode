package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type AgentIdentityImportRequest struct {
	Content                 string         `json:"content" binding:"required"`
	Name                    string         `json:"name"`
	Notes                   *string        `json:"notes"`
	GroupIDs                []int64        `json:"group_ids"`
	ProxyID                 *int64         `json:"proxy_id"`
	Concurrency             *int           `json:"concurrency"`
	Priority                *int           `json:"priority"`
	RateMultiplier          *float64       `json:"rate_multiplier"`
	LoadFactor              *int           `json:"load_factor"`
	Extra                   map[string]any `json:"extra"`
	UpdateExisting          *bool          `json:"update_existing"`
	ConfirmMixedChannelRisk *bool          `json:"confirm_mixed_channel_risk"`
}

type AgentIdentityImportResult struct {
	Total   int                       `json:"total"`
	Created int                       `json:"created"`
	Updated int                       `json:"updated"`
	Failed  int                       `json:"failed"`
	Items   []AgentIdentityImportItem `json:"items"`
}

type AgentIdentityImportItem struct {
	Index     int    `json:"index"`
	Name      string `json:"name,omitempty"`
	Action    string `json:"action"`
	AccountID int64  `json:"account_id,omitempty"`
	Message   string `json:"message,omitempty"`
}

type agentIdentityImportAccount struct {
	RuntimeID        string
	PrivateKey       string
	TaskID           string
	ChatGPTAccountID string
	ChatGPTUserID    string
	Email            string
	PlanType         string
	FedRAMP          bool
}

// ImportAgentIdentity imports one or more Codex Agent Identity auth.json
// objects without persisting OAuth access, refresh, or ID tokens.
func (h *AccountHandler) ImportAgentIdentity(c *gin.Context) {
	var req AgentIdentityImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Concurrency != nil && *req.Concurrency < 0 {
		response.BadRequest(c, "concurrency must be >= 0")
		return
	}
	if req.Priority != nil && *req.Priority < 0 {
		response.BadRequest(c, "priority must be >= 0")
		return
	}
	if req.RateMultiplier != nil && *req.RateMultiplier < 0 {
		response.BadRequest(c, "rate_multiplier must be >= 0")
		return
	}
	if req.LoadFactor != nil && *req.LoadFactor > 10000 {
		response.BadRequest(c, "load_factor must be <= 10000")
		return
	}
	values, err := parseAgentIdentityImportContent(req.Content)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	executeAdminIdempotentJSON(c, "admin.accounts.import_agent_identity", req, service.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.importAgentIdentityAccounts(ctx, req, values)
	})
}

func (h *AccountHandler) importAgentIdentityAccounts(ctx context.Context, req AgentIdentityImportRequest, values []any) (AgentIdentityImportResult, error) {
	result := AgentIdentityImportResult{
		Total: len(values),
		Items: make([]AgentIdentityImportItem, 0, len(values)),
	}
	existingAccounts, err := h.listAccountsFiltered(ctx, service.PlatformOpenAI, service.AccountTypeOAuth, "", "", 0, "", "created_at", "desc")
	if err != nil {
		return result, err
	}
	updateExisting := req.UpdateExisting == nil || *req.UpdateExisting
	skipMixedChannelCheck := req.ConfirmMixedChannelRisk != nil && *req.ConfirmMixedChannelRisk
	seen := make(map[string]struct{}, len(values))

	for index, value := range values {
		itemIndex := index + 1
		item, normalizeErr := normalizeAgentIdentityImportValue(value)
		if normalizeErr != nil {
			result.Failed++
			result.Items = append(result.Items, AgentIdentityImportItem{Index: itemIndex, Action: "failed", Message: normalizeErr.Error()})
			continue
		}
		identityKey := agentIdentityImportMatchKey(item.ChatGPTAccountID, item.ChatGPTUserID)
		if _, duplicate := seen[identityKey]; duplicate {
			result.Failed++
			result.Items = append(result.Items, AgentIdentityImportItem{Index: itemIndex, Action: "failed", Message: "同一批次中存在重复的 Agent Identity Team/用户"})
			continue
		}
		seen[identityKey] = struct{}{}

		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = firstNonEmptyAgentIdentityImportString(item.Email, item.ChatGPTAccountID, item.ChatGPTUserID, "Agent Identity")
		} else if len(values) > 1 {
			name = fmt.Sprintf("%s #%d", name, itemIndex)
		}
		credentials := buildAgentIdentityImportCredentials(item)
		extra := cloneAgentIdentityImportMap(req.Extra)
		extra["import_source"] = "agent_identity"
		extra["imported_at"] = time.Now().UTC().Format(time.RFC3339)

		existing := findAgentIdentityImportAccount(existingAccounts, item.ChatGPTAccountID, item.ChatGPTUserID)
		if existing != nil && updateExisting {
			mergedCredentials := mergeAgentIdentityImportCredentials(existing.Credentials, credentials, item.TaskID != "")
			mergedExtra := cloneAgentIdentityImportMap(existing.Extra)
			for key, value := range extra {
				mergedExtra[key] = value
			}
			updateInput := &service.UpdateAccountInput{
				Credentials:           mergedCredentials,
				Extra:                 mergedExtra,
				ProxyID:               req.ProxyID,
				Concurrency:           req.Concurrency,
				Priority:              req.Priority,
				RateMultiplier:        req.RateMultiplier,
				LoadFactor:            req.LoadFactor,
				SkipMixedChannelCheck: skipMixedChannelCheck,
			}
			if len(req.GroupIDs) > 0 {
				groupIDs := append([]int64(nil), req.GroupIDs...)
				updateInput.GroupIDs = &groupIDs
			}
			updated, updateErr := h.adminService.UpdateAccount(ctx, existing.ID, updateInput)
			if updateErr != nil {
				result.Failed++
				result.Items = append(result.Items, AgentIdentityImportItem{Index: itemIndex, Name: name, Action: "failed", Message: updateErr.Error()})
				continue
			}
			if h.tokenCacheInvalidator != nil && updated != nil {
				_ = h.tokenCacheInvalidator.InvalidateToken(ctx, updated)
			}
			result.Updated++
			accountID := existing.ID
			if updated != nil {
				accountID = updated.ID
				for accountIndex := range existingAccounts {
					if existingAccounts[accountIndex].ID == updated.ID {
						existingAccounts[accountIndex] = *updated
					}
				}
			}
			result.Items = append(result.Items, AgentIdentityImportItem{Index: itemIndex, Name: name, Action: "updated", AccountID: accountID})
			continue
		}

		concurrency := 3
		if req.Concurrency != nil {
			concurrency = *req.Concurrency
		}
		priority := 50
		if req.Priority != nil {
			priority = *req.Priority
		}
		created, createErr := h.adminService.CreateAccount(ctx, &service.CreateAccountInput{
			Name:                  name,
			Notes:                 req.Notes,
			Platform:              service.PlatformOpenAI,
			Type:                  service.AccountTypeOAuth,
			Credentials:           credentials,
			Extra:                 extra,
			ProxyID:               req.ProxyID,
			Concurrency:           concurrency,
			Priority:              priority,
			RateMultiplier:        req.RateMultiplier,
			LoadFactor:            req.LoadFactor,
			GroupIDs:              append([]int64(nil), req.GroupIDs...),
			SkipMixedChannelCheck: skipMixedChannelCheck,
		})
		if createErr != nil {
			result.Failed++
			result.Items = append(result.Items, AgentIdentityImportItem{Index: itemIndex, Name: name, Action: "failed", Message: createErr.Error()})
			continue
		}
		result.Created++
		createdID := int64(0)
		if created != nil {
			createdID = created.ID
			existingAccounts = append(existingAccounts, *created)
		}
		result.Items = append(result.Items, AgentIdentityImportItem{Index: itemIndex, Name: name, Action: "created", AccountID: createdID})
	}
	return result, nil
}

func parseAgentIdentityImportContent(content string) ([]any, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("请输入 Agent Identity auth.json")
	}
	values, err := decodeAgentIdentityJSONValues(content)
	if err == nil {
		return flattenAgentIdentityImportValues(values), nil
	}

	lines := strings.Split(content, "\n")
	out := make([]any, 0, len(lines))
	for lineIndex, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineValues, lineErr := decodeAgentIdentityJSONValues(line)
		if lineErr != nil {
			return nil, fmt.Errorf("第 %d 行 JSON 解析失败: %w", lineIndex+1, lineErr)
		}
		out = append(out, flattenAgentIdentityImportValues(lineValues)...)
	}
	if len(out) == 0 {
		return nil, errors.New("请输入 Agent Identity auth.json")
	}
	return out, nil
}

func decodeAgentIdentityJSONValues(content string) ([]any, error) {
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.UseNumber()
	values := make([]any, 0, 1)
	for {
		var value any
		err := decoder.Decode(&value)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("JSON 内容为空")
	}
	return values, nil
}

func flattenAgentIdentityImportValues(values []any) []any {
	out := make([]any, 0, len(values))
	var appendValue func(any)
	appendValue = func(value any) {
		if array, ok := value.([]any); ok {
			for _, item := range array {
				appendValue(item)
			}
			return
		}
		out = append(out, value)
	}
	for _, value := range values {
		appendValue(value)
	}
	return out
}

func normalizeAgentIdentityImportValue(value any) (agentIdentityImportAccount, error) {
	root, ok := value.(map[string]any)
	if !ok {
		return agentIdentityImportAccount{}, errors.New("Agent Identity 导入项必须是 JSON 对象")
	}
	authMode := firstAgentIdentityImportString(root, []string{"auth_mode"}, []string{"authMode"})
	nested, nestedOK := firstAgentIdentityImportMap(root, []string{"agent_identity"}, []string{"agentIdentity"})
	if !nestedOK {
		nested = root
	}
	if !strings.EqualFold(strings.TrimSpace(authMode), service.OpenAIAuthModeAgentIdentity) && !nestedOK {
		return agentIdentityImportAccount{}, errors.New("auth.json 必须使用 auth_mode=agentIdentity")
	}
	item := agentIdentityImportAccount{
		RuntimeID:        firstAgentIdentityImportString(nested, []string{"agent_runtime_id"}, []string{"agentRuntimeId"}),
		PrivateKey:       firstAgentIdentityImportString(nested, []string{"agent_private_key"}, []string{"agentPrivateKey"}),
		TaskID:           firstAgentIdentityImportString(nested, []string{"task_id"}, []string{"taskId"}),
		ChatGPTAccountID: firstAgentIdentityImportString(nested, []string{"chatgpt_account_id"}, []string{"chatgptAccountId"}, []string{"account_id"}, []string{"accountId"}),
		ChatGPTUserID:    firstAgentIdentityImportString(nested, []string{"chatgpt_user_id"}, []string{"chatgptUserId"}, []string{"user_id"}, []string{"userId"}),
		Email:            firstAgentIdentityImportString(nested, []string{"email"}),
		PlanType:         firstAgentIdentityImportString(nested, []string{"plan_type"}, []string{"planType"}),
		FedRAMP:          firstAgentIdentityImportBool(nested, []string{"chatgpt_account_is_fedramp"}, []string{"chatgptAccountIsFedramp"}),
	}
	if item.RuntimeID == "" || item.PrivateKey == "" || item.ChatGPTAccountID == "" || item.ChatGPTUserID == "" {
		return agentIdentityImportAccount{}, errors.New("Agent Identity 缺少 agent_runtime_id、agent_private_key、account_id 或 chatgpt_user_id")
	}
	if err := service.ValidateOpenAIAgentIdentityPrivateKey(item.PrivateKey); err != nil {
		return agentIdentityImportAccount{}, errors.New("Agent Identity private key 格式无效")
	}
	return item, nil
}

func buildAgentIdentityImportCredentials(item agentIdentityImportAccount) map[string]any {
	credentials := map[string]any{
		"auth_mode":                  service.OpenAIAuthModeAgentIdentity,
		"agent_runtime_id":           item.RuntimeID,
		"agent_private_key":          item.PrivateKey,
		"chatgpt_account_id":         item.ChatGPTAccountID,
		"chatgpt_user_id":            item.ChatGPTUserID,
		"chatgpt_account_is_fedramp": item.FedRAMP,
	}
	if item.TaskID != "" {
		credentials["task_id"] = item.TaskID
	}
	if item.Email != "" {
		credentials["email"] = item.Email
	}
	if item.PlanType != "" {
		credentials["plan_type"] = item.PlanType
	}
	return credentials
}

func mergeAgentIdentityImportCredentials(existing, incoming map[string]any, hasTaskID bool) map[string]any {
	out := cloneAgentIdentityImportMap(existing)
	for _, key := range []string{"access_token", "refresh_token", "id_token", "client_id", "token_type", "expires_at"} {
		delete(out, key)
	}
	if !hasTaskID {
		delete(out, "task_id")
	}
	for key, value := range incoming {
		out[key] = value
	}
	return out
}

func findAgentIdentityImportAccount(accounts []service.Account, accountID, userID string) *service.Account {
	accountID = strings.TrimSpace(accountID)
	userID = strings.TrimSpace(userID)
	for index := range accounts {
		account := &accounts[index]
		if !account.IsOpenAIAgentIdentity() || strings.TrimSpace(account.GetCredential("chatgpt_account_id")) != accountID {
			continue
		}
		storedUserID := strings.TrimSpace(account.GetCredential("chatgpt_user_id"))
		if userID != "" && storedUserID != "" && storedUserID != userID {
			continue
		}
		return account
	}
	return nil
}

func agentIdentityImportMatchKey(accountID, userID string) string {
	return strings.TrimSpace(accountID) + "\x00" + strings.TrimSpace(userID)
}

func cloneAgentIdentityImportMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input)+2)
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmptyAgentIdentityImportString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func firstAgentIdentityImportString(object map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if value, ok := agentIdentityImportPathValue(object, path); ok {
			if stringValue, ok := value.(string); ok && strings.TrimSpace(stringValue) != "" {
				return strings.TrimSpace(stringValue)
			}
		}
	}
	return ""
}

func firstAgentIdentityImportMap(object map[string]any, paths ...[]string) (map[string]any, bool) {
	for _, path := range paths {
		if value, ok := agentIdentityImportPathValue(object, path); ok {
			mapped, mappedOK := value.(map[string]any)
			if mappedOK {
				return mapped, true
			}
		}
	}
	return nil, false
}

func firstAgentIdentityImportBool(object map[string]any, paths ...[]string) bool {
	for _, path := range paths {
		if value, ok := agentIdentityImportPathValue(object, path); ok {
			switch typed := value.(type) {
			case bool:
				return typed
			case string:
				return strings.EqualFold(strings.TrimSpace(typed), "true")
			}
		}
	}
	return false
}

func agentIdentityImportPathValue(object map[string]any, path []string) (any, bool) {
	var current any = object
	for _, key := range path {
		mapped, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapped[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

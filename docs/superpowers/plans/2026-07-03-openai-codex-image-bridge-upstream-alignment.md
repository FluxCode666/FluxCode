# OpenAI Codex Image Bridge Upstream Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Align local OpenAI Codex `image_generation` bridge behavior with upstream semantics without using git merge, cherry-pick, or copying upstream implementation blocks.

**Architecture:** Add local intent and bridge-policy helpers, then wire them into HTTP `/v1/responses` and WS ingress. Default behavior must not inject `image_generation`; bridge injection only happens when policy enables it and group image generation is allowed.

**Tech Stack:** Go, Gin, Ent, SQL migrations, `gjson`, existing OpenAI gateway service tests.

---

## Current Context

The workspace may still contain a temporary no-op change in:

- `backend/internal/service/openai_codex_transform.go`
- `backend/internal/service/openai_codex_transform_test.go`

Do not preserve that no-op behavior as the final implementation. Replace it with the policy-gated behavior described here.

The deleted document `docs/superpowers/specs/2026-07-02-openai-responses-image-generation-account-whitelist-design.md` is intentionally out of scope. Do not reintroduce account-level `gpt-image-*` whitelist scheduling in this implementation.

Run Go commands from `backend/`. Run git commands from the repository root `/Volumes/T7/project/new/FluxCode`.

## File Structure

- Create `backend/internal/service/image_generation_intent.go`: request intent helpers, group gate helper, and stable permission message.
- Create `backend/internal/service/codex_image_generation_bridge.go`: bridge policy override parsing for account, channel, and global config.
- Modify `backend/internal/config/config.go`: add `GatewayConfig.CodexImageGenerationBridgeEnabled` and default `false`.
- Modify group model files to add `AllowImageGeneration`:
  - `backend/internal/service/group.go`
  - `backend/ent/schema/group.go`
  - generated Ent files after running generation if the repo requires it
  - repository/handler/DTO mappers that create, update, select, or serialize groups
- Create migration `backend/migrations/125_add_group_allow_image_generation.sql`.
- Modify `backend/internal/service/openai_codex_transform.go`: restore image tool ensure helper, add `tool_choice:auto`, add Spark tool stripping.
- Modify `backend/internal/service/openai_gateway_service.go`: gate explicit image intent, policy-gate bridge injection, exempt compact requests, strip Spark tools.
- Modify `backend/internal/service/openai_ws_forwarder.go`: apply the same policy to WS ingress payloads.
- Add/modify tests in focused files:
  - `backend/internal/service/openai_codex_transform_test.go`
  - `backend/internal/service/openai_image_generation_intent_test.go`
  - `backend/internal/service/codex_image_generation_bridge_test.go`
  - `backend/internal/service/openai_image_generation_controls_test.go`
  - `backend/internal/service/openai_ws_forwarder_success_test.go`

---

### Task 1: Add Group Image Permission Field

**Files:**
- Modify: `backend/ent/schema/group.go`
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/handler/admin/group_handler.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/repository/group_repo.go`
- Modify: `backend/internal/repository/api_key_repo.go`
- Create: `backend/migrations/125_add_group_allow_image_generation.sql`
- Test: existing group/admin/API key tests touched by compilation

- [ ] **Step 1: Write failing DTO/model compilation tests**

Add focused assertions to existing group service/admin tests so `AllowImageGeneration` must round-trip through create/update/list DTO paths. If there is already a group create/update test in `backend/internal/service/admin_service_group_test.go`, extend it with:

```go
require.True(t, group.AllowImageGeneration)
require.True(t, repo.created.AllowImageGeneration)
```

For the update test, include:

```go
allowImageGeneration := true
input.AllowImageGeneration = &allowImageGeneration
require.True(t, repo.updated.AllowImageGeneration)
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/service -run 'TestAdminService.*Group|Test.*Group' -count=1
```

Expected: FAIL or compile error because `AllowImageGeneration` does not exist on service group structs/requests yet.

- [ ] **Step 3: Add Ent schema field and migration**

In `backend/ent/schema/group.go`, add the field near image-related group fields:

```go
field.Bool("allow_image_generation").
	Default(false).
	Comment("是否允许该分组使用图片生成能力"),
```

Create `backend/migrations/125_add_group_allow_image_generation.sql`:

```sql
-- 125_add_group_allow_image_generation.sql
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS allow_image_generation BOOLEAN NOT NULL DEFAULT FALSE;
```

- [ ] **Step 4: Add service and DTO fields**

In `backend/internal/service/group.go`, add:

```go
AllowImageGeneration bool
```

near the image pricing fields.

In `backend/internal/handler/admin/group_handler.go`, add:

```go
AllowImageGeneration bool `json:"allow_image_generation"`
```

to `CreateGroupRequest`, and:

```go
AllowImageGeneration *bool `json:"allow_image_generation"`
```

to `UpdateGroupRequest`.

In `backend/internal/handler/dto/types.go`, add:

```go
AllowImageGeneration bool `json:"allow_image_generation"`
```

to the group response DTO.

- [ ] **Step 5: Wire repository create/update/select/mappers**

In `backend/internal/repository/group_repo.go`, add:

```go
SetAllowImageGeneration(groupIn.AllowImageGeneration)
```

to create and update builders.

Where group fields are selected, include:

```go
group.FieldAllowImageGeneration,
```

Where Ent group models are converted to service groups, assign:

```go
AllowImageGeneration: entGroup.AllowImageGeneration,
```

In `backend/internal/repository/api_key_repo.go`, add `group.FieldAllowImageGeneration` to group eager-load selects and map it onto the attached service group.

In DTO mappers, assign:

```go
AllowImageGeneration: g.AllowImageGeneration,
```

- [ ] **Step 6: Generate Ent code**

Run:

```bash
go generate ./ent
```

Expected: generated `ent/group` files gain `allow_image_generation`.

- [ ] **Step 7: Verify group field tests pass**

Run:

```bash
go test ./internal/service ./internal/repository ./internal/handler -run 'Test.*Group|Test.*APIKey' -count=1
```

Expected: relevant tests PASS. If unrelated handler/repository tests require DB services not present locally, rerun the narrower failing package/test names and document the limitation.

- [ ] **Step 8: Commit**

```bash
git add backend/ent backend/internal/service/group.go backend/internal/handler/admin/group_handler.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/repository/group_repo.go backend/internal/repository/api_key_repo.go backend/migrations/125_add_group_allow_image_generation.sql
git commit -m "feat: add group image generation permission"
```

---

### Task 2: Add Image Generation Intent Helpers

**Files:**
- Create: `backend/internal/service/image_generation_intent.go`
- Test: `backend/internal/service/openai_image_generation_intent_test.go`

- [ ] **Step 1: Write failing tests**

Create `backend/internal/service/openai_image_generation_intent_test.go`:

```go
package service

import "testing"

func TestGroupAllowsImageGeneration(t *testing.T) {
	if !GroupAllowsImageGeneration(nil) {
		t.Fatalf("nil group must preserve existing ungrouped-key allow behavior")
	}
	if GroupAllowsImageGeneration(&Group{AllowImageGeneration: false}) {
		t.Fatalf("group with AllowImageGeneration=false must deny image generation")
	}
	if !GroupAllowsImageGeneration(&Group{AllowImageGeneration: true}) {
		t.Fatalf("group with AllowImageGeneration=true must allow image generation")
	}
}

func TestIsImageGenerationIntent(t *testing.T) {
	cases := []struct {
		name           string
		endpoint       string
		requestedModel string
		body           []byte
		want           bool
	}{
		{name: "images endpoint", endpoint: "/v1/images/generations", want: true},
		{name: "image requested model", endpoint: "/v1/responses", requestedModel: "gpt-image-2", want: true},
		{name: "body model image", endpoint: "/v1/responses", body: []byte(`{"model":"gpt-image-2"}`), want: true},
		{name: "image tool", endpoint: "/v1/responses", body: []byte(`{"model":"gpt-5.5","tools":[{"type":"image_generation"}]}`), want: true},
		{name: "tool choice string", endpoint: "/v1/responses", body: []byte(`{"tool_choice":"image_generation"}`), want: true},
		{name: "tool choice object", endpoint: "/v1/responses", body: []byte(`{"tool_choice":{"type":"image_generation"}}`), want: true},
		{name: "plain text", endpoint: "/v1/responses", requestedModel: "gpt-5.5", body: []byte(`{"input":"write code"}`), want: false},
		{name: "invalid json", endpoint: "/v1/responses", requestedModel: "gpt-5.5", body: []byte(`{bad`), want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsImageGenerationIntent(tt.endpoint, tt.requestedModel, tt.body); got != tt.want {
				t.Fatalf("IsImageGenerationIntent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsImageGenerationIntentMap(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{map[string]any{"type": "image_generation"}},
	}
	if !IsImageGenerationIntentMap("/v1/responses", "gpt-5.5", reqBody) {
		t.Fatalf("map with image_generation tool must be image intent")
	}
	if IsImageGenerationIntentMap("/v1/responses", "gpt-5.5", map[string]any{"input": "write code"}) {
		t.Fatalf("plain map must not be image intent")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/service -run 'Test(GroupAllowsImageGeneration|IsImageGenerationIntent)' -count=1
```

Expected: FAIL with undefined helper functions.

- [ ] **Step 3: Implement helper**

Create `backend/internal/service/image_generation_intent.go`:

```go
package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const imageGenerationPermissionMessage = "Image generation is not enabled for this group"

func ImageGenerationPermissionMessage() string {
	return imageGenerationPermissionMessage
}

func GroupAllowsImageGeneration(group *Group) bool {
	return group == nil || group.AllowImageGeneration
}

func IsImageGenerationIntent(endpoint string, requestedModel string, body []byte) bool {
	if isImageGenerationEndpoint(endpoint) {
		return true
	}
	if isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	if model := strings.TrimSpace(gjson.GetBytes(body, "model").String()); isOpenAIImageGenerationModel(model) {
		return true
	}
	if openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools")) {
		return true
	}
	return openAIJSONToolChoiceSelectsImageGeneration(gjson.GetBytes(body, "tool_choice"))
}

func IsImageGenerationIntentMap(endpoint string, requestedModel string, reqBody map[string]any) bool {
	if isImageGenerationEndpoint(endpoint) || isOpenAIImageGenerationModel(requestedModel) {
		return true
	}
	if reqBody == nil {
		return false
	}
	if isOpenAIImageGenerationModel(firstNonEmptyString(reqBody["model"])) {
		return true
	}
	if hasOpenAIImageGenerationTool(reqBody) {
		return true
	}
	return openAIAnyToolChoiceSelectsImageGeneration(reqBody["tool_choice"])
}

func isImageGenerationEndpoint(endpoint string) bool {
	switch normalizeImageGenerationEndpoint(endpoint) {
	case "/v1/images/generations", "/v1/images/edits", "/images/generations", "/images/edits":
		return true
	default:
		return false
	}
}

func normalizeImageGenerationEndpoint(endpoint string) string {
	endpoint = strings.TrimSpace(strings.ToLower(endpoint))
	endpoint = strings.TrimPrefix(endpoint, "https://api.openai.com")
	if idx := strings.IndexByte(endpoint, '?'); idx >= 0 {
		endpoint = endpoint[:idx]
	}
	return strings.TrimRight(endpoint, "/")
}

func openAIJSONToolsContainImageGeneration(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) == "image_generation" {
			found = true
			return false
		}
		return true
	})
	return found
}

func openAIRequestBodyHasImageGenerationTool(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	return openAIJSONToolsContainImageGeneration(gjson.GetBytes(body, "tools"))
}

func openAIRequestBodyImageGenerationToolNeedsNormalization(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return false
	}
	needsNormalization := false
	tools.ForEach(func(_, item gjson.Result) bool {
		if strings.TrimSpace(item.Get("type").String()) != "image_generation" {
			return true
		}
		if item.Get("format").Exists() || item.Get("compression").Exists() {
			needsNormalization = true
			return false
		}
		return true
	})
	return needsNormalization
}

func openAIJSONToolChoiceSelectsImageGeneration(choice gjson.Result) bool {
	if !choice.Exists() {
		return false
	}
	if choice.Type == gjson.String {
		return strings.TrimSpace(choice.String()) == "image_generation"
	}
	if !choice.IsObject() {
		return false
	}
	if strings.TrimSpace(choice.Get("type").String()) == "image_generation" {
		return true
	}
	if strings.TrimSpace(choice.Get("tool.type").String()) == "image_generation" {
		return true
	}
	if strings.TrimSpace(choice.Get("function.name").String()) == "image_generation" {
		return true
	}
	return false
}

func openAIAnyToolChoiceSelectsImageGeneration(choice any) bool {
	switch v := choice.(type) {
	case string:
		return strings.TrimSpace(v) == "image_generation"
	case map[string]any:
		if strings.TrimSpace(firstNonEmptyString(v["type"])) == "image_generation" {
			return true
		}
		if tool, ok := v["tool"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(tool["type"])) == "image_generation" {
			return true
		}
		if fn, ok := v["function"].(map[string]any); ok && strings.TrimSpace(firstNonEmptyString(fn["name"])) == "image_generation" {
			return true
		}
	}
	return false
}

func getAPIKeyFromContext(c interface{ Get(string) (any, bool) }) *APIKey {
	if c == nil {
		return nil
	}
	v, exists := c.Get("api_key")
	if !exists {
		return nil
	}
	apiKey, _ := v.(*APIKey)
	return apiKey
}

func apiKeyGroup(apiKey *APIKey) *Group {
	if apiKey == nil {
		return nil
	}
	return apiKey.Group
}
```

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
go test ./internal/service -run 'Test(GroupAllowsImageGeneration|IsImageGenerationIntent)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/image_generation_intent.go backend/internal/service/openai_image_generation_intent_test.go
git commit -m "feat: add openai image generation intent helpers"
```

---

### Task 3: Add Codex Image Bridge Policy Helper

**Files:**
- Create: `backend/internal/service/codex_image_generation_bridge.go`
- Modify: `backend/internal/config/config.go`
- Test: `backend/internal/service/codex_image_generation_bridge_test.go`

- [ ] **Step 1: Write failing policy tests**

Create `backend/internal/service/codex_image_generation_bridge_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func TestCodexImageGenerationBridgeOverridePrecedence(t *testing.T) {
	groupID := int64(7)
	tests := []struct {
		name    string
		global  bool
		channel *Channel
		account *Account
		want    bool
	}{
		{name: "global enabled", global: true, account: &Account{Platform: PlatformOpenAI}, want: true},
		{name: "channel disables global", global: true, channel: &Channel{FeaturesConfig: map[string]any{"codex_image_generation_bridge": map[string]any{PlatformOpenAI: false}}}, account: &Account{Platform: PlatformOpenAI}, want: false},
		{name: "channel enables global disabled", global: false, channel: &Channel{FeaturesConfig: map[string]any{"codex_image_generation_bridge": map[string]any{PlatformOpenAI: true}}}, account: &Account{Platform: PlatformOpenAI}, want: true},
		{name: "account disables channel", global: true, channel: &Channel{FeaturesConfig: map[string]any{"codex_image_generation_bridge": true}}, account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{"codex_image_generation_bridge": false}}, want: false},
		{name: "account nested enables", global: false, account: &Account{Platform: PlatformOpenAI, Extra: map[string]any{PlatformOpenAI: map[string]any{"codex_image_generation_bridge_enabled": true}}}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var channelService *ChannelService
			if tt.channel != nil {
				tt.channel.GroupIDs = []int64{groupID}
				tt.channel.Status = StatusActive
				channelService = newTestChannelService(makeStandardRepo(*tt.channel, map[int64]string{groupID: PlatformOpenAI}))
			}
			svc := &OpenAIGatewayService{
				cfg:            &config.Config{Gateway: config.GatewayConfig{CodexImageGenerationBridgeEnabled: tt.global}},
				channelService: channelService,
			}
			apiKey := &APIKey{GroupID: &groupID}
			if got := svc.isCodexImageGenerationBridgeEnabled(context.Background(), tt.account, apiKey); got != tt.want {
				t.Fatalf("isCodexImageGenerationBridgeEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/service -run TestCodexImageGenerationBridgeOverridePrecedence -count=1
```

Expected: FAIL with missing `CodexImageGenerationBridgeEnabled` or helper methods.

- [ ] **Step 3: Add config field and default**

In `backend/internal/config/config.go`, add to `GatewayConfig` near `ForceCodexCLI`:

```go
// CodexImageGenerationBridgeEnabled controls whether Codex /v1/responses requests
// may receive an automatic image_generation bridge. Default false.
CodexImageGenerationBridgeEnabled bool `mapstructure:"codex_image_generation_bridge_enabled"`
```

In the default section near `gateway.force_codex_cli`, add:

```go
viper.SetDefault("gateway.codex_image_generation_bridge_enabled", false)
```

- [ ] **Step 4: Implement local policy helper**

Create `backend/internal/service/codex_image_generation_bridge.go`:

```go
package service

import (
	"context"
	"log/slog"
	"strings"
)

const featureKeyCodexImageGenerationBridge = "codex_image_generation_bridge"

func boolOverride(v bool) *bool {
	return &v
}

func boolOverrideFromMap(values map[string]any, keys ...string) *bool {
	if values == nil {
		return nil
	}
	for _, key := range keys {
		if v, ok := values[key].(bool); ok {
			return boolOverride(v)
		}
	}
	return nil
}

func platformBoolOverride(values map[string]any, key string, platform string) *bool {
	if values == nil {
		return nil
	}
	if v, ok := values[key].(bool); ok {
		return boolOverride(v)
	}
	nested, _ := values[key].(map[string]any)
	if nested == nil {
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil
	}
	if v, ok := nested[platform].(bool); ok {
		return boolOverride(v)
	}
	return nil
}

func (c *Channel) CodexImageGenerationBridgeOverride(platform string) *bool {
	if c == nil {
		return nil
	}
	return platformBoolOverride(c.FeaturesConfig, featureKeyCodexImageGenerationBridge, platform)
}

func (a *Account) CodexImageGenerationBridgeOverride() *bool {
	if a == nil || a.Platform != PlatformOpenAI {
		return nil
	}
	if override := boolOverrideFromMap(a.Extra, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled"); override != nil {
		return override
	}
	openaiConfig, _ := a.Extra[PlatformOpenAI].(map[string]any)
	return boolOverrideFromMap(openaiConfig, featureKeyCodexImageGenerationBridge, "codex_image_generation_bridge_enabled")
}

func (s *OpenAIGatewayService) isCodexImageGenerationBridgeEnabled(ctx context.Context, account *Account, apiKey *APIKey) bool {
	if override := account.CodexImageGenerationBridgeOverride(); override != nil {
		return *override
	}
	if s != nil && s.channelService != nil && apiKey != nil && apiKey.GroupID != nil {
		ch, err := s.channelService.GetChannelForGroup(ctx, *apiKey.GroupID)
		if err != nil {
			slog.Warn("failed to resolve codex image generation bridge channel override", "group_id", *apiKey.GroupID, "error", err)
		} else if override := ch.CodexImageGenerationBridgeOverride(PlatformOpenAI); override != nil {
			return *override
		}
	}
	return s != nil && s.cfg != nil && s.cfg.Gateway.CodexImageGenerationBridgeEnabled
}
```

- [ ] **Step 5: Run policy tests**

Run:

```bash
go test ./internal/service -run TestCodexImageGenerationBridgeOverridePrecedence -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/config/config.go backend/internal/service/codex_image_generation_bridge.go backend/internal/service/codex_image_generation_bridge_test.go
git commit -m "feat: add codex image bridge policy"
```

---

### Task 4: Restore Transform Helpers With Policy-Gated Semantics

**Files:**
- Modify: `backend/internal/service/openai_codex_transform.go`
- Modify: `backend/internal/service/openai_codex_transform_test.go`

- [ ] **Step 1: Replace the temporary no-op test with upstream-aligned transform tests**

In `backend/internal/service/openai_codex_transform_test.go`, remove `TestEnsureOpenAIResponsesImageGenerationTool_DoesNotInjectForPlainTextRequest` and add:

```go
func TestEnsureOpenAIResponsesImageGenerationTool_NoTools(t *testing.T) {
	reqBody := map[string]any{"model": "gpt-5.5", "input": "write code"}
	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.True(t, modified)
	require.True(t, hasOpenAIImageGenerationTool(reqBody))
	require.Equal(t, "png", reqBody["tools"].([]any)[0].(map[string]any)["output_format"])
}

func TestEnsureOpenAIResponsesImageGenerationTool_AppendsToExistingTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{map[string]any{"type": "web_search"}},
	}
	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.True(t, modified)
	tools := reqBody["tools"].([]any)
	require.Len(t, tools, 2)
	require.Equal(t, "web_search", tools[0].(map[string]any)["type"])
	require.Equal(t, "image_generation", tools[1].(map[string]any)["type"])
}

func TestEnsureOpenAIResponsesImageGenerationTool_PreservesExistingImageTool(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{map[string]any{"type": "image_generation", "output_format": "webp"}},
	}
	modified := ensureOpenAIResponsesImageGenerationTool(reqBody)
	require.False(t, modified)
	require.Equal(t, "webp", reqBody["tools"].([]any)[0].(map[string]any)["output_format"])
}

func TestEnsureOpenAIResponsesImageGenerationToolChoiceAuto(t *testing.T) {
	reqBody := map[string]any{"tools": []any{map[string]any{"type": "image_generation"}}}
	modified := ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody)
	require.True(t, modified)
	require.Equal(t, "auto", reqBody["tool_choice"])

	modified = ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody)
	require.False(t, modified)
	require.Equal(t, "auto", reqBody["tool_choice"])
}

func TestStripCodexSparkImageGenerationTools(t *testing.T) {
	reqBody := map[string]any{
		"model": "gpt-5.3-codex-spark",
		"tools": []any{
			map[string]any{"type": "image_generation"},
			map[string]any{"type": "web_search"},
		},
	}
	require.True(t, stripCodexSparkImageGenerationTools(reqBody))
	require.False(t, hasOpenAIImageGenerationTool(reqBody))
	require.Len(t, reqBody["tools"].([]any), 1)
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/service -run 'TestEnsureOpenAIResponsesImageGenerationTool|TestStripCodexSparkImageGenerationTools' -count=1
```

Expected: FAIL because current no-op does not add tools and new helpers do not exist.

- [ ] **Step 3: Implement transform helpers**

In `backend/internal/service/openai_codex_transform.go`, replace the temporary no-op `ensureOpenAIResponsesImageGenerationTool` with:

```go
func ensureOpenAIResponsesImageGenerationTool(reqBody map[string]any) bool {
	if len(reqBody) == 0 || isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
	}
	tool := map[string]any{
		"type":          "image_generation",
		"output_format": "png",
	}
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		reqBody["tools"] = []any{tool}
		return true
	}
	tools, ok := rawTools.([]any)
	if !ok {
		reqBody["tools"] = []any{tool}
		return true
	}
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if ok && strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			return false
		}
	}
	reqBody["tools"] = append(tools, tool)
	return true
}
```

Add below it:

```go
func ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody map[string]any) bool {
	if len(reqBody) == 0 || !hasOpenAIImageGenerationTool(reqBody) {
		return false
	}
	if isCodexSparkModel(firstNonEmptyString(reqBody["model"])) {
		return false
	}
	if _, ok := reqBody["tool_choice"]; ok {
		return false
	}
	reqBody["tool_choice"] = "auto"
	return true
}

func stripCodexSparkImageGenerationTools(reqBody map[string]any) bool {
	rawTools, ok := reqBody["tools"]
	if !ok || rawTools == nil {
		return false
	}
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}
	filtered := make([]any, 0, len(tools))
	removed := false
	for _, rawTool := range tools {
		toolMap, ok := rawTool.(map[string]any)
		if ok && strings.TrimSpace(firstNonEmptyString(toolMap["type"])) == "image_generation" {
			removed = true
			continue
		}
		filtered = append(filtered, rawTool)
	}
	if !removed {
		return false
	}
	if len(filtered) == 0 {
		delete(reqBody, "tools")
	} else {
		reqBody["tools"] = filtered
	}
	return true
}
```

- [ ] **Step 4: Run transform tests**

Run:

```bash
go test ./internal/service -run 'TestEnsureOpenAIResponsesImageGenerationTool|TestStripCodexSparkImageGenerationTools|TestApplyCodexImageGenerationBridgeInstructions' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openai_codex_transform.go backend/internal/service/openai_codex_transform_test.go
git commit -m "feat: restore policy-gated codex image transform helpers"
```

---

### Task 5: Wire HTTP Gateway Behavior

**Files:**
- Modify: `backend/internal/service/openai_gateway_service.go`
- Test: `backend/internal/service/openai_image_generation_controls_test.go`

- [ ] **Step 1: Write failing HTTP behavior tests**

Create `backend/internal/service/openai_image_generation_controls_test.go` with focused tests using `httpUpstreamRecorder`:

```go
package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newImageControlService(upstream *httpUpstreamRecorder) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	return &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
}

func newImageControlContext(allowImages bool, userAgent string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", userAgent)
	c.Set("api_key", &APIKey{Group: &Group{AllowImageGeneration: allowImages}})
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return c, rec
}

func newImageControlAccount() *Account {
	return &Account{
		ID:          91,
		Name:        "openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://example.com/v1"},
	}
}

func TestOpenAIGatewayForward_CodexImageBridgeDefaultDoesNotInject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp","usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := newImageControlService(upstream)
	c, _ := newImageControlContext(true, "codex_cli_rs/0.98.0")
	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"write code","stream":false}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
}

func TestOpenAIGatewayForward_CodexImageBridgeInjectsWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp","usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := newImageControlService(upstream)
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	c, _ := newImageControlContext(true, "codex_cli_rs/0.98.0")
	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"write code","stream":false}`))
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.Equal(t, "auto", gjson.GetBytes(upstream.lastBody, "tool_choice").String())
	require.Contains(t, gjson.GetBytes(upstream.lastBody, "instructions").String(), codexImageGenerationBridgeMarker)
}

func TestOpenAIGatewayForward_ExplicitImageToolDeniedWhenGroupDisallows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{}
	svc := newImageControlService(upstream)
	c, rec := newImageControlContext(false, "codex_cli_rs/0.98.0")
	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"draw","tools":[{"type":"image_generation"}]}`))
	require.Error(t, err)
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), ImageGenerationPermissionMessage())
	require.Nil(t, upstream.lastReq)
}

func TestOpenAIGatewayForward_CompactSkipsCodexImageBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp","usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc := newImageControlService(upstream)
	svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
	c, _ := newImageControlContext(true, "codex_cli_rs/0.98.0")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	_, err := svc.Forward(context.Background(), c, newImageControlAccount(), []byte(`{"model":"gpt-5.5","input":"summarize","stream":false}`))
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation")`).Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "tool_choice").Exists())
}
```

- [ ] **Step 2: Run HTTP tests to verify failure**

Run:

```bash
go test ./internal/service -run 'TestOpenAIGatewayForward_(CodexImageBridge|ExplicitImageTool|Compact)' -count=1
```

Expected: at least one test FAILS because HTTP path is not policy-gated yet.

- [ ] **Step 3: Implement HTTP gate and policy injection**

In `OpenAIGatewayService.Forward`, after `isCodexCLI` is computed and after `reqBody` is available, add:

```go
apiKey := getAPIKeyFromContext(c)
imageGenerationAllowed := GroupAllowsImageGeneration(apiKeyGroup(apiKey))
codexImageGenerationBridgeEnabled := isCodexCLI && imageGenerationAllowed && s.isCodexImageGenerationBridgeEnabled(ctx, account, apiKey)
imageIntent := IsImageGenerationIntent("/v1/responses", reqModel, body)
if imageIntent && !imageGenerationAllowed {
	setOpsUpstreamError(c, http.StatusForbidden, ImageGenerationPermissionMessage(), "")
	c.JSON(http.StatusForbidden, response.WithErrorCorrelation(c, gin.H{
		"error": gin.H{"type": "permission_error", "message": ImageGenerationPermissionMessage()},
	}))
	return nil, errors.New("image generation disabled for group")
}
```

After model mapping resolves `upstreamModel`, add a second check:

```go
imageIntent = imageIntent || IsImageGenerationIntentMap("/v1/responses", upstreamModel, reqBody)
if imageIntent && !imageGenerationAllowed {
	setOpsUpstreamError(c, http.StatusForbidden, ImageGenerationPermissionMessage(), "")
	c.JSON(http.StatusForbidden, response.WithErrorCorrelation(c, gin.H{
		"error": gin.H{"type": "permission_error", "message": ImageGenerationPermissionMessage()},
	}))
	return nil, errors.New("image generation disabled for group")
}
```

Replace the current unconditional Codex injection block:

```go
if isCodexCLI && ensureOpenAIResponsesImageGenerationTool(reqBody) {
```

with:

```go
isCompactRequest := isOpenAIResponsesCompactPath(c)
if codexImageGenerationBridgeEnabled && !isCompactRequest && ensureOpenAIResponsesImageGenerationTool(reqBody) {
```

Immediately after it, add:

```go
if codexImageGenerationBridgeEnabled && !isCompactRequest && ensureOpenAIResponsesImageGenerationToolChoiceAuto(reqBody) {
	bodyModified = true
	disablePatch()
	logger.LegacyPrintfContext(ctx, "service.openai_gateway", "[OpenAI] Set /responses image_generation tool_choice=auto for Codex client")
}
```

Gate bridge instructions:

```go
if codexImageGenerationBridgeEnabled && !isCompactRequest && applyCodexImageGenerationBridgeInstructions(reqBody) {
```

Before upstream request building, after `validateCodexSparkInput`, add:

```go
if isCodexSparkModel(upstreamModel) && stripCodexSparkImageGenerationTools(reqBody) {
	bodyModified = true
	disablePatch()
}
```

- [ ] **Step 4: Run HTTP tests**

Run:

```bash
go test ./internal/service -run 'TestOpenAIGatewayForward_(CodexImageBridge|ExplicitImageTool|Compact)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/service/openai_gateway_service.go backend/internal/service/openai_image_generation_controls_test.go
git commit -m "feat: gate codex image bridge in openai responses"
```

---

### Task 6: Wire WS Ingress Behavior

**Files:**
- Modify: `backend/internal/service/openai_ws_forwarder.go`
- Test: `backend/internal/service/openai_ws_forwarder_success_test.go`

- [ ] **Step 1: Write failing WS test for bridge injection**

Add `TestOpenAIGatewayService_Forward_WSv2_CodexImageBridgeInjectsWhenEnabled` to `backend/internal/service/openai_ws_forwarder_success_test.go`. Use the same websocket `httptest.Server` structure as `TestOpenAIGatewayService_Forward_WSv2_SuccessAndBindSticky`, but capture `requestJSON` from the first upstream request and assert:

```go
require.True(t, gjson.Get(requestJSON, `tools.#(type=="image_generation")`).Exists())
require.Equal(t, "auto", gjson.Get(requestJSON, "tool_choice").String())
require.Contains(t, gjson.Get(requestJSON, "instructions").String(), codexImageGenerationBridgeMarker)
```

Set up the request context with:

```go
svc.cfg.Gateway.CodexImageGenerationBridgeEnabled = true
c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
c.Set("api_key", &APIKey{Group: &Group{AllowImageGeneration: true}})
```

- [ ] **Step 2: Write failing WS test for group denial**

Add a WS test that sends:

```json
{"model":"gpt-5.5","input":"draw","tools":[{"type":"image_generation"}]}
```

with:

```go
c.Set("api_key", &APIKey{Group: &Group{AllowImageGeneration: false}})
```

Expected close/error reason includes:

```go
ImageGenerationPermissionMessage()
```

- [ ] **Step 3: Run WS tests to verify failure**

Run:

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_Forward_WSv2_.*Image|Test.*WS.*Image' -count=1
```

Expected: FAIL because WS ingress does not yet apply image bridge/gate policy.

- [ ] **Step 4: Implement WS policy**

In `backend/internal/service/openai_ws_forwarder.go`, immediately after this line:

```go
isCodexCLI := openai.IsCodexOfficialClientByHeaders(c.GetHeader("User-Agent"), c.GetHeader("originator")) || (s.cfg != nil && s.cfg.Gateway.ForceCodexCLI)
```

mutate `firstPayload.payloadRaw` using the same helpers:

```go
apiKey := getAPIKeyFromContext(c)
imageGenerationAllowed := GroupAllowsImageGeneration(apiKeyGroup(apiKey))
if IsImageGenerationIntent("/v1/responses", firstPayload.originalModel, firstPayload.payloadRaw) && !imageGenerationAllowed {
	return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, ImageGenerationPermissionMessage(), nil)
}

codexBridgeEnabled := isCodexCLI && imageGenerationAllowed && s.isCodexImageGenerationBridgeEnabled(ctx, account, apiKey)
if codexBridgeEnabled {
	payloadMap := make(map[string]any)
	if err := json.Unmarshal(firstPayload.payloadRaw, &payloadMap); err != nil {
		return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", err)
	}
	bridgeModified := false
	if ensureOpenAIResponsesImageGenerationTool(payloadMap) {
		bridgeModified = true
	}
	if ensureOpenAIResponsesImageGenerationToolChoiceAuto(payloadMap) {
		bridgeModified = true
	}
	if normalizeOpenAIResponsesImageGenerationTools(payloadMap) {
		bridgeModified = true
	}
	if applyCodexImageGenerationBridgeInstructions(payloadMap) {
		bridgeModified = true
	}
	if stripCodexSparkImageGenerationTools(payloadMap) {
		bridgeModified = true
	}
	if bridgeModified {
		rebuilt, err := json.Marshal(payloadMap)
		if err != nil {
			return NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", err)
		}
		firstPayload.payloadRaw = rebuilt
		firstPayload.payloadBytes = len(rebuilt)
	}
}
```

Do not overwrite `firstPayload.rawForHash`; sticky session hashing should continue to use the original client payload.

- [ ] **Step 5: Run WS tests**

Run:

```bash
go test ./internal/service -run 'TestOpenAIGatewayService_Forward_WSv2_.*Image|Test.*WS.*Image' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/openai_ws_forwarder.go backend/internal/service/openai_ws_forwarder_success_test.go backend/internal/service/openai_ws_forwarder_ingress_session_test.go
git commit -m "feat: apply codex image bridge policy to ws ingress"
```

---

### Task 7: Final Verification and Cleanup

**Files:**
- Review all files changed by Tasks 1-6.
- Do not stage or commit the deleted deprecated spec unless the user explicitly wants that deletion committed.

- [ ] **Step 1: Run formatting**

```bash
gofmt -w backend/internal/service backend/internal/handler backend/internal/repository backend/internal/config backend/ent
```

Expected: no command output.

- [ ] **Step 2: Run targeted tests**

```bash
go test ./internal/service -run 'Test(GroupAllowsImageGeneration|IsImageGenerationIntent|CodexImageGenerationBridge|EnsureOpenAIResponsesImageGenerationTool|StripCodexSparkImageGenerationTools|OpenAIGatewayForward_CodexImageBridge|OpenAIGatewayForward_ExplicitImageTool|OpenAIGatewayForward_Compact)' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run broader package tests**

```bash
go test ./internal/service -count=1
```

Expected: PASS, or document known unrelated failures. Before this work, the package had unrelated failures in `TestOpenAIGatewayServiceRecordUsage_*` and `TestOpenAISelectAccountForModelWithExclusions_NoAccounts`; if they still fail unchanged, report them as pre-existing.

- [ ] **Step 4: Check for deprecated whitelist design references**

```bash
rg -n 'openai-responses-image-generation-account-whitelist|该主题已有独立设计|Responses image account whitelist' docs backend
```

Expected: only the alignment spec non-goal line may mention `Responses image account whitelist`; no reference should point to the deleted design file.

- [ ] **Step 5: Check git status**

```bash
git status --short --branch
```

Expected: only intentional implementation changes remain. Do not accidentally include `docs/plans/2026-06-22-openai-image-generation-api.md` unless the user asks.

- [ ] **Step 6: Final commit if needed**

If formatting or small cleanup changed files after previous task commits:

```bash
git add backend/internal/service backend/internal/handler backend/internal/repository backend/internal/config backend/ent backend/migrations
git commit -m "test: verify codex image bridge alignment"
```

When `git diff --quiet` exits with status 0, skip this cleanup commit.

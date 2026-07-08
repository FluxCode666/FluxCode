# Model Pricing Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增公开模型定价页 `/model-pricing`，复用渠道管理页维护模型展示、支持分组和特殊定价，同时移除旧“定价方案”运行时代码入口。

**Architecture:** 后端新增只读聚合服务，从启用渠道的 `channel_model_pricing.models` 推导模型列表与分组详情，价格计算复用官方价加渠道覆盖再乘分组倍率的既有计费语义。前端新增公开页面、API 类型、搜索防抖和导航入口；渠道管理页增加展示用能力标签。旧 pricing plan 前后端入口、服务、仓储、页面和测试引用删除，但保留数据库表和支付/订阅购买功能。

**Tech Stack:** Go 1.26.2、Gin、Wire、PostgreSQL JSONB migrations、Vue 3、Vite、Vue Router、Pinia、vue-i18n、Vitest、Tailwind CSS。

## Global Constraints

- 所有用户交互回复必须使用简体中文。
- 公开模型定价页路由为 `/model-pricing`。
- 官网顶部菜单和控制台侧边栏都新增“模型定价”，控制台入口跳转同一个公开页。
- 模型列表来源为启用渠道里的 `channel_model_pricing.models` 具体模型并集。
- 通配符模型可用于详情支持判断，但不作为公开模型列表项直接展示。
- 官方价来自远程 `model_pricing.json` 经 `PricingService` / `BillingService.GetModelPricing(model)` 返回的基础价。
- 渠道特殊价格覆盖非空字段，空字段回退官方价。
- 最终展示价格乘分组默认 `rate_multiplier`。
- 公开页面展示相对官方价倍率，不展示“渠道定价”“特殊定价”“官方倍率”等后台来源标签。
- 某分组不支持某模型时直接隐藏。
- 能力标签允许值固定为 `chat`、`image`、`video`，仅用于展示，不参与调用、路由、模型限制、账号选择或计费。
- 搜索框支持模型 ID、展示名、平台、能力标签，输入 300ms 防抖。
- 搜索异常时页面提示“查询异常”并提供重试。
- 删除旧公开 `/pricing` 页面、后端 `/api/v1/pricing/plan-groups`、管理员 `/admin/pricing-plans` 页面和 admin pricing plan API。
- 不写 `DROP TABLE` 迁移，不删除 `pricing_plan_groups` / `pricing_plans` 表。
- 保留 `/purchase`、`/orders`、`/admin/orders/plans`、支付套餐、订阅购买、订单管理和相关 payment API。

---

## File Structure

### Backend

- `backend/migrations/127_channel_model_pricing_capabilities.sql`: 给 `channel_model_pricing` 增加 `capabilities JSONB NOT NULL DEFAULT '[]'`。
- `backend/internal/service/channel.go`: `ChannelModelPricing` 增加 `Capabilities []string`，`Clone()` 复制该切片，新增能力标签常量和规范化函数。
- `backend/internal/service/channel_service.go`: 创建/更新渠道时规范化主模型定价和账号统计定价里的 `Capabilities`。
- `backend/internal/repository/channel_repo_pricing.go`: 查询、插入、更新 `channel_model_pricing.capabilities`。
- `backend/internal/handler/admin/channel_handler.go`: admin 渠道请求和响应加入 `capabilities` 字段。
- `backend/internal/handler/admin/channel_handler_test.go`: 覆盖渠道 CRUD 的能力标签读写和过滤。
- `backend/internal/repository/channel_repo_test.go`: 覆盖仓储读写 `capabilities`。
- `backend/internal/service/model_pricing_page.go`: 新增公开模型定价聚合服务、响应 DTO、搜索过滤、价格倍率计算。
- `backend/internal/service/model_pricing_page_test.go`: 覆盖聚合、倍率、通配符、搜索、官方价缺失过滤。
- `backend/internal/handler/model_pricing_handler.go`: 新增公开 handler。
- `backend/internal/handler/model_pricing_handler_test.go`: 覆盖列表、详情、查询异常 HTTP 行为。
- `backend/internal/server/routes/model_pricing.go`: 注册 `/api/v1/model-pricing/models` 和 `/api/v1/model-pricing/models/:model`。
- `backend/internal/server/routes/model_pricing_test.go`: 覆盖公开路由注册。
- `backend/internal/server/router.go`: 替换旧 pricing plan 路由注册为 model pricing 路由。
- `backend/internal/handler/handler.go`: 移除 `PricingPlan` handler 字段，新增 `ModelPricing` handler 字段。
- `backend/internal/handler/wire.go`: 注入 `NewModelPricingHandler`，移除 pricing plan handler 注入。
- `backend/internal/service/wire.go`: 注入 `NewModelPricingPageService`，移除 `NewPricingPlanService`。
- `backend/internal/repository/wire.go`: 移除 `NewPricingPlanRepository`。
- `backend/cmd/server/wire_gen.go`: 通过 `go generate ./cmd/server` 更新生成结果。
- 删除旧 pricing plan 运行时代码和测试引用：
  - `backend/internal/server/routes/pricing_plan.go`
  - `backend/internal/server/routes/pricing_plan_test.go`
  - `backend/internal/handler/pricing_plan_handler.go`
  - `backend/internal/handler/pricing_plan_handler_test.go`
  - `backend/internal/handler/admin/pricing_plan_handler.go`
  - `backend/internal/handler/admin/pricing_plan_handler_test.go`
  - `backend/internal/service/pricing_plan.go`
  - `backend/internal/service/pricing_plan_service.go`
  - `backend/internal/repository/pricing_plan_repo.go`
  - `backend/internal/repository/pricing_plan_repo_integration_test.go`

### Frontend

- `frontend/src/api/modelPricing.ts`: 新增公开模型定价 API 类型和请求函数。
- `frontend/src/views/ModelPricingView.vue`: 新增公开模型列表、搜索、防抖、筛选、详情区域和错误状态。
- `frontend/src/views/__tests__/ModelPricingView.spec.ts`: 覆盖渲染、详情、搜索防抖、查询异常、筛选组合。
- `frontend/src/router/index.ts`: 删除 `/pricing` 和 `/admin/pricing-plans`，新增 `/model-pricing`。
- `frontend/src/components/layout/PublicHeader.vue`: 官网顶部桌面和移动菜单新增“模型定价”。
- `frontend/src/components/layout/AppSidebar.vue`: 用户和管理员控制台侧边栏新增“模型定价”，删除管理员“定价方案”。
- `frontend/src/components/layout/__tests__/PublicHeader.spec.ts`: 覆盖顶部菜单入口。
- `frontend/src/components/layout/__tests__/AppSidebar.modelPricing.spec.ts`: 覆盖控制台侧边栏入口和旧菜单移除。
- `frontend/src/api/admin/channels.ts`: `ChannelModelPricing` 增加 `capabilities: ModelCapability[]`。
- `frontend/src/components/admin/channel/types.ts`: `PricingFormEntry` 增加 `capabilities`，新增能力标签常量与格式化 helper。
- `frontend/src/components/admin/channel/PricingEntryCard.vue`: 定价条目增加能力标签多选。
- `frontend/src/views/admin/ChannelsView.vue`: 表单初始化、API 转换、保存和账号统计定价规则同步 `capabilities`。
- `frontend/src/components/admin/channel/__tests__/PricingEntryCard.capabilities.spec.ts`: 覆盖选择/取消能力标签。
- `frontend/src/views/admin/__tests__/ChannelsView.capabilities.spec.ts`: 覆盖渠道编辑保存能力标签。
- `frontend/src/i18n/locales/zh.ts`: 增加模型定价页、能力标签、查询异常等中文文案，移除旧定价方案导航文案引用。
- `frontend/src/i18n/locales/en.ts`: 增加英文文案，移除旧定价方案导航文案引用。
- `frontend/src/i18n/__tests__/navigationLocales.spec.ts`: 更新导航文案断言。
- 删除旧 pricing plan 前端代码和测试引用：
  - `frontend/src/views/PricingView.vue`
  - `frontend/src/views/__tests__/PricingView.spec.ts`
  - `frontend/src/views/admin/PricingPlansView.vue`
  - `frontend/src/views/admin/__tests__/PricingPlansView.spec.ts`
  - `frontend/src/api/pricingPlans.ts`
  - `frontend/src/api/admin/pricingPlans.ts`
  - `frontend/src/api/admin/index.ts` 中的 `pricingPlans` 导出。

---

### Task 1: 渠道能力标签持久化

**Files:**
- Create: `backend/migrations/127_channel_model_pricing_capabilities.sql`
- Modify: `backend/internal/service/channel.go`
- Modify: `backend/internal/service/channel_service.go`
- Modify: `backend/internal/repository/channel_repo_pricing.go`
- Modify: `backend/internal/handler/admin/channel_handler.go`
- Test: `backend/internal/service/channel_test.go`
- Test: `backend/internal/repository/channel_repo_test.go`
- Test: `backend/internal/handler/admin/channel_handler_test.go`

**Interfaces:**
- Consumes: existing `service.ChannelModelPricing`, `channelModelPricingRequest`, `channelModelPricingResponse`, `pricingRequestToService`, `pricingToResponse`.
- Produces:
  - `type ModelCapability string`
  - `const ModelCapabilityChat = "chat"`, `ModelCapabilityImage = "image"`, `ModelCapabilityVideo = "video"`
  - `func NormalizeModelCapabilities(input []string) []string`
  - `ChannelModelPricing.Capabilities []string`
  - JSON field `capabilities: string[]` on admin channel model pricing request/response.

- [ ] **Step 1: Add failing service tests for capability normalization and cloning**

Add to `backend/internal/service/channel_test.go`:

```go
func TestNormalizeModelCapabilities(t *testing.T) {
	got := NormalizeModelCapabilities([]string{"chat", "", "image", "bad", "chat", " video "})
	require.Equal(t, []string{"chat", "image", "video"}, got)
}

func TestChannelModelPricingCloneCopiesCapabilities(t *testing.T) {
	original := ChannelModelPricing{
		Models:       []string{"claude-sonnet-4"},
		Capabilities: []string{"chat", "image"},
	}

	cloned := original.Clone()
	cloned.Capabilities[0] = "video"

	require.Equal(t, []string{"chat", "image"}, original.Capabilities)
	require.Equal(t, []string{"video", "image"}, cloned.Capabilities)
}
```

- [ ] **Step 2: Run service tests to verify they fail**

Run:

```bash
cd backend && go test ./internal/service -run 'TestNormalizeModelCapabilities|TestChannelModelPricingCloneCopiesCapabilities' -count=1
```

Expected: FAIL because `NormalizeModelCapabilities`, `ModelCapability*`, and `ChannelModelPricing.Capabilities` do not exist yet.

- [ ] **Step 3: Implement capability constants, normalization, and clone support**

In `backend/internal/service/channel.go`, add near billing constants:

```go
type ModelCapability string

const (
	ModelCapabilityChat  ModelCapability = "chat"
	ModelCapabilityImage ModelCapability = "image"
	ModelCapabilityVideo ModelCapability = "video"
)

var allowedModelCapabilities = map[string]struct{}{
	string(ModelCapabilityChat):  {},
	string(ModelCapabilityImage): {},
	string(ModelCapabilityVideo): {},
}

func NormalizeModelCapabilities(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, raw := range input {
		capability := strings.ToLower(strings.TrimSpace(raw))
		if _, ok := allowedModelCapabilities[capability]; !ok {
			continue
		}
		if _, exists := seen[capability]; exists {
			continue
		}
		seen[capability] = struct{}{}
		out = append(out, capability)
	}
	return out
}
```

Extend `ChannelModelPricing`:

```go
Capabilities     []string          // 展示用能力标签：chat/image/video
```

Extend `ChannelModelPricing.Clone()`:

```go
if p.Capabilities != nil {
	cp.Capabilities = make([]string, len(p.Capabilities))
	copy(cp.Capabilities, p.Capabilities)
}
```

- [ ] **Step 4: Normalize capabilities during channel create/update validation**

In `backend/internal/service/channel_service.go`, add:

```go
func normalizePricingCapabilities(pricing []ChannelModelPricing) {
	for i := range pricing {
		pricing[i].Capabilities = NormalizeModelCapabilities(pricing[i].Capabilities)
	}
}
```

Call it in `validatePricingEntries(pricing []ChannelModelPricing)` before conflict checks:

```go
normalizePricingCapabilities(pricing)
```

In `validateChannelConfig(pricing []ChannelModelPricing, mapping map[string]map[string]string) error`, keep the existing call to `validatePricingEntries(pricing)` so both create and update are normalized.

- [ ] **Step 5: Add migration**

Create `backend/migrations/127_channel_model_pricing_capabilities.sql`:

```sql
-- 127_channel_model_pricing_capabilities.sql
-- Add display-only capability tags for public model pricing pages.

ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
```

- [ ] **Step 6: Add failing repository tests**

Add to `backend/internal/repository/channel_repo_test.go`:

```go
func TestChannelRepositoryModelPricingCapabilitiesRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	repo := NewChannelRepository(db)
	pricing := service.ChannelModelPricing{
		ChannelID:    12,
		Platform:     "anthropic",
		Models:       []string{"claude-sonnet-4"},
		Capabilities: []string{"chat", "image"},
		BillingMode:  service.BillingModeToken,
	}

	mock.ExpectQuery(`INSERT INTO channel_model_pricing`).
		WithArgs(
			int64(12),
			"anthropic",
			[]byte(`["claude-sonnet-4"]`),
			service.BillingModeToken,
			pricing.InputPrice,
			pricing.OutputPrice,
			pricing.CacheWritePrice,
			pricing.CacheReadPrice,
			pricing.ImageOutputPrice,
			pricing.PerRequestPrice,
			[]byte(`["chat","image"]`),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "updated_at"}).
			AddRow(int64(99), time.Now(), time.Now()))

	err = repo.CreateModelPricing(context.Background(), &pricing)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	require.Equal(t, int64(99), pricing.ID)
}
```

If the existing tests use a custom helper instead of direct `sqlmock.New()`, keep the local helper style but assert the same INSERT args and JSON.

- [ ] **Step 7: Run repository test to verify it fails**

Run:

```bash
cd backend && go test ./internal/repository -run TestChannelRepositoryModelPricingCapabilitiesRoundTrip -count=1
```

Expected: FAIL because SQL does not include `capabilities`.

- [ ] **Step 8: Update repository SQL and scanner**

In `backend/internal/repository/channel_repo_pricing.go`, update both SELECT statements to include `capabilities` after `models`:

```sql
SELECT id, channel_id, platform, models, capabilities, billing_mode, input_price, output_price, cache_write_price, cache_read_price, image_output_price, per_request_price, created_at, updated_at
```

In `scanModelPricingRows`, add:

```go
var capabilitiesJSON []byte
```

Scan it between `modelsJSON` and `&p.BillingMode`, then unmarshal:

```go
if err := json.Unmarshal(capabilitiesJSON, &p.Capabilities); err != nil {
	p.Capabilities = []string{}
}
p.Capabilities = service.NormalizeModelCapabilities(p.Capabilities)
```

In `createModelPricingExec`, marshal and insert:

```go
capabilitiesJSON, err := json.Marshal(service.NormalizeModelCapabilities(pricing.Capabilities))
if err != nil {
	return fmt.Errorf("marshal capabilities: %w", err)
}
```

Use SQL:

```sql
INSERT INTO channel_model_pricing (channel_id, platform, models, billing_mode, input_price, output_price, cache_write_price, cache_read_price, image_output_price, per_request_price, capabilities)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id, created_at, updated_at
```

In `UpdateModelPricing`, marshal and set:

```sql
SET models = $1, billing_mode = $2, input_price = $3, output_price = $4, cache_write_price = $5, cache_read_price = $6, image_output_price = $7, per_request_price = $8, platform = $9, capabilities = $10, updated_at = NOW()
WHERE id = $11
```

- [ ] **Step 9: Add failing admin handler tests**

Add to `backend/internal/handler/admin/channel_handler_test.go` a unit test around mapper functions:

```go
func TestChannelPricingCapabilitiesMapperRoundTrip(t *testing.T) {
	reqs := []channelModelPricingRequest{{
		Platform:     "anthropic",
		Models:       []string{"claude-sonnet-4"},
		Capabilities: []string{"chat", "bad", "chat", "image"},
		BillingMode:  "token",
	}}

	pricing := pricingRequestToService(reqs)
	require.Len(t, pricing, 1)
	require.Equal(t, []string{"chat", "image"}, pricing[0].Capabilities)

	resp := pricingToResponse(&pricing[0])
	require.Equal(t, []string{"chat", "image"}, resp.Capabilities)
}
```

- [ ] **Step 10: Update admin channel DTOs**

In `backend/internal/handler/admin/channel_handler.go`, extend `channelModelPricingRequest` and `channelModelPricingResponse`:

```go
Capabilities []string `json:"capabilities"`
```

In `pricingToResponse`, set:

```go
capabilities := service.NormalizeModelCapabilities(p.Capabilities)
if capabilities == nil {
	capabilities = []string{}
}
```

Return `Capabilities: capabilities`.

In `pricingRequestToService`, set:

```go
Capabilities: service.NormalizeModelCapabilities(r.Capabilities),
```

- [ ] **Step 11: Run focused backend tests**

Run:

```bash
cd backend && go test ./internal/service -run 'TestNormalizeModelCapabilities|TestChannelModelPricingCloneCopiesCapabilities' -count=1
cd backend && go test ./internal/repository -run TestChannelRepositoryModelPricingCapabilitiesRoundTrip -count=1
cd backend && go test ./internal/handler/admin -run TestChannelPricingCapabilitiesMapperRoundTrip -count=1
```

Expected: PASS.

- [ ] **Step 12: Commit Task 1**

```bash
git add backend/migrations/127_channel_model_pricing_capabilities.sql backend/internal/service/channel.go backend/internal/service/channel_service.go backend/internal/repository/channel_repo_pricing.go backend/internal/handler/admin/channel_handler.go backend/internal/service/channel_test.go backend/internal/repository/channel_repo_test.go backend/internal/handler/admin/channel_handler_test.go
git commit -m "feat: persist channel model capabilities"
```

---

### Task 2: 后端公开模型定价聚合接口

**Files:**
- Create: `backend/internal/service/model_pricing_page.go`
- Create: `backend/internal/service/model_pricing_page_test.go`
- Create: `backend/internal/handler/model_pricing_handler.go`
- Create: `backend/internal/handler/model_pricing_handler_test.go`
- Create: `backend/internal/server/routes/model_pricing.go`
- Create: `backend/internal/server/routes/model_pricing_test.go`
- Modify: `backend/internal/server/router.go`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/cmd/server/wire_gen.go`

**Interfaces:**
- Consumes:
  - `ChannelService.List(ctx, pagination.PaginationParams, status, search string)`
  - `ChannelService.GetChannelModelPricing(ctx, groupID int64, model string)`
  - `GroupService.ListActive(ctx context.Context)`
  - `BillingService.GetModelPricing(model string)`
  - `service.ModelPricing`
- Produces:
  - `func NewModelPricingPageService(channelService *ChannelService, groupService *GroupService, billingService *BillingService) *ModelPricingPageService`
  - `func (s *ModelPricingPageService) ListModels(ctx context.Context, query ModelPricingQuery) ([]ModelPricingModelSummary, error)`
  - `func (s *ModelPricingPageService) GetModel(ctx context.Context, model string) (*ModelPricingModelDetail, error)`
  - `type ModelPricingQuery struct { Q string; Platform string; Capability string }`
  - `type ModelPricingHandler struct`
  - `GET /api/v1/model-pricing/models`
  - `GET /api/v1/model-pricing/models/:model`

- [ ] **Step 1: Write failing service tests for aggregation**

Create `backend/internal/service/model_pricing_page_test.go` with test stubs using in-memory inputs:

```go
package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type modelPricingChannelListerStub struct {
	channels []Channel
	err      error
}

func (s *modelPricingChannelListerStub) List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	return s.channels, &pagination.PaginationResult{Total: int64(len(s.channels))}, nil
}

type modelPricingGroupListerStub struct {
	groups []Group
	err    error
}

func (s *modelPricingGroupListerStub) ListActive(ctx context.Context) ([]Group, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.groups, nil
}

type modelPricingBillingStub struct {
	prices map[string]*ModelPricing
	errs   map[string]error
}

func (s *modelPricingBillingStub) GetModelPricing(model string) (*ModelPricing, error) {
	if err := s.errs[model]; err != nil {
		return nil, err
	}
	price, ok := s.prices[model]
	if !ok {
		return nil, errors.New("missing model price")
	}
	cp := *price
	return &cp, nil
}

func TestModelPricingPageServiceListModelsAggregatesConcreteEnabledChannelModels(t *testing.T) {
	input := floatPtr(0.000003)
	output := floatPtr(0.000015)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:      10,
			Status:  StatusActive,
			GroupIDs: []int64{1, 2},
			ModelPricing: []ChannelModelPricing{{
				Platform:     "anthropic",
				Models:       []string{"claude-sonnet-4", "claude-*"},
				Capabilities: []string{"chat"},
				BillingMode:  BillingModeToken,
				InputPrice:   input,
				OutputPrice:  output,
			}},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1.2},
			{ID: 2, Name: "禁用组", Platform: "anthropic", Status: StatusDisabled, RateMultiplier: 1.0},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {
				InputPricePerToken:         0.000003,
				OutputPricePerToken:        0.000015,
				CacheCreationPricePerToken: 0.00000375,
				CacheReadPricePerToken:     0.0000003,
			},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "claude-sonnet-4", models[0].ID)
	require.Equal(t, "anthropic", models[0].Platform)
	require.Equal(t, []string{"chat"}, models[0].Capabilities)
	require.Equal(t, 1, models[0].SupportedGroupCount)
	require.Equal(t, 0.000003, models[0].OfficialPrice.InputPrice)
}

func TestModelPricingPageServiceGetModelAppliesChannelOverrideAndGroupMultiplier(t *testing.T) {
	input := floatPtr(0.000006)
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{
			{
				ID:      10,
				Status:  StatusActive,
				GroupIDs: []int64{},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-opus-4"},
					Capabilities: []string{"chat"},
					BillingMode:  BillingModeToken,
				}},
			},
			{
				ID:      11,
				Status:  StatusActive,
				GroupIDs: []int64{1},
				ModelPricing: []ChannelModelPricing{{
					Platform:     "anthropic",
					Models:       []string{"claude-*"},
					Capabilities: []string{"image"},
					BillingMode:  BillingModeToken,
					InputPrice:   input,
				}},
			}},
		}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "专业组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 2},
			{ID: 2, Name: "未绑定组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-opus-4": {
				InputPricePerToken:  0.000003,
				OutputPricePerToken: 0.000015,
			},
		}},
	)

	detail, err := svc.GetModel(context.Background(), "claude-opus-4")
	require.NoError(t, err)
	require.Equal(t, "claude-opus-4", detail.ID)
	require.Equal(t, []string{"chat", "image"}, detail.Capabilities)
	require.Len(t, detail.Groups, 1)
	require.Equal(t, "专业组", detail.Groups[0].GroupName)
	require.Equal(t, 0.000012, detail.Groups[0].Price.InputPrice)
	require.Equal(t, 4.0, detail.Groups[0].Multipliers.InputPrice)
	require.Equal(t, 0.00003, detail.Groups[0].Price.OutputPrice)
	require.Equal(t, 2.0, detail.Groups[0].Multipliers.OutputPrice)
}

func TestModelPricingPageServiceListModelsFiltersSearchPlatformAndCapability(t *testing.T) {
	svc := NewModelPricingPageServiceForTest(
		&modelPricingChannelListerStub{channels: []Channel{{
			ID:      10,
			Status:  StatusActive,
			GroupIDs: []int64{1},
			ModelPricing: []ChannelModelPricing{
				{Platform: "anthropic", Models: []string{"claude-sonnet-4"}, Capabilities: []string{"chat"}, BillingMode: BillingModeToken},
				{Platform: "openai", Models: []string{"gpt-image-1"}, Capabilities: []string{"image"}, BillingMode: BillingModeToken},
			},
		}}},
		&modelPricingGroupListerStub{groups: []Group{
			{ID: 1, Name: "基础组", Platform: "anthropic", Status: StatusActive, RateMultiplier: 1},
		}},
		&modelPricingBillingStub{prices: map[string]*ModelPricing{
			"claude-sonnet-4": {InputPricePerToken: 0.000003},
			"gpt-image-1":     {InputPricePerToken: 0.000001},
		}},
	)

	models, err := svc.ListModels(context.Background(), ModelPricingQuery{Q: "chat", Platform: "anthropic", Capability: "chat"})
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "claude-sonnet-4", models[0].ID)
}

func floatPtr(v float64) *float64 { return &v }
```

These tests intentionally reference `NewModelPricingPageServiceForTest`; implement it as an unexported test-only constructor in production code by accepting small interfaces rather than concrete services.

- [ ] **Step 2: Run service tests to verify they fail**

Run:

```bash
cd backend && go test ./internal/service -run ModelPricingPageService -count=1
```

Expected: FAIL because `ModelPricingPageService`, query/response types, and test constructor do not exist.

- [ ] **Step 3: Implement service interfaces and response DTOs**

Create `backend/internal/service/model_pricing_page.go`:

```go
package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type modelPricingChannelLister interface {
	List(ctx context.Context, params pagination.PaginationParams, status, search string) ([]Channel, *pagination.PaginationResult, error)
}

type modelPricingGroupLister interface {
	ListActive(ctx context.Context) ([]Group, error)
}

type modelPricingBilling interface {
	GetModelPricing(model string) (*ModelPricing, error)
}

type ModelPricingPageService struct {
	channels modelPricingChannelLister
	groups   modelPricingGroupLister
	billing  modelPricingBilling
}

func NewModelPricingPageService(channelService *ChannelService, groupService *GroupService, billingService *BillingService) *ModelPricingPageService {
	return &ModelPricingPageService{channels: channelService, groups: groupService, billing: billingService}
}

func NewModelPricingPageServiceForTest(channels modelPricingChannelLister, groups modelPricingGroupLister, billing modelPricingBilling) *ModelPricingPageService {
	return &ModelPricingPageService{channels: channels, groups: groups, billing: billing}
}

type ModelPricingQuery struct {
	Q          string
	Platform   string
	Capability string
}

type ModelPricingAmount struct {
	InputPrice       float64                 `json:"input_price"`
	OutputPrice      float64                 `json:"output_price"`
	CacheWritePrice  float64                 `json:"cache_write_price"`
	CacheReadPrice   float64                 `json:"cache_read_price"`
	ImageOutputPrice float64                 `json:"image_output_price"`
	PerRequestPrice  float64                 `json:"per_request_price"`
	Intervals        []ModelPricingInterval  `json:"intervals"`
}

type ModelPricingInterval struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens"`
	TierLabel       string   `json:"tier_label"`
	InputPrice      float64  `json:"input_price"`
	OutputPrice     float64  `json:"output_price"`
	CacheWritePrice float64  `json:"cache_write_price"`
	CacheReadPrice  float64  `json:"cache_read_price"`
	PerRequestPrice float64  `json:"per_request_price"`
}

type ModelPricingMultipliers struct {
	InputPrice       float64 `json:"input_price"`
	OutputPrice      float64 `json:"output_price"`
	CacheWritePrice  float64 `json:"cache_write_price"`
	CacheReadPrice   float64 `json:"cache_read_price"`
	ImageOutputPrice float64 `json:"image_output_price"`
	PerRequestPrice  float64 `json:"per_request_price"`
}

type ModelPricingModelSummary struct {
	ID                  string             `json:"id"`
	DisplayName         string             `json:"display_name"`
	Platform            string             `json:"platform"`
	Capabilities        []string           `json:"capabilities"`
	SupportedGroupCount int                `json:"supported_group_count"`
	OfficialPrice       ModelPricingAmount `json:"official_price"`
}

type ModelPricingGroupPrice struct {
	GroupID        int64                   `json:"group_id"`
	GroupName      string                  `json:"group_name"`
	RateMultiplier float64                 `json:"rate_multiplier"`
	BillingMode    string                  `json:"billing_mode"`
	Price          ModelPricingAmount      `json:"price"`
	Multipliers    ModelPricingMultipliers `json:"multipliers"`
}

type ModelPricingModelDetail struct {
	ID           string                   `json:"id"`
	DisplayName  string                   `json:"display_name"`
	Platform     string                   `json:"platform"`
	Capabilities []string                 `json:"capabilities"`
	OfficialPrice ModelPricingAmount      `json:"official_price"`
	Groups       []ModelPricingGroupPrice  `json:"groups"`
}
```

- [ ] **Step 4: Implement aggregation helpers**

Add the helper code to `backend/internal/service/model_pricing_page.go`:

```go
func (s *ModelPricingPageService) ListModels(ctx context.Context, query ModelPricingQuery) ([]ModelPricingModelSummary, error) {
	catalog, err := s.buildCatalog(ctx)
	if err != nil {
		return nil, err
	}
	models := make([]ModelPricingModelSummary, 0, len(catalog))
	for _, item := range catalog {
		if !matchesModelPricingQuery(item, query) {
			continue
		}
		models = append(models, ModelPricingModelSummary{
			ID:                  item.ID,
			DisplayName:         displayModelName(item.ID),
			Platform:            item.Platform,
			Capabilities:        sortedStrings(item.Capabilities),
			SupportedGroupCount: len(item.Groups),
			OfficialPrice:       modelPricingToAmount(item.Official),
		})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Platform == models[j].Platform {
			return models[i].ID < models[j].ID
		}
		return models[i].Platform < models[j].Platform
	})
	return models, nil
}

func (s *ModelPricingPageService) GetModel(ctx context.Context, model string) (*ModelPricingModelDetail, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, errors.New("model is required")
	}
	catalog, err := s.buildCatalog(ctx)
	if err != nil {
		return nil, err
	}
	item, ok := catalog[model]
	if !ok {
		return nil, ErrModelPricingNotFound
	}
	groups := make([]ModelPricingGroupPrice, 0, len(item.Groups))
	for _, group := range item.Groups {
		final := applyGroupMultiplier(group.Resolved, group.RateMultiplier)
		groups = append(groups, ModelPricingGroupPrice{
			GroupID:        group.GroupID,
			GroupName:      group.GroupName,
			RateMultiplier: group.RateMultiplier,
			BillingMode:    string(group.BillingMode),
			Price:          final,
			Multipliers:    amountMultipliers(final, modelPricingToAmount(item.Official)),
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].RateMultiplier == groups[j].RateMultiplier {
			return groups[i].GroupName < groups[j].GroupName
		}
		return groups[i].RateMultiplier < groups[j].RateMultiplier
	})
	return &ModelPricingModelDetail{
		ID:            item.ID,
		DisplayName:   displayModelName(item.ID),
		Platform:      item.Platform,
		Capabilities:  sortedStrings(item.Capabilities),
		OfficialPrice: modelPricingToAmount(item.Official),
		Groups:        groups,
	}, nil
}
```

Also add:

```go
var ErrModelPricingNotFound = errors.New("model pricing not found")

type modelCatalogItem struct {
	ID           string
	Platform     string
	Capabilities map[string]struct{}
	Official     *ModelPricing
	Groups       []modelCatalogGroup
}

type modelCatalogGroup struct {
	GroupID        int64
	GroupName      string
	RateMultiplier float64
	BillingMode    BillingMode
	Resolved       ModelPricingAmount
}
```

Implement `buildCatalog` with these exact rules:

```go
func (s *ModelPricingPageService) buildCatalog(ctx context.Context) (map[string]*modelCatalogItem, error) {
	channels, _, err := s.channels.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10000, SortBy: "id", SortOrder: "asc"}, StatusActive, "")
	if err != nil {
		return nil, err
	}
	groups, err := s.groups.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	groupsByID := make(map[int64]Group, len(groups))
	for _, group := range groups {
		if group.Status == StatusActive {
			groupsByID[group.ID] = group
		}
	}
	catalog := map[string]*modelCatalogItem{}
	for i := range channels {
		channel := channels[i]
		if !channel.IsActive() {
			continue
		}
		for _, pricing := range channel.ModelPricing {
			for _, model := range pricing.Models {
				if isWildcardModelPattern(model) {
					continue
				}
				official, err := s.billing.GetModelPricing(model)
				if err != nil || official == nil {
					continue
				}
				item := catalog[model]
				if item == nil {
					item = &modelCatalogItem{
						ID:           model,
						Platform:     pricing.Platform,
						Capabilities: map[string]struct{}{},
						Official:     official,
					}
					catalog[model] = item
				}
				for _, capability := range NormalizeModelCapabilities(pricing.Capabilities) {
					item.Capabilities[capability] = struct{}{}
				}
				for _, groupID := range channel.GroupIDs {
					group, ok := groupsByID[groupID]
					if !ok || group.Platform != pricing.Platform {
						continue
					}
					resolved := resolveDisplayPricing(*official, pricing)
					item.Groups = appendOrReplaceGroup(item.Groups, modelCatalogGroup{
						GroupID:        group.ID,
						GroupName:      group.Name,
						RateMultiplier: normalizeRateMultiplier(group.RateMultiplier),
						BillingMode:    normalizeBillingMode(pricing.BillingMode),
						Resolved:       resolved,
					})
				}
			}
		}
	}
	for model, item := range catalog {
		attachWildcardSupportedGroups(item, channels, groupsByID)
	}
	for model, item := range catalog {
		if len(item.Groups) == 0 {
			delete(catalog, model)
		}
	}
	return catalog, nil
}
```

Implement helpers:

```go
func isWildcardModelPattern(model string) bool { return strings.Contains(model, "*") }

func modelPatternMatches(pattern, model string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(model, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == model
}

func displayModelName(model string) string { return model }

func normalizeRateMultiplier(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}

func normalizeBillingMode(v BillingMode) BillingMode {
	if v == "" {
		return BillingModeToken
	}
	return v
}

func resolveDisplayPricing(official ModelPricing, pricing ChannelModelPricing) ModelPricingAmount {
	base := modelPricingToAmount(&official)
	if pricing.InputPrice != nil {
		base.InputPrice = *pricing.InputPrice
	}
	if pricing.OutputPrice != nil {
		base.OutputPrice = *pricing.OutputPrice
	}
	if pricing.CacheWritePrice != nil {
		base.CacheWritePrice = *pricing.CacheWritePrice
	}
	if pricing.CacheReadPrice != nil {
		base.CacheReadPrice = *pricing.CacheReadPrice
	}
	if pricing.ImageOutputPrice != nil {
		base.ImageOutputPrice = *pricing.ImageOutputPrice
	}
	if pricing.PerRequestPrice != nil {
		base.PerRequestPrice = *pricing.PerRequestPrice
	}
	base.Intervals = intervalsToDisplayAmounts(pricing.Intervals)
	return base
}

func modelPricingToAmount(pricing *ModelPricing) ModelPricingAmount {
	if pricing == nil {
		return ModelPricingAmount{}
	}
	return ModelPricingAmount{
		InputPrice:       pricing.InputPricePerToken,
		OutputPrice:      pricing.OutputPricePerToken,
		CacheWritePrice:  pricing.CacheCreationPricePerToken,
		CacheReadPrice:   pricing.CacheReadPricePerToken,
		ImageOutputPrice: pricing.ImageOutputPricePerToken,
	}
}

func intervalsToDisplayAmounts(intervals []PricingInterval) []ModelPricingInterval {
	out := make([]ModelPricingInterval, 0, len(intervals))
	for _, iv := range filterValidIntervals(intervals) {
		out = append(out, ModelPricingInterval{
			MinTokens:       iv.MinTokens,
			MaxTokens:       iv.MaxTokens,
			TierLabel:       iv.TierLabel,
			InputPrice:      derefFloat(iv.InputPrice),
			OutputPrice:     derefFloat(iv.OutputPrice),
			CacheWritePrice: derefFloat(iv.CacheWritePrice),
			CacheReadPrice:  derefFloat(iv.CacheReadPrice),
			PerRequestPrice: derefFloat(iv.PerRequestPrice),
		})
	}
	return out
}

func applyGroupMultiplier(amount ModelPricingAmount, multiplier float64) ModelPricingAmount {
	multiplier = normalizeRateMultiplier(multiplier)
	amount.InputPrice *= multiplier
	amount.OutputPrice *= multiplier
	amount.CacheWritePrice *= multiplier
	amount.CacheReadPrice *= multiplier
	amount.ImageOutputPrice *= multiplier
	amount.PerRequestPrice *= multiplier
	for i := range amount.Intervals {
		amount.Intervals[i].InputPrice *= multiplier
		amount.Intervals[i].OutputPrice *= multiplier
		amount.Intervals[i].CacheWritePrice *= multiplier
		amount.Intervals[i].CacheReadPrice *= multiplier
		amount.Intervals[i].PerRequestPrice *= multiplier
	}
	return amount
}

func amountMultipliers(final, official ModelPricingAmount) ModelPricingMultipliers {
	return ModelPricingMultipliers{
		InputPrice:       ratio(final.InputPrice, official.InputPrice),
		OutputPrice:      ratio(final.OutputPrice, official.OutputPrice),
		CacheWritePrice:  ratio(final.CacheWritePrice, official.CacheWritePrice),
		CacheReadPrice:   ratio(final.CacheReadPrice, official.CacheReadPrice),
		ImageOutputPrice: ratio(final.ImageOutputPrice, official.ImageOutputPrice),
		PerRequestPrice:  ratio(final.PerRequestPrice, official.PerRequestPrice),
	}
}

func ratio(value, base float64) float64 {
	if base == 0 {
		return 0
	}
	return value / base
}

func derefFloat(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func appendOrReplaceGroup(groups []modelCatalogGroup, next modelCatalogGroup) []modelCatalogGroup {
	for i := range groups {
		if groups[i].GroupID == next.GroupID {
			groups[i] = next
			return groups
		}
	}
	return append(groups, next)
}

func attachWildcardSupportedGroups(item *modelCatalogItem, channels []Channel, groupsByID map[int64]Group) {
	for i := range channels {
		channel := channels[i]
		if !channel.IsActive() {
			continue
		}
		for _, pricing := range channel.ModelPricing {
			if !pricingMatchesModel(pricing, item.ID) {
				continue
			}
			for _, capability := range NormalizeModelCapabilities(pricing.Capabilities) {
				item.Capabilities[capability] = struct{}{}
			}
			for _, groupID := range channel.GroupIDs {
				group, ok := groupsByID[groupID]
				if !ok || group.Platform != pricing.Platform {
					continue
				}
				resolved := resolveDisplayPricing(*item.Official, pricing)
				item.Groups = appendOrReplaceGroup(item.Groups, modelCatalogGroup{
					GroupID:        group.ID,
					GroupName:      group.Name,
					RateMultiplier: normalizeRateMultiplier(group.RateMultiplier),
					BillingMode:    normalizeBillingMode(pricing.BillingMode),
					Resolved:       resolved,
				})
			}
		}
	}
}

func pricingMatchesModel(pricing ChannelModelPricing, model string) bool {
	for _, pattern := range pricing.Models {
		if modelPatternMatches(pattern, model) {
			return true
		}
	}
	return false
}
```

Implement filtering helpers:

```go
func matchesModelPricingQuery(item *modelCatalogItem, query ModelPricingQuery) bool {
	if query.Platform != "" && !strings.EqualFold(item.Platform, strings.TrimSpace(query.Platform)) {
		return false
	}
	capability := strings.ToLower(strings.TrimSpace(query.Capability))
	if capability != "" {
		if _, ok := item.Capabilities[capability]; !ok {
			return false
		}
	}
	q := strings.ToLower(strings.TrimSpace(query.Q))
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(item.ID), q) || strings.Contains(strings.ToLower(displayModelName(item.ID)), q) || strings.Contains(strings.ToLower(item.Platform), q) {
		return true
	}
	for capability := range item.Capabilities {
		if strings.Contains(capability, q) {
			return true
		}
	}
	return false
}

func sortedStrings(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 5: Run service tests**

Run:

```bash
cd backend && go test ./internal/service -run ModelPricingPageService -count=1
```

Expected: PASS.

- [ ] **Step 6: Write failing handler and route tests**

Create `backend/internal/handler/model_pricing_handler_test.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPricingHandlerListModelsReturnsQueryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&failingModelPricingChannels{},
		&emptyModelPricingGroups{},
		&emptyModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/models", h.ListModels)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models?q=claude", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Contains(t, rec.Body.String(), "MODEL_PRICING_QUERY_FAILED")
}

func TestModelPricingHandlerGetModelReturnsDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
		&singleModelPricingChannels{},
		&singleModelPricingGroups{},
		&singleModelPricingBilling{},
	))
	router := gin.New()
	router.GET("/api/v1/model-pricing/models/:model", h.GetModel)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models/claude-sonnet-4", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data service.ModelPricingModelDetail `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "claude-sonnet-4", body.Data.ID)
	require.Len(t, body.Data.Groups, 1)
}
```

Create `backend/internal/server/routes/model_pricing_test.go`:

```go
package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	handlerpkg "github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPricingRoutesPublicModelsPathIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	RegisterModelPricingRoutes(v1, &handlerpkg.Handlers{
		ModelPricing: handlerpkg.NewModelPricingHandler(service.NewModelPricingPageServiceForTest(
			&singleModelPricingChannels{},
			&singleModelPricingGroups{},
			&singleModelPricingBilling{},
		)),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/model-pricing/models", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
```

Place local stubs in each package so tests compile without importing test helpers from another package.

- [ ] **Step 7: Implement handler and route**

Create `backend/internal/handler/model_pricing_handler.go`:

```go
package handler

import (
	"errors"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelPricingHandler struct {
	service *service.ModelPricingPageService
}

func NewModelPricingHandler(service *service.ModelPricingPageService) *ModelPricingHandler {
	return &ModelPricingHandler{service: service}
}

func (h *ModelPricingHandler) ListModels(c *gin.Context) {
	models, err := h.service.ListModels(c.Request.Context(), service.ModelPricingQuery{
		Q:          strings.TrimSpace(c.Query("q")),
		Platform:   strings.TrimSpace(c.Query("platform")),
		Capability: strings.TrimSpace(c.Query("capability")),
	})
	if err != nil {
		response.ErrorFrom(c, infraerrors.Internal("MODEL_PRICING_QUERY_FAILED", "model pricing query failed"))
		return
	}
	response.Success(c, models)
}

func (h *ModelPricingHandler) GetModel(c *gin.Context) {
	model, err := h.service.GetModel(c.Request.Context(), c.Param("model"))
	if err != nil {
		if errors.Is(err, service.ErrModelPricingNotFound) {
			response.Error(c, http.StatusNotFound, "MODEL_PRICING_NOT_FOUND", "model pricing not found")
			return
		}
		response.ErrorFrom(c, infraerrors.Internal("MODEL_PRICING_QUERY_FAILED", "model pricing query failed"))
		return
	}
	response.Success(c, model)
}
```

Create `backend/internal/server/routes/model_pricing.go`:

```go
package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/gin-gonic/gin"
)

func RegisterModelPricingRoutes(v1 *gin.RouterGroup, h *handler.Handlers) {
	if h == nil || h.ModelPricing == nil {
		return
	}
	modelPricing := v1.Group("/model-pricing")
	{
		modelPricing.GET("/models", h.ModelPricing.ListModels)
		modelPricing.GET("/models/:model", h.ModelPricing.GetModel)
	}
}
```

In `backend/internal/server/router.go`, replace:

```go
routes.RegisterPricingPlanRoutes(v1, h)
```

with:

```go
routes.RegisterModelPricingRoutes(v1, h)
```

In `backend/internal/handler/handler.go`, add:

```go
ModelPricing *ModelPricingHandler
```

to `Handlers`, and keep `PricingPlan` until Task 6 removes old code if this task is executed before removal.

In `backend/internal/handler/wire.go`, add `modelPricingHandler *ModelPricingHandler` to `ProvideHandlers`, set `ModelPricing: modelPricingHandler`, and add `NewModelPricingHandler` to `ProviderSet`.

In `backend/internal/service/wire.go`, add `NewModelPricingPageService` to `ProviderSet`.

- [ ] **Step 8: Regenerate Wire output**

Run:

```bash
cd backend && go generate ./cmd/server
```

Expected: command exits 0 and updates `backend/cmd/server/wire_gen.go`.

- [ ] **Step 9: Run focused backend tests**

Run:

```bash
cd backend && go test ./internal/service -run ModelPricingPageService -count=1
cd backend && go test ./internal/handler -run ModelPricingHandler -count=1
cd backend && go test ./internal/server/routes -run ModelPricingRoutes -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit Task 2**

```bash
git add backend/internal/service/model_pricing_page.go backend/internal/service/model_pricing_page_test.go backend/internal/handler/model_pricing_handler.go backend/internal/handler/model_pricing_handler_test.go backend/internal/server/routes/model_pricing.go backend/internal/server/routes/model_pricing_test.go backend/internal/server/router.go backend/internal/handler/handler.go backend/internal/handler/wire.go backend/internal/service/wire.go backend/cmd/server/wire_gen.go
git commit -m "feat: add public model pricing api"
```

---

### Task 3: 前端模型定价 API 与公开页面

**Files:**
- Create: `frontend/src/api/modelPricing.ts`
- Create: `frontend/src/views/ModelPricingView.vue`
- Create: `frontend/src/views/__tests__/ModelPricingView.spec.ts`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

**Interfaces:**
- Consumes:
  - `GET /api/v1/model-pricing/models?q=&platform=&capability=`
  - `GET /api/v1/model-pricing/models/:model`
- Produces:
  - `modelPricingAPI.listModels(params, options)`
  - `modelPricingAPI.getModel(model, options)`
  - public route `/model-pricing`
  - visible error text `查询异常`

- [ ] **Step 1: Create API client types**

Create `frontend/src/api/modelPricing.ts`:

```ts
import { apiClient } from './client'

export type ModelCapability = 'chat' | 'image' | 'video'

export interface ModelPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number
  output_price: number
  cache_write_price: number
  cache_read_price: number
  per_request_price: number
}

export interface ModelPricingAmount {
  input_price: number
  output_price: number
  cache_write_price: number
  cache_read_price: number
  image_output_price: number
  per_request_price: number
  intervals: ModelPricingInterval[]
}

export interface ModelPricingMultipliers {
  input_price: number
  output_price: number
  cache_write_price: number
  cache_read_price: number
  image_output_price: number
  per_request_price: number
}

export interface ModelPricingSummary {
  id: string
  display_name: string
  platform: string
  capabilities: ModelCapability[]
  supported_group_count: number
  official_price: ModelPricingAmount
}

export interface ModelPricingGroupPrice {
  group_id: number
  group_name: string
  rate_multiplier: number
  billing_mode: 'token' | 'per_request' | 'image'
  price: ModelPricingAmount
  multipliers: ModelPricingMultipliers
}

export interface ModelPricingDetail extends ModelPricingSummary {
  groups: ModelPricingGroupPrice[]
}

export interface ListModelPricingParams {
  q?: string
  platform?: string
  capability?: ModelCapability | ''
}

export async function listModels(
  params: ListModelPricingParams = {},
  options?: { signal?: AbortSignal }
): Promise<ModelPricingSummary[]> {
  const { data } = await apiClient.get<ModelPricingSummary[]>('/model-pricing/models', {
    params,
    signal: options?.signal
  })
  return data
}

export async function getModel(
  model: string,
  options?: { signal?: AbortSignal }
): Promise<ModelPricingDetail> {
  const { data } = await apiClient.get<ModelPricingDetail>(`/model-pricing/models/${encodeURIComponent(model)}`, {
    signal: options?.signal
  })
  return data
}

export const modelPricingAPI = { listModels, getModel }
export default modelPricingAPI
```

- [ ] **Step 2: Add failing view tests**

Create `frontend/src/views/__tests__/ModelPricingView.spec.ts`:

```ts
import { mount, flushPromises } from '@vue/test-utils'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import ModelPricingView from '../ModelPricingView.vue'

const listModels = vi.fn()
const getModel = vi.fn()

vi.mock('@/api/modelPricing', () => ({
  modelPricingAPI: { listModels, getModel },
  default: { listModels, getModel }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn(), siteName: 'FluxCode' }),
  useAuthStore: () => ({ isAuthenticated: false, isAdmin: false })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback || key
  })
}))

describe('ModelPricingView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listModels.mockResolvedValue([
      {
        id: 'claude-sonnet-4',
        display_name: 'claude-sonnet-4',
        platform: 'anthropic',
        capabilities: ['chat'],
        supported_group_count: 2,
        official_price: {
          input_price: 0.000003,
          output_price: 0.000015,
          cache_write_price: 0.00000375,
          cache_read_price: 0.0000003,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        }
      }
    ])
    getModel.mockResolvedValue({
      id: 'claude-sonnet-4',
      display_name: 'claude-sonnet-4',
      platform: 'anthropic',
      capabilities: ['chat'],
      supported_group_count: 2,
      official_price: {
        input_price: 0.000003,
        output_price: 0.000015,
        cache_write_price: 0.00000375,
        cache_read_price: 0.0000003,
        image_output_price: 0,
        per_request_price: 0,
        intervals: []
      },
      groups: [{
        group_id: 1,
        group_name: '基础组',
        rate_multiplier: 2,
        billing_mode: 'token',
        price: {
          input_price: 0.000006,
          output_price: 0.00003,
          cache_write_price: 0.0000075,
          cache_read_price: 0.0000006,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        },
        multipliers: {
          input_price: 2,
          output_price: 2,
          cache_write_price: 2,
          cache_read_price: 2,
          image_output_price: 0,
          per_request_price: 0
        }
      }]
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('renders model cards and loads model detail after click', async () => {
    const wrapper = mount(ModelPricingView)
    await flushPromises()

    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('anthropic')
    expect(wrapper.text()).toContain('2')

    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenCalledWith('claude-sonnet-4', expect.any(Object))
    expect(wrapper.text()).toContain('基础组')
    expect(wrapper.text()).toContain('2.00x')
  })

  it('debounces search for 300ms', async () => {
    const wrapper = mount(ModelPricingView)
    await flushPromises()

    await wrapper.get('[data-testid="model-pricing-search"]').setValue('claude')
    expect(listModels).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(299)
    expect(listModels).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()
    expect(listModels).toHaveBeenCalledWith({ q: 'claude', platform: '', capability: '' }, expect.any(Object))
  })

  it('shows query error and retries', async () => {
    listModels.mockRejectedValueOnce(new Error('network'))
    const wrapper = mount(ModelPricingView)
    await flushPromises()

    expect(wrapper.text()).toContain('查询异常')
    listModels.mockResolvedValueOnce([])
    await wrapper.get('[data-testid="model-pricing-retry"]').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenCalledTimes(2)
  })
})
```

- [ ] **Step 3: Run view tests to verify they fail**

Run:

```bash
pnpm --dir frontend vitest run src/views/__tests__/ModelPricingView.spec.ts
```

Expected: FAIL because `ModelPricingView.vue` and API module do not exist.

- [ ] **Step 4: Implement public model pricing view**

Create `frontend/src/views/ModelPricingView.vue` with this structure:

```vue
<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="mx-auto max-w-7xl px-4 pb-16 pt-24 sm:px-6 lg:px-8">
      <div class="mb-6 flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
            {{ t('modelPricing.title', '模型定价') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">
            {{ t('modelPricing.description', '按模型查看不同分组的调用价格') }}
          </p>
        </div>
        <div class="grid gap-2 sm:grid-cols-[minmax(220px,1fr)_160px_160px]">
          <input
            data-testid="model-pricing-search"
            v-model="searchInput"
            class="input"
            :placeholder="t('modelPricing.searchPlaceholder', '搜索模型、平台或能力')"
          />
          <select v-model="platformFilter" class="input">
            <option value="">{{ t('modelPricing.allPlatforms', '全部平台') }}</option>
            <option v-for="platform in platforms" :key="platform" :value="platform">{{ platform }}</option>
          </select>
          <select v-model="capabilityFilter" class="input">
            <option value="">{{ t('modelPricing.allCapabilities', '全部能力') }}</option>
            <option value="chat">{{ t('modelPricing.capabilities.chat', '对话') }}</option>
            <option value="image">{{ t('modelPricing.capabilities.image', '图片') }}</option>
            <option value="video">{{ t('modelPricing.capabilities.video', '视频') }}</option>
          </select>
        </div>
      </div>

      <div v-if="error" class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/30 dark:text-red-300">
        <div class="flex items-center justify-between gap-3">
          <span>{{ t('modelPricing.queryError', '查询异常') }}</span>
          <button data-testid="model-pricing-retry" type="button" class="btn btn-secondary" @click="loadModels">
            {{ t('common.retry', '重试') }}
          </button>
        </div>
      </div>

      <div v-else class="grid gap-5 lg:grid-cols-[minmax(0,380px)_1fr]">
        <section class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-900">
          <div v-if="loading" class="p-6 text-sm text-gray-500">{{ t('common.loading', '加载中...') }}</div>
          <div v-else-if="models.length === 0" class="p-6 text-sm text-gray-500">{{ t('modelPricing.empty', '未找到匹配模型') }}</div>
          <button
            v-for="model in models"
            :key="model.id"
            :data-testid="`model-card-${model.id}`"
            type="button"
            class="mb-2 block w-full rounded-md border p-3 text-left transition hover:border-primary-300 hover:bg-primary-50/40 dark:hover:bg-primary-950/20"
            :class="selectedModelId === model.id ? 'border-primary-400 bg-primary-50 dark:border-primary-600 dark:bg-primary-950/30' : 'border-gray-200 dark:border-dark-700'"
            @click="selectModel(model.id)"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="truncate text-sm font-semibold">{{ model.display_name || model.id }}</div>
                <div class="mt-1 text-xs text-gray-500">{{ model.platform }}</div>
              </div>
              <span class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-dark-200">
                {{ model.supported_group_count }}
              </span>
            </div>
            <div class="mt-3 flex flex-wrap gap-1">
              <span v-for="capability in model.capabilities" :key="capability" class="rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-dark-700">
                {{ capabilityLabel(capability) }}
              </span>
            </div>
            <div class="mt-3 text-xs text-gray-500">
              {{ t('modelPricing.inputOutput', '输入/输出') }}:
              {{ formatTokenPrice(model.official_price.input_price) }} /
              {{ formatTokenPrice(model.official_price.output_price) }}
            </div>
          </button>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
          <div v-if="detailLoading" class="text-sm text-gray-500">{{ t('common.loading', '加载中...') }}</div>
          <div v-else-if="!detail" class="text-sm text-gray-500">{{ t('modelPricing.selectHint', '选择模型查看分组价格') }}</div>
          <div v-else>
            <div class="mb-4">
              <h2 class="text-xl font-semibold">{{ detail.display_name || detail.id }}</h2>
              <div class="mt-2 flex flex-wrap items-center gap-2 text-sm text-gray-500">
                <span>{{ detail.platform }}</span>
                <span v-for="capability in detail.capabilities" :key="capability" class="rounded bg-gray-100 px-2 py-0.5 text-xs dark:bg-dark-700">
                  {{ capabilityLabel(capability) }}
                </span>
              </div>
            </div>

            <div class="overflow-x-auto">
              <table class="min-w-full text-sm">
                <thead>
                  <tr class="border-b border-gray-200 text-left text-xs text-gray-500 dark:border-dark-700">
                    <th class="py-2 pr-4">{{ t('modelPricing.group', '分组') }}</th>
                    <th class="py-2 pr-4">{{ t('modelPricing.rate', '倍率') }}</th>
                    <th class="py-2 pr-4">{{ t('modelPricing.input', '输入') }}</th>
                    <th class="py-2 pr-4">{{ t('modelPricing.output', '输出') }}</th>
                    <th class="py-2 pr-4">{{ t('modelPricing.cacheWrite', '缓存写入') }}</th>
                    <th class="py-2 pr-4">{{ t('modelPricing.cacheRead', '缓存读取') }}</th>
                    <th class="py-2 pr-4">{{ t('modelPricing.requestOrImage', '按次/图片') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="group in detail.groups" :key="group.group_id" class="border-b border-gray-100 dark:border-dark-800">
                    <td class="py-3 pr-4 font-medium">{{ group.group_name }}</td>
                    <td class="py-3 pr-4">{{ group.rate_multiplier.toFixed(2) }}x</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.input_price, group.multipliers.input_price) }}</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.output_price, group.multipliers.output_price) }}</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.cache_write_price, group.multipliers.cache_write_price) }}</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.cache_read_price, group.multipliers.cache_read_price) }}</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.per_request_price || group.price.image_output_price, group.multipliers.per_request_price || group.multipliers.image_output_price) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import { modelPricingAPI, type ModelCapability, type ModelPricingDetail, type ModelPricingSummary } from '@/api/modelPricing'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')
const models = ref<ModelPricingSummary[]>([])
const detail = ref<ModelPricingDetail | null>(null)
const selectedModelId = ref('')
const searchInput = ref('')
const debouncedSearch = ref('')
const platformFilter = ref('')
const capabilityFilter = ref<ModelCapability | ''>('')
const loading = ref(false)
const detailLoading = ref(false)
const error = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

const platforms = computed(() => Array.from(new Set(models.value.map((model) => model.platform))).sort())

watch(searchInput, (value) => {
  if (searchTimer) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    debouncedSearch.value = value.trim()
  }, 300)
})

watch([debouncedSearch, platformFilter, capabilityFilter], () => {
  loadModels()
})

async function loadModels() {
  loading.value = true
  error.value = false
  try {
    models.value = await modelPricingAPI.listModels({
      q: debouncedSearch.value,
      platform: platformFilter.value,
      capability: capabilityFilter.value
    }, {})
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

async function selectModel(modelId: string) {
  selectedModelId.value = modelId
  detailLoading.value = true
  try {
    detail.value = await modelPricingAPI.getModel(modelId, {})
  } finally {
    detailLoading.value = false
  }
}

function capabilityLabel(capability: string): string {
  if (capability === 'chat') return t('modelPricing.capabilities.chat', '对话')
  if (capability === 'image') return t('modelPricing.capabilities.image', '图片')
  if (capability === 'video') return t('modelPricing.capabilities.video', '视频')
  return capability
}

function formatTokenPrice(value: number): string {
  if (!value) return '-'
  return `$${(value * 1_000_000).toPrecision(6)}/MTok`
}

function formatPriceWithMultiplier(price: number, multiplier: number): string {
  if (!price) return '-'
  const priceText = price < 0.001 ? formatTokenPrice(price) : `$${price.toFixed(6)}`
  return `${priceText} · ${multiplier.toFixed(2)}x`
}

onMounted(loadModels)
</script>
```

- [ ] **Step 5: Register route and i18n keys**

In `frontend/src/router/index.ts`, add public route after `/home`:

```ts
{
  path: '/model-pricing',
  name: 'ModelPricing',
  component: () => import('@/views/ModelPricingView.vue'),
  meta: {
    requiresAuth: false,
    title: 'Model Pricing',
    titleKey: 'modelPricing.title'
  }
},
```

In `frontend/src/i18n/locales/zh.ts`, add:

```ts
modelPricing: {
  title: '模型定价',
  description: '按模型查看不同分组的调用价格',
  searchPlaceholder: '搜索模型、平台或能力',
  allPlatforms: '全部平台',
  allCapabilities: '全部能力',
  queryError: '查询异常',
  empty: '未找到匹配模型',
  selectHint: '选择模型查看分组价格',
  inputOutput: '输入/输出',
  group: '分组',
  rate: '倍率',
  input: '输入',
  output: '输出',
  cacheWrite: '缓存写入',
  cacheRead: '缓存读取',
  requestOrImage: '按次/图片',
  capabilities: {
    chat: '对话',
    image: '图片',
    video: '视频'
  }
}
```

In `frontend/src/i18n/locales/en.ts`, add matching keys:

```ts
modelPricing: {
  title: 'Model Pricing',
  description: 'View model prices by supported group',
  searchPlaceholder: 'Search model, platform, or capability',
  allPlatforms: 'All platforms',
  allCapabilities: 'All capabilities',
  queryError: 'Query failed',
  empty: 'No matching models found',
  selectHint: 'Select a model to view group pricing',
  inputOutput: 'Input/output',
  group: 'Group',
  rate: 'Rate',
  input: 'Input',
  output: 'Output',
  cacheWrite: 'Cache write',
  cacheRead: 'Cache read',
  requestOrImage: 'Request/image',
  capabilities: {
    chat: 'Chat',
    image: 'Image',
    video: 'Video'
  }
}
```

- [ ] **Step 6: Run front-end focused tests**

Run:

```bash
pnpm --dir frontend vitest run src/views/__tests__/ModelPricingView.spec.ts
```

Expected: PASS.

- [ ] **Step 7: Commit Task 3**

```bash
git add frontend/src/api/modelPricing.ts frontend/src/views/ModelPricingView.vue frontend/src/views/__tests__/ModelPricingView.spec.ts frontend/src/router/index.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: add public model pricing page"
```

---

### Task 4: 渠道管理页能力标签 UI

**Files:**
- Modify: `frontend/src/api/admin/channels.ts`
- Modify: `frontend/src/components/admin/channel/types.ts`
- Modify: `frontend/src/components/admin/channel/PricingEntryCard.vue`
- Modify: `frontend/src/views/admin/ChannelsView.vue`
- Create: `frontend/src/components/admin/channel/__tests__/PricingEntryCard.capabilities.spec.ts`
- Create: `frontend/src/views/admin/__tests__/ChannelsView.capabilities.spec.ts`

**Interfaces:**
- Consumes: admin channel API `model_pricing[].capabilities`.
- Produces:
  - `type ModelCapability = 'chat' | 'image' | 'video'`
  - `PricingFormEntry.capabilities: ModelCapability[]`
  - UI labels `对话`、`图片`、`视频`

- [ ] **Step 1: Update TypeScript API and form types**

In `frontend/src/api/admin/channels.ts`, add:

```ts
export type ModelCapability = 'chat' | 'image' | 'video'
```

Extend `ChannelModelPricing`:

```ts
capabilities: ModelCapability[]
```

In `frontend/src/components/admin/channel/types.ts`, import and use `ModelCapability`:

```ts
import type { BillingMode, PricingInterval, ModelCapability } from '@/api/admin/channels'
```

Extend `PricingFormEntry`:

```ts
capabilities: ModelCapability[]
```

Add:

```ts
export const MODEL_CAPABILITY_OPTIONS: { value: ModelCapability; label: string }[] = [
  { value: 'chat', label: '对话' },
  { value: 'image', label: '图片' },
  { value: 'video', label: '视频' }
]

export function normalizeCapabilities(input: unknown): ModelCapability[] {
  if (!Array.isArray(input)) return []
  const allowed: ModelCapability[] = ['chat', 'image', 'video']
  const out: ModelCapability[] = []
  for (const item of input) {
    if (allowed.includes(item as ModelCapability) && !out.includes(item as ModelCapability)) {
      out.push(item as ModelCapability)
    }
  }
  return out
}
```

- [ ] **Step 2: Add failing PricingEntryCard tests**

Create `frontend/src/components/admin/channel/__tests__/PricingEntryCard.capabilities.spec.ts`:

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import PricingEntryCard from '../PricingEntryCard.vue'
import type { PricingFormEntry } from '../types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (_key: string, fallback?: string) => fallback || _key })
}))

const baseEntry: PricingFormEntry = {
  models: ['claude-sonnet-4'],
  capabilities: ['chat'],
  billing_mode: 'token',
  input_price: null,
  output_price: null,
  cache_write_price: null,
  cache_read_price: null,
  image_output_price: null,
  per_request_price: null,
  intervals: []
}

describe('PricingEntryCard capabilities', () => {
  it('emits updated capabilities when labels are toggled', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: baseEntry, platform: 'anthropic' },
      global: {
        stubs: {
          Icon: true,
          Select: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    await wrapper.get('[data-testid="capability-image"]').setValue(true)
    const updates = wrapper.emitted('update') || []
    expect(updates.at(-1)?.[0]).toMatchObject({
      capabilities: ['chat', 'image']
    })
  })
})
```

- [ ] **Step 3: Implement capability controls**

In `frontend/src/components/admin/channel/PricingEntryCard.vue`, import:

```ts
import { MODEL_CAPABILITY_OPTIONS, normalizeCapabilities } from './types'
import type { ModelCapability } from '@/api/admin/channels'
```

Add this block under model list and billing mode row:

```vue
<div class="mt-3">
  <label class="text-xs font-medium text-gray-500 dark:text-gray-400">
    {{ t('admin.channels.form.capabilities', '能力标签') }}
  </label>
  <div class="mt-2 flex flex-wrap gap-2">
    <label
      v-for="option in capabilityOptions"
      :key="option.value"
      class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-200 px-2 py-1 text-xs dark:border-dark-600"
    >
      <input
        :data-testid="`capability-${option.value}`"
        type="checkbox"
        class="rounded border-gray-300 text-primary-600"
        :checked="normalizedCapabilities.includes(option.value)"
        @change="toggleCapability(option.value, ($event.target as HTMLInputElement).checked)"
      />
      <span>{{ capabilityLabel(option.value) }}</span>
    </label>
  </div>
</div>
```

Add script:

```ts
const capabilityOptions = MODEL_CAPABILITY_OPTIONS
const normalizedCapabilities = computed(() => normalizeCapabilities(props.entry.capabilities))

function capabilityLabel(value: ModelCapability): string {
  if (value === 'chat') return t('admin.channels.form.capabilityChat', '对话')
  if (value === 'image') return t('admin.channels.form.capabilityImage', '图片')
  return t('admin.channels.form.capabilityVideo', '视频')
}

function toggleCapability(value: ModelCapability, checked: boolean) {
  const next = normalizedCapabilities.value.filter((item) => item !== value)
  if (checked) next.push(value)
  emit('update', { ...props.entry, capabilities: next })
}
```

- [ ] **Step 4: Thread capabilities through ChannelsView forms**

In `frontend/src/views/admin/ChannelsView.vue`, when adding a pricing entry, include:

```ts
capabilities: [],
```

In `formToAPI()`, include:

```ts
capabilities: normalizeCapabilities(entry.capabilities),
```

In API-to-form conversion for channel `model_pricing`, include:

```ts
capabilities: normalizeCapabilities(p.capabilities),
```

In account stats pricing rule conversion, include the same `capabilities` property so rule pricing forms remain type-compatible.

- [ ] **Step 5: Add ChannelsView save test**

Create `frontend/src/views/admin/__tests__/ChannelsView.capabilities.spec.ts` with a focused mapper test if `formToAPI` is exportable. If not exportable, mount `ChannelsView` and assert the update request payload:

```ts
it('saves model pricing capabilities in channel payload', async () => {
  channelsAPI.update.mockResolvedValue({ ...channelFixture, model_pricing: [] })
  const wrapper = mount(ChannelsView, { global: channelViewStubs })
  await flushPromises()

  await openEditDialog(wrapper)
  await wrapper.get('[data-testid="capability-image"]').setValue(true)
  await wrapper.get('[data-testid="channel-save"]').trigger('click')
  await flushPromises()

  expect(channelsAPI.update).toHaveBeenCalledWith(expect.any(Number), expect.objectContaining({
    model_pricing: expect.arrayContaining([
      expect.objectContaining({ capabilities: expect.arrayContaining(['image']) })
    ])
  }))
})
```

Use existing `ChannelsView` test helper style in this repository for `channelViewStubs`, `openEditDialog`, and mocked APIs.

- [ ] **Step 6: Run frontend focused tests**

Run:

```bash
pnpm --dir frontend vitest run src/components/admin/channel/__tests__/PricingEntryCard.capabilities.spec.ts src/views/admin/__tests__/ChannelsView.capabilities.spec.ts
```

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

```bash
git add frontend/src/api/admin/channels.ts frontend/src/components/admin/channel/types.ts frontend/src/components/admin/channel/PricingEntryCard.vue frontend/src/views/admin/ChannelsView.vue frontend/src/components/admin/channel/__tests__/PricingEntryCard.capabilities.spec.ts frontend/src/views/admin/__tests__/ChannelsView.capabilities.spec.ts
git commit -m "feat: configure model capability tags"
```

---

### Task 5: 导航入口与旧菜单移除

**Files:**
- Modify: `frontend/src/components/layout/PublicHeader.vue`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Modify: `frontend/src/router/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/i18n/__tests__/navigationLocales.spec.ts`
- Modify: `frontend/src/router/__tests__/admin-routes.spec.ts`
- Test: `frontend/src/components/layout/__tests__/PublicHeader.spec.ts`
- Create: `frontend/src/components/layout/__tests__/AppSidebar.modelPricing.spec.ts`

**Interfaces:**
- Consumes: route `/model-pricing`.
- Produces: top menu label `模型定价`; sidebar item path `/model-pricing`; removed admin sidebar path `/admin/pricing-plans`; removed public route `/pricing`.

- [ ] **Step 1: Add failing navigation locale tests**

Update `frontend/src/i18n/__tests__/navigationLocales.spec.ts`:

```ts
expect(zh.nav.modelPricing).toBe('模型定价')
expect(en.nav.modelPricing).toBe('Model Pricing')
expect(zh.home.nav.modelPricing).toBe('模型定价')
expect(en.home.nav.modelPricing).toBe('Model Pricing')
expect(zh.nav.pricingPlans).toBeUndefined()
expect(en.nav.pricingPlans).toBeUndefined()
```

- [ ] **Step 2: Add failing route/menu tests**

Update `frontend/src/router/__tests__/admin-routes.spec.ts`:

```ts
it('does not register old admin pricing plans route', () => {
  const oldRoute = router.getRoutes().find((route) => route.path === '/admin/pricing-plans')
  expect(oldRoute).toBeUndefined()
})

it('registers public model pricing route', () => {
  const route = router.getRoutes().find((route) => route.path === '/model-pricing')
  expect(route?.meta.requiresAuth).toBe(false)
})
```

Update `frontend/src/components/layout/__tests__/PublicHeader.spec.ts` to assert `模型定价` link exists with `href="/model-pricing"`.

Create `frontend/src/components/layout/__tests__/AppSidebar.modelPricing.spec.ts`:

```ts
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AppSidebar from '../AppSidebar.vue'

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    mobileSidebarOpen: true,
    backendModeEnabled: false,
    cachedPublicSettings: { custom_menu_items: [] },
    channelMonitorEnabled: false
  }),
  useAuthStore: () => ({
    isAdmin: false,
    isSimpleMode: false,
    user: { role: 'user' }
  }),
  useAdminSettingsStore: () => ({
    customMenuItems: [],
    channelMonitorEnabled: false,
    opsMonitoringEnabled: false,
    paymentEnabled: false
  })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard' }),
  RouterLink: { template: '<a :href="to"><slot /></a>', props: ['to'] }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => ({ 'nav.modelPricing': '模型定价' }[key] || key) })
}))

describe('AppSidebar model pricing', () => {
  it('shows model pricing and hides pricing plans', () => {
    const wrapper = mount(AppSidebar, {
      global: { stubs: { VersionBadge: true } }
    })
    expect(wrapper.text()).toContain('模型定价')
    expect(wrapper.html()).toContain('/model-pricing')
    expect(wrapper.html()).not.toContain('/admin/pricing-plans')
  })
})
```

- [ ] **Step 3: Update i18n keys**

In `frontend/src/i18n/locales/zh.ts`:

```ts
nav: {
  modelPricing: '模型定价',
}
home: {
  nav: {
    modelPricing: '模型定价',
  }
}
```

Remove `nav.pricingPlans` if no remaining code references it after this task.

In `frontend/src/i18n/locales/en.ts`:

```ts
nav: {
  modelPricing: 'Model Pricing',
}
home: {
  nav: {
    modelPricing: 'Model Pricing',
  }
}
```

- [ ] **Step 4: Update PublicHeader**

In `frontend/src/components/layout/PublicHeader.vue`, add desktop link near purchase/docs links:

```vue
<router-link
  to="/model-pricing"
  class="rounded-full px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-black/5 hover:text-gray-900 dark:text-dark-200 dark:hover:bg-white/10 dark:hover:text-white"
  @click="closeMobileMenu"
>
  {{ t('home.nav.modelPricing') }}
</router-link>
```

Add matching mobile link:

```vue
<router-link
  to="/model-pricing"
  class="rounded-2xl border border-black/5 bg-white/70 px-4 py-3 text-sm font-medium text-gray-800 shadow-sm backdrop-blur transition-colors hover:bg-white/90 dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-100 dark:hover:bg-dark-900/55"
  @click="closeMobileMenu"
>
  {{ t('home.nav.modelPricing') }}
</router-link>
```

- [ ] **Step 5: Update AppSidebar**

In `frontend/src/components/layout/AppSidebar.vue`, add a model pricing nav item to `userNavItems`:

```ts
{ path: '/model-pricing', label: t('nav.modelPricing'), icon: CurrencyYenIcon },
```

Add a model pricing nav item to `personalNavItems` so admins also see the public page in their personal section:

```ts
{ path: '/model-pricing', label: t('nav.modelPricing'), icon: CurrencyYenIcon },
```

Remove this admin item from `adminNavItems`:

```ts
{
  path: '/admin/pricing-plans',
  label: t('nav.pricingPlans'),
  icon: CurrencyYenIcon,
  hideInSimpleMode: true
},
```

- [ ] **Step 6: Remove public `/pricing` and admin `/admin/pricing-plans` routes**

In `frontend/src/router/index.ts`, delete the route with:

```ts
path: '/pricing',
name: 'Pricing',
component: () => import('@/views/PricingView.vue')
```

Delete the route with:

```ts
path: '/admin/pricing-plans',
name: 'AdminPricingPlans',
component: () => import('@/views/admin/PricingPlansView.vue')
```

Keep `/purchase`, `/orders`, and `/admin/orders/plans` unchanged.

- [ ] **Step 7: Run navigation tests**

Run:

```bash
pnpm --dir frontend vitest run src/i18n/__tests__/navigationLocales.spec.ts src/router/__tests__/admin-routes.spec.ts src/components/layout/__tests__/PublicHeader.spec.ts src/components/layout/__tests__/AppSidebar.modelPricing.spec.ts
```

Expected: PASS.

- [ ] **Step 8: Commit Task 5**

```bash
git add frontend/src/components/layout/PublicHeader.vue frontend/src/components/layout/AppSidebar.vue frontend/src/router/index.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/i18n/__tests__/navigationLocales.spec.ts frontend/src/router/__tests__/admin-routes.spec.ts frontend/src/components/layout/__tests__/PublicHeader.spec.ts frontend/src/components/layout/__tests__/AppSidebar.modelPricing.spec.ts
git commit -m "feat: add model pricing navigation"
```

---

### Task 6: 移除旧“定价方案”前后端运行时代码

**Files:**
- Delete: `backend/internal/server/routes/pricing_plan.go`
- Delete: `backend/internal/server/routes/pricing_plan_test.go`
- Delete: `backend/internal/handler/pricing_plan_handler.go`
- Delete: `backend/internal/handler/pricing_plan_handler_test.go`
- Delete: `backend/internal/handler/admin/pricing_plan_handler.go`
- Delete: `backend/internal/handler/admin/pricing_plan_handler_test.go`
- Delete: `backend/internal/service/pricing_plan.go`
- Delete: `backend/internal/service/pricing_plan_service.go`
- Delete: `backend/internal/repository/pricing_plan_repo.go`
- Delete: `backend/internal/repository/pricing_plan_repo_integration_test.go`
- Delete: `frontend/src/views/PricingView.vue`
- Delete: `frontend/src/views/__tests__/PricingView.spec.ts`
- Delete: `frontend/src/views/admin/PricingPlansView.vue`
- Delete: `frontend/src/views/admin/__tests__/PricingPlansView.spec.ts`
- Delete: `frontend/src/api/pricingPlans.ts`
- Delete: `frontend/src/api/admin/pricingPlans.ts`
- Modify: `backend/internal/handler/handler.go`
- Modify: `backend/internal/handler/wire.go`
- Modify: `backend/internal/service/wire.go`
- Modify: `backend/internal/repository/wire.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/cmd/server/wire_gen.go`
- Modify: `frontend/src/api/admin/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/types/index.ts` if it defines pricing plan DTOs.

**Interfaces:**
- Consumes: Task 2 has already provided replacement `ModelPricing`.
- Produces: no route handler, service, repository, frontend page, or API import references `PricingPlan`, `pricingPlansAPI`, `/pricing/plan-groups`, or `/admin/pricing-plans`.

- [ ] **Step 1: Search references before deletion**

Run:

```bash
rg -n "PricingPlan|pricingPlans|pricing-plans|/pricing|plan-groups|PricingView|PricingPlansView" backend frontend
```

Expected: output shows only files listed in this task plus migration/schema historical references. Keep migration references in `backend/migrations/079_pricing_plan_tables.sql` and schema integration checks; they protect retained DB tables.

- [ ] **Step 2: Remove backend route registrations and DI fields**

In `backend/internal/server/routes/admin.go`, delete:

```go
registerPricingPlanRoutes(admin, h)
```

and delete the `registerPricingPlanRoutes` function.

In `backend/internal/handler/handler.go`, remove:

```go
PricingPlan *admin.PricingPlanHandler
PricingPlan *PricingPlanHandler
```

from admin and public handler structs.

In `backend/internal/handler/wire.go`, remove `pricingPlanHandler *admin.PricingPlanHandler` and `pricingPlanHandler *PricingPlanHandler` constructor parameters, struct assignments, and provider entries:

```go
NewPricingPlanHandler
admin.NewPricingPlanHandler
```

In `backend/internal/service/wire.go`, remove:

```go
NewPricingPlanService
```

In `backend/internal/repository/wire.go`, remove:

```go
NewPricingPlanRepository
```

- [ ] **Step 3: Delete backend pricing plan files**

Delete:

```bash
rm backend/internal/server/routes/pricing_plan.go
rm backend/internal/server/routes/pricing_plan_test.go
rm backend/internal/handler/pricing_plan_handler.go
rm backend/internal/handler/pricing_plan_handler_test.go
rm backend/internal/handler/admin/pricing_plan_handler.go
rm backend/internal/handler/admin/pricing_plan_handler_test.go
rm backend/internal/service/pricing_plan.go
rm backend/internal/service/pricing_plan_service.go
rm backend/internal/repository/pricing_plan_repo.go
rm backend/internal/repository/pricing_plan_repo_integration_test.go
```

Use normal file deletion through the executor; do not delete migration files.

- [ ] **Step 4: Regenerate Wire and run backend compile tests**

Run:

```bash
cd backend && go generate ./cmd/server
cd backend && go test ./internal/handler ./internal/server/routes ./internal/service ./internal/repository -count=1
```

Expected: PASS. If a package still references pricing plan symbols, remove that import or assertion and rerun the same command.

- [ ] **Step 5: Remove frontend old pricing plan files and imports**

Delete:

```bash
rm frontend/src/views/PricingView.vue
rm frontend/src/views/__tests__/PricingView.spec.ts
rm frontend/src/views/admin/PricingPlansView.vue
rm frontend/src/views/admin/__tests__/PricingPlansView.spec.ts
rm frontend/src/api/pricingPlans.ts
rm frontend/src/api/admin/pricingPlans.ts
```

In `frontend/src/api/admin/index.ts`, remove:

```ts
import pricingPlansAPI from './pricingPlans'
pricingPlans: pricingPlansAPI,
pricingPlansAPI,
```

In `frontend/src/types/index.ts`, if pricing plan interfaces are only used by removed pages/API, remove:

```ts
PricingPlan
PricingPlanGroup
PricingPlanContactMethod
```

In `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`, remove `admin.pricingPlans` blocks if `rg "admin\\.pricingPlans|pricingPlans"` shows no remaining runtime references.

- [ ] **Step 6: Verify removed references**

Run:

```bash
rg -n "PricingPlan|pricingPlans|pricing-plans|/pricing/plan-groups|/admin/pricing-plans|PricingView|PricingPlansView" backend frontend
```

Expected: no runtime code references. Acceptable remaining matches are:

```text
backend/migrations/079_pricing_plan_tables.sql
backend/internal/repository/migrations_schema_integration_test.go
```

- [ ] **Step 7: Run frontend compile tests**

Run:

```bash
pnpm --dir frontend run typecheck
pnpm --dir frontend vitest run src/router/__tests__/admin-routes.spec.ts src/i18n/__tests__/navigationLocales.spec.ts
```

Expected: PASS.

- [ ] **Step 8: Commit Task 6**

```bash
git add -A backend/internal/server/routes backend/internal/handler backend/internal/service backend/internal/repository backend/cmd/server/wire_gen.go frontend/src/views frontend/src/api frontend/src/types frontend/src/i18n frontend/src/router
git commit -m "refactor: remove pricing plan runtime"
```

---

### Task 7: 集成验证与收口

**Files:**
- Modify only files needed to fix issues revealed by verification.

**Interfaces:**
- Consumes all previous tasks.
- Produces: passing focused backend/frontend tests, build sanity, no stale old pricing plan runtime references, and a working local dev URL if the user wants to inspect UI.

- [ ] **Step 1: Run backend focused test suite**

Run:

```bash
cd backend && go test ./internal/service ./internal/handler ./internal/handler/admin ./internal/server/routes ./internal/repository -count=1
```

Expected: PASS.

- [ ] **Step 2: Run frontend focused and type tests**

Run:

```bash
pnpm --dir frontend run typecheck
pnpm --dir frontend vitest run src/views/__tests__/ModelPricingView.spec.ts src/components/admin/channel/__tests__/PricingEntryCard.capabilities.spec.ts src/i18n/__tests__/navigationLocales.spec.ts src/router/__tests__/admin-routes.spec.ts
```

Expected: PASS.

- [ ] **Step 3: Run stale-reference checks**

Run:

```bash
rg -n "PricingPlan|pricingPlans|pricing-plans|/pricing/plan-groups|/admin/pricing-plans|PricingView|PricingPlansView" backend frontend
```

Expected: no runtime matches. Accept only retained database history:

```text
backend/migrations/079_pricing_plan_tables.sql
backend/internal/repository/migrations_schema_integration_test.go
```

Run:

```bash
rg -n "modelPricing|model-pricing|capabilities" backend/internal frontend/src | head -n 200
```

Expected: shows new service, handler, route, API, view, nav, channel UI and tests.

- [ ] **Step 4: Run build smoke checks**

Run:

```bash
cd backend && go test ./cmd/server -count=1
pnpm --dir frontend run build
```

Expected: PASS.

- [ ] **Step 5: Optional local UI smoke**

If visual inspection is requested during execution, start frontend dev server:

```bash
pnpm --dir frontend run dev --host 127.0.0.1 --port 5173
```

Open:

```text
http://127.0.0.1:5173/model-pricing
```

Verify:
- 顶部菜单存在“模型定价”。
- 页面首屏是模型列表，不是营销页。
- 搜索输入 300ms 后请求列表。
- 搜索请求失败时显示“查询异常”和重试按钮。
- 点击模型后展示分组价格表和倍率。
- 控制台侧边栏存在“模型定价”。
- 管理员侧边栏不存在“定价方案”。
- 渠道管理定价条目可选择“对话 / 图片 / 视频”。

- [ ] **Step 6: Commit verification fixes**

If Step 1-4 required any fixes:

```bash
git add -A
git commit -m "test: verify model pricing page integration"
```

If no fixes were required, do not create an empty commit.

---

## Self-Review

### Spec Coverage

- 新增公开 `/model-pricing`: Task 2 后端 API、Task 3 前端页面、Task 5 路由导航覆盖。
- 官网顶部菜单和控制台入口: Task 5 覆盖。
- 首页按模型展示列表、点击模型展示详情: Task 3 覆盖。
- 详情列出不同分组价格和相对官方倍率: Task 2 聚合 DTO 与 Task 3 表格覆盖。
- 复用渠道管理页配置支持模型和特殊价格: Task 1、Task 2、Task 4 覆盖。
- 模型列表来自启用渠道具体模型并集: Task 2 覆盖。
- 通配符不作为列表项但参与支持判断: Task 2 覆盖。
- 不支持分组隐藏: Task 2 只聚合绑定启用渠道且命中模型的 active group。
- 渠道空价格字段回退官方价、非空覆盖、再乘分组倍率: Task 2 `resolveDisplayPricing` 和 `applyGroupMultiplier` 覆盖。
- 不在用户页标注渠道/特殊定价来源: Task 2 DTO 不含来源字段，Task 3 UI 不渲染来源字段。
- 能力标签只展示不参与调用: Task 1-4 只写入和展示 `capabilities`，不改网关、调度或限制逻辑。
- 搜索框、300ms 防抖、异常提示“查询异常”: Task 3 覆盖。
- 移除旧“定价方案”菜单页与运行时代码: Task 5、Task 6 覆盖。
- 保留 DB 表、购买、订单、支付套餐: Task 6 明确不删 migrations，不改 `/purchase`、`/orders`、`/admin/orders/plans`。

### Placeholder Scan

- 本计划未使用待补充占位词或延后实现标记。
- 每个任务包含具体文件、接口、测试命令、实现要点和提交命令。
- 涉及代码变更的步骤给出明确代码片段或精确删除目标。

### Type Consistency

- 后端能力标签字段统一为 `Capabilities []string` 和 JSON `capabilities`。
- 前端能力标签字段统一为 `capabilities: ModelCapability[]`。
- 后端公开 API 使用 `/api/v1/model-pricing/models`，前端 API client 使用相对路径 `/model-pricing/models`，符合现有 `apiClient` base URL 模式。
- 旧 pricing plan 删除任务安排在模型定价 API、页面和导航之后，避免中间状态没有公开定价入口。

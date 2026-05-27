# 推广管理手动完成与奖励分账 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为管理端推广列表新增“手动完成”能力，并把充值本体、推广奖励额度、销售返佣现金拆成可独立发放与独立记录的链路。

**Architecture:** 以后端服务拆分为主：先把支付成功后的“充值本体履约”和“推广侧奖励履约”从 `PaymentService.doBalance` 中解耦，再在 `ReferralService` 新增管理端手动完成入口，复用推广侧奖励发放但跳过充值本体与订阅本体。前端只消费新的列表字段和新接口，在推广列表里补操作列、备注确认和 `ID tag + 邮箱` 展示。

**Tech Stack:** Go、Gin、PostgreSQL、Vue 3、TypeScript、Vitest、Go test

---

## File Map

- Modify: `backend/internal/service/referral.go`
  - 扩展 `Referral` 结构和 `ReferralRepository` 接口，补齐推广人邮箱、备注、按 ID 查询、手动完成更新能力。
- Modify: `backend/internal/repository/referral_repo.go`
  - 调整管理端列表查询，返回推广人邮箱；新增按 ID 查询和手动完成更新 SQL。
- Modify: `backend/internal/service/referral_service.go`
  - 拆出推广完成奖励履约方法；新增管理员手动完成入口；保证仅补发推广侧奖励。
- Modify: `backend/internal/service/payment_fulfillment.go`
  - 从 `doBalance` 中拆分充值本体履约、推广奖励履约、销售返佣履约的调用顺序。
- Modify: `backend/internal/handler/admin/referral_handler.go`
  - 新增 `POST /admin/referral/:id/mark-completed` 处理器。
- Modify: `backend/internal/server/routes/admin.go`
  - 注册新路由。
- Modify: `backend/internal/service/sales_commission_service.go`
  - 增加可被手动完成路径调用的销售返佣补发入口，避免只能绑定 `payment_order_id` 正常回调链路。
- Modify: `backend/internal/repository/sales_commission_repo.go`
  - 如现有记录模型不足，补充支持“手动完成推广返佣”的插入路径与 note 备注写入。
- Modify: `backend/internal/repository/gift_balance_repo.go`
  - 只在需要时补充更细 `note`/`source_ref_id` 校验，不改变 FIFO 逻辑。
- Modify: `frontend/src/api/admin/referral.ts`
  - 扩展 `ReferralListItem` 字段，新增 `markReferralCompleted()` API。
- Modify: `frontend/src/views/admin/ReferralManageView.vue`
  - 推广人列改为 `ID tag + 邮箱`；新增操作列和手动完成确认交互。
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
  - 增加手动完成相关文案。
- Create or Modify Tests:
  - `backend/internal/service/referral_service_test.go`
  - `backend/internal/service/payment_fulfillment_skip_completed_test.go`
  - `backend/internal/handler/admin/referral_handler_test.go`
  - `frontend/src/views/admin/__tests__/ReferralManageView.spec.ts`（若不存在则创建）

### Task 1: 后端先锁住手动完成和分账语义的测试

**Files:**
- Modify: `backend/internal/service/referral_service_test.go`
- Modify: `backend/internal/service/payment_fulfillment_skip_completed_test.go`
- Create: `backend/internal/handler/admin/referral_handler_test.go`

- [ ] **Step 1: 在 `referral_service_test.go` 增加“手动完成只补发推广侧奖励”的失败测试**

```go
func TestReferralService_AdminMarkCompleted_GrantsReferralRewardsOnly(t *testing.T) {
	ctx := context.Background()
	referralRepo := &referralRepoStub{
		byID: &Referral{
			ID:                  9,
			ReferrerID:          12,
			RefereeID:           34,
			Status:              ReferralStatusPending,
			InviteeRewardAmount: 10,
			InviterRewardAmount: 8,
		},
	}
	giftRepo := &giftBalanceRepoStub{}
	userRepo := &userRepoStub{
		usersByID: map[int64]*User{
			12: {ID: 12, Email: "referrer@example.com"},
			34: {ID: 34, Email: "buyer@example.com"},
		},
	}
	svc := NewReferralService(userRepo, referralRepo, giftRepo, defaultReferralConfigResolver())

	err := svc.AdminMarkReferralCompleted(ctx, 9, "webhook missed")
	require.NoError(t, err)
	require.Len(t, giftRepo.created, 1)
	require.Equal(t, GiftBalanceSourceReferralInviter, giftRepo.created[0].Source)
	require.Equal(t, int64(12), giftRepo.created[0].UserID)
	require.Equal(t, int64(9), *giftRepo.created[0].SourceRefID)
	require.Equal(t, ReferralStatusCompleted, referralRepo.updatedStatus)
	require.NotContains(t, giftRepo.created[0].Note, "充值本体")
}
```

- [ ] **Step 2: 增加“已完成记录不能重复手动完成”的失败测试**

```go
func TestReferralService_AdminMarkCompleted_RejectsCompletedReferral(t *testing.T) {
	ctx := context.Background()
	referralRepo := &referralRepoStub{
		byID: &Referral{ID: 9, Status: ReferralStatusCompleted},
	}
	svc := NewReferralService(&userRepoStub{}, referralRepo, &giftBalanceRepoStub{}, defaultReferralConfigResolver())

	err := svc.AdminMarkReferralCompleted(ctx, 9, "duplicate")
	require.Error(t, err)
}
```

- [ ] **Step 3: 在 `payment_fulfillment_skip_completed_test.go` 增加“正常充值路径仍然先充值、后推广奖励、再返佣”的失败测试**

```go
func TestPaymentService_DoBalance_SplitsRechargeAndReferralRewards(t *testing.T) {
	// 断言 redeem 先发生，再触发 referral inviter reward，再触发 sales commission
	// 使用 stub 记录调用顺序：
	// []string{"redeem", "markCompleted", "referral:first_recharge", "sales_commission"}
}
```

- [ ] **Step 4: 新增 `referral_handler_test.go`，覆盖接口参数校验**

```go
func TestReferralHandler_MarkCompleted_RequiresNotes(t *testing.T) {
	r := gin.New()
	h := &ReferralHandler{referralService: &service.ReferralService{}}
	r.POST("/admin/referral/:id/mark-completed", h.MarkCompleted)

	req := httptest.NewRequest(http.MethodPost, "/admin/referral/1/mark-completed", strings.NewReader(`{"notes":"   "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}
```

- [ ] **Step 5: 运行后端测试，确认先失败**

Run:

```bash
go test ./backend/internal/service ./backend/internal/handler/admin -run 'Referral|Payment' -count=1
```

Expected: 新增断言失败，提示 `AdminMarkReferralCompleted` / `MarkCompleted` 尚不存在或行为不符合预期。

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/referral_service_test.go backend/internal/service/payment_fulfillment_skip_completed_test.go backend/internal/handler/admin/referral_handler_test.go
git commit -m "test: cover referral manual completion flow"
```

### Task 2: 拆出后端推广奖励履约与手动完成接口

**Files:**
- Modify: `backend/internal/service/referral.go`
- Modify: `backend/internal/repository/referral_repo.go`
- Modify: `backend/internal/service/referral_service.go`
- Modify: `backend/internal/handler/admin/referral_handler.go`
- Modify: `backend/internal/server/routes/admin.go`

- [ ] **Step 1: 扩展 `Referral` 模型和仓储接口**

在 `backend/internal/service/referral.go` 中补字段和接口：

```go
type Referral struct {
	ID                 int64      `json:"id"`
	ReferrerID         int64      `json:"referrer_id"`
	ReferrerEmail      string     `json:"referrer_email,omitempty"`
	RefereeID          int64      `json:"referee_id"`
	RefereeEmail       string     `json:"referee_email,omitempty"`
	Status             string     `json:"status"`
	ManualCompleteNote string     `json:"manual_complete_note,omitempty"`
	InviterRewardedAt  *time.Time `json:"inviter_rewarded_at"`
}

type ReferralRepository interface {
	GetByID(ctx context.Context, id int64) (*Referral, error)
	MarkCompleted(ctx context.Context, id int64, inviterRewardAmount float64, note string) error
}
```

- [ ] **Step 2: 在 `referral_repo.go` 实现 `GetByID`、`MarkCompleted`，并让 `ListAll` 返回推广人邮箱**

关键 SQL 片段：

```go
`SELECT r.id, r.referrer_id, COALESCE(ru.email, ''), r.referee_id, COALESCE(u.email, ''),
        r.referral_code, r.status, r.invitee_reward_amount, r.inviter_reward_amount,
        r.invitee_rewarded_at, r.inviter_rewarded_at, r.ongoing_reward_count, r.ongoing_reward_total,
        r.created_at, r.updated_at, COALESCE(r.manual_complete_note, '')
 FROM referrals r
 LEFT JOIN users u ON r.referee_id = u.id
 LEFT JOIN users ru ON r.referrer_id = ru.id
 WHERE r.id = $1`
```

以及：

```go
`UPDATE referrals
 SET status = 'completed',
     inviter_reward_amount = $1,
     inviter_rewarded_at = COALESCE(inviter_rewarded_at, NOW()),
     manual_complete_note = $2,
     updated_at = NOW()
 WHERE id = $3 AND status = 'pending'`
```

- [ ] **Step 3: 在 `referral_service.go` 拆出“推广侧奖励履约”方法**

建议新增三个方法：

```go
func (s *ReferralService) completeReferralRewards(ctx context.Context, ref *Referral, rechargeAmount float64, note string) error
func (s *ReferralService) grantInviterFirstRechargeReward(ctx context.Context, ref *Referral, cfg *EffectiveReferralConfig, note string) (float64, error)
func (s *ReferralService) AdminMarkReferralCompleted(ctx context.Context, referralID int64, note string) error
```

`AdminMarkReferralCompleted()` 的核心顺序：

```go
ref, err := s.referralRepo.GetByID(ctx, referralID)
if err != nil || ref == nil { ... }
if ref.Status != ReferralStatusPending { ... }
cfg := s.configResolver.Resolve(ctx, ref.ReferrerID)
rewardAmount, err := s.grantInviterFirstRechargeReward(ctx, ref, cfg, note)
if err != nil { return err }
if err := s.referralRepo.MarkCompleted(ctx, ref.ID, rewardAmount, note); err != nil { return err }
return nil
```

- [ ] **Step 4: 在 `referral_service.go` 保持正常充值路径仍可复用推广奖励履约**

把现有：

```go
s.referralService.HandleInviterRewardOnFirstRecharge(ctx, o.UserID, o.Amount)
s.referralService.HandleOngoingRewardOnRecharge(ctx, o.UserID, o.Amount)
```

改造成内部共享的组合入口，避免手动完成去调用支付主链路方法。

- [ ] **Step 5: 在 `referral_handler.go` 增加 `MarkCompleted` 处理器**

```go
type markCompletedRequest struct {
	Notes string `json:"notes"`
}

func (h *ReferralHandler) MarkCompleted(c *gin.Context) {
	referralID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if referralID <= 0 {
		response.BadRequest(c, "Invalid referral ID")
		return
	}
	var req markCompletedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Notes) == "" {
		response.BadRequest(c, "notes is required")
		return
	}
	if err := h.referralService.AdminMarkReferralCompleted(c.Request.Context(), referralID, req.Notes); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}
```

- [ ] **Step 6: 在 `admin.go` 注册路由**

```go
adminReferral.POST("/referral/:id/mark-completed", referralHandler.MarkCompleted)
```

- [ ] **Step 7: 运行后端测试，确认通过**

Run:

```bash
go test ./backend/internal/service ./backend/internal/handler/admin -run 'Referral|Payment' -count=1
```

Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add backend/internal/service/referral.go backend/internal/repository/referral_repo.go backend/internal/service/referral_service.go backend/internal/handler/admin/referral_handler.go backend/internal/server/routes/admin.go
git commit -m "feat: add admin referral manual completion flow"
```

### Task 3: 拆分销售返佣补发与充值链路顺序

**Files:**
- Modify: `backend/internal/service/sales_commission_service.go`
- Modify: `backend/internal/repository/sales_commission_repo.go`
- Modify: `backend/internal/service/payment_fulfillment.go`
- Modify: `backend/internal/service/sales_commission_service_test.go`

- [ ] **Step 1: 在 `sales_commission_service_test.go` 增加“手动完成也能补返佣现金”的失败测试**

```go
func TestSalesCommissionService_HandleReferralManualCompletion_CreatesCommissionRecord(t *testing.T) {
	// 构造 referral + sales user stub
	// 断言创建独立 sales_commission_records，并且 note 带上 manual completion 备注
}
```

- [ ] **Step 2: 在 `sales_commission_service.go` 新增手动完成补返佣入口**

```go
func (s *SalesCommissionService) HandleReferralManualCompletion(ctx context.Context, ref *Referral, note string) error
```

该方法职责：

- 判断推广人是否为销售用户
- 如果不是，直接返回
- 如果是，创建或补齐独立销售返佣记录
- 备注写入手动完成原因，避免和正常 `payment_order_id` 路径混淆

- [ ] **Step 3: 如果现有 `sales_commission_records` 必须依赖 `payment_order_id`，先在仓储层补一个手动补发兼容入口**

示意：

```go
func (r *salesCommissionRepository) CreateForManualReferral(ctx context.Context, input *service.SalesCommissionCreate) error {
	// 允许 note 区分 manual completion
	// 避免与正常 order 流程的唯一键冲突
}
```

如果现有 schema 已经能复用，则保持最小改动，不新增无必要字段。

- [ ] **Step 4: 调整 `payment_fulfillment.go` 中正常顺序，明确分三段调用**

把 `doBalance()` 收成：

```go
if _, err := s.redeemService.Redeem(...); err != nil { ... }
if err := s.markCompleted(...); err != nil { ... }
if s.referralService != nil {
	s.referralService.HandleReferralRewardsAfterRecharge(ctx, o.UserID, o.Amount)
}
if s.salesCommissionService != nil {
	s.salesCommissionService.HandleBalanceRechargeCompleted(ctx, &completedOrder)
}
```

重点是让“充值本体完成”与“推广奖励/返佣完成”在代码结构上清晰分层。

- [ ] **Step 5: 运行返佣与支付相关测试**

Run:

```bash
go test ./backend/internal/service -run 'SalesCommission|Payment' -count=1
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add backend/internal/service/sales_commission_service.go backend/internal/repository/sales_commission_repo.go backend/internal/service/payment_fulfillment.go backend/internal/service/sales_commission_service_test.go
git commit -m "refactor: split referral rewards from recharge fulfillment"
```

### Task 4: 前端接入新字段与手动完成交互

**Files:**
- Modify: `frontend/src/api/admin/referral.ts`
- Modify: `frontend/src/views/admin/ReferralManageView.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Create or Modify: `frontend/src/views/admin/__tests__/ReferralManageView.spec.ts`

- [ ] **Step 1: 扩展 API 类型并新增请求方法**

在 `frontend/src/api/admin/referral.ts` 中增加字段和方法：

```ts
export interface ReferralListItem {
  id: number
  referrer_id: number
  referrer_email?: string
  referee_id: number
  referee_email?: string
  status: string
  manual_complete_note?: string
}

export async function markReferralCompleted(
  id: number,
  request: { notes: string }
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    `/admin/referral/${id}/mark-completed`,
    request
  )
  return data
}
```

- [ ] **Step 2: 先写 `ReferralManageView` 失败测试**

```ts
it('renders referrer id tag and email and shows manual-complete action for pending rows', async () => {
  listReferrals.mockResolvedValue({
    items: [{
      id: 1,
      referrer_id: 12,
      referrer_email: 'referrer@example.com',
      referee_id: 34,
      referee_email: 'buyer@example.com',
      status: 'pending',
      invitee_reward_amount: 10,
      inviter_reward_amount: 8,
      ongoing_reward_count: 0,
      ongoing_reward_total: 0,
      created_at: '2026-05-14T00:00:00Z',
    }],
    total: 1,
    page: 1,
  })
  // 断言页面出现 #12、referrer@example.com、手动完成按钮
})
```

- [ ] **Step 3: 在 `ReferralManageView.vue` 新增操作列和备注确认**

建议最小实现：

```ts
const completingReferralId = ref<number | null>(null)

async function handleMarkCompleted(item: ReferralListItem) {
  const notes = window.prompt(t('adminReferral.manualCompletePrompt'), '')
  if (!notes || !notes.trim()) {
    toast.error(t('adminReferral.manualCompleteNotesRequired'))
    return
  }
  completingReferralId.value = item.id
  try {
    await markReferralCompleted(item.id, { notes: notes.trim() })
    toast.success(t('adminReferral.manualCompleteSuccess'))
    await loadList()
  } catch (error) {
    toast.error(t('adminReferral.manualCompleteFailed'))
  } finally {
    completingReferralId.value = null
  }
}
```

模板里把推广人列改成：

```vue
<td class="py-3 pr-4 text-gray-700 dark:text-dark-200">
  <div class="flex items-center gap-2">
    <span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-dark-200">
      #{{ item.referrer_id }}
    </span>
    <span>{{ item.referrer_email || `#${item.referrer_id}` }}</span>
  </div>
</td>
```

- [ ] **Step 4: 补国际化文案**

在 `zh.ts` / `en.ts` 中新增：

```ts
manualComplete: '手动完成'
manualCompletePrompt: '请输入手动完成原因。该操作仅补发推广侧奖励，不补发充值本体或订阅本体。'
manualCompleteSuccess: '推广记录已手动完成'
manualCompleteFailed: '手动完成失败'
manualCompleteNotesRequired: '请输入手动完成备注'
actions: '操作'
```

- [ ] **Step 5: 运行前端测试**

Run:

```bash
cd frontend && pnpm vitest run src/views/admin/__tests__/ReferralManageView.spec.ts
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/api/admin/referral.ts frontend/src/views/admin/ReferralManageView.vue frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts frontend/src/views/admin/__tests__/ReferralManageView.spec.ts
git commit -m "feat: add admin referral manual completion UI"
```

### Task 5: 全量回归与收尾

**Files:**
- Modify: 已变更文件集合

- [ ] **Step 1: 运行后端定向回归**

```bash
go test ./backend/internal/service ./backend/internal/handler/admin ./backend/internal/repository -count=1
```

Expected: PASS

- [ ] **Step 2: 运行前端定向回归**

```bash
cd frontend && pnpm vitest run src/views/admin/__tests__/ReferralManageView.spec.ts src/views/admin/__tests__/SalesCommissionsView.spec.ts src/views/admin/__tests__/SubscriptionsView.spec.ts
```

Expected: PASS

- [ ] **Step 3: 检查工作区差异**

```bash
git status --short
git diff -- backend/internal/service/referral_service.go backend/internal/service/payment_fulfillment.go frontend/src/views/admin/ReferralManageView.vue
```

Expected: 只剩本次相关改动，无意外文件抖动。

- [ ] **Step 4: 最终 Commit**

```bash
git add backend frontend
git commit -m "feat: support admin referral manual completion"
```

## Self-Review

- Spec coverage:
  - 手动完成接口：Task 2
  - 推广人 `ID tag + 邮箱`：Task 4
  - 充值本体与推广奖励分账：Task 1 / Task 3
  - 销售返佣现金纳入手动完成：Task 3
  - 只用现有充值/额度/现金日志体系：Task 2 / Task 3
- Placeholder scan:
  - 计划中没有 `TODO` / `TBD`
  - 每个任务都给了具体文件和命令
- Type consistency:
  - 统一使用 `AdminMarkReferralCompleted`
  - 前端接口统一使用 `markReferralCompleted`
  - 手动完成路由统一为 `POST /admin/referral/:id/mark-completed`

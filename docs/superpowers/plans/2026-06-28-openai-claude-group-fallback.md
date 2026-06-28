# OpenAI 与 Claude 分组兜底 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Claude 与 OpenAI HTTP 网关增加显式启用的分组级兜底，并阻止兜底分组被 API Key 直接绑定。

**Architecture:** 复用 `fallback_group_id` 作为入口分组到兜底分组的引用，新增 `is_fallback_group` 标记兜底目标。后端把平台/订阅/绑定限制沉淀在 service 层，handler 只在账号调度失败或账号 failover 耗尽时切换一次兜底上下文。前端只展示合法兜底目标，并隐藏废弃的 invalid request 兜底配置。

**Tech Stack:** Go、Gin、Ent、PostgreSQL migrations、Vue 3、TypeScript、Vitest、Go unit tests with `-tags unit`。

---

## File Structure

- Modify `backend/ent/schema/group.go`: add `is_fallback_group` field and deprecation comment for `fallback_group_id_on_invalid_request`.
- Add `backend/migrations/122_add_group_fallback_flag.sql`: add `groups.is_fallback_group` and index.
- Regenerate `backend/ent/*`: Ent generated field accessors for `is_fallback_group`.
- Modify `backend/internal/service/group.go`: add `IsFallbackGroup` and deprecated field comment.
- Modify `backend/internal/handler/dto/types.go` and `backend/internal/handler/dto/mappers.go`: expose `is_fallback_group`, mark old field deprecated.
- Modify `backend/internal/repository/group_repo.go`: persist and map `is_fallback_group`.
- Modify `backend/internal/service/api_key_auth_cache.go` and `backend/internal/service/api_key_auth_cache_impl.go`: cache `is_fallback_group`; keep old field with deprecation comment.
- Modify `backend/internal/service/admin_service.go`: validate fallback group enablement and target selection; block API Key direct binding by admin.
- Modify `backend/internal/service/api_key_service.go`: block user-created/updated API Keys from binding fallback groups.
- Modify `backend/internal/handler/gateway_handler.go`: Claude HTTP fallback using `fallback_group_id`, preserving `claude_code_only`.
- Modify `backend/internal/handler/openai_gateway_handler.go`, `backend/internal/handler/openai_chat_completions.go`, `backend/internal/handler/openai_images.go`: OpenAI HTTP fallback.
- Modify `frontend/src/types/index.ts`: add `is_fallback_group`, deprecate old invalid request field comments.
- Modify `frontend/src/views/admin/GroupsView.vue`: add fallback target toggle, filter fallback target options, hide old invalid request fallback UI.
- Modify `frontend/src/views/user/KeysView.vue`: hide fallback groups from creation/edit options, keep filter options unchanged.
- Modify `frontend/src/components/admin/user/UserAllowedGroupsModal.vue` and `frontend/src/components/admin/user/UserApiKeysModal.vue`: hide fallback groups from selectable assignment lists.
- Modify `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`: new labels and deprecated text.
- Add/modify unit tests in existing backend/frontend test files listed in tasks.

---

### Task 1: Add The Fallback Group Flag To Data Model

**Files:**
- Modify: `backend/ent/schema/group.go`
- Create: `backend/migrations/122_add_group_fallback_flag.sql`
- Modify after generation: `backend/ent/*`
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/dto/mappers.go`
- Modify: `backend/internal/repository/group_repo.go`
- Modify: `backend/internal/service/api_key_auth_cache.go`
- Modify: `backend/internal/service/api_key_auth_cache_impl.go`

- [ ] **Step 1: Write the failing backend mapping test**

Add this test to `backend/internal/service/admin_service_group_test.go`:

```go
func TestAdminService_CreateGroup_PersistsFallbackGroupFlag(t *testing.T) {
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:               "fallback-openai",
		Platform:           PlatformOpenAI,
		SubscriptionType:   SubscriptionTypeStandard,
		IsFallbackGroup:    true,
		RateMultiplier:     1,
		SystemPromptMode:   "inherit",
		SupportedModelScopes: []string{},
	})

	require.NoError(t, err)
	require.NotNil(t, group)
	require.NotNil(t, repo.created)
	require.True(t, repo.created.IsFallbackGroup)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run TestAdminService_CreateGroup_PersistsFallbackGroupFlag -count=1
```

Expected: compile fails with `unknown field IsFallbackGroup`.

- [ ] **Step 3: Add migration**

Create `backend/migrations/122_add_group_fallback_flag.sql`:

```sql
-- 添加分组兜底目标标记
ALTER TABLE groups
ADD COLUMN IF NOT EXISTS is_fallback_group BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_groups_is_fallback_group
ON groups(is_fallback_group)
WHERE deleted_at IS NULL AND is_fallback_group = TRUE;

COMMENT ON COLUMN groups.is_fallback_group IS '是否允许作为其他分组的兜底目标';
```

- [ ] **Step 4: Add Ent schema field**

In `backend/ent/schema/group.go`, add the field after `fallback_group_id`:

```go
field.Bool("is_fallback_group").
	Default(false).
	Comment("是否允许作为其他分组的兜底目标"),
field.Int64("fallback_group_id_on_invalid_request").
	Optional().
	Nillable().
	Comment("Deprecated: will be removed in next version. 无效请求兜底使用的分组 ID，不再参与运行时逻辑"),
```

- [ ] **Step 5: Regenerate Ent code**

Run:

```bash
cd backend
go generate ./ent
```

Expected: generated files under `backend/ent/` include `IsFallbackGroup`, `SetIsFallbackGroup`, and `FieldIsFallbackGroup`.

- [ ] **Step 6: Add service field and deprecated comment**

In `backend/internal/service/group.go`, update the group fields:

```go
// Claude Code 客户端限制与通用分组兜底
ClaudeCodeOnly  bool
FallbackGroupID *int64
IsFallbackGroup bool
// Deprecated: will be removed in next version.
// 无效请求兜底分组不再参与运行时逻辑；prompt too long 等兜底统一使用 FallbackGroupID。
FallbackGroupIDOnInvalidRequest *int64
```

- [ ] **Step 7: Add DTO field and deprecated comment**

In `backend/internal/handler/dto/types.go`, update the group DTO:

```go
FallbackGroupID *int64 `json:"fallback_group_id"`
IsFallbackGroup bool   `json:"is_fallback_group"`
// Deprecated: will be removed in next version. 不再参与运行时逻辑。
FallbackGroupIDOnInvalidRequest *int64 `json:"fallback_group_id_on_invalid_request"`
```

In `backend/internal/handler/dto/mappers.go`, add:

```go
IsFallbackGroup: g.IsFallbackGroup,
```

next to `FallbackGroupID`.

- [ ] **Step 8: Persist field in repository**

In `backend/internal/repository/group_repo.go`, add `SetIsFallbackGroup(groupIn.IsFallbackGroup)` to both create and update builders:

```go
SetClaudeCodeOnly(groupIn.ClaudeCodeOnly).
SetNillableFallbackGroupID(groupIn.FallbackGroupID).
SetIsFallbackGroup(groupIn.IsFallbackGroup).
SetNillableFallbackGroupIDOnInvalidRequest(groupIn.FallbackGroupIDOnInvalidRequest).
```

Update `groupEntityToService` in the same file to map:

```go
IsFallbackGroup:                 m.IsFallbackGroup,
FallbackGroupIDOnInvalidRequest: m.FallbackGroupIDOnInvalidRequest,
```

- [ ] **Step 9: Cache field in API key auth cache**

In `backend/internal/service/api_key_auth_cache.go`, add:

```go
IsFallbackGroup bool `json:"is_fallback_group,omitempty"`
// Deprecated: will be removed in next version. 不再参与运行时逻辑。
FallbackGroupIDOnInvalidRequest *int64 `json:"fallback_group_id_on_invalid_request,omitempty"`
```

In `backend/internal/service/api_key_auth_cache_impl.go`, when snapshotting/restoring group data, add:

```go
IsFallbackGroup: apiKey.Group.IsFallbackGroup,
```

and:

```go
IsFallbackGroup: snapshot.Group.IsFallbackGroup,
```

- [ ] **Step 10: Run test to verify it passes**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run TestAdminService_CreateGroup_PersistsFallbackGroupFlag -count=1
```

Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add backend/ent backend/migrations/122_add_group_fallback_flag.sql backend/internal/service/group.go backend/internal/handler/dto/types.go backend/internal/handler/dto/mappers.go backend/internal/repository/group_repo.go backend/internal/service/api_key_auth_cache.go backend/internal/service/api_key_auth_cache_impl.go backend/internal/service/admin_service_group_test.go
git commit -m "feat: add fallback group flag"
```

---

### Task 2: Validate Fallback Group Configuration

**Files:**
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/service/admin_service_group_test.go`
- Modify: `backend/internal/service/group_service.go` if interface additions are needed

- [ ] **Step 1: Write failing validation tests**

Add these tests to `backend/internal/service/admin_service_group_test.go`:

```go
func TestAdminService_CreateGroup_FallbackGroupFlagRejectsUnsupportedPlatform(t *testing.T) {
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "gemini-fallback",
		Platform:         PlatformGemini,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup:  true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group flag only supported for anthropic or openai standard groups")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackGroupFlagRejectsSubscription(t *testing.T) {
	repo := &groupRepoStubForInvalidRequestFallback{}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-sub-fallback",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeSubscription,
		IsFallbackGroup:  true,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group must be standard billing type")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetRequiresEnabledFlag(t *testing.T) {
	fallbackID := int64(20)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {ID: fallbackID, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeStandard, Status: StatusActive},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group must be enabled as fallback group")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_FallbackTargetMustMatchPlatform(t *testing.T) {
	fallbackID := int64(21)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:              fallbackID,
				Platform:        PlatformAnthropic,
				SubscriptionType: SubscriptionTypeStandard,
				Status:          StatusActive,
				IsFallbackGroup: true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	_, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "openai group fallback target must be openai platform")
	require.Nil(t, repo.created)
}

func TestAdminService_CreateGroup_MultipleGroupsMayShareFallbackTarget(t *testing.T) {
	fallbackID := int64(22)
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			fallbackID: {
				ID:              fallbackID,
				Platform:        PlatformOpenAI,
				SubscriptionType: SubscriptionTypeStandard,
				Status:          StatusActive,
				IsFallbackGroup: true,
			},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	first, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry-1",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := svc.CreateGroup(context.Background(), &CreateGroupInput{
		Name:             "openai-entry-2",
		Platform:         PlatformOpenAI,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID:  &fallbackID,
	})
	require.NoError(t, err)
	require.NotNil(t, second)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_CreateGroup_Fallback(GroupFlag|Target|Multiple)' -count=1
```

Expected: compile or assertion failures because `IsFallbackGroup` validation does not exist.

- [ ] **Step 3: Add request fields**

In `backend/internal/service/admin_service.go`, add to `CreateGroupInput` and `UpdateGroupInput`:

```go
IsFallbackGroup *bool // UpdateGroupInput
```

For `CreateGroupInput`, use:

```go
IsFallbackGroup bool
```

In `backend/internal/handler/admin/group_handler.go`, add JSON fields to create/update request structs:

```go
IsFallbackGroup bool `json:"is_fallback_group"`
```

and:

```go
IsFallbackGroup *bool `json:"is_fallback_group"`
```

Pass the fields into service inputs in create/update handlers.

- [ ] **Step 4: Add validation helpers**

In `backend/internal/service/admin_service.go`, add:

```go
func isFallbackCapablePlatform(platform string) bool {
	return platform == PlatformAnthropic || platform == PlatformOpenAI
}

func (s *adminServiceImpl) validateFallbackGroupFlag(ctx context.Context, group *Group) error {
	if group == nil || !group.IsFallbackGroup {
		return nil
	}
	if !isFallbackCapablePlatform(group.Platform) {
		return fmt.Errorf("fallback group flag only supported for anthropic or openai standard groups")
	}
	if group.SubscriptionType == SubscriptionTypeSubscription {
		return fmt.Errorf("fallback group must be standard billing type")
	}
	if group.FallbackGroupID != nil && *group.FallbackGroupID > 0 {
		return fmt.Errorf("fallback group cannot have fallback_group_id configured")
	}
	if group.ID > 0 && s.apiKeyRepo != nil {
		count, err := s.apiKeyRepo.CountByGroupID(ctx, group.ID)
		if err != nil {
			return fmt.Errorf("count api keys by group: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("fallback group has bound api keys; migrate or unbind them first")
		}
	}
	return nil
}

func (s *adminServiceImpl) validateGeneralFallbackGroup(ctx context.Context, currentGroupID int64, platform string, fallbackGroupID int64) error {
	if currentGroupID > 0 && currentGroupID == fallbackGroupID {
		return fmt.Errorf("cannot set self as fallback group")
	}
	if !isFallbackCapablePlatform(platform) {
		return fmt.Errorf("fallback group only supported for anthropic or openai groups")
	}
	fallbackGroup, err := s.groupRepo.GetByIDLite(ctx, fallbackGroupID)
	if err != nil {
		return fmt.Errorf("fallback group not found: %w", err)
	}
	if fallbackGroup.Status != StatusActive {
		return fmt.Errorf("fallback group must be active")
	}
	if !fallbackGroup.IsFallbackGroup {
		return fmt.Errorf("fallback group must be enabled as fallback group")
	}
	if fallbackGroup.SubscriptionType == SubscriptionTypeSubscription {
		return fmt.Errorf("fallback group cannot be subscription type")
	}
	if fallbackGroup.FallbackGroupID != nil && *fallbackGroup.FallbackGroupID > 0 {
		return fmt.Errorf("fallback group cannot have fallback_group_id configured")
	}
	switch platform {
	case PlatformAnthropic:
		if fallbackGroup.Platform != PlatformAnthropic {
			return fmt.Errorf("anthropic group fallback target must be anthropic platform")
		}
	case PlatformOpenAI:
		if fallbackGroup.Platform != PlatformOpenAI {
			return fmt.Errorf("openai group fallback target must be openai platform")
		}
	}
	return nil
}
```

- [ ] **Step 5: Use validation in create/update**

In `CreateGroup`, normalize and validate:

```go
group := &Group{
	// existing fields...
	IsFallbackGroup: input.IsFallbackGroup,
	FallbackGroupID: input.FallbackGroupID,
}
if err := s.validateFallbackGroupFlag(ctx, group); err != nil {
	return nil, err
}
if group.FallbackGroupID != nil && *group.FallbackGroupID > 0 {
	if err := s.validateGeneralFallbackGroup(ctx, 0, group.Platform, *group.FallbackGroupID); err != nil {
		return nil, err
	}
}
```

In `UpdateGroup`, after applying input fields:

```go
if input.IsFallbackGroup != nil {
	group.IsFallbackGroup = *input.IsFallbackGroup
}
if err := s.validateFallbackGroupFlag(ctx, group); err != nil {
	return nil, err
}
if group.FallbackGroupID != nil && *group.FallbackGroupID > 0 {
	if err := s.validateGeneralFallbackGroup(ctx, id, group.Platform, *group.FallbackGroupID); err != nil {
		return nil, err
	}
}
```

Keep `validateFallbackGroupOnInvalidRequest` only for legacy compatibility; do not call it for runtime fallback.

- [ ] **Step 6: Run tests to verify they pass**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_CreateGroup_Fallback(GroupFlag|Target|Multiple)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/admin_service.go backend/internal/service/admin_service_group_test.go backend/internal/handler/admin/group_handler.go
git commit -m "feat: validate group fallback configuration"
```

---

### Task 3: Block API Key Binding To Fallback Groups

**Files:**
- Modify: `backend/internal/service/api_key_service.go`
- Modify: `backend/internal/service/api_key_service_test.go` or existing API key service test file
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/service/admin_service_apikey_test.go`

- [ ] **Step 1: Write failing user API key tests**

Add to an existing API key service test file:

```go
func TestAPIKeyService_CreateRejectsFallbackGroup(t *testing.T) {
	groupID := int64(10)
	user := &User{ID: 42, Status: StatusActive}
	group := &Group{
		ID:              groupID,
		Platform:        PlatformOpenAI,
		Status:          StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup: true,
	}
	svc := newAPIKeyServiceForGroupBindingTest(user, group)

	_, err := svc.Create(context.Background(), user.ID, CreateAPIKeyRequest{
		Name:    "test",
		GroupID: &groupID,
	})

	require.ErrorIs(t, err, ErrGroupNotAllowed)
}

func TestAPIKeyService_UpdateRejectsFallbackGroup(t *testing.T) {
	groupID := int64(10)
	user := &User{ID: 42, Status: StatusActive}
	group := &Group{
		ID:              groupID,
		Platform:        PlatformOpenAI,
		Status:          StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup: true,
	}
	svc := newAPIKeyServiceForGroupBindingTest(user, group)

	_, err := svc.Update(context.Background(), 100, user.ID, UpdateAPIKeyRequest{
		GroupID: &groupID,
	})

	require.ErrorIs(t, err, ErrGroupNotAllowed)
}
```

Add this helper in the same test file:

```go
type apiKeyFallbackUserRepoStub struct {
	UserRepository
	user *User
}

func (s *apiKeyFallbackUserRepoStub) GetByID(context.Context, int64) (*User, error) {
	return s.user, nil
}

type apiKeyFallbackGroupRepoStub struct {
	GroupRepository
	group *Group
}

func (s *apiKeyFallbackGroupRepoStub) GetByID(context.Context, int64) (*Group, error) {
	return s.group, nil
}

type apiKeyFallbackRepoStub struct {
	APIKeyRepository
	key *APIKey
}

func (s *apiKeyFallbackRepoStub) GetByID(context.Context, int64) (*APIKey, error) {
	return s.key, nil
}

func (s *apiKeyFallbackRepoStub) ExistsByKey(context.Context, string) (bool, error) {
	return false, nil
}

func (s *apiKeyFallbackRepoStub) Create(context.Context, *APIKey) error {
	return nil
}

func newAPIKeyServiceForGroupBindingTest(user *User, group *Group) *APIKeyService {
	return &APIKeyService{
		apiKeyRepo: &apiKeyFallbackRepoStub{key: &APIKey{ID: 100, UserID: user.ID, Key: "sk-test"}},
		userRepo:   &apiKeyFallbackUserRepoStub{user: user},
		groupRepo:  &apiKeyFallbackGroupRepoStub{group: group},
	}
}
```

- [ ] **Step 2: Write failing admin API key test**

Add to `backend/internal/service/admin_service_apikey_test.go`:

```go
func TestAdminService_AdminUpdateAPIKeyGroupID_FallbackGroupBlocked(t *testing.T) {
	existing := &APIKey{ID: 1, UserID: 42, Key: "sk-test", GroupID: nil}
	apiKeyRepo := &apiKeyRepoStubForGroupUpdate{existing: existing}
	groupRepo := &groupRepoStubForGroupUpdate{
		group: &Group{
			ID:              10,
			Name:            "fallback",
			Status:          StatusActive,
			Platform:        PlatformOpenAI,
			SubscriptionType: SubscriptionTypeStandard,
			IsFallbackGroup: true,
		},
	}
	svc := &adminServiceImpl{apiKeyRepo: apiKeyRepo, groupRepo: groupRepo}

	_, err := svc.AdminUpdateAPIKeyGroupID(context.Background(), 1, int64Ptr(10))

	require.Error(t, err)
	require.Contains(t, err.Error(), "fallback group cannot be bound to api keys")
	require.Nil(t, apiKeyRepo.updated)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'Test(APIKeyService_(Create|Update)RejectsFallbackGroup|AdminService_AdminUpdateAPIKeyGroupID_FallbackGroupBlocked)' -count=1
```

Expected: tests fail because fallback groups are still bindable.

- [ ] **Step 4: Add service guard**

In `backend/internal/service/api_key_service.go`, update `canUserBindGroup`:

```go
func (s *APIKeyService) canUserBindGroup(ctx context.Context, user *User, group *Group) bool {
	if group == nil || group.IsFallbackGroup {
		return false
	}
	if group.IsSubscriptionType() {
		_, err := s.userSubRepo.GetActiveByUserIDAndGroupID(ctx, user.ID, group.ID)
		return err == nil
	}
	return user.CanBindGroup(group.ID, group.IsExclusive)
}
```

- [ ] **Step 5: Add admin guard**

In `backend/internal/service/admin_service.go`, in `AdminUpdateAPIKeyGroupID` after active status check:

```go
if group.IsFallbackGroup {
	return nil, infraerrors.BadRequest("FALLBACK_GROUP_NOT_BINDABLE", "fallback group cannot be bound to api keys")
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'Test(APIKeyService_(Create|Update)RejectsFallbackGroup|AdminService_AdminUpdateAPIKeyGroupID_FallbackGroupBlocked)' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/service/api_key_service.go backend/internal/service/admin_service.go backend/internal/service/*api_key*_test.go backend/internal/service/admin_service_apikey_test.go
git commit -m "feat: block api keys from fallback groups"
```

---

### Task 4: Add Shared Runtime Fallback Helpers

**Files:**
- Modify: `backend/internal/handler/gateway_handler.go`
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/service/gateway_service.go`

- [ ] **Step 1: Write failing helper tests**

Add to `backend/internal/handler/openai_gateway_handler_test.go`:

```go
func TestResolveRuntimeFallbackAPIKey_Success(t *testing.T) {
	originalGroupID := int64(1)
	fallbackID := int64(2)
	apiKey := &service.APIKey{
		ID:      10,
		UserID:  42,
		GroupID: &originalGroupID,
		Group: &service.Group{
			ID:              originalGroupID,
			Platform:        service.PlatformOpenAI,
			SubscriptionType: service.SubscriptionTypeStandard,
			FallbackGroupID: &fallbackID,
		},
		User: &service.User{ID: 42},
	}
	fallbackGroup := &service.Group{
		ID:              fallbackID,
		Platform:        service.PlatformOpenAI,
		Status:          service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup: true,
	}
	h := &OpenAIGatewayHandler{
		gatewayService:      &service.OpenAIGatewayService{},
		billingCacheService: nil,
	}

	got, ok := resolveRuntimeFallbackAPIKeyForTest(context.Background(), apiKey, fallbackGroup)

	require.True(t, ok)
	require.NotNil(t, got)
	require.NotNil(t, got.GroupID)
	require.Equal(t, fallbackID, *got.GroupID)
	require.Same(t, fallbackGroup, got.Group)
	require.Same(t, apiKey.User, got.User)
	_ = h
}
```

This test expects a small exported-for-test wrapper:

```go
func resolveRuntimeFallbackAPIKeyForTest(ctx context.Context, apiKey *service.APIKey, fallbackGroup *service.Group) (*service.APIKey, bool) {
	return cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup), true
}
```

- [ ] **Step 2: Run helper test to verify it fails**

Run:

```bash
cd backend
go test ./internal/handler -run TestResolveRuntimeFallbackAPIKey_Success -count=1
```

Expected: compile fails because helper is missing.

- [ ] **Step 3: Add generic API key clone helper**

In `backend/internal/handler/gateway_handler.go`, keep existing `cloneAPIKeyWithGroup` if present, and add:

```go
func cloneAPIKeyWithFallbackGroup(apiKey *service.APIKey, fallbackGroup *service.Group) *service.APIKey {
	if apiKey == nil || fallbackGroup == nil || fallbackGroup.ID <= 0 {
		return nil
	}
	cloned := *apiKey
	groupID := fallbackGroup.ID
	cloned.GroupID = &groupID
	cloned.Group = fallbackGroup
	return &cloned
}
```

If `cloneAPIKeyWithGroup` already does exactly this, rename it to this helper or make it call this helper.

- [ ] **Step 4: Add fallback target resolver**

In `backend/internal/service/gateway_service.go`, add methods for both gateway services:

```go
func (s *GatewayService) ResolveRuntimeFallbackGroup(ctx context.Context, group *Group) (*Group, error) {
	return resolveRuntimeFallbackGroup(ctx, s.resolveGroupByID, group)
}

func (s *OpenAIGatewayService) ResolveRuntimeFallbackGroup(ctx context.Context, group *Group) (*Group, error) {
	if s == nil || s.groupRepo == nil {
		return nil, fmt.Errorf("group repository unavailable")
	}
	return resolveRuntimeFallbackGroup(ctx, s.groupRepo.GetByIDLite, group)
}

func resolveRuntimeFallbackGroup(ctx context.Context, resolve func(context.Context, int64) (*Group, error), group *Group) (*Group, error) {
	if group == nil || group.FallbackGroupID == nil || *group.FallbackGroupID <= 0 {
		return nil, nil
	}
	fallback, err := resolve(ctx, *group.FallbackGroupID)
	if err != nil {
		return nil, err
	}
	if fallback == nil || !fallback.IsFallbackGroup || !fallback.IsActive() || fallback.IsSubscriptionType() {
		return nil, fmt.Errorf("fallback group is not eligible")
	}
	if fallback.FallbackGroupID != nil && *fallback.FallbackGroupID > 0 {
		return nil, fmt.Errorf("fallback group cannot have fallback_group_id configured")
	}
	if group.Platform != fallback.Platform {
		return nil, fmt.Errorf("fallback group platform mismatch")
	}
	return fallback, nil
}
```

- [ ] **Step 5: Add test wrapper**

In `backend/internal/handler/openai_gateway_handler_test.go` or a `_test.go` helper file under handler package, add:

```go
func resolveRuntimeFallbackAPIKeyForTest(ctx context.Context, apiKey *service.APIKey, fallbackGroup *service.Group) (*service.APIKey, bool) {
	cloned := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)
	return cloned, cloned != nil
}
```

- [ ] **Step 6: Run helper test to verify it passes**

Run:

```bash
cd backend
go test ./internal/handler -run TestResolveRuntimeFallbackAPIKey_Success -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handler/gateway_handler.go backend/internal/handler/openai_gateway_handler_test.go backend/internal/service/gateway_service.go
git commit -m "feat: add runtime fallback helpers"
```

---

### Task 5: Implement Claude HTTP Runtime Fallback

**Files:**
- Modify: `backend/internal/handler/gateway_handler.go`
- Modify: `backend/internal/handler/gateway_handler_test.go`
- Modify: `backend/internal/service/gateway_multiplatform_test.go`

- [ ] **Step 1: Write failing selection fallback test**

Add to `backend/internal/service/gateway_multiplatform_test.go`:

```go
func TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback(t *testing.T) {
	groupID := int64(70)
	fallbackID := int64(71)
	group := &Group{
		ID:              groupID,
		Platform:        PlatformAnthropic,
		Status:          StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		FallbackGroupID: &fallbackID,
	}
	fallback := &Group{
		ID:              fallbackID,
		Platform:        PlatformAnthropic,
		Status:          StatusActive,
		SubscriptionType: SubscriptionTypeStandard,
		IsFallbackGroup: true,
	}
	svc := &GatewayService{
		groupRepo: &mockGroupRepoForGateway{
			groups: map[int64]*Group{fallbackID: fallback},
		},
	}

	got, err := svc.ResolveRuntimeFallbackGroup(context.Background(), group)

	require.NoError(t, err)
	require.Same(t, fallback, got)
}
```

- [ ] **Step 2: Run test to verify it fails or passes with helper**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run TestGatewayService_ResolveRuntimeFallbackGroup_AllowsAnthropicFallback -count=1
```

Expected before Task 4: compile fail. Expected after Task 4: PASS.

- [ ] **Step 3: Add Claude fallback control function**

In `backend/internal/handler/gateway_handler.go`, add:

```go
func (h *GatewayHandler) trySwitchToClaudeFallbackGroup(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	streamStarted bool,
) (*service.APIKey, bool) {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.FallbackGroupID == nil {
		return nil, false
	}
	fallbackGroup, err := h.gatewayService.ResolveRuntimeFallbackGroup(c.Request.Context(), apiKey.Group)
	if err != nil {
		reqLog.Warn("gateway.runtime_fallback_group_unavailable", zap.Error(err), zap.Any("group_id", apiKey.GroupID))
		return nil, false
	}
	fallbackAPIKey := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)
	if fallbackAPIKey == nil {
		return nil, false
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), fallbackAPIKey.User, fallbackAPIKey, fallbackGroup, nil); err != nil {
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return nil, false
	}
	reqLog.Warn("gateway.runtime_fallback_group_switch",
		zap.Any("original_group_id", apiKey.GroupID),
		zap.Int64("fallback_group_id", fallbackGroup.ID),
	)
	return fallbackAPIKey, true
}
```

- [ ] **Step 4: Use fallback on initial account selection failure**

In the Claude HTTP loop in `GatewayHandler.Messages`, replace the `len(fs.FailedAccountIDs) == 0` branch with:

```go
if len(fs.FailedAccountIDs) == 0 {
	if !fallbackUsed {
		if fallbackAPIKey, ok := h.trySwitchToClaudeFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
			currentAPIKey = fallbackAPIKey
			currentSubscription = nil
			fallbackUsed = true
			retryWithFallback = true
			break
		}
	}
	h.handleStreamingAwareError(c, http.StatusServiceUnavailable, "api_error", "No available accounts: "+err.Error(), streamStarted)
	return
}
```

- [ ] **Step 5: Use fallback on failover exhausted**

In the `FailoverExhausted` action and `UpstreamFailoverError` handling, before returning `handleFailoverExhausted`, add:

```go
if !fallbackUsed && c.Writer.Size() == writerSizeBeforeForward {
	if fallbackAPIKey, ok := h.trySwitchToClaudeFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
		currentAPIKey = fallbackAPIKey
		currentSubscription = nil
		fallbackUsed = true
		retryWithFallback = true
		break
	}
}
```

Keep the existing guard:

```go
if c.Writer.Size() != writerSizeBeforeForward {
	h.handleFailoverExhausted(c, failoverErr, account.Platform, true)
	return
}
```

- [ ] **Step 6: Move prompt too long fallback to `fallback_group_id`**

In the `PromptTooLongError` branch, remove all use of `FallbackGroupIDOnInvalidRequest` and use:

```go
if !fallbackUsed {
	if fallbackAPIKey, ok := h.trySwitchToClaudeFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
		ctx := context.WithValue(c.Request.Context(), ctxkey.ForcePlatform, "")
		c.Request = c.Request.WithContext(ctx)
		currentAPIKey = fallbackAPIKey
		currentSubscription = nil
		fallbackUsed = true
		retryWithFallback = true
		break
	}
}
```

When no fallback is available, leave the existing terminal path intact:

```go
_ = h.antigravityGatewayService.WriteMappedClaudeError(c, account, promptTooLongErr.StatusCode, promptTooLongErr.RequestID, promptTooLongErr.Body)
return
```

- [ ] **Step 7: Run Claude handler/service tests**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'TestGatewayService_ResolveRuntimeFallbackGroup|TestGatewayService_GroupResolution|TestGroupIsolation' -count=1
go test ./internal/handler -run 'TestGatewayErrorResponseIncludesTraceID|TestOpenAIHandleStreamingAwareError' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/handler/gateway_handler.go backend/internal/handler/gateway_handler_test.go backend/internal/service/gateway_multiplatform_test.go
git commit -m "feat: add claude group runtime fallback"
```

---

### Task 6: Implement OpenAI HTTP Runtime Fallback

**Files:**
- Modify: `backend/internal/handler/openai_gateway_handler.go`
- Modify: `backend/internal/handler/openai_chat_completions.go`
- Modify: `backend/internal/handler/openai_images.go`
- Modify: `backend/internal/handler/openai_gateway_handler_test.go`

- [ ] **Step 1: Write failing OpenAI fallback helper test**

Add to `backend/internal/handler/openai_gateway_handler_test.go`:

```go
func TestOpenAIRuntimeFallbackContext_SwitchesGroupAndPlatform(t *testing.T) {
	originalID := int64(100)
	fallbackID := int64(101)
	apiKey := &service.APIKey{
		ID:      1,
		UserID: 42,
		GroupID: &originalID,
		Group: &service.Group{
			ID:              originalID,
			Platform:        service.PlatformOpenAI,
			SubscriptionType: service.SubscriptionTypeStandard,
			FallbackGroupID: &fallbackID,
		},
		User: &service.User{ID: 42},
	}
	fallbackGroup := &service.Group{
		ID:              fallbackID,
		Platform:        service.PlatformOpenAI,
		Status:          service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
		IsFallbackGroup: true,
	}

	got := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)

	require.NotNil(t, got)
	require.Equal(t, fallbackID, *got.GroupID)
	require.Equal(t, service.PlatformOpenAI, got.Group.Platform)
	require.Equal(t, service.PlatformOpenAI, resolveOpenAICompatibleGroupPlatform(got))
}
```

- [ ] **Step 2: Run test to verify it passes after helper**

Run:

```bash
cd backend
go test ./internal/handler -run TestOpenAIRuntimeFallbackContext_SwitchesGroupAndPlatform -count=1
```

Expected: PASS after Task 4 helper exists.

- [ ] **Step 3: Add OpenAI fallback helper**

In `backend/internal/handler/openai_gateway_handler.go`, add:

```go
func (h *OpenAIGatewayHandler) trySwitchToOpenAIFallbackGroup(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	streamStarted bool,
) (*service.APIKey, bool) {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.FallbackGroupID == nil {
		return nil, false
	}
	fallbackGroup, err := h.gatewayService.ResolveRuntimeFallbackGroup(c.Request.Context(), apiKey.Group)
	if err != nil {
		reqLog.Warn("openai.runtime_fallback_group_unavailable", zap.Error(err), zap.Any("group_id", apiKey.GroupID))
		return nil, false
	}
	if fallbackGroup.Platform != service.PlatformOpenAI {
		reqLog.Warn("openai.runtime_fallback_group_platform_mismatch",
			zap.String("fallback_platform", fallbackGroup.Platform),
			zap.Int64("fallback_group_id", fallbackGroup.ID),
		)
		return nil, false
	}
	fallbackAPIKey := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)
	if fallbackAPIKey == nil {
		return nil, false
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), fallbackAPIKey.User, fallbackAPIKey, fallbackGroup, nil); err != nil {
		status, code, message := billingErrorDetails(err)
		h.handleStreamingAwareError(c, status, code, message, streamStarted)
		return nil, false
	}
	reqLog.Warn("openai.runtime_fallback_group_switch",
		zap.Any("original_group_id", apiKey.GroupID),
		zap.Int64("fallback_group_id", fallbackGroup.ID),
	)
	return fallbackAPIKey, true
}
```

- [ ] **Step 4: Update Responses handler loop**

In `Responses`, introduce:

```go
currentAPIKey := apiKey
currentSubscription := subscription
fallbackUsed := false
```

Use `currentAPIKey` in:

```go
platform := resolveOpenAICompatibleGroupPlatform(currentAPIKey)
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForPlatform(..., currentAPIKey.GroupID, ...)
accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, currentAPIKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), currentAPIKey.GroupID, reqModel)
```

On selection failure with no failed accounts:

```go
if len(failedAccountIDs) == 0 && !fallbackUsed {
	if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
		currentAPIKey = fallbackAPIKey
		currentSubscription = nil
		fallbackUsed = true
		switchCount = 0
		failedAccountIDs = make(map[int64]struct{})
		sameAccountRetryCount = make(map[int64]int)
		lastFailoverErr = nil
		continue
	}
}
```

On failover exhaustion before returning:

```go
if switchCount >= maxAccountSwitches {
	if !fallbackUsed {
		if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
			currentAPIKey = fallbackAPIKey
			currentSubscription = nil
			fallbackUsed = true
			switchCount = 0
			failedAccountIDs = make(map[int64]struct{})
			sameAccountRetryCount = make(map[int64]int)
			lastFailoverErr = nil
			continue
		}
	}
	h.handleFailoverExhausted(c, failoverErr, streamStarted)
	return
}
```

Record usage with `currentAPIKey` and `currentSubscription`.

- [ ] **Step 5: Update ChatCompletions handler loop**

In `backend/internal/handler/openai_chat_completions.go`, introduce these variables after the first billing check:

```go
currentAPIKey := apiKey
currentSubscription := subscription
fallbackUsed := false
```

Use `currentAPIKey` in account selection, account slot acquisition, channel mapping, and usage recording:

```go
platform := resolveOpenAICompatibleGroupPlatform(currentAPIKey)
channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), currentAPIKey.GroupID, reqModel)
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForPlatform(
	c.Request.Context(),
	platform,
	currentAPIKey.GroupID,
	"",
	sessionHash,
	reqModel,
	failedAccountIDs,
	service.OpenAIUpstreamTransportAny,
)
accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, currentAPIKey.GroupID, sessionHash, selection, reqStream, &streamStarted, reqLog)
```

On selection failure with no failed accounts, add:

```go
if len(failedAccountIDs) == 0 && !fallbackUsed {
	if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
		currentAPIKey = fallbackAPIKey
		currentSubscription = nil
		fallbackUsed = true
		switchCount = 0
		failedAccountIDs = make(map[int64]struct{})
		sameAccountRetryCount = make(map[int64]int)
		lastFailoverErr = nil
		continue
	}
}
```

On failover exhaustion, add this block before returning:

```go
if switchCount >= maxAccountSwitches {
	if !fallbackUsed {
		if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
			currentAPIKey = fallbackAPIKey
			currentSubscription = nil
			fallbackUsed = true
			switchCount = 0
			failedAccountIDs = make(map[int64]struct{})
			sameAccountRetryCount = make(map[int64]int)
			lastFailoverErr = nil
			continue
		}
	}
	h.handleFailoverExhausted(c, failoverErr, streamStarted)
	return
}
```

Recompute:

```go
defaultMappedModel := resolveOpenAIForwardDefaultMappedModel(currentAPIKey, c.GetString("openai_chat_completions_fallback_model"))
```

Record usage with:

```go
APIKey:       currentAPIKey,
User:         currentAPIKey.User,
Subscription: currentSubscription,
```

- [ ] **Step 6: Update Images handler loop**

In `backend/internal/handler/openai_images.go`, introduce:

```go
currentAPIKey := apiKey
currentSubscription := subscription
fallbackUsed := false
```

Call:

```go
channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), currentAPIKey.GroupID, parsed.Model)
selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForImagesForPlatform(
	c.Request.Context(),
	resolveOpenAICompatibleGroupPlatform(currentAPIKey),
	currentAPIKey.GroupID,
	sessionHash,
	parsed.Model,
	failedAccountIDs,
	parsed.RequiredCapability,
)
accountReleaseFunc, acquired := h.acquireResponsesAccountSlot(c, currentAPIKey.GroupID, sessionHash, selection, parsed.Stream, &streamStarted, reqLog)
```

On selection failure with no failed accounts, add:

```go
if len(failedAccountIDs) == 0 && !fallbackUsed {
	if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
		currentAPIKey = fallbackAPIKey
		currentSubscription = nil
		fallbackUsed = true
		switchCount = 0
		failedAccountIDs = make(map[int64]struct{})
		sameAccountRetryCount = make(map[int64]int)
		lastFailoverErr = nil
		continue
	}
}
```

On failover exhaustion, add:

```go
if switchCount >= maxAccountSwitches {
	if !fallbackUsed {
		if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroup(c, reqLog, currentAPIKey, streamStarted); ok {
			currentAPIKey = fallbackAPIKey
			currentSubscription = nil
			fallbackUsed = true
			switchCount = 0
			failedAccountIDs = make(map[int64]struct{})
			sameAccountRetryCount = make(map[int64]int)
			lastFailoverErr = nil
			continue
		}
	}
	h.handleFailoverExhausted(c, failoverErr, streamStarted)
	return
}
```

Record usage with:

```go
APIKey:       currentAPIKey,
User:         currentAPIKey.User,
Subscription: currentSubscription,
```

- [ ] **Step 7: Update OpenAI Messages handler loop**

In `backend/internal/handler/openai_gateway_handler.go` `Messages`, introduce:

```go
currentAPIKey := apiKey
currentSubscription := subscription
fallbackUsed := false
```

Add this Anthropic-format wrapper next to `trySwitchToOpenAIFallbackGroup`:

```go
func (h *OpenAIGatewayHandler) trySwitchToOpenAIFallbackGroupForAnthropicMessages(
	c *gin.Context,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	streamStarted bool,
) (*service.APIKey, bool) {
	if apiKey == nil || apiKey.Group == nil || apiKey.Group.FallbackGroupID == nil {
		return nil, false
	}
	fallbackGroup, err := h.gatewayService.ResolveRuntimeFallbackGroup(c.Request.Context(), apiKey.Group)
	if err != nil {
		reqLog.Warn("openai_messages.runtime_fallback_group_unavailable", zap.Error(err), zap.Any("group_id", apiKey.GroupID))
		return nil, false
	}
	if fallbackGroup.Platform != service.PlatformOpenAI {
		return nil, false
	}
	fallbackAPIKey := cloneAPIKeyWithFallbackGroup(apiKey, fallbackGroup)
	if fallbackAPIKey == nil {
		return nil, false
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), fallbackAPIKey.User, fallbackAPIKey, fallbackGroup, nil); err != nil {
		status, code, message := billingErrorDetails(err)
		h.anthropicStreamingAwareError(c, status, code, message, streamStarted)
		return nil, false
	}
	reqLog.Warn("openai_messages.runtime_fallback_group_switch",
		zap.Any("original_group_id", apiKey.GroupID),
		zap.Int64("fallback_group_id", fallbackGroup.ID),
	)
	return fallbackAPIKey, true
}
```

On selection failure with no failed accounts, add:

```go
if len(failedAccountIDs) == 0 && !fallbackUsed {
	if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroupForAnthropicMessages(c, reqLog, currentAPIKey, streamStarted); ok {
		currentAPIKey = fallbackAPIKey
		currentSubscription = nil
		fallbackUsed = true
		switchCount = 0
		failedAccountIDs = make(map[int64]struct{})
		sameAccountRetryCount = make(map[int64]int)
		lastFailoverErr = nil
		continue
	}
}
```

On failover exhaustion, add:

```go
if switchCount >= maxAccountSwitches {
	if !fallbackUsed {
		if fallbackAPIKey, ok := h.trySwitchToOpenAIFallbackGroupForAnthropicMessages(c, reqLog, currentAPIKey, streamStarted); ok {
			currentAPIKey = fallbackAPIKey
			currentSubscription = nil
			fallbackUsed = true
			switchCount = 0
			failedAccountIDs = make(map[int64]struct{})
			sameAccountRetryCount = make(map[int64]int)
			lastFailoverErr = nil
			continue
		}
	}
	h.handleAnthropicFailoverExhausted(c, failoverErr, streamStarted)
	return
}
```

Record usage with:

```go
APIKey:       currentAPIKey,
User:         currentAPIKey.User,
Subscription: currentSubscription,
```

- [ ] **Step 8: Run OpenAI handler tests**

Run:

```bash
cd backend
go test ./internal/handler -run 'TestOpenAI|TestShouldLogOpenAI|TestResolveOpenAI|TestOpenAIRuntimeFallback' -count=1
go test -tags unit ./internal/service -run 'TestOpenAIGatewayService_SelectAccountWithScheduler|TestOpenAIGatewayService_SelectAccountForModelWithExclusions' -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add backend/internal/handler/openai_gateway_handler.go backend/internal/handler/openai_chat_completions.go backend/internal/handler/openai_images.go backend/internal/handler/openai_gateway_handler_test.go
git commit -m "feat: add openai http group fallback"
```

---

### Task 7: Deprecate Invalid Request Fallback Runtime

**Files:**
- Modify: `backend/internal/service/group.go`
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/admin/group_handler.go`
- Modify: `backend/internal/service/admin_service.go`
- Modify: `backend/internal/handler/gateway_handler.go`
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/types/index.ts`

- [ ] **Step 1: Write failing test for old field not used**

Add to `backend/internal/service/admin_service_group_test.go`:

```go
func TestAdminService_UpdateGroup_InvalidRequestFallbackClearedWhenDeprecatedFieldSent(t *testing.T) {
	fallbackID := int64(10)
	existing := &Group{
		ID:              1,
		Name:            "g1",
		Platform:        PlatformAnthropic,
		SubscriptionType: SubscriptionTypeStandard,
		Status:          StatusActive,
	}
	repo := &groupRepoStubForInvalidRequestFallback{
		groups: map[int64]*Group{
			existing.ID: existing,
			fallbackID: {ID: fallbackID, Platform: PlatformAnthropic, SubscriptionType: SubscriptionTypeStandard, Status: StatusActive},
		},
	}
	svc := &adminServiceImpl{groupRepo: repo}

	group, err := svc.UpdateGroup(context.Background(), existing.ID, &UpdateGroupInput{
		FallbackGroupIDOnInvalidRequest: &fallbackID,
	})

	require.NoError(t, err)
	require.NotNil(t, group)
	require.Nil(t, repo.updated.FallbackGroupIDOnInvalidRequest)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run TestAdminService_UpdateGroup_InvalidRequestFallbackClearedWhenDeprecatedFieldSent -count=1
```

Expected: fails because old field is still persisted.

- [ ] **Step 3: Stop persisting old field from admin inputs**

In `CreateGroup`, force:

```go
fallbackOnInvalidRequest := (*int64)(nil)
```

Do not validate or store `input.FallbackGroupIDOnInvalidRequest`.

In `UpdateGroup`, force:

```go
group.FallbackGroupIDOnInvalidRequest = nil
```

Add nearby comment:

```go
// Deprecated: fallback_group_id_on_invalid_request will be removed in next version.
// Runtime fallback now uses fallback_group_id.
```

- [ ] **Step 4: Add deprecation comments to handler request structs**

In `backend/internal/handler/admin/group_handler.go`, add:

```go
// Deprecated: will be removed in next version. Use fallback_group_id.
FallbackGroupIDOnInvalidRequest *int64 `json:"fallback_group_id_on_invalid_request"`
```

- [ ] **Step 5: Hide old UI and keep payload deterministic**

In `frontend/src/views/admin/GroupsView.vue`, wrap every visible `fallback_group_id_on_invalid_request` form block with `v-if="false"` and place this comment directly above the first hidden block:

```vue
<!-- Deprecated: fallback_group_id_on_invalid_request will be removed in next version. Use fallback_group_id. -->
```

Delete watchers whose only purpose is clearing `fallback_group_id_on_invalid_request` when platform changes. In both create and update payload builders, send the old field as `null` so existing values are cleared by the backend no-op path:

```ts
fallback_group_id_on_invalid_request: null,
```

- [ ] **Step 6: Update TypeScript comments**

In `frontend/src/types/index.ts`, update comments:

```ts
// Deprecated: will be removed in next version. Use fallback_group_id.
fallback_group_id_on_invalid_request: number | null
```

- [ ] **Step 7: Run tests**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_.*InvalidRequestFallback|TestAdminService_UpdateGroup_InvalidRequestFallbackClearedWhenDeprecatedFieldSent' -count=1
```

Expected: PASS with the deprecated field cleared or ignored.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/service/admin_service.go backend/internal/service/admin_service_group_test.go backend/internal/handler/admin/group_handler.go backend/internal/service/group.go backend/internal/handler/dto/types.go frontend/src/views/admin/GroupsView.vue frontend/src/types/index.ts
git commit -m "chore: deprecate invalid request fallback field"
```

---

### Task 8: Update Admin Group UI

**Files:**
- Modify: `frontend/src/views/admin/GroupsView.vue`
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Modify: `frontend/src/views/admin/__tests__` or create `frontend/src/views/admin/__tests__/GroupsView.fallback.spec.ts`

- [ ] **Step 1: Write failing frontend tests for options**

Create `frontend/src/views/admin/__tests__/GroupsView.fallback.spec.ts` with focused pure-function tests. If `GroupsView.vue` does not export helpers, extract these helpers in Step 3.

```ts
import { describe, expect, it } from 'vitest'
import type { AdminGroup } from '@/types'
import {
  canEnableFallbackGroup,
  buildFallbackTargetOptions,
  isApiKeyBindableGroup,
} from '../GroupsView.fallback'

const baseGroup = (overrides: Partial<AdminGroup>): AdminGroup => ({
  id: 1,
  name: 'group',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  system_prompt: '',
  system_prompt_mode: 'inherit',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  is_fallback_group: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  sort_order: 0,
  ...overrides,
})

describe('group fallback helpers', () => {
  it('only allows openai and anthropic standard groups to enable fallback flag', () => {
    expect(canEnableFallbackGroup(baseGroup({ platform: 'openai', subscription_type: 'standard' }))).toBe(true)
    expect(canEnableFallbackGroup(baseGroup({ platform: 'anthropic', subscription_type: 'standard' }))).toBe(true)
    expect(canEnableFallbackGroup(baseGroup({ platform: 'gemini', subscription_type: 'standard' }))).toBe(false)
    expect(canEnableFallbackGroup(baseGroup({ platform: 'openai', subscription_type: 'subscription' }))).toBe(false)
  })

  it('builds same-platform fallback targets only', () => {
    const groups = [
      baseGroup({ id: 1, platform: 'openai', is_fallback_group: false }),
      baseGroup({ id: 2, platform: 'openai', is_fallback_group: true }),
      baseGroup({ id: 3, platform: 'anthropic', is_fallback_group: true }),
    ]
    expect(buildFallbackTargetOptions(groups, baseGroup({ id: 1, platform: 'openai' })).map(o => o.value)).toEqual([null, 2])
  })

  it('api key binding excludes fallback groups', () => {
    expect(isApiKeyBindableGroup(baseGroup({ is_fallback_group: false }))).toBe(true)
    expect(isApiKeyBindableGroup(baseGroup({ is_fallback_group: true }))).toBe(false)
  })
})
```

- [ ] **Step 2: Run frontend test to verify it fails**

Run:

```bash
cd frontend
npm run test -- GroupsView.fallback.spec.ts
```

Expected: fails because `GroupsView.fallback` helper does not exist.

- [ ] **Step 3: Extract fallback UI helpers**

Create `frontend/src/views/admin/GroupsView.fallback.ts`:

```ts
import type { AdminGroup } from '@/types'

export interface FallbackOption {
  value: number | null
  label: string
}

export function canEnableFallbackGroup(group: Pick<AdminGroup, 'platform' | 'subscription_type'>): boolean {
  return (group.platform === 'openai' || group.platform === 'anthropic') && group.subscription_type === 'standard'
}

export function isApiKeyBindableGroup(group: Pick<AdminGroup, 'is_fallback_group'>): boolean {
  return !group.is_fallback_group
}

export function buildFallbackTargetOptions(
  groups: AdminGroup[],
  current: Pick<AdminGroup, 'id' | 'platform'>,
  noFallbackLabel = 'No Fallback',
): FallbackOption[] {
  const options: FallbackOption[] = [{ value: null, label: noFallbackLabel }]
  for (const group of groups) {
    if (
      group.id !== current.id &&
      group.status === 'active' &&
      group.subscription_type === 'standard' &&
      group.is_fallback_group &&
      group.platform === current.platform &&
      !group.fallback_group_id
    ) {
      options.push({ value: group.id, label: group.name })
    }
  }
  return options
}
```

- [ ] **Step 4: Add types**

In `frontend/src/types/index.ts`, add to `Group`:

```ts
is_fallback_group: boolean
// Deprecated: will be removed in next version. Use fallback_group_id.
fallback_group_id_on_invalid_request: number | null
```

Add to create/update request types:

```ts
is_fallback_group?: boolean
```

- [ ] **Step 5: Update GroupsView forms**

In `frontend/src/views/admin/GroupsView.vue`:

Import helpers:

```ts
import { buildFallbackTargetOptions, canEnableFallbackGroup } from './GroupsView.fallback'
```

Add to create/edit forms:

```ts
is_fallback_group: false,
```

Add UI near fallback settings:

```vue
<div v-if="canEnableFallbackGroup(createForm)" class="border-t pt-4">
  <label class="input-label">{{ t('admin.groups.fallbackGroup.enabled') }}</label>
  <button
    type="button"
    @click="createForm.is_fallback_group = !createForm.is_fallback_group"
    class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
    :class="createForm.is_fallback_group ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'"
  >
    <span class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out" :class="createForm.is_fallback_group ? 'translate-x-6' : 'translate-x-1'" />
  </button>
  <p class="input-hint">{{ t('admin.groups.fallbackGroup.enabledHint') }}</p>
</div>
```

Add the edit form block:

```vue
<div v-if="canEnableFallbackGroup(editForm)" class="border-t pt-4">
  <label class="input-label">{{ t('admin.groups.fallbackGroup.enabled') }}</label>
  <button
    type="button"
    @click="editForm.is_fallback_group = !editForm.is_fallback_group"
    class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
    :class="editForm.is_fallback_group ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'"
  >
    <span class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out" :class="editForm.is_fallback_group ? 'translate-x-6' : 'translate-x-1'" />
  </button>
  <p class="input-hint">{{ t('admin.groups.fallbackGroup.enabledHint') }}</p>
</div>
```

Replace old `invalidRequestFallbackOptions` with:

```ts
const fallbackGroupOptions = computed(() =>
  buildFallbackTargetOptions(groups.value, {
    id: 0,
    platform: createForm.platform,
  } as AdminGroup, t('admin.groups.fallbackGroup.noFallback'))
)

const fallbackGroupOptionsForEdit = computed(() => {
  const current = editingGroup.value
  if (!current) return [{ value: null, label: t('admin.groups.fallbackGroup.noFallback') }]
  return buildFallbackTargetOptions(groups.value, current, t('admin.groups.fallbackGroup.noFallback'))
})
```

Bind `fallback_group_id` select to these options for Claude/OpenAI standard entry groups.

- [ ] **Step 6: Add i18n keys**

In `frontend/src/i18n/locales/zh.ts` under `admin.groups`, add:

```ts
fallbackGroup: {
  enabled: '启用为兜底分组',
  enabledHint: '启用后，此分组只能作为其他分组的兜底目标，不能被 API Key 直接选择。',
  target: '兜底分组',
  targetHint: '当前分组不可用或上游可重试错误耗尽后，将切换到该分组重试一次。',
  noFallback: '不使用兜底',
  badge: '兜底分组'
},
```

In `frontend/src/i18n/locales/en.ts`, add:

```ts
fallbackGroup: {
  enabled: 'Enable as Fallback Group',
  enabledHint: 'When enabled, this group can only be used as a fallback target and cannot be selected directly by API keys.',
  target: 'Fallback Group',
  targetHint: 'When the current group is unavailable or retryable upstream errors are exhausted, requests switch to this group once.',
  noFallback: 'No Fallback',
  badge: 'Fallback Group'
},
```

- [ ] **Step 7: Run frontend tests**

Run:

```bash
cd frontend
npm run test -- GroupsView.fallback.spec.ts
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add frontend/src/views/admin/GroupsView.vue frontend/src/views/admin/GroupsView.fallback.ts frontend/src/views/admin/__tests__/GroupsView.fallback.spec.ts frontend/src/types/index.ts frontend/src/i18n/locales/zh.ts frontend/src/i18n/locales/en.ts
git commit -m "feat: add fallback group admin ui"
```

---

### Task 9: Hide Fallback Groups From API Key Selection

**Files:**
- Modify: `frontend/src/views/user/KeysView.vue`
- Modify: `frontend/src/components/admin/user/UserAllowedGroupsModal.vue`
- Modify: `frontend/src/components/admin/user/UserApiKeysModal.vue`
- Modify or add frontend tests under matching `__tests__`

- [ ] **Step 1: Write failing helper test**

Create `frontend/src/views/user/__tests__/KeysView.groups.spec.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { isSelectableApiKeyGroup } from '../KeysView.groups'

describe('KeysView group selection', () => {
  it('excludes fallback groups from api key selection', () => {
    expect(isSelectableApiKeyGroup({ is_fallback_group: false })).toBe(true)
    expect(isSelectableApiKeyGroup({ is_fallback_group: true })).toBe(false)
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd frontend
npm run test -- KeysView.groups.spec.ts
```

Expected: fails because helper does not exist.

- [ ] **Step 3: Add helper**

Create `frontend/src/views/user/KeysView.groups.ts`:

```ts
export function isSelectableApiKeyGroup(group: { is_fallback_group?: boolean }): boolean {
  return group.is_fallback_group !== true
}
```

- [ ] **Step 4: Filter user key options**

In `frontend/src/views/user/KeysView.vue`, import:

```ts
import { isSelectableApiKeyGroup } from './KeysView.groups'
```

Change `groupOptions`:

```ts
const groupOptions = computed(() =>
  groups.value
    .filter(isSelectableApiKeyGroup)
    .map((group) => ({
      value: group.id,
      label: group.name,
      description: group.description,
      rate: group.rate_multiplier,
      userRate: userGroupRates.value[group.id] ?? null,
      subscriptionType: group.subscription_type,
      platform: group.platform
    }))
)
```

Keep `groupFilterOptions` unchanged so list filtering can still inspect old or unexpected bindings.

- [ ] **Step 5: Filter admin user group assignment**

In `frontend/src/components/admin/user/UserAllowedGroupsModal.vue`, update the load filter:

```ts
groups.value = res.items.filter((g) =>
  g.subscription_type === 'standard' &&
  g.status === 'active' &&
  !g.is_fallback_group
)
```

In `frontend/src/components/admin/user/UserApiKeysModal.vue`, filter `allGroups` options:

```ts
allGroups.value = groups.filter((g) => !g.is_fallback_group)
```

- [ ] **Step 6: Run tests**

Run:

```bash
cd frontend
npm run test -- KeysView.groups.spec.ts
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/views/user/KeysView.vue frontend/src/views/user/KeysView.groups.ts frontend/src/views/user/__tests__/KeysView.groups.spec.ts frontend/src/components/admin/user/UserAllowedGroupsModal.vue frontend/src/components/admin/user/UserApiKeysModal.vue
git commit -m "feat: hide fallback groups from api key selection"
```

---

### Task 10: End-To-End Verification And Cleanup

**Files:**
- Modify: only files already touched in Tasks 1-9 when a verification command reports a concrete failure.
- No new feature files expected.

- [ ] **Step 1: Run focused backend tests**

Run:

```bash
cd backend
go test -tags unit ./internal/service -run 'TestAdminService_.*Fallback|TestAPIKeyService_.*Fallback|TestGatewayService_.*Fallback|TestOpenAIGatewayService_SelectAccount' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run focused handler tests**

Run:

```bash
cd backend
go test ./internal/handler -run 'TestOpenAI|TestGateway|TestShouldLog|TestResolveRuntimeFallback|TestOpenAIRuntimeFallback' -count=1
```

Expected: PASS.

- [ ] **Step 3: Run frontend focused tests**

Run:

```bash
cd frontend
npm run test -- GroupsView.fallback.spec.ts KeysView.groups.spec.ts
```

Expected: PASS.

- [ ] **Step 4: Run formatting and static checks**

Run:

```bash
cd backend
gofmt -w internal/service/admin_service.go internal/service/api_key_service.go internal/handler/gateway_handler.go internal/handler/openai_gateway_handler.go internal/handler/openai_chat_completions.go internal/handler/openai_images.go internal/repository/group_repo.go internal/handler/dto/types.go internal/handler/dto/mappers.go internal/handler/admin/group_handler.go
go test -tags unit ./internal/service -count=1
go test ./internal/handler -count=1
```

Expected: PASS.

Run:

```bash
cd frontend
npm run test -- --run
```

Expected: PASS.

- [ ] **Step 5: Check generated/migration consistency**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors. Status only includes intended files.

- [ ] **Step 6: Confirm spec matches implementation**

Open `docs/superpowers/specs/2026-06-28-openai-claude-group-fallback-design.md` and verify these facts are still true:

```text
fallback_group_id is the runtime fallback field
is_fallback_group gates fallback targets
fallback_group_id_on_invalid_request is deprecated and runtime-inactive
OpenAI WebSocket is not covered
fallback groups cannot be API Key entry groups
```

When all five lines match the implementation, leave the spec unchanged.

- [ ] **Step 7: Final commit**

```bash
git add backend frontend docs/superpowers/specs/2026-06-28-openai-claude-group-fallback-design.md
git commit -m "test: verify openai and claude group fallback"
```

Expected: commit succeeds when Task 10 produced verification fixes. When `git status --short` is empty after Step 5, skip this commit and report `verification produced no diff`.

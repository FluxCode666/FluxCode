# Admin Gift Balance User Select Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make admin balance displays include gift balance and replace manual user ID entry with searchable email selectors in referral admin workflows.

**Architecture:** Add a read-only admin DTO field for gift balance remaining, populate it in the admin user list handler, and keep the persisted `balance` unchanged. Reuse the existing Vue `UserSearchSelect` component in referral management and calculate the combined balance in the users table.

**Tech Stack:** Go/Gin service layer and handlers, Vue 3, TypeScript, Vitest.

---

### Task 1: Backend Gift Balance Field

**Files:**
- Modify: `backend/internal/handler/dto/types.go`
- Modify: `backend/internal/handler/admin/user_handler.go`
- Test: `backend/internal/handler/admin/user_handler_test.go`

- [ ] Write a failing handler test that lists a user with a gift balance repository returning `12.5` and expects `gift_balance_remaining: 12.5`.
- [ ] Run the targeted Go unit test and confirm it fails because the field is missing or zero.
- [ ] Add `GiftBalanceRemaining` to the admin user DTO and populate it from `GiftBalanceRepository.GetTotalRemainingByUserID`.
- [ ] Run the targeted Go unit test and confirm it passes.

### Task 2: User List Balance Display

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/views/admin/UsersView.vue`
- Test: `frontend/src/views/admin/__tests__/UsersView.spec.ts`

- [ ] Write a failing Vitest case where a user has `balance: 10` and `gift_balance_remaining: 2.5`; expect the balance cell to show `$12.50`.
- [ ] Run the targeted Vitest file and confirm it fails.
- [ ] Add the optional TypeScript field and display helper for combined balance plus split details.
- [ ] Run the targeted Vitest file and confirm it passes.

### Task 3: Referral Management Selectors and Copy

**Files:**
- Modify: `frontend/src/views/admin/ReferralManageView.vue`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`
- Test: `frontend/src/views/admin/__tests__/ReferralManageView.spec.ts`

- [ ] Write failing Vitest cases for the renamed tab, fallback/audit hint, grant user selector, and user config selector.
- [ ] Run the targeted Vitest file and confirm the new cases fail.
- [ ] Replace the manual numeric inputs with `UserSearchSelect` and bind selected IDs through string refs.
- [ ] Update i18n copy for the renamed tab, user search placeholder, and fallback/audit hint.
- [ ] Run the targeted Vitest file and confirm it passes.

### Task 4: Verification and Audit

- [ ] Run targeted Go tests for the admin handler/service area.
- [ ] Run targeted frontend tests for users and referral management.
- [ ] Run typecheck/build if targeted tests expose type drift.
- [ ] Review the final diff against all three user requirements.

# Growth Dashboard UA Audience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add aggregated UA audience charts to the admin-only Growth Dashboard without exposing raw User-Agent values.

**Architecture:** Reuse the existing Growth Handler → Service → Repository pattern. Add four independent REST endpoints under `/admin/growth/audience/*`, one front-end API method per chart, and one dashboard section with four cards.

**Tech Stack:** Go, Gin, PostgreSQL SQL via `database/sql`, Vue 3, Chart.js, Vitest, sqlmock.

---

## Files

- Modify: `backend/internal/service/growth.go`
- Modify: `backend/internal/service/growth_test.go`
- Modify: `backend/internal/repository/growth_repo.go`
- Modify: `backend/internal/repository/growth_repo_test.go`
- Modify: `backend/internal/handler/admin/growth_handler.go`
- Modify: `backend/internal/server/routes/admin.go`
- Modify: `backend/internal/server/routes/growth_test.go`
- Modify: `frontend/src/api/admin/growth.ts`
- Modify: `frontend/src/api/admin/__tests__/growth.spec.ts`
- Modify: `frontend/src/views/admin/GrowthDashboardView.vue`
- Modify: `frontend/src/views/admin/__tests__/GrowthDashboardView.spec.ts`
- Modify: `frontend/src/i18n/locales/zh.ts`
- Modify: `frontend/src/i18n/locales/en.ts`

## Tasks

- [x] Add failing backend tests for `GetAudienceDevices`, `GetAudienceOS`, `GetAudienceBrowsers`, and `GetAudienceClients`.
- [x] Implement service DTOs, repository interface methods, handler methods, and route registration.
- [x] Implement repository SQL aggregation using CASE expressions over `usage_logs.user_agent`.
- [x] Run targeted Go tests and fix failures.
- [x] Add failing front-end API/view tests for the four independent audience endpoints.
- [x] Implement TypeScript types, API methods, dashboard section, chart data, and i18n labels.
- [x] Run targeted front-end tests and fix failures.
- [x] Run `git diff --check` and report any unverified items.

## API Contract

Each endpoint accepts the existing growth query params:

```text
start_date=YYYY-MM-DD
end_date=YYYY-MM-DD
granularity=day|week|month
```

Each endpoint returns:

```json
{
  "items": [
    {
      "key": "desktop",
      "label": "Desktop",
      "users": 12,
      "requests": 40,
      "user_ratio": 0.3
    }
  ]
}
```

Errors follow existing Growth handler behavior: invalid range returns `400`; repository failures return the existing internal error envelope.

## Verification Commands

```bash
go test ./internal/service ./internal/repository ./internal/handler/admin ./internal/server/routes -run 'TestGrowth|TestAdmin|TestRoute' -count=1
pnpm test:run src/api/admin/__tests__/growth.spec.ts src/views/admin/__tests__/GrowthDashboardView.spec.ts src/i18n/__tests__/staticLocaleCoverage.spec.ts
git diff --check
```

# Ops Alert Deep-Link Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复运维告警详情“查看相关日志”深链在页面初始化阶段触发 `ReferenceError` 的问题。

**Architecture:** 保持现有路由参数与弹窗状态设计不变，只把首次 `applyRouteQueryToState()` 调用移动到其依赖的响应式状态声明之后。通过组件级 Vitest 用例从带深链参数的初始路由挂载页面，验证错误详情弹窗能够正常打开。

**Tech Stack:** Vue 3、Vue Router、TypeScript、Vitest、Vue Test Utils

---

## 文件结构

- 新建 `frontend/src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts`：覆盖初始路由深链的回归行为。
- 修改 `frontend/src/views/admin/ops/OpsDashboard.vue`：仅调整首次路由查询解析的调用顺序。

### Task 1: 用回归测试驱动深链初始化修复

**Files:**
- Create: `frontend/src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts`
- Modify: `frontend/src/views/admin/ops/OpsDashboard.vue:313,381`
- Test: `frontend/src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts`

- [ ] **Step 1: 编写失败的深链回归测试**

创建以下组件测试。测试从带 `open_error_details=1&error_type=request` 的初始路由挂载页面，并读取错误详情弹窗桩组件的属性：

```ts
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent } from 'vue'
import OpsDashboard from '../OpsDashboard.vue'

const { replace, fetchSettings } = vi.hoisted(() => ({
  replace: vi.fn(),
  fetchSettings: vi.fn().mockResolvedValue(undefined)
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {
      open_error_details: '1',
      error_type: 'request'
    }
  }),
  useRouter: () => ({ replace })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('@vueuse/core', () => ({
  useDebounceFn: (fn: (...args: unknown[]) => unknown) => fn,
  useIntervalFn: () => ({ pause: vi.fn(), resume: vi.fn() })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: vi.fn() }),
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: true,
    opsQueryModeDefault: 'auto',
    fetch: fetchSettings
  })
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    getAdvancedSettings: vi.fn().mockResolvedValue({
      display_alert_events: true,
      display_openai_token_stats: false,
      auto_refresh_enabled: false,
      auto_refresh_interval_seconds: 30
    }),
    getDashboardSnapshotV2: vi.fn().mockResolvedValue({
      overview: null,
      throughput_trend: null,
      error_trend: null
    }),
    getThroughputTrend: vi.fn().mockResolvedValue({ points: [] }),
    getLatencyHistogram: vi.fn().mockResolvedValue(null),
    getErrorDistribution: vi.fn().mockResolvedValue(null),
    getMetricThresholds: vi.fn().mockResolvedValue({})
  }
}))

const ErrorDetailsStub = defineComponent({
  props: {
    show: { type: Boolean, default: false },
    errorType: { type: String, default: '' }
  },
  template: '<div data-testid="error-details" :data-show="String(show)" :data-error-type="errorType" />'
})

let wrapper: VueWrapper | undefined

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.clearAllMocks()
})

describe('OpsDashboard deep links', () => {
  it('opens request error details from the initial alert log deep link', () => {
    wrapper = mount(OpsDashboard, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          BaseDialog: true,
          OpsDashboardSkeleton: true,
          OpsDashboardHeader: true,
          OpsConcurrencyCard: true,
          OpsSwitchRateTrendChart: true,
          OpsThroughputTrendChart: true,
          OpsLatencyChart: true,
          OpsErrorDistributionChart: true,
          OpsErrorTrendChart: true,
          OpsOpenAITokenStatsCard: true,
          OpsAlertEventsCard: true,
          OpsSystemLogTable: true,
          OpsSettingsDialog: true,
          OpsAlertRulesCard: true,
          OpsErrorDetailModal: true,
          OpsRequestDetailsModal: true,
          OpsErrorDetailsModal: ErrorDetailsStub
        }
      }
    })

    const errorDetails = wrapper.get('[data-testid="error-details"]')
    expect(errorDetails.attributes('data-show')).toBe('true')
    expect(errorDetails.attributes('data-error-type')).toBe('request')
  })
})
```

- [ ] **Step 2: 运行测试并确认当前实现失败**

Run:

```bash
cd frontend
pnpm test:run -- src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts
```

Expected: FAIL，挂载 `OpsDashboard` 时出现类似 `Cannot access 'errorDetailsType' before initialization` 的 `ReferenceError`。

- [ ] **Step 3: 实施最小调用顺序修复**

从当前 `buildQueryFromState` 之前删除首次调用：

```ts
applyRouteQueryToState()
```

并在所有深链依赖状态声明后执行：

```ts
const showSettingsDialog = ref(false)
const showAlertRulesCard = ref(false)

applyRouteQueryToState()
```

不要修改 `applyRouteQueryToState()` 函数内容，也不要修改运行期间的 `route.query` 监听器。

- [ ] **Step 4: 运行专项测试并确认通过**

Run:

```bash
cd frontend
pnpm test:run -- src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts
```

Expected: PASS，1 个测试通过，错误详情弹窗的 `show=true` 且 `errorType=request`。

- [ ] **Step 5: 运行相关回归与类型检查**

Run:

```bash
cd frontend
pnpm test:run -- src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts src/views/admin/ops/components/__tests__/OpsOpenAITokenStatsCard.spec.ts
pnpm typecheck
```

Expected: 两个测试文件全部通过，`vue-tsc --noEmit` 退出码为 0。

- [ ] **Step 6: 精确提交实现**

```bash
git add frontend/src/views/admin/ops/OpsDashboard.vue frontend/src/views/admin/ops/__tests__/OpsDashboard.deepLink.spec.ts
git commit -m "fix(ops): repair alert log deep link"
```

提交前确认没有暂存既有的 `docs/plans/2026-06-22-openai-image-generation-api.md` 或 `frontend/pnpm-workspace.yaml`。

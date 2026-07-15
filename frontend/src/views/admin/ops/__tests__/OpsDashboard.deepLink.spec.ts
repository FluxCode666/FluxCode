import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
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

vi.mock('@/api/admin/ops', () => {
  const opsAPI = {
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
  return { opsAPI, default: opsAPI }
})

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
  it('opens request error details from the initial alert log deep link', async () => {
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
    await flushPromises()

    const errorDetails = wrapper.get('[data-testid="error-details"]')
    expect(errorDetails.attributes('data-show')).toBe('true')
    expect(errorDetails.attributes('data-error-type')).toBe('request')
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import type { DashboardStats } from '@/types'
import DashboardView from '../DashboardView.vue'

const {
  getSnapshotV2,
  getUserUsageTrend,
  getSubscriptionExhaustionTrend,
  getUserSpendingRanking,
  getProxyUsageSummary
} = vi.hoisted(() => ({
  getSnapshotV2: vi.fn(),
  getUserUsageTrend: vi.fn(),
  getSubscriptionExhaustionTrend: vi.fn(),
  getUserSpendingRanking: vi.fn(),
  getProxyUsageSummary: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    dashboard: {
      getSnapshotV2,
      getUserUsageTrend,
      getSubscriptionExhaustionTrend,
      getUserSpendingRanking,
      getProxyUsageSummary
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push: vi.fn()
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const createDashboardStats = (): DashboardStats => ({
  total_users: 0,
  today_new_users: 0,
  active_users: 0,
  hourly_active_users: 0,
  stats_updated_at: '',
  stats_stale: false,
  total_api_keys: 0,
  active_api_keys: 0,
  total_accounts: 0,
  normal_accounts: 0,
  error_accounts: 0,
  ratelimit_accounts: 0,
  overload_accounts: 0,
  total_requests: 0,
  total_input_tokens: 0,
  total_output_tokens: 0,
  total_cache_creation_tokens: 0,
  total_cache_read_tokens: 0,
  total_tokens: 0,
  total_cost: 0,
  total_actual_cost: 0,
  total_account_cost: 0,
  today_requests: 0,
  today_input_tokens: 0,
  today_output_tokens: 0,
  today_cache_creation_tokens: 0,
  today_cache_read_tokens: 0,
  today_tokens: 0,
  today_cost: 0,
  today_actual_cost: 0,
  today_account_cost: 0,
  average_duration_ms: 0,
  uptime: 0,
  rpm: 0,
  tpm: 0
})

describe('admin DashboardView', () => {
  beforeEach(() => {
    getSnapshotV2.mockReset()
    getUserUsageTrend.mockReset()
    getSubscriptionExhaustionTrend.mockReset()
    getUserSpendingRanking.mockReset()
    getProxyUsageSummary.mockReset()

    getSnapshotV2.mockResolvedValue({
      stats: createDashboardStats(),
      trend: [],
      models: []
    })
    getUserUsageTrend.mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'hour'
    })
    getSubscriptionExhaustionTrend.mockResolvedValue({
      trend: [],
      start_date: '',
      end_date: '',
      granularity: 'day'
    })
    getUserSpendingRanking.mockResolvedValue({
      ranking: [],
      total_actual_cost: 0,
      total_requests: 0,
      total_tokens: 0,
      start_date: '',
      end_date: ''
    })
    getProxyUsageSummary.mockResolvedValue({
      items: [],
      start_date: '',
      end_date: '',
      granularity: 'hour'
    })
  })

  it('uses last 24 hours as default dashboard range', async () => {
    mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          RequestCountTrend: true,
          SubscriptionExhaustionTrend: true,
          ProxyUsageSummaryChart: true,
          Line: true
        }
      }
    })

    await flushPromises()

    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))
    expect(getSubscriptionExhaustionTrend).not.toHaveBeenCalled()
  })

  it('places the collapsed proxy usage card below recent usage and loads it on expansion', async () => {
    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: {
            template: '<div data-test="token-usage-trend" />'
          },
          RequestCountTrend: {
            template: '<div data-test="request-count-trend" />'
          },
          SubscriptionExhaustionTrend: {
            template: '<div data-test="subscription-exhaustion-trend" />'
          },
          Line: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.find('[data-test="token-usage-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="request-count-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="subscription-exhaustion-trend"]').exists()).toBe(false)
    const recentUsageCard = wrapper.get('[data-test="recent-user-usage-card"]')
    const proxyUsageCard = wrapper.get('[data-test="proxy-usage-summary-card"]')
    expect(
      Boolean(recentUsageCard.element.compareDocumentPosition(proxyUsageCard.element) & Node.DOCUMENT_POSITION_FOLLOWING)
    ).toBe(true)
    expect(wrapper.find('[data-test="proxy-usage-summary-content"]').exists()).toBe(false)
    expect(getProxyUsageSummary).not.toHaveBeenCalled()

    await wrapper.get('[data-test="proxy-usage-summary-toggle"]').trigger('click')
    await flushPromises()

    expect(getProxyUsageSummary).toHaveBeenCalledWith(expect.objectContaining({ granularity: 'hour' }))
    expect(wrapper.find('[data-test="proxy-usage-summary-content"]').exists()).toBe(true)
  })

  it('still renders dashboard when the on-demand proxy usage request fails', async () => {
    getProxyUsageSummary.mockRejectedValueOnce({ status: 404, message: 'Not Found' })

    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: true,
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: {
            template: '<div data-test="token-usage-trend" />'
          },
          RequestCountTrend: {
            template: '<div data-test="request-count-trend" />'
          },
          SubscriptionExhaustionTrend: true,
          ProxyUsageSummaryChart: {
            emits: ['expand'],
            template: '<button data-test="proxy-usage-summary-toggle" @click="$emit(\'expand\')" />'
          },
          Line: true
        }
      }
    })

    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    expect(getProxyUsageSummary).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="token-usage-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="request-count-trend"]').exists()).toBe(true)

    await wrapper.get('[data-test="proxy-usage-summary-toggle"]').trigger('click')
    await flushPromises()

    expect(getProxyUsageSummary).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="token-usage-trend"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="request-count-trend"]').exists()).toBe(true)
  })

  it('loads and renders subscription exhaustion trend only for day granularity', async () => {
    const wrapper = mount(DashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true,
          DateRangePicker: {
            template:
              '<button data-test="range-day" @click="$emit(\'update:startDate\', \'2026-03-01\'); $emit(\'update:endDate\', \'2026-03-07\'); $emit(\'change\', { startDate: \'2026-03-01\', endDate: \'2026-03-07\', preset: null })" />'
          },
          Select: true,
          ModelDistributionChart: true,
          TokenUsageTrend: true,
          RequestCountTrend: true,
          SubscriptionExhaustionTrend: {
            template: '<div data-test="subscription-exhaustion-trend" />',
            props: ['trendData', 'loading']
          },
          ProxyUsageSummaryChart: true,
          Line: true
        }
      }
    })

    await flushPromises()
    expect(getSubscriptionExhaustionTrend).not.toHaveBeenCalled()
    expect(wrapper.find('[data-test="subscription-exhaustion-trend"]').exists()).toBe(false)

    await wrapper.find('[data-test="range-day"]').trigger('click')
    await flushPromises()

    expect(getSubscriptionExhaustionTrend).toHaveBeenCalledTimes(1)
    expect(getSubscriptionExhaustionTrend).toHaveBeenCalledWith({
      start_date: '2026-03-01',
      end_date: '2026-03-07'
    })
    expect(wrapper.find('[data-test="subscription-exhaustion-trend"]').exists()).toBe(true)
  })
})

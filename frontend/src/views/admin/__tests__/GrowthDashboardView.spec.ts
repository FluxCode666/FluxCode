import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import GrowthDashboardView from '../GrowthDashboardView.vue'

const {
  getOverview,
  getUserTrend,
  getUserSources,
  getSourcePaymentRates,
  getRetentionMatrix,
  getRetentionTrend,
  getPaymentFunnel,
  getPaymentPlans,
  getFirstPayment,
  getFeatureRanking,
  getSessionMetrics,
  getAudienceDevices,
  getAudienceOS,
  getAudienceBrowsers,
  getAudienceClients,
  showError
} = vi.hoisted(() => ({
  getOverview: vi.fn(),
  getUserTrend: vi.fn(),
  getUserSources: vi.fn(),
  getSourcePaymentRates: vi.fn(),
  getRetentionMatrix: vi.fn(),
  getRetentionTrend: vi.fn(),
  getPaymentFunnel: vi.fn(),
  getPaymentPlans: vi.fn(),
  getFirstPayment: vi.fn(),
  getFeatureRanking: vi.fn(),
  getSessionMetrics: vi.fn(),
  getAudienceDevices: vi.fn(),
  getAudienceOS: vi.fn(),
  getAudienceBrowsers: vi.fn(),
  getAudienceClients: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    growth: {
      getOverview,
      getUserTrend,
      getUserSources,
      getSourcePaymentRates,
      getRetentionMatrix,
      getRetentionTrend,
      getPaymentFunnel,
      getPaymentPlans,
      getFirstPayment,
      getFeatureRanking,
      getSessionMetrics,
      getAudienceDevices,
      getAudienceOS,
      getAudienceBrowsers,
      getAudienceClients
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError
  })
}))

vi.mock('vue-chartjs', () => ({
  Line: {
    name: 'Line',
    template: '<div data-test="line-chart" />'
  },
  Bar: {
    name: 'Bar',
    template: '<div data-test="bar-chart" />'
  }
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

const apiMethods = [
  getOverview,
  getUserTrend,
  getUserSources,
  getSourcePaymentRates,
  getRetentionMatrix,
  getRetentionTrend,
  getPaymentFunnel,
  getPaymentPlans,
  getFirstPayment,
  getFeatureRanking,
  getSessionMetrics,
  getAudienceDevices,
  getAudienceOS,
  getAudienceBrowsers,
  getAudienceClients
]

function mockSuccessfulResponses() {
  getOverview.mockResolvedValue({
    total_users: 1000,
    dau: 88,
    mau: 620,
    today_new_users: 12,
    today_paid_users: 4,
    month_revenue: 12800,
    arpu: 12.8,
    payment_conversion_rate: 0.18,
    repurchase_rate: 0.32
  })
  getUserTrend.mockResolvedValue({
    series: [{ date: '2026-05-30', new_registered: 12, new_activated: 9, new_paid: 4 }]
  })
  getUserSources.mockResolvedValue({
    items: [{ source: 'Direct', users: 32 }]
  })
  getSourcePaymentRates.mockResolvedValue({
    items: [{ source: 'Direct', registered_users: 32, paid_users: 8, conversion_rate: 0.25 }]
  })
  getRetentionMatrix.mockResolvedValue({
    columns: ['D1', 'D7', 'D30'],
    cohorts: [{ date: '2026-05-30', new_users: 12, retention: { D1: 0.4, D7: 0.18, D30: 0.08 } }]
  })
  getRetentionTrend.mockResolvedValue({
    series: [{ date: '2026-05-30', d1: 0.4, d7: 0.18, d30: 0.08 }]
  })
  getPaymentFunnel.mockResolvedValue({
    tracking_ready: false,
    steps: [{ key: 'payment_page', label: '进入充值页', users: 20, count: 24, conversion_rate: 1 }]
  })
  getPaymentPlans.mockResolvedValue({
    items: [{ plan_id: 1, plan_name: '月卡', category: 'subscription', sales: 8, revenue: 388 }]
  })
  getFirstPayment.mockResolvedValue({
    items: [{ bucket: 'within_1_day', label: '1天内', users: 6, ratio: 0.5 }]
  })
  getFeatureRanking.mockResolvedValue({
    items: [{ feature: 'chat', label: '聊天', uses: 120, users: 60, user_ratio: 0.6 }]
  })
  getSessionMetrics.mockResolvedValue({
    average_turns: { available: true, value: 5.8 },
    average_session_duration_seconds: { available: false, value: 0 },
    average_input_tokens: { available: true, value: 640 },
    average_output_tokens: { available: true, value: 1280 }
  })
  getAudienceDevices.mockResolvedValue({
    items: [{ key: 'desktop', label: 'Desktop', users: 12, requests: 40, user_ratio: 0.3 }]
  })
  getAudienceOS.mockResolvedValue({
    items: [{ key: 'macos', label: 'macOS', users: 8, requests: 24, user_ratio: 0.2 }]
  })
  getAudienceBrowsers.mockResolvedValue({
    items: [{ key: 'chrome', label: 'Chrome', users: 9, requests: 28, user_ratio: 0.225 }]
  })
  getAudienceClients.mockResolvedValue({
    items: [{ key: 'codex_cli', label: 'Codex CLI', users: 5, requests: 18, user_ratio: 0.125 }]
  })
}

function mountView() {
  return mount(GrowthDashboardView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        LoadingSpinner: true,
        Icon: true,
        DateRangePicker: true,
        Select: true,
        Line: true,
        Bar: true
      }
    }
  })
}

describe('admin GrowthDashboardView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-05-30T12:00:00+08:00'))
    apiMethods.forEach((method) => method.mockReset())
    showError.mockReset()
    mockSuccessfulResponses()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('loads every dashboard card and chart from an independent endpoint on mount', async () => {
    mountView()
    await flushPromises()

    const params = {
      start_date: '2026-05-01',
      end_date: '2026-05-30',
      granularity: 'day'
    }

    for (const method of apiMethods) {
      expect(method).toHaveBeenCalledTimes(1)
      expect(method).toHaveBeenCalledWith(params)
    }
  })

  it('keeps the rest of the dashboard visible when one chart endpoint fails', async () => {
    getRetentionTrend.mockRejectedValueOnce(new Error('retention trend failed'))

    const wrapper = mountView()
    await flushPromises()

    expect(getRetentionTrend).toHaveBeenCalledTimes(1)
    expect(getFeatureRanking).toHaveBeenCalledTimes(1)
    expect(getSessionMetrics).toHaveBeenCalledTimes(1)
    expect(wrapper.find('[data-test="growth-dashboard"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="growth-overview"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="growth-retention-trend-error"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('admin.growth.featureRanking')
    expect(wrapper.text()).toContain('admin.growth.audienceProfile')
    expect(showError).toHaveBeenCalledWith('admin.growth.failedToLoad')
  })
})

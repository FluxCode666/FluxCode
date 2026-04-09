import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'

import SubscriptionsView from '../SubscriptionsView.vue'

const { getMySubscriptions, getSubscriptionGrants, showError } = vi.hoisted(() => ({
  getMySubscriptions: vi.fn(),
  getSubscriptionGrants: vi.fn(),
  showError: vi.fn()
}))

const messages: Record<string, string> = {
  'userSubscriptions.noActiveSubscriptions': '暂无生效订阅',
  'userSubscriptions.noActiveSubscriptionsDesc': '当前没有可用订阅。',
  'userSubscriptions.failedToLoad': '加载订阅失败',
  'userSubscriptions.closeDetail': '关闭详情',
  'userSubscriptions.detailTitle': '订阅详情',
  'userSubscriptions.detailLoadFailed': '加载订阅详情失败',
  'userSubscriptions.expires': '到期时间',
  'userSubscriptions.noExpiration': '无到期时间',
  'userSubscriptions.daily': '每日',
  'userSubscriptions.weekly': '每周',
  'userSubscriptions.monthly': '每月',
  'userSubscriptions.unlimited': '无限制',
  'userSubscriptions.unlimitedDesc': '该订阅无用量限制',
  'userSubscriptions.daysRemaining': '剩余 {days} 天',
  'userSubscriptions.resetIn': '{time} 后重置',
  'userSubscriptions.windowNotActive': '等待首次使用',
  'userSubscriptions.status.active': '生效中',
  'userSubscriptions.status.expired': '已过期',
  'userSubscriptions.status.suspended': '已暂停',
  'userSubscriptions.status.revoked': '已撤销',
  'userSubscriptions.timeline.title': '额度时间分段',
  'userSubscriptions.timeline.now': '现在',
  'userSubscriptions.timeline.segmentCount': '{count} 个区间',
  'userSubscriptions.timeline.segmentLine': '{start} ~ {end}，额度：{quota}'
}

const translate = (key: string, params?: Record<string, unknown>) => {
  const template = messages[key] ?? key
  if (!params) return template
  return Object.entries(params).reduce(
    (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
    template
  )
}

vi.mock('@/api/subscriptions', () => ({
  __esModule: true,
  default: {
    getMySubscriptions,
    getSubscriptionGrants
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError
  })
}))

vi.mock('@/utils/format', () => ({
  formatCurrency: (amount: number | null | undefined) => `$${(amount ?? 0).toFixed(2)}`,
  formatDateTime: (date: string | Date | null | undefined) => {
    if (!date) return ''
    return new Date(date).toISOString().slice(0, 16).replace('T', ' ')
  }
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate
    })
  }
})

const TimelineStub = defineComponent({
  props: {
    title: {
      type: String,
      required: true
    },
    segments: {
      type: Array,
      default: () => []
    }
  },
  computed: {
    segmentSummary(): string {
      return (this.segments as Array<{ quotaText: string }>)
        .map((segment) => segment.quotaText)
        .join('|')
    }
  },
  template: `
    <div data-test="timeline">
      {{ title }} {{ segmentSummary }}
    </div>
  `
})

describe('user SubscriptionsView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-20T00:00:00Z'))

    getMySubscriptions.mockReset()
    getSubscriptionGrants.mockReset()
    showError.mockReset()

    getMySubscriptions.mockResolvedValue([
      {
        id: 91,
        user_id: 7,
        group_id: 3,
        status: 'active',
        quota_multiplier: 2,
        daily_usage_usd: 4,
        weekly_usage_usd: 0,
        monthly_usage_usd: 0,
        daily_window_start: null,
        weekly_window_start: null,
        monthly_window_start: null,
        created_at: '2026-04-01T00:00:00Z',
        updated_at: '2026-04-01T00:00:00Z',
        expires_at: '2026-04-30T00:00:00Z',
        group: {
          id: 3,
          name: 'Pro',
          description: 'Stacked plan',
          daily_limit_usd: 10,
          weekly_limit_usd: null,
          monthly_limit_usd: null
        }
      }
    ])

    getSubscriptionGrants.mockResolvedValue({
      subscription_id: 91,
      group_id: 3,
      group_name: 'Pro',
      grants: [
        {
          grant_id: 1,
          starts_at: '2026-04-15T00:00:00Z',
          expires_at: '2026-04-25T00:00:00Z',
          daily_usage_usd: 2,
          weekly_usage_usd: 0,
          monthly_usage_usd: 0
        },
        {
          grant_id: 2,
          starts_at: '2026-04-20T00:00:00Z',
          expires_at: '2026-04-30T00:00:00Z',
          daily_usage_usd: 2,
          weekly_usage_usd: 0,
          monthly_usage_usd: 0
        }
      ]
    })

    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      configurable: true,
      value: vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn()
      })
    })

    Object.defineProperty(HTMLElement.prototype, 'animate', {
      writable: true,
      configurable: true,
      value: vi.fn(() => ({
        finished: Promise.resolve(),
        cancel: vi.fn()
      }))
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  const mountView = () =>
    mount(SubscriptionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Icon: true,
          SubscriptionQuotaTimeline: TimelineStub
        }
      }
    })

  it('renders quota multiplier and multiplied limits in subscription cards', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).toContain('×2')
    expect(wrapper.text()).toContain('$4.00 / $20.00')
  })

  it('opens subscription detail and renders quota timeline from grants', async () => {
    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button.card').trigger('click')
    await flushPromises()
    await nextTick()

    expect(getSubscriptionGrants).toHaveBeenCalledWith(91)
    expect(wrapper.text()).toContain('订阅详情')
    expect(wrapper.text()).toContain('额度时间分段')
    expect(wrapper.findAll('[data-test="timeline"]')).toHaveLength(1)
    expect(wrapper.find('[data-test="timeline"]').text()).toContain('$20.00|$10.00')
  })
})

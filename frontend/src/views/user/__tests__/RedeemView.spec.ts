import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'

import RedeemView from '../RedeemView.vue'

const {
  redeem,
  getHistory,
  getPublicSettings,
  refreshUser,
  fetchActiveSubscriptions,
  showError,
  showSuccess,
  showWarning
} = vi.hoisted(() => ({
  redeem: vi.fn(),
  getHistory: vi.fn(),
  getPublicSettings: vi.fn(),
  refreshUser: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn()
}))

const messages: Record<string, string> = {
  'redeem.currentBalance': '当前余额',
  'redeem.concurrency': '并发数',
  'redeem.requests': '请求',
  'redeem.redeemCodeLabel': '兑换码',
  'redeem.redeemCodePlaceholder': '请输入兑换码',
  'redeem.redeemCodeHint': '兑换码区分大小写',
  'redeem.redeeming': '兑换中...',
  'redeem.redeemButton': '兑换',
  'redeem.redeemFailed': '兑换失败',
  'redeem.failedToRedeem': '兑换失败，请检查兑换码后重试。',
  'redeem.codeRedeemSuccess': '兑换成功！',
  'redeem.subscriptionRefreshFailed': '兑换成功，但订阅状态刷新失败。',
  'redeem.pleaseEnterCode': '请输入兑换码',
  'redeem.aboutCodes': '关于兑换码',
  'redeem.codeRule1': '每个兑换码只能使用一次',
  'redeem.codeRule2': '兑换码可以增加余额、并发数或试用权限',
  'redeem.codeRule3': '如有兑换问题，请联系客服',
  'redeem.codeRule4': '余额和并发数即时更新',
  'redeem.recentActivity': '最近活动',
  'redeem.historyWillAppear': '您的兑换历史将显示在这里',
  'redeem.subscriptionChoiceTitle': '订阅兑换方式',
  'redeem.subscriptionChoiceDesc': '请选择延长有效期或叠加额度。',
  'redeem.subscriptionChoiceGroup': '分组',
  'redeem.subscriptionChoiceCurrentExpires': '当前到期',
  'redeem.subscriptionChoiceValidityDays': '本次有效期（天）',
  'redeem.subscriptionChoiceCurrentMultiplier': '当前额度',
  'redeem.subscriptionChoicePreviewTitle': '兑换后预览',
  'redeem.subscriptionChoiceResultExpires': '结果到期',
  'redeem.subscriptionChoiceResultMultiplier': '结果额度',
  'redeem.subscriptionChoiceStackUntil': '叠加额度有效期至',
  'redeem.subscriptionChoiceExtend': '延长有效期',
  'redeem.subscriptionChoiceStack': '叠加额度（从现在开始）',
  'redeem.subscriptionChoiceOptionExtendDesc': '延长 {days} 天，额度倍数不变',
  'redeem.subscriptionChoiceOptionStackDesc': '从现在开始叠加额度 {days} 天，额度倍数 +1',
  'common.cancel': '取消',
  'common.confirm': '确认'
}

const translate = (key: string, params?: Record<string, unknown>) => {
  const template = messages[key] ?? key
  if (!params) {
    return template
  }
  return Object.entries(params).reduce(
    (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
    template
  )
}

vi.mock('@/api', () => ({
  redeemAPI: {
    redeem,
    getHistory
  },
  authAPI: {
    getPublicSettings
  }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: {
      balance: 12.34,
      concurrency: 2
    },
    refreshUser
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning
  })
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    fetchActiveSubscriptions
  })
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

const BaseDialogStub = defineComponent({
  props: {
    show: {
      type: Boolean,
      default: false
    },
    title: {
      type: String,
      default: ''
    },
    width: {
      type: String,
      default: 'normal'
    }
  },
  template: `
    <div v-if="show" :data-width="width">
      <h2>{{ title }}</h2>
      <slot />
      <slot name="footer" />
    </div>
  `
})

describe('user RedeemView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-04-20T00:00:00Z'))

    redeem.mockReset()
    getHistory.mockReset()
    getPublicSettings.mockReset()
    refreshUser.mockReset()
    fetchActiveSubscriptions.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()

    getHistory.mockResolvedValue([])
    getPublicSettings.mockResolvedValue({ contact_info: '' })
    refreshUser.mockResolvedValue(undefined)
    fetchActiveSubscriptions.mockResolvedValue(undefined)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('asks for stack mode and retries redeem with subscription_mode', async () => {
    redeem
      .mockRejectedValueOnce({
        reason: 'SUBSCRIPTION_REDEEM_CHOICE_REQUIRED',
        metadata: {
          group_name: 'Pro',
          current_expires_at: '2026-05-01T00:00:00Z',
          validity_days: '30',
          current_quota_multiplier: '2'
        }
      })
      .mockResolvedValueOnce({
        message: 'success',
        type: 'subscription',
        value: 30,
        group_name: 'Pro',
        validity_days: 30
      })

    const wrapper = mount(RedeemView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          BaseDialog: BaseDialogStub,
          Icon: true
        }
      }
    })

    await flushPromises()

    await wrapper.get('#code').setValue('STACK-CODE')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(redeem).toHaveBeenCalledWith('STACK-CODE')
    expect(wrapper.find('[data-test="subscription-choice-dialog"]').exists()).toBe(true)
    expect(wrapper.find('[data-width="wide"]').exists()).toBe(true)

    await wrapper.get('[data-test="subscription-mode-stack"]').trigger('click')
    await nextTick()

    const setupState = (wrapper.vm as any).$?.setupState
    expect(setupState.previewTotalExpiresAt).toContain('2026-05-20')
    expect(wrapper.text()).toContain('订阅兑换方式')

    await wrapper.get('[data-test="subscription-choice-confirm"]').trigger('click')
    await flushPromises()

    expect(redeem).toHaveBeenNthCalledWith(2, 'STACK-CODE', 'stack')
    expect(showSuccess).toHaveBeenCalledWith('兑换成功！')
  })
})

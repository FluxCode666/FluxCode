import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PaymentView from '../PaymentView.vue'

const {
  getCheckoutInfo,
  getAvailablePromotions,
  previewOrder,
  createOrder,
  showError,
  fetchActiveSubscriptions,
  refreshUser,
} = vi.hoisted(() => ({
  getCheckoutInfo: vi.fn(),
  getAvailablePromotions: vi.fn(),
  previewOrder: vi.fn(),
  createOrder: vi.fn(),
  showError: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  refreshUser: vi.fn(),
}))

const messages: Record<string, string> = {
  'payment.tabTopUp': '充值',
  'payment.tabSubscribe': '订阅',
  'payment.rechargeAccount': '充值账户',
  'payment.currentBalance': '当前余额',
  'payment.selectPromotion': '促销活动（可选）',
  'payment.promoModeDiscount': '立减',
  'payment.promoModeBonus': '充送',
  'payment.promoRemaining': '剩余 {n} 次',
  'payment.paymentAmount': '支付金额',
  'payment.createOrder': '确认支付',
  'payment.amountLabel': '充值金额',
  'payment.noPlans': '暂无可用订阅套餐',
  'payment.activeSubscription': '当前订阅',
  'payment.planCard.rate': '倍率',
  'payment.planCard.quota': '额度',
  'payment.planCard.unlimited': '无限制',
  'payment.days': '天',
  'common.cancel': '取消',
  'common.processing': '处理中',
  'common.error': '错误',
}

const translate = (key: string, params?: Record<string, unknown>) => {
  const template = messages[key] ?? key
  if (!params) return template
  return Object.entries(params).reduce(
    (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
    template,
  )
}

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
    getAvailablePromotions,
    previewOrder,
    createOrder,
  },
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { id: 7, username: 'tester', balance: 12.34 },
    refreshUser,
  }),
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder,
  }),
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: translate,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => false,
}))

const AmountInputStub = defineComponent({
  props: {
    modelValue: {
      type: Number,
      default: null,
    },
  },
  emits: ['update:modelValue'],
  template: `
    <button type="button" data-test="set-amount" @click="$emit('update:modelValue', 100)">
      set amount
    </button>
  `,
})

const SubscriptionPlanCardStub = defineComponent({
  props: {
    plan: {
      type: Object,
      required: true,
    },
  },
  emits: ['select'],
  template: `
    <button type="button" data-test="select-plan" @click="$emit('select', plan)">
      {{ plan.name }}
    </button>
  `,
})

const checkoutInfo = {
  methods: {
    alipay: {
      daily_limit: 0,
      daily_used: 0,
      daily_remaining: 0,
      single_min: 1,
      single_max: 1000,
      fee_rate: 0,
      available: true,
    },
  },
  global_min: 1,
  global_max: 1000,
  config_min_amount: 0,
  config_max_amount: 0,
  balance_disabled: false,
  balance_recharge_multiplier: 1,
  recharge_fee_rate: 0,
  help_text: '',
  help_image_url: '',
  stripe_publishable_key: '',
  plans: [
    {
      id: 88,
      group_id: 9,
      group_platform: 'openai',
      group_name: 'Pro Group',
      rate_multiplier: 1,
      daily_limit_usd: null,
      weekly_limit_usd: null,
      monthly_limit_usd: null,
      supported_model_scopes: [],
      name: 'Pro Plan',
      description: 'Subscription plan',
      price: 100,
      validity_days: 30,
      validity_unit: 'day',
      features: [],
      for_sale: true,
      sort_order: 1,
    },
  ],
}

function mountView() {
  return mount(PaymentView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        AmountInput: AmountInputStub,
        PaymentMethodSelector: { template: '<div data-test="method-selector" />' },
        SubscriptionPlanCard: SubscriptionPlanCardStub,
        PaymentStatusPanel: true,
        StripePaymentInline: true,
        Icon: true,
        Teleport: true,
        Transition: false,
      },
    },
  })
}

function findButtonByText(wrapper: ReturnType<typeof mount>, text: string) {
  const button = wrapper.findAll('button').find((item) => item.text() === text)
  if (!button) throw new Error(`button not found: ${text}`)
  return button
}

describe('PaymentView promotion remaining uses', () => {
  beforeEach(() => {
    getCheckoutInfo.mockReset()
    getAvailablePromotions.mockReset()
    previewOrder.mockReset()
    createOrder.mockReset()
    showError.mockReset()
    fetchActiveSubscriptions.mockReset()
    refreshUser.mockReset()

    getCheckoutInfo.mockResolvedValue({ data: checkoutInfo })
    previewOrder.mockResolvedValue({ data: { hit: false } })
    fetchActiveSubscriptions.mockResolvedValue([])
  })

  it('shows remaining uses for limited recharge promotions and hides unlimited promotions', async () => {
    getAvailablePromotions.mockResolvedValueOnce({
      data: [
        {
          id: 1,
          name: '充值限次活动',
          description: '',
          promotion_type: 'recharge',
          discount_mode: 'reduce_pay',
          used_count: 1,
          max_uses: 3,
        },
        {
          id: 2,
          name: '不限次活动',
          description: '',
          promotion_type: 'recharge',
          discount_mode: 'bonus_credit',
          used_count: 99,
          max_uses: 0,
        },
      ],
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="set-amount"]').trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('充值限次活动')
    expect(wrapper.text()).toContain('剩余 2 次')
    expect(wrapper.text()).toContain('不限次活动')
    expect(wrapper.text()).not.toContain('剩余 -')
    expect(wrapper.text().match(/剩余 \d+ 次/g)).toEqual(['剩余 2 次'])
  })

  it('shows remaining uses for limited subscription promotions', async () => {
    getAvailablePromotions
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValueOnce({
        data: [
          {
            id: 10,
            name: '订阅限次活动',
            description: '',
            promotion_type: 'subscription',
            discount_mode: 'rate',
            used_count: 3,
            max_uses: 5,
          },
        ],
      })

    const wrapper = mountView()
    await flushPromises()

    await findButtonByText(wrapper, '订阅').trigger('click')
    await nextTick()
    await wrapper.get('[data-test="select-plan"]').trigger('click')
    await flushPromises()

    expect(getAvailablePromotions).toHaveBeenLastCalledWith({
      order_type: 'subscription',
      plan_id: 88,
    })
    expect(wrapper.text()).toContain('订阅限次活动')
    expect(wrapper.text()).toContain('剩余 2 次')
  })
})

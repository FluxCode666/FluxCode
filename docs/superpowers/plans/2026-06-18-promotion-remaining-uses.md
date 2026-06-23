# Promotion Remaining Uses Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show each limited promotion's remaining personal usage count on the user recharge page and subscription promotion selector.

**Architecture:** Reuse the existing `/payment/promotions/available` response fields (`max_uses`, `used_count`) in `PaymentView.vue`. Add a tiny shared calculation helper in the component script and render a right-aligned badge in both promotion list templates. Cover the behavior with a focused Vue component test that mocks the API and exercises both recharge and subscription flows.

**Tech Stack:** Vue 3 `<script setup>`, TypeScript, Vue Test Utils, Vitest, existing Tailwind utility classes and vue-i18n keys.

---

## File Structure

- Create `frontend/src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts`: focused component tests for limited and unlimited promotion badges.
- Modify `frontend/src/views/user/PaymentView.vue`: add shared remaining-count helpers and render badges in both promotion selectors.
- Read-only reference `frontend/src/types/payment.ts`: confirms `AvailablePromotion` already has `used_count` and `max_uses`.
- Read-only reference `frontend/src/i18n/locales/zh.ts` and `frontend/src/i18n/locales/en.ts`: confirms `payment.promoRemaining` already exists.

### Task 1: Add Failing Component Tests

**Files:**
- Create: `frontend/src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts`
- Read: `frontend/src/views/user/PaymentView.vue`
- Read: `frontend/src/types/payment.ts`

- [x] **Step 1: Write the failing test file**

Create `frontend/src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts` with this content:

```ts
import { mount, flushPromises } from '@vue/test-utils'
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
  refreshUser
} = vi.hoisted(() => ({
  getCheckoutInfo: vi.fn(),
  getAvailablePromotions: vi.fn(),
  previewOrder: vi.fn(),
  createOrder: vi.fn(),
  showError: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  refreshUser: vi.fn()
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
  'common.error': '错误'
}

const translate = (key: string, params?: Record<string, unknown>) => {
  const template = messages[key] ?? key
  if (!params) return template
  return Object.entries(params).reduce(
    (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
    template
  )
}

vi.mock('@/api/payment', () => ({
  paymentAPI: {
    getCheckoutInfo,
    getAvailablePromotions,
    previewOrder,
    createOrder
  }
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    user: { id: 7, username: 'tester', balance: 12.34 },
    refreshUser
  })
}))

vi.mock('@/stores/payment', () => ({
  usePaymentStore: () => ({
    createOrder
  })
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [],
    fetchActiveSubscriptions
  })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError
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

vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} })
}))

vi.mock('@/utils/device', () => ({
  isMobileDevice: () => false
}))

const AmountInputStub = defineComponent({
  props: {
    modelValue: {
      type: Number,
      default: null
    }
  },
  emits: ['update:modelValue'],
  template: `
    <button type="button" data-test="set-amount" @click="$emit('update:modelValue', 100)">
      set amount
    </button>
  `
})

const SubscriptionPlanCardStub = defineComponent({
  props: {
    plan: {
      type: Object,
      required: true
    }
  },
  emits: ['select'],
  template: `
    <button type="button" data-test="select-plan" @click="$emit('select', plan)">
      {{ plan.name }}
    </button>
  `
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
      available: true
    }
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
      sort_order: 1
    }
  ]
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
        Transition: false
      }
    }
  })
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
          max_uses: 3
        },
        {
          id: 2,
          name: '不限次活动',
          description: '',
          promotion_type: 'recharge',
          discount_mode: 'bonus_credit',
          used_count: 99,
          max_uses: 0
        }
      ]
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="set-amount"]').trigger('click')
    await nextTick()

    expect(wrapper.text()).toContain('充值限次活动')
    expect(wrapper.text()).toContain('剩余 2 次')
    expect(wrapper.text()).toContain('不限次活动')
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
            max_uses: 5
          }
        ]
      })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('button:nth-of-type(2)').trigger('click')
    await nextTick()
    await wrapper.get('[data-test="select-plan"]').trigger('click')
    await flushPromises()

    expect(getAvailablePromotions).toHaveBeenLastCalledWith({
      order_type: 'subscription',
      plan_id: 88
    })
    expect(wrapper.text()).toContain('订阅限次活动')
    expect(wrapper.text()).toContain('剩余 2 次')
  })
})
```

- [x] **Step 2: Run the focused test and verify RED**

Run:

```bash
pnpm -C frontend test:run src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts
```

Expected: the test file runs, and at least the assertions for `剩余 2 次` fail because `PaymentView.vue` does not yet render the remaining-uses badge.

### Task 2: Render Remaining-Uses Badges

**Files:**
- Modify: `frontend/src/views/user/PaymentView.vue`
- Test: `frontend/src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts`

- [x] **Step 1: Add shared helper functions**

In `frontend/src/views/user/PaymentView.vue`, after the promotion state declarations:

```ts
// Promotion state
const availablePromotions = ref<AvailablePromotion[]>([])
const subAvailablePromotions = ref<AvailablePromotion[]>([])
const selectedPromotionId = ref<number | null>(null)
const subSelectedPromotionId = ref<number | null>(null)
const promoPreview = ref<PromotionPreview | null>(null)
const subPromoPreview = ref<PromotionPreview | null>(null)

function hasPromotionUseLimit(promo: AvailablePromotion): boolean {
  return promo.max_uses > 0
}

function promotionRemainingUses(promo: AvailablePromotion): number {
  if (!hasPromotionUseLimit(promo)) return 0
  return Math.max(promo.max_uses - promo.used_count, 0)
}
```

- [x] **Step 2: Render the recharge promotion badge**

In the recharge promotion item template, replace the content block after the selection circle:

```vue
<div class="flex min-w-0 flex-1 items-start justify-between gap-3">
  <div class="min-w-0 flex-1">
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ promo.name }}</span>
      <span v-if="promo.discount_mode === 'reduce_pay' || promo.discount_mode === 'rate' || promo.discount_mode === 'amount'" class="inline-flex items-center rounded-md bg-orange-100 px-1.5 py-0.5 text-[10px] font-medium text-orange-700 ring-1 ring-inset ring-orange-200 dark:bg-orange-900/30 dark:text-orange-400 dark:ring-orange-800/50">
        {{ t('payment.promoModeDiscount') }}
      </span>
      <span v-else-if="promo.discount_mode === 'bonus_credit'" class="inline-flex items-center rounded-md bg-blue-100 px-1.5 py-0.5 text-[10px] font-medium text-blue-700 ring-1 ring-inset ring-blue-200 dark:bg-blue-900/30 dark:text-blue-400 dark:ring-blue-800/50">
        {{ t('payment.promoModeBonus') }}
      </span>
    </div>
    <p v-if="promo.description" class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ promo.description }}</p>
  </div>
  <span
    v-if="hasPromotionUseLimit(promo)"
    class="mt-0.5 shrink-0 whitespace-nowrap rounded-md bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 ring-1 ring-inset ring-amber-200 dark:bg-amber-900/20 dark:text-amber-300 dark:ring-amber-800/60"
  >
    {{ t('payment.promoRemaining', { n: promotionRemainingUses(promo) }) }}
  </span>
</div>
```

- [x] **Step 3: Render the subscription promotion badge**

In the subscription promotion item template, replace the content block after the selection circle:

```vue
<div class="flex min-w-0 flex-1 items-start justify-between gap-3">
  <div class="min-w-0 flex-1">
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-sm font-semibold text-gray-800 dark:text-gray-100">{{ promo.name }}</span>
      <span v-if="promo.discount_mode === 'rate' || promo.discount_mode === 'amount'" class="inline-flex items-center rounded-md bg-orange-100 px-1.5 py-0.5 text-[10px] font-medium text-orange-700 ring-1 ring-inset ring-orange-200 dark:bg-orange-900/30 dark:text-orange-400 dark:ring-orange-800/50">
        {{ t('payment.promoModeDiscount') }}
      </span>
    </div>
    <p v-if="promo.description" class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">{{ promo.description }}</p>
  </div>
  <span
    v-if="hasPromotionUseLimit(promo)"
    class="mt-0.5 shrink-0 whitespace-nowrap rounded-md bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 ring-1 ring-inset ring-amber-200 dark:bg-amber-900/20 dark:text-amber-300 dark:ring-amber-800/60"
  >
    {{ t('payment.promoRemaining', { n: promotionRemainingUses(promo) }) }}
  </span>
</div>
```

- [x] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
pnpm -C frontend test:run src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts
```

Expected: all tests in `PaymentView.promotionRemaining.spec.ts` pass.

- [x] **Step 5: Run type checking for the changed frontend code**

Run:

```bash
pnpm -C frontend typecheck
```

Expected: command exits with code 0 and no TypeScript errors from `PaymentView.vue` or the new spec file.

- [x] **Step 6: Review the final diff**

Run:

```bash
git diff -- frontend/src/views/user/PaymentView.vue frontend/src/views/user/__tests__/PaymentView.promotionRemaining.spec.ts docs/superpowers/plans/2026-06-18-promotion-remaining-uses.md
```

Expected: diff only includes the new test file, the `PaymentView.vue` remaining-uses helper and badge markup, and this plan.

## Self-Review

- Spec coverage: Task 1 covers limited recharge, limited subscription, and unlimited hidden behavior. Task 2 implements the shared helper and both UI locations.
- Placeholder scan: no placeholder tokens or ambiguous implementation steps remain.
- Type consistency: the plan uses `AvailablePromotion.max_uses`, `AvailablePromotion.used_count`, `hasPromotionUseLimit`, and `promotionRemainingUses` consistently across tests and implementation.

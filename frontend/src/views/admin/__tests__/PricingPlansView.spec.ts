import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import PricingPlansView from '../PricingPlansView.vue'

const {
  listPlanGroups,
  createPlanGroup,
  updatePlanGroup,
  deletePlanGroup,
  createPlan,
  updatePlan,
  deletePlan,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listPlanGroups: vi.fn(),
  createPlanGroup: vi.fn(),
  updatePlanGroup: vi.fn(),
  deletePlanGroup: vi.fn(),
  createPlan: vi.fn(),
  updatePlan: vi.fn(),
  deletePlan: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    pricingPlans: {
      listPlanGroups,
      createPlanGroup,
      updatePlanGroup,
      deletePlanGroup,
      createPlan,
      updatePlan,
      deletePlan
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
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

describe('admin PricingPlansView', () => {
  beforeEach(() => {
    listPlanGroups.mockReset()
    createPlanGroup.mockReset()
    updatePlanGroup.mockReset()
    deletePlanGroup.mockReset()
    createPlan.mockReset()
    updatePlan.mockReset()
    deletePlan.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    listPlanGroups.mockResolvedValue([
      {
        id: 1,
        name: 'Starter',
        description: 'Simple plan group',
        sort_order: 1,
        status: 'active',
        plans: [
          {
            id: 10,
            group_id: 1,
            name: 'Monthly',
            description: 'For monthly usage',
            icon_url: null,
            badge_text: null,
            tagline: null,
            price_amount: 9.9,
            price_currency: 'CNY',
            price_period: 'month',
            price_text: null,
            features: ['Fast'],
            contact_methods: [],
            is_featured: false,
            sort_order: 1,
            status: 'active',
            created_at: '2026-03-24T00:00:00Z',
            updated_at: '2026-03-24T00:00:00Z'
          }
        ],
        created_at: '2026-03-24T00:00:00Z',
        updated_at: '2026-03-24T00:00:00Z'
      }
    ])
  })

  it('loads and renders pricing plan groups on mount', async () => {
    const wrapper = mount(PricingPlansView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="actions" /><slot name="table" /></div>'
          },
          DataTable: { template: '<div />' },
          BaseDialog: { template: '<div><slot /></div>' },
          ConfirmDialog: true,
          EmptyState: true,
          Select: true,
          Toggle: true
        }
      }
    })

    await flushPromises()

    expect(listPlanGroups).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Starter')
    expect(wrapper.text()).toContain('Simple plan group')
  })
})

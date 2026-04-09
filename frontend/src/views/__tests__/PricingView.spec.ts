import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import PricingView from '../PricingView.vue'

const { listPublicPlanGroups, fetchPublicSettings, copyToClipboard } = vi.hoisted(() => ({
  listPublicPlanGroups: vi.fn(),
  fetchPublicSettings: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/pricingPlans', () => ({
  pricingPlansAPI: {
    listPublicPlanGroups
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: 'FluxCode',
    siteLogo: '',
    fetchPublicSettings
  })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' }
    })
  }
})

describe('PricingView', () => {
  beforeEach(() => {
    listPublicPlanGroups.mockReset()
    fetchPublicSettings.mockReset()
    copyToClipboard.mockReset()

    listPublicPlanGroups.mockResolvedValue([
      {
        id: 1,
        name: 'Starter',
        description: 'Simple plan',
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
            tagline: 'Most popular',
            price_amount: 9.9,
            price_currency: 'CNY',
            price_period: 'month',
            price_text: null,
            features: ['Fast'],
            contact_methods: [{ type: 'telegram', value: '@flux' }],
            is_featured: true,
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

  it('loads and renders pricing groups from api', async () => {
    const wrapper = mount(PricingView, {
      global: {
        stubs: {
          PublicHeader: true,
          LoadingSpinner: true
        }
      }
    })

    await flushPromises()

    expect(fetchPublicSettings).toHaveBeenCalledTimes(1)
    expect(listPublicPlanGroups).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('Starter')
    expect(wrapper.text()).toContain('Monthly')
    expect(wrapper.text()).toContain('¥9.90/mo')
  })
})

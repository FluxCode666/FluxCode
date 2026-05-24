import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UserEditModal from '../UserEditModal.vue'

const mocks = vi.hoisted(() => ({
  update: vi.fn().mockResolvedValue({}),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { update: mocks.update },
    userAttributes: { updateUserAttributeValues: vi.fn() }
  }
}))

vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('UserEditModal sales fields', () => {
  it('submits tiered sales commission config', async () => {
    const wrapper = mount(UserEditModal, {
      props: {
        show: true,
        user: {
          id: 1,
          email: 'sales@example.com',
          username: '',
          role: 'user',
          balance: 0,
          concurrency: 5,
          status: 'active',
          allowed_groups: [],
          balance_notify_enabled: true,
          balance_notify_threshold: null,
          balance_notify_extra_emails: [],
          created_at: '2026-05-05T00:00:00Z',
          updated_at: '2026-05-05T00:00:00Z',
          notes: '',
          is_sales: false,
          sales_commission_rate: 0,
          sales_commission_mode: 'fixed',
          sales_commission_min_monthly_sales: 0,
          sales_commission_tiers: []
        }
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          UserAttributeForm: true,
          Icon: true
        }
      }
    })

    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('[data-testid="sales-mode-tiered"]').trigger('click')
    await wrapper.get('input[name="sales_commission_min_monthly_sales"]').setValue('100')
    await wrapper.get('input[name="sales_commission_tier_from_0"]').setValue('0')
    await wrapper.get('input[name="sales_commission_tier_to_0"]').setValue('200')
    await wrapper.get('input[name="sales_commission_tier_rate_0"]').setValue('10')
    await wrapper.get('[data-testid="sales-add-tier"]').trigger('click')
    await wrapper.get('input[name="sales_commission_tier_from_1"]').setValue('200')
    await wrapper.get('input[name="sales_commission_tier_rate_1"]').setValue('20')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mocks.update).toHaveBeenCalledWith(1, expect.objectContaining({
      is_sales: true,
      sales_commission_rate: 0,
      sales_commission_mode: 'tiered',
      sales_commission_min_monthly_sales: 100,
      sales_commission_tiers: [
        {
          month_sales_from_cny: 0,
          month_sales_to_cny: 200,
          commission_rate: 10,
          sort_order: 1
        },
        {
          month_sales_from_cny: 200,
          month_sales_to_cny: null,
          commission_rate: 20,
          sort_order: 2
        }
      ]
    }))

    // spec §8.4 提示：当 isSales 启用后应该展示 "配置仅影响下月" 横幅。
    expect(wrapper.find('[data-testid="sales-monthly-snapshot-hint"]').exists()).toBe(true)
  })
})

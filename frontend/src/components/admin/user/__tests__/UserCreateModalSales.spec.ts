import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import UserCreateModal from '../UserCreateModal.vue'

const mocks = vi.hoisted(() => ({
  create: vi.fn().mockResolvedValue({}),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { create: mocks.create }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: mocks.showError,
    showSuccess: mocks.showSuccess
  })
}))

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('UserCreateModal sales fields', () => {
  it('submits fixed sales commission config', async () => {
    const wrapper = mount(UserCreateModal, {
      props: {
        show: true
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true
        }
      }
    })

    await wrapper.get('input[type="email"]').setValue('sales@example.com')
    await wrapper.get('input[type="text"]').setValue('StrongPass123!')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('input[name="sales_commission_rate"]').setValue('12.5')
    await wrapper.find('form').trigger('submit.prevent')
    await flushPromises()

    expect(mocks.create).toHaveBeenCalledWith(expect.objectContaining({
      email: 'sales@example.com',
      password: 'StrongPass123!',
      is_sales: true,
      sales_commission_rate: 12.5,
      sales_commission_mode: 'fixed',
      sales_commission_min_monthly_sales: 0,
      sales_commission_tiers: []
    }))
  })
})

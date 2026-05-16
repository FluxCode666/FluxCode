import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
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
  it('submits sales flag and commission rate', async () => {
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
          is_sales: true,
          sales_commission_rate: 10
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

    await wrapper.find('form').trigger('submit.prevent')

    expect(mocks.update).toHaveBeenCalledWith(1, expect.objectContaining({
      is_sales: true,
      sales_commission_rate: 10
    }))
  })
})

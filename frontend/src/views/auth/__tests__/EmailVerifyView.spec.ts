import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import EmailVerifyView from '../EmailVerifyView.vue'

const { push, register, getPublicSettings, sendVerifyCode, showError, showSuccess } = vi.hoisted(
  () => ({
    push: vi.fn(),
    register: vi.fn(),
    getPublicSettings: vi.fn(),
    sendVerifyCode: vi.fn(),
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
)

vi.mock('vue-router', () => ({
  useRouter: () => ({
    push
  })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      locale: 'zh',
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key
    })
  }
})

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    register
  }),
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings,
  sendVerifyCode
}))

describe('EmailVerifyView', () => {
  beforeEach(() => {
    sessionStorage.clear()
    register.mockReset()
    push.mockReset()
    getPublicSettings.mockReset()
    sendVerifyCode.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    getPublicSettings.mockResolvedValue({
      turnstile_enabled: false,
      turnstile_site_key: '',
      site_name: 'FluxCode',
      registration_email_suffix_whitelist: []
    })
    sendVerifyCode.mockResolvedValue({ message: 'ok', countdown: 60 })
    register.mockResolvedValue({ id: 1, email: 'new@example.com' })
  })

  afterEach(() => {
    sessionStorage.clear()
  })

  it('邮箱验证注册时保留推广码', async () => {
    sessionStorage.setItem(
      'register_data',
      JSON.stringify({
        email: 'new@example.com',
        password: 'secret123',
        referral_code: 'REF-2026',
        promo_code: 'PROMO',
        invitation_code: 'INVITE'
      })
    )

    const wrapper = mount(EmailVerifyView, {
      global: {
        stubs: {
          AuthLayout: { template: '<main><slot /><slot name="footer" /></main>' },
          Icon: true,
          TurnstileWidget: true
        }
      }
    })

    await flushPromises()

    await wrapper.get('#code').setValue('123456')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(register).toHaveBeenCalledWith(
      expect.objectContaining({
        email: 'new@example.com',
        password: 'secret123',
        verify_code: '123456',
        promo_code: 'PROMO',
        invitation_code: 'INVITE',
        referral_code: 'REF-2026'
      })
    )
  })
})

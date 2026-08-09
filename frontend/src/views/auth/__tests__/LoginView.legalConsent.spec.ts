import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { createMemoryHistory, createRouter } from 'vue-router'
import LoginView from '@/views/auth/LoginView.vue'

const mockLogin = vi.fn()
const mockAcceptLegalTerms = vi.fn()

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'zh-CN' }
    })
  }
})

vi.mock('@/api', () => ({
  authAPI: {
    login: (...args: unknown[]) => mockLogin(...args),
    login2FA: vi.fn(),
    logout: vi.fn(),
    getCurrentUser: vi.fn(),
    register: vi.fn(),
    refreshToken: vi.fn()
  },
  userAPI: {
    acceptLegalTerms: (...args: unknown[]) => mockAcceptLegalTerms(...args)
  },
  isTotp2FARequired: (response: { requires_2fa?: boolean }) => response.requires_2fa === true
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn().mockResolvedValue({
    turnstile_enabled: false,
    linuxdo_oauth_enabled: false,
    oidc_oauth_enabled: false,
    backend_mode_enabled: false,
    password_reset_enabled: false
  }),
  isTotp2FARequired: (response: { requires_2fa?: boolean }) => response.requires_2fa === true
}))

describe('LoginView legal consent', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  it('keeps login disabled until the legal consent checkbox is checked', async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: '/login', component: LoginView },
        { path: '/register', component: { template: '<div />' } },
        { path: '/dashboard', component: { template: '<div />' } },
        { path: '/legal/:document', component: { template: '<div />' } }
      ]
    })
    await router.push('/login')
    await router.isReady()

    const wrapper = mount(LoginView, {
      global: {
        plugins: [router],
        stubs: {
          AuthLayout: { template: '<div><slot /><slot name="footer" /></div>' },
          Icon: true,
          TurnstileWidget: true,
          TotpLoginModal: true
        }
      }
    })

    const submit = wrapper.get('button[type="submit"]')
    expect(submit.attributes('disabled')).toBeDefined()

    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(submit.attributes('disabled')).toBeUndefined()

    mockLogin.mockResolvedValue({
      access_token: 'access-token',
      refresh_token: 'refresh-token',
      expires_in: 3600,
      token_type: 'Bearer',
      user: {
        id: 1,
        email: 'user@example.com',
        username: 'user',
        role: 'user',
        status: 'active',
        legal_terms_accepted: false
      }
    })
    mockAcceptLegalTerms.mockResolvedValue({
      id: 1,
      email: 'user@example.com',
      username: 'user',
      role: 'user',
      status: 'active',
      legal_terms_accepted: true,
      legal_terms_version: '1.1'
    })

    await wrapper.get('#email').setValue('user@example.com')
    await wrapper.get('#password').setValue('password123')
    await wrapper.get('form').trigger('submit')
    await flushPromises()

    expect(mockLogin).toHaveBeenCalledWith({
      email: 'user@example.com',
      password: 'password123',
      turnstile_token: undefined
    })
    expect(mockAcceptLegalTerms).toHaveBeenCalledOnce()
  })
})

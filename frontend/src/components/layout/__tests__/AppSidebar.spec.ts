import { beforeAll, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AppSidebar from '../AppSidebar.vue'

const fetchAdminSettings = vi.fn()

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    mobileOpen: false,
    backendModeEnabled: false,
    siteName: 'FluxCode',
    siteLogo: '',
    siteVersion: 'test',
    publicSettingsLoaded: true,
    cachedPublicSettings: {
      custom_menu_items: [],
      sora_client_enabled: false,
      purchase_subscription_enabled: false
    },
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn()
  }),
  useAuthStore: () => ({
    isAdmin: true,
    isSimpleMode: false
  }),
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn().mockReturnValue(false),
    nextStep: vi.fn()
  }),
  useAdminSettingsStore: () => ({
    opsMonitoringEnabled: false,
    customMenuItems: [],
    fetch: fetchAdminSettings
  })
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => ({
      path: '/admin/dashboard'
    })
  }
})

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('AppSidebar admin navigation', () => {
  beforeAll(() => {
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation(() => ({
        matches: false,
        media: '(prefers-color-scheme: dark)',
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn()
      }))
    })
  })

  it('shows pricing plans and pool monitor entries for admins', () => {
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          VersionBadge: true,
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })

    const paths = wrapper.findAll('a').map((node) => node.attributes('data-to'))

    expect(paths).toContain('/admin/pricing-plans')
    expect(paths).toContain('/admin/pool-monitor')
  })
})

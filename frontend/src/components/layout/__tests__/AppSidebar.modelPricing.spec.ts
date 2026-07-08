import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import AppSidebar from '../AppSidebar.vue'

const { authStoreState } = vi.hoisted(() => ({
  authStoreState: {
    isAdmin: false,
    isSimpleMode: false,
    user: { role: 'user', is_sales: false }
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    mobileOpen: true,
    backendModeEnabled: false,
    siteName: 'FluxCode',
    siteLogo: '',
    siteVersion: '1.0.0',
    publicSettingsLoaded: true,
    cachedPublicSettings: {
      custom_menu_items: [],
      payment_enabled: false,
      referral_enabled: false,
      referral_sales_enabled: false
    },
    channelMonitorEnabled: false,
    toggleSidebar: vi.fn(),
    closeMobileSidebar: vi.fn()
  }),
  useAuthStore: () => authStoreState,
  useOnboardingStore: () => ({
    currentTour: null,
    isCurrentStep: vi.fn().mockReturnValue(false),
    nextStep: vi.fn()
  }),
  useAdminSettingsStore: () => ({
    customMenuItems: [],
    channelMonitorEnabled: false,
    opsMonitoringEnabled: false,
    paymentEnabled: false,
    fetch: vi.fn()
  })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ path: '/dashboard' })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) =>
      (
        {
          'nav.dashboard': '仪表盘',
          'nav.apiKeys': 'API 密钥',
          'nav.usage': '使用记录',
          'nav.mySubscriptions': '我的订阅',
          'nav.redeem': '兑换',
          'nav.profile': '个人资料',
          'nav.modelPricing': '模型定价',
          'nav.darkMode': '深色模式',
          'nav.collapse': '收起'
        } as Record<string, string>
      )[key] || key
  })
}))

vi.mock('@/components/common/VersionBadge.vue', () => ({
  default: {
    props: ['version'],
    template: '<div data-testid="version-badge">{{ version }}</div>'
  }
}))

describe('AppSidebar model pricing', () => {
  beforeEach(() => {
    authStoreState.isAdmin = false
    authStoreState.isSimpleMode = false
    authStoreState.user = { role: 'user', is_sales: false }
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false }))
    localStorage.clear()
  })

  it('shows model pricing and hides pricing plans', () => {
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          transition: false,
          'router-link': {
            props: ['to'],
            template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('模型定价')
    expect(wrapper.html()).toContain('href="/model-pricing"')
    expect(wrapper.html()).not.toContain('/admin/pricing-plans')
  })

  it('shows model pricing in admin personal section and hides pricing plans', () => {
    authStoreState.isAdmin = true
    authStoreState.user = { role: 'admin', is_sales: false }

    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          transition: false,
          'router-link': {
            props: ['to'],
            template: '<a :href="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })

    expect(wrapper.text()).toContain('模型定价')
    expect(wrapper.html()).toContain('href="/model-pricing"')
    expect(wrapper.html()).not.toContain('/admin/pricing-plans')
  })
})

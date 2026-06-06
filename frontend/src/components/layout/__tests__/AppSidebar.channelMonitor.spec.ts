import { beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AppSidebar from '../AppSidebar.vue'

const { routeState, authStoreState, appStoreState, adminSettingsStoreState } = vi.hoisted(() => ({
  routeState: {
    path: '/admin/dashboard'
  },
  authStoreState: {
    isAdmin: true,
    isSimpleMode: false,
    user: null
  },
  appStoreState: {
    sidebarCollapsed: false,
    mobileOpen: false,
    publicSettingsLoaded: true,
    siteName: 'FluxCode',
    siteLogo: '',
    siteVersion: 'test',
    cachedPublicSettings: {
      channel_monitor_enabled: true,
      payment_enabled: false,
      custom_menu_items: []
    },
    channelMonitorEnabled: true,
    backendModeEnabled: false,
    toggleSidebar: vi.fn(),
    setMobileOpen: vi.fn()
  },
  adminSettingsStoreState: {
    channelMonitorEnabled: false,
    opsMonitoringEnabled: true,
    paymentEnabled: false,
    customMenuItems: [],
    fetch: vi.fn()
  }
}))

vi.mock('vue-router', () => ({
  useRoute: () => routeState
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => {
      const map: Record<string, string> = {
        'nav.dashboard': '仪表盘',
        'nav.channelStatus': '渠道状态',
        'nav.channels': '渠道管理',
        'nav.channelManagement': '渠道管理',
        'nav.channelMonitor': '渠道监控',
        'nav.ops': '运行监控',
        'nav.users': '用户管理',
        'nav.groups': '分组管理',
        'nav.pricingPlans': '定价方案',
        'nav.subscriptions': '订阅管理',
        'nav.accounts': '账号管理',
        'nav.announcements': '公告管理',
        'nav.proxies': '代理管理',
        'nav.redeemCodes': '兑换码',
        'nav.promoCodes': '优惠码',
        'nav.referralManagement': '邀请管理',
        'nav.salesCommissions': '销售佣金',
        'nav.promotions': '推广管理',
        'nav.usage': '用量统计',
        'nav.poolMonitor': '池监控',
        'nav.settings': '系统设置',
        'nav.myAccount': '我的账号',
        'nav.apiKeys': 'API 密钥',
        'nav.profile': '个人资料',
        'nav.lightMode': '浅色模式',
        'nav.darkMode': '深色模式',
        'nav.collapse': '收起',
        'nav.expand': '展开'
      }
      return map[key] || fallback || key
    }
  })
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStoreState,
  useAuthStore: () => authStoreState,
  useOnboardingStore: () => ({
    isCurrentStep: vi.fn(() => false),
    nextStep: vi.fn()
  }),
  useAdminSettingsStore: () => adminSettingsStoreState
}))

vi.mock('@/components/common/VersionBadge.vue', () => ({
  default: {
    props: ['version'],
    template: '<span data-testid="version-badge">{{ version }}</span>'
  }
}))

describe('AppSidebar channel monitor menu', () => {
  beforeEach(() => {
    routeState.path = '/admin/dashboard'
    authStoreState.isAdmin = true
    authStoreState.isSimpleMode = false
    appStoreState.sidebarCollapsed = false
    appStoreState.mobileOpen = false
    appStoreState.channelMonitorEnabled = true
    appStoreState.cachedPublicSettings.channel_monitor_enabled = true
    adminSettingsStoreState.channelMonitorEnabled = false
    vi.clearAllMocks()
    localStorage.clear()
    document.documentElement.className = ''
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false }))
  })

  it('shows the admin channel monitor entry when public settings already enable it', async () => {
    const wrapper = mount(AppSidebar, {
      global: {
        stubs: {
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })

    await wrapper.vm.$nextTick()
    const channelGroupButton = wrapper.findAll('button.sidebar-link')
      .find((button) => button.text().includes('渠道管理'))
    expect(channelGroupButton).toBeTruthy()

    await channelGroupButton!.trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('a[data-to="/admin/channels/monitor"]').exists()).toBe(true)
  })
})

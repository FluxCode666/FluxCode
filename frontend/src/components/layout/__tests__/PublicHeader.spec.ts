import { ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import PublicHeader from '../PublicHeader.vue'

const { authStoreState, appStoreState } = vi.hoisted(() => ({
  authStoreState: {
    isAuthenticated: false,
    isAdmin: false,
    user: { email: 'user@example.com' },
    checkAuth: vi.fn()
  },
  appStoreState: {
    channelMonitorEnabled: true
  }
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => authStoreState,
  useAppStore: () => appStoreState
}))

vi.mock('@/components/common/LocaleSwitcher.vue', () => ({
  default: {
    template: '<div data-testid="locale-switcher" />'
  }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => {
      const map: Record<string, string> = {
        'home.nav.pricing': '购买入口',
        'home.nav.integrationDocs': '接入文档',
        'home.nav.docs': '使用文档',
        'home.nav.channelStatus': '渠道状态',
        'home.nav.menu': '菜单',
        'home.dashboard': '控制台',
        'home.login': '登录',
        'home.switchToLight': '切换到浅色模式',
        'home.switchToDark': '切换到深色模式'
      }
      return map[key] || key
    }
  })
}))

vi.mock('@vueuse/core', () => ({
  useWindowScroll: () => ({
    y: ref(0)
  })
}))

describe('PublicHeader', () => {
  beforeEach(() => {
    authStoreState.checkAuth.mockClear()
    appStoreState.channelMonitorEnabled = true
    localStorage.clear()
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: false }))
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows channel status in the public nav when enabled', async () => {
    const wrapper = mount(PublicHeader, {
      props: {
        siteName: 'FluxCode',
        siteLogo: ''
      },
      global: {
        stubs: {
          LocaleSwitcher: true,
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })

    await wrapper.vm.$nextTick()

    expect(wrapper.find('a[data-to="/channel-status"]').exists()).toBe(true)
    expect(wrapper.find('a[data-to="/integration-docs"]').exists()).toBe(true)

    await wrapper.get('button[title="菜单"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.findAll('a[data-to="/channel-status"]').length).toBe(2)
    expect(wrapper.findAll('a[data-to="/integration-docs"]').length).toBe(2)
    expect(authStoreState.checkAuth).toHaveBeenCalledTimes(1)
  })

  it('hides channel status when the feature is disabled', async () => {
    appStoreState.channelMonitorEnabled = false

    const wrapper = mount(PublicHeader, {
      props: {
        siteName: 'FluxCode',
        siteLogo: ''
      },
      global: {
        stubs: {
          LocaleSwitcher: true,
          RouterLink: {
            props: ['to'],
            template: '<a :data-to="typeof to === \'string\' ? to : to.path"><slot /></a>'
          }
        }
      }
    })

    await wrapper.vm.$nextTick()

    expect(wrapper.find('a[data-to="/channel-status"]').exists()).toBe(false)
    expect(wrapper.find('a[data-to="/integration-docs"]').exists()).toBe(true)

    await wrapper.get('button[title="菜单"]').trigger('click')
    await wrapper.vm.$nextTick()

    expect(wrapper.find('a[data-to="/channel-status"]').exists()).toBe(false)
    expect(wrapper.findAll('a[data-to="/integration-docs"]').length).toBe(2)
  })
})

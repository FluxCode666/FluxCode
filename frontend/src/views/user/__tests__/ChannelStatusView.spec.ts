import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelStatusView from '../ChannelStatusView.vue'

const { listChannelMonitors, appStoreState } = vi.hoisted(() => ({
  listChannelMonitors: vi.fn().mockResolvedValue({ items: [] }),
  appStoreState: {
    cachedPublicSettings: { channel_monitor_enabled: true } as { channel_monitor_enabled?: boolean },
    showError: vi.fn(),
    showSuccess: vi.fn()
  }
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      setLocaleMessage: vi.fn()
    },
    install: vi.fn()
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'channelStatus.title') return '渠道状态'
      return params ? `${key}:${JSON.stringify(params)}` : key
    }
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStoreState
}))

vi.mock('@/api/channelMonitor', () => ({
  default: {
    list: listChannelMonitors,
    status: vi.fn()
  },
  list: listChannelMonitors,
  status: vi.fn()
}))

describe('ChannelStatusView', () => {
  it('renders empty state when no monitors exist', async () => {
    appStoreState.cachedPublicSettings = { channel_monitor_enabled: true }
    listChannelMonitors.mockClear()

    const wrapper = mount(ChannelStatusView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          MonitorHero: true,
          MonitorCardGrid: true,
          MonitorDetailDialog: true
        }
      }
    })

    await vi.dynamicImportSettled()

    expect(wrapper.text()).toContain('渠道状态')
    expect(listChannelMonitors).toHaveBeenCalled()
  })

  it('does not request monitor data when channel monitor is disabled', async () => {
    appStoreState.cachedPublicSettings = { channel_monitor_enabled: false }
    listChannelMonitors.mockClear()

    mount(ChannelStatusView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          MonitorHero: true,
          MonitorCardGrid: true,
          MonitorDetailDialog: true
        }
      }
    })

    await vi.dynamicImportSettled()

    expect(listChannelMonitors).not.toHaveBeenCalled()
  })
})

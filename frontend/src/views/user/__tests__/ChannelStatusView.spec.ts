import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelStatusView from '../ChannelStatusView.vue'

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
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_enabled: true },
    showError: vi.fn(),
    showSuccess: vi.fn()
  })
}))

vi.mock('@/api/channelMonitor', () => ({
  default: {
    list: vi.fn().mockResolvedValue({ items: [] }),
    status: vi.fn()
  },
  list: vi.fn().mockResolvedValue({ items: [] }),
  status: vi.fn()
}))

describe('ChannelStatusView', () => {
  it('renders empty state when no monitors exist', async () => {
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
  })
})

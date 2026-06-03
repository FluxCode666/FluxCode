import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ChannelMonitorView from '../ChannelMonitorView.vue'

const { listMonitors } = vi.hoisted(() => ({
  listMonitors: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
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
      if (key === 'admin.channelMonitor.title') return '渠道监控'
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

vi.mock('@/api/admin/channelMonitor', () => ({
  default: {
    list: listMonitors,
    runNow: vi.fn()
  }
}))

vi.mock('@/api/admin/channelMonitorTemplate', () => ({
  default: {
    list: vi.fn().mockResolvedValue({ items: [] })
  }
}))

describe('ChannelMonitorView', () => {
  it('renders monitor management shell', async () => {
    const wrapper = mount(ChannelMonitorView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' },
          DataTable: { template: '<div><slot name="empty" /></div>' },
          MonitorFiltersBar: true,
          MonitorFormDialog: true,
          MonitorTemplateManagerDialog: true,
          MonitorRunResultDialog: true,
          ConfirmDialog: true,
          Pagination: true
        }
      }
    })

    await vi.dynamicImportSettled()

    expect(wrapper.text()).toContain('渠道监控')
    expect(listMonitors).toHaveBeenCalled()
  })
})

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useAdminSettingsStore } from '@/stores/adminSettings'

const { mockGetSettings, mockGetPaymentConfig } = vi.hoisted(() => ({
  mockGetSettings: vi.fn(),
  mockGetPaymentConfig: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    settings: {
      getSettings: mockGetSettings
    },
    payment: {
      getConfig: mockGetPaymentConfig
    }
  }
}))

describe('useAdminSettingsStore channel monitor cache', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('does not trust stale cached channel monitor enabled state before fetch', () => {
    localStorage.setItem('channel_monitor_enabled_cached', 'true')

    const store = useAdminSettingsStore()

    expect(store.channelMonitorEnabled).toBe(false)
  })

  it('updates channel monitor state from admin settings fetch', async () => {
    mockGetSettings.mockResolvedValue({
      channel_monitor_enabled: true,
      channel_monitor_default_interval_seconds: 120,
      custom_menu_items: []
    })
    mockGetPaymentConfig.mockResolvedValue({ data: { enabled: false } })

    const store = useAdminSettingsStore()
    await store.fetch()

    expect(store.channelMonitorEnabled).toBe(true)
    expect(store.channelMonitorDefaultIntervalSeconds).toBe(120)
    expect(localStorage.getItem('channel_monitor_enabled_cached')).toBe('true')
    expect(localStorage.getItem('channel_monitor_default_interval_seconds_cached')).toBe('120')
  })
})

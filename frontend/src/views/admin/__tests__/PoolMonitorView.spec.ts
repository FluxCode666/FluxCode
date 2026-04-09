import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import PoolMonitorView from '../PoolMonitorView.vue'

const {
  getPoolMonitorConfig,
  updatePoolMonitorConfig,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  getPoolMonitorConfig: vi.fn(),
  updatePoolMonitorConfig: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    poolMonitor: {
      getPoolMonitorConfig,
      updatePoolMonitorConfig
    }
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const ToggleStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      required: true
    }
  },
  emits: ['update:modelValue'],
  template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />'
})

describe('admin PoolMonitorView', () => {
  beforeEach(() => {
    getPoolMonitorConfig.mockReset()
    updatePoolMonitorConfig.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    getPoolMonitorConfig.mockResolvedValue({
      platform: 'openai',
      pool_threshold_enabled: true,
      proxy_failure_enabled: true,
      proxy_active_probe_enabled: true,
      disabled_proxy_schedule_mode: 'direct_without_proxy',
      available_count_threshold: 2,
      available_ratio_threshold: 20,
      check_interval_minutes: 5,
      proxy_probe_interval_minutes: 5,
      proxy_failure_window_minutes: 10,
      proxy_failure_threshold: 5,
      alert_emails: [],
      alert_cooldown_minutes: 5
    })
    updatePoolMonitorConfig.mockResolvedValue({
      platform: 'openai',
      pool_threshold_enabled: true,
      proxy_failure_enabled: true,
      proxy_active_probe_enabled: true,
      disabled_proxy_schedule_mode: 'direct_without_proxy',
      available_count_threshold: 2,
      available_ratio_threshold: 20,
      check_interval_minutes: 5,
      proxy_probe_interval_minutes: 5,
      proxy_failure_window_minutes: 10,
      proxy_failure_threshold: 5,
      alert_emails: ['ops@example.com'],
      alert_cooldown_minutes: 9
    })
  })

  it('loads config on mount and saves alert section updates', async () => {
    const wrapper = mount(PoolMonitorView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' },
          Toggle: ToggleStub
        }
      }
    })

    await flushPromises()

    expect(getPoolMonitorConfig).toHaveBeenCalledWith('openai')

    await wrapper.get('input[type="number"]').setValue('9')
    await wrapper.get('button.btn-secondary.btn-sm').trigger('click')

    const emailInput = wrapper.get('input[type="email"]')
    await emailInput.setValue('ops@example.com')

    const saveButtons = wrapper.findAll('button.btn-primary')
    await saveButtons[0].trigger('click')
    await flushPromises()

    expect(updatePoolMonitorConfig).toHaveBeenCalledWith('openai', {
      alert_cooldown_minutes: 9,
      alert_emails: ['ops@example.com']
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.poolMonitor.saved')
  })
})

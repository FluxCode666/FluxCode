import { describe, expect, it, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import MonitorFormDialog from '../MonitorFormDialog.vue'
import type { ChannelMonitor } from '@/api/admin/channelMonitor'

const { createMonitor, updateMonitor, listTemplates } = vi.hoisted(() => ({
  createMonitor: vi.fn(),
  updateMonitor: vi.fn(),
  listTemplates: vi.fn().mockResolvedValue({ items: [] }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_default_interval_seconds: 60 },
    showError: vi.fn(),
    showSuccess: vi.fn(),
  }),
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    channelMonitorDefaultIntervalSeconds: 60,
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    channelMonitor: {
      create: createMonitor,
      update: updateMonitor,
    },
    channelMonitorTemplate: {
      list: listTemplates,
    },
  },
}))

vi.mock('@/api/keys', () => ({
  keysAPI: {
    list: vi.fn(),
  },
}))

vi.mock('@/api/groups', () => ({
  userGroupsAPI: {
    getUserGroupRates: vi.fn(),
  },
}))

vi.mock('@/composables/useChannelMonitorFormat', () => ({
  useChannelMonitorFormat: () => ({
    providerPickerClass: () => 'provider-picker',
  }),
}))

const baseMonitor: ChannelMonitor = {
  id: 7,
  name: 'OpenAI monitor',
  provider: 'openai',
  api_mode: 'chat_completions',
  endpoint: 'https://api.example.com',
  api_key_masked: 'sk-***',
  primary_model: 'gpt-4o-mini',
  extra_models: [],
  group_name: '',
  enabled: true,
  interval_seconds: 60,
  jitter_seconds: 12,
  last_checked_at: null,
  created_by: 1,
  created_at: '2026-07-06T00:00:00Z',
  updated_at: '2026-07-06T00:00:00Z',
  primary_status: '',
  primary_latency_ms: null,
  availability_7d: 0,
  extra_models_status: [],
  template_id: null,
  extra_headers: {},
  body_override_mode: 'off',
  body_override: null,
}

function mountDialog(props: { show?: boolean; monitor?: ChannelMonitor | null } = {}) {
  return mount(MonitorFormDialog, {
    props: {
      show: true,
      monitor: null,
      ...props,
    },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
        Toggle: { template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', true)" />', props: ['modelValue'] },
        Select: true,
        ModelTagInput: true,
        MonitorKeyPickerDialog: true,
        MonitorAdvancedRequestConfig: true,
        ProviderIcon: true,
      },
    },
  })
}

describe('MonitorFormDialog jitter', () => {
  beforeEach(() => {
    createMonitor.mockReset()
    updateMonitor.mockReset()
    listTemplates.mockClear()
    createMonitor.mockResolvedValue({})
    updateMonitor.mockResolvedValue({})
  })

  it('defaults jitter_seconds to zero for new monitors', () => {
    const wrapper = mountDialog()

    const input = wrapper.get<HTMLInputElement>('[data-testid="monitor-jitter-input"]')

    expect(input.element.value).toBe('0')
  })

  it('loads existing monitor jitter_seconds when editing', async () => {
    const wrapper = mountDialog({ monitor: baseMonitor })

    await vi.dynamicImportSettled()

    const input = wrapper.get<HTMLInputElement>('[data-testid="monitor-jitter-input"]')
    expect(input.element.value).toBe('12')
  })

  it('submits jitter_seconds in create payload', async () => {
    const wrapper = mountDialog()

    await wrapper.get('input[placeholder="admin.channelMonitor.form.namePlaceholder"]').setValue('Monitor')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.endpointPlaceholder"]').setValue('https://api.example.com')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.apiKeyPlaceholder"]').setValue('sk-test')
    await wrapper.get('input[placeholder="admin.channelMonitor.form.primaryModelPlaceholder"]').setValue('claude-sonnet-4')
    await wrapper.get('[data-testid="monitor-jitter-input"]').setValue(9)
    await wrapper.get('form').trigger('submit.prevent')

    expect(createMonitor).toHaveBeenCalledWith(expect.objectContaining({
      jitter_seconds: 9,
    }))
  })

  it('clamps jitter max when interval is reduced', async () => {
    const wrapper = mountDialog()

    await wrapper.get('[data-testid="monitor-jitter-input"]').setValue(50)
    await wrapper.get('[data-testid="monitor-interval-input"]').setValue(30)

    const input = wrapper.get<HTMLInputElement>('[data-testid="monitor-jitter-input"]')
    expect(input.element.value).toBe('15')
  })
})

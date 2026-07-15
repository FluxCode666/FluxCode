import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: true
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      update: updateAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({ account_quota_notify_enabled: false })
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
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

import EditAccountModal from '../EditAccountModal.vue'
import MediaConfigEditor from '../MediaConfigEditor.vue'
import type { Account, MediaAccountConfigWire } from '@/types'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: {
    show: {
      type: Boolean,
      default: false
    }
  },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
})

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: `
    <div>
      <button
        type="button"
        data-testid="rewrite-to-snapshot"
        @click="$emit('update:modelValue', ['gpt-5.2-2025-12-11'])"
      >
        rewrite
      </button>
      <span data-testid="model-whitelist-value">
        {{ Array.isArray(modelValue) ? modelValue.join(',') : '' }}
      </span>
    </div>
  `
})

const SelectStub = defineComponent({
  name: 'SelectStub',
  inheritAttrs: false,
  props: {
    modelValue: {
      type: [String, Number, Boolean],
      default: ''
    },
    options: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  methods: {
    handleChange(event: Event) {
      this.$emit('update:modelValue', (event.target as HTMLSelectElement).value)
    }
  },
  template: `
    <select v-bind="$attrs" :value="modelValue" @change="handleChange">
      <option
        v-for="option in options"
        :key="String(option.value)"
        :value="option.value"
      >
        {{ option.label }}
      </option>
    </select>
  `
})

function buildAccount(overrides: Partial<Account> = {}): Account {
  const base: Account = {
    id: 1,
    name: 'OpenAI Key',
    notes: '',
    platform: 'openai',
    type: 'apikey',
    credentials: {
      api_key: 'sk-test',
      base_url: 'https://api.openai.com',
      model_mapping: {
        'gpt-5.2': 'gpt-5.2'
      }
    },
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 1,
    rate_multiplier: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    group_ids: [],
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null
  }
  return {
    ...base,
    ...overrides,
    credentials: {
      ...base.credentials,
      ...overrides.credentials
    },
    extra: overrides.extra ?? base.extra
  }
}

function mountModal(account = buildAccount()) {
  return mount(EditAccountModal, {
    props: {
      show: true,
      account,
      proxies: [],
      groups: []
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
        Icon: true,
        ProxySelector: true,
        GroupSelector: true,
        ModelWhitelistSelector: ModelWhitelistSelectorStub
      }
    }
  })
}

async function makeMediaConfigInvalid(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
  await wrapper.get('[data-test="media-override-model-0"]').setValue('duplicate')
  await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
  await wrapper.get('[data-test="media-override-model-1"]').setValue(' duplicate ')
}

const editExtraCases: Array<{ name: string; account: Partial<Account> }> = [
  {
    name: 'Antigravity OAuth',
    account: { platform: 'antigravity', type: 'oauth', credentials: { access_token: 'ag-token' } }
  },
  {
    name: 'Anthropic OAuth',
    account: { platform: 'anthropic', type: 'oauth', credentials: { access_token: 'anthropic-token' } }
  },
  {
    name: 'Anthropic API Key',
    account: {
      platform: 'anthropic',
      type: 'apikey',
      credentials: { api_key: 'sk-ant-test', base_url: 'https://api.anthropic.com' }
    }
  },
  {
    name: 'OpenAI OAuth',
    account: { platform: 'openai', type: 'oauth', credentials: { access_token: 'openai-token' } }
  },
  {
    name: 'Codex2api API Key',
    account: {
      platform: 'codex2api',
      type: 'apikey',
      credentials: { api_key: 'codex-token', base_url: 'https://codex.example.com' }
    }
  },
  {
    name: 'Gemini API Key',
    account: {
      platform: 'gemini',
      type: 'apikey',
      credentials: { api_key: 'gemini-token', base_url: 'https://generativelanguage.googleapis.com' }
    }
  },
  {
    name: 'Bedrock',
    account: {
      platform: 'anthropic',
      type: 'bedrock',
      credentials: { auth_mode: 'apikey', api_key: 'bedrock-token', aws_region: 'us-east-1' }
    }
  }
]

describe('EditAccountModal', () => {
  beforeEach(() => {
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
  })
  it('hydrates and updates media_config without deleting existing extra keys', async () => {
    const account = buildAccount()
    account.extra = {
      allow_overages: true,
      media_config: {
        adapter: 'gemini',
        native_async_mode: 'optional',
        model_overrides: {}
      }
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: 'gemini',
      native_async_mode: 'optional',
      model_overrides: {}
    })
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      adapter: 'gemini',
      native_async_mode: 'unsupported',
      model_overrides: {}
    })
    await nextTick()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({
      allow_overages: true,
      media_config: {
        adapter: 'gemini',
        native_async_mode: 'unsupported',
        model_overrides: {}
      }
    })
  })

  it('removes media_config when adapter is cleared while preserving other extra keys', async () => {
    const account = buildAccount()
    account.extra = {
      allow_overages: true,
      media_config: {
        adapter: 'gemini',
        native_async_mode: 'optional',
        model_overrides: {}
      }
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      adapter: '   ',
      native_async_mode: 'optional',
      model_overrides: {}
    })
    await nextTick()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({ allow_overages: true })
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('media_config')
  })

  it('rehydrates media_config when the same account modal is reopened', async () => {
    const account = buildAccount()
    account.extra = {
      media_config: {
        adapter: 'gemini',
        native_async_mode: 'optional',
        model_overrides: { veo: { upstream_model: 'veo-upstream' } }
      }
    }
    const wrapper = mountModal(account)

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      adapter: 'xai',
      native_async_mode: 'required',
      model_overrides: {}
    })
    await nextTick()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: 'gemini',
      native_async_mode: 'optional',
      model_overrides: { veo: { upstream_model: 'veo-upstream' } }
    })
  })

  it('blocks update while media overrides are invalid', async () => {
    const account = buildAccount()
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    await makeMediaConfigInvalid(wrapper)
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(wrapper.get('[data-test="media-override-duplicate-error"]').exists()).toBe(true)
    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('resets invalid media state when reopened before updating', async () => {
    const account = buildAccount()
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    await makeMediaConfigInvalid(wrapper)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(wrapper.find('[data-test="media-override-duplicate-error"]').exists()).toBe(false)
    expect(updateAccountMock).toHaveBeenCalledTimes(1)
  })

  it.each(editExtraCases)('merges existing Extra and media_config for $name', async ({ account: overrides }) => {
    const wire: MediaAccountConfigWire = { adapter: 'legacy-adapter' }
    const account = buildAccount({
      ...overrides,
      extra: {
        preserved_extra: 'keep-me',
        media_config: wire
      }
    })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      adapter: 'updated-adapter',
      native_async_mode: 'required',
      model_overrides: { image: { upstream_model: 'image-upstream' } }
    })
    await nextTick()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({
      preserved_extra: 'keep-me',
      media_config: {
        adapter: 'updated-adapter',
        native_async_mode: 'required',
        model_overrides: { image: { upstream_model: 'image-upstream' } }
      }
    })
  })

  it.each([
    { name: 'missing', wire: undefined },
    { name: 'explicit null', wire: null },
    { name: 'malformed object', wire: { adapter: 42, native_async_mode: 'sometimes', model_overrides: [] } }
  ])('normalizes $name media_config to editor defaults', ({ wire }) => {
    const account = buildAccount({ extra: { preserved_extra: 'keep-me' } })
    ;(account.extra as Record<string, unknown>).media_config = wire

    const wrapper = mountModal(account)

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: '',
      native_async_mode: 'unsupported',
      model_overrides: {}
    })
  })

  it('normalizes legacy wire media_config with omitted mode and overrides', () => {
    const wire: MediaAccountConfigWire = { adapter: ' gemini ' }
    const account = buildAccount({ extra: { media_config: wire } })

    const wrapper = mountModal(account)

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: 'gemini',
      native_async_mode: 'unsupported',
      model_overrides: {}
    })
  })

  it('hydrates media_config when switching to a different account', async () => {
    const first = buildAccount({
      id: 1,
      extra: {
        media_config: {
          adapter: 'gemini',
          native_async_mode: 'optional'
        }
      }
    })
    const second = buildAccount({
      id: 2,
      name: 'Second account',
      extra: {
        media_config: {
          adapter: 'xai',
          native_async_mode: 'required',
          model_overrides: { image: { upstream_model: 'image-upstream' } }
        }
      }
    })
    const wrapper = mountModal(first)

    await wrapper.setProps({ account: second })

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: 'xai',
      native_async_mode: 'required',
      model_overrides: { image: { upstream_model: 'image-upstream' } }
    })
  })

  it('reopening the same account rehydrates the OpenAI whitelist from props', async () => {
    const account = buildAccount()
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('[data-testid="rewrite-to-snapshot"]').trigger('click')
    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2-2025-12-11')

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.get('[data-testid="model-whitelist-value"]').text()).toBe('gpt-5.2')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.credentials?.model_mapping).toEqual({
      'gpt-5.2': 'gpt-5.2'
    })
  })

  it('edits the OpenAI image response URL mode in account extra', async () => {
    const account = buildAccount()
    account.extra = {
      openai_image_response_url_mode: 'http_url'
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const modeSelect = wrapper.get<HTMLSelectElement>('[data-testid="edit-openai-image-response-url-mode"]')

    expect(modeSelect.element.value).toBe('http_url')

    await modeSelect.setValue('base64_url')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.openai_image_response_url_mode).toBe('base64_url')
  })

  it('defaults missing OpenAI image response URL mode to HTTP URL', () => {
    const account = buildAccount()

    const wrapper = mountModal(account)
    const modeSelect = wrapper.get<HTMLSelectElement>('[data-testid="edit-openai-image-response-url-mode"]')

    expect(modeSelect.element.value).toBe('http_url')
  })

  it('edits the OpenAI Codex image bridge override in account extra', async () => {
    const account = buildAccount()
    account.extra = {
      codex_image_generation_bridge: false
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)

    const wrapper = mountModal(account)
    const overrideSelect = wrapper.get<HTMLSelectElement>('[data-testid="edit-openai-codex-image-generation-bridge"]')

    expect(overrideSelect.element.value).toBe('disabled')

    await overrideSelect.setValue('enabled')
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra?.codex_image_generation_bridge).toBe(true)
  })
})

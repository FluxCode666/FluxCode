import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

const { updateAccountMock, checkMixedChannelRiskMock, listEnabledMock } = vi.hoisted(() => ({
  updateAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  listEnabledMock: vi.fn()
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
    },
    mediaModels: {
      listEnabled: listEnabledMock
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
import type { Account, MediaAccountConfigWire, MediaModelDefinition } from '@/types'

const readyEditRegistryModels: MediaModelDefinition[] = [{
  id: 1,
  model_id: 'duplicate',
  vendor: 'test-vendor',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
  adapter_resolution: {
    status: 'ready',
    resolved_adapter: 'test-adapter',
    matched_by: 'exact',
    matched_family: '',
    capabilities: {
      operations: ['text_to_image'],
      sync_upstream: true,
      native_async_upstream: true,
      content_fetch: false
    },
    reason_code: ''
  }
}, {
  id: 2,
  model_id: 'seedance',
  vendor: 'bytedance',
  media_type: 'video',
  operations: ['text_to_video'],
  constraints: {},
  billing_unit: 'second',
  enabled: true,
  aliases: [],
  adapter_resolution: {
    status: 'ready',
    resolved_adapter: 'volcengine-seedance',
    matched_by: 'exact',
    matched_family: '',
    capabilities: {
      operations: ['text_to_video'],
      sync_upstream: false,
      native_async_upstream: true,
      content_fetch: false
    },
    reason_code: ''
  }
}, {
  id: 3,
  model_id: 'grok',
  vendor: 'xai',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
  adapter_resolution: {
    status: 'ready',
    resolved_adapter: 'xai-image',
    matched_by: 'exact',
    matched_family: '',
    capabilities: {
      operations: ['text_to_image'],
      sync_upstream: true,
      native_async_upstream: false,
      content_fetch: false
    },
    reason_code: ''
  }
}]

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

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
  await flushPromises()
  await wrapper.get('[data-test="media-add-model"]').trigger('click')
  await wrapper.get('[data-test="media-model-id-0"]').setValue('duplicate')
  await wrapper.get('[data-test="media-upstream-model-0"]').setValue('upstream-a')
  await wrapper.get('[data-test="media-add-model"]').trigger('click')
  await wrapper.get('[data-test="media-model-id-1"]').setValue('duplicate')
  await wrapper.get('[data-test="media-upstream-model-1"]').setValue('upstream-b')
}

function buildMediaAccount(overrides: Partial<Account> = {}): Account {
  return buildAccount({
    name: 'Media Provider',
    platform: 'media',
    type: 'apikey',
    credentials: { api_key: 'media-key', base_url: 'https://media.example.com' },
    extra: {
      media_config: {
        version: 1,
        provider: 'volcengine',
        models: {
          seedance: {
            enabled: true,
            upstream_model_id: 'doubao-seedance',
            async_mode: 'native',
            request_mapping: {}
          }
        }
      }
    },
    ...overrides
  })
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
    listEnabledMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    listEnabledMock.mockResolvedValue(readyEditRegistryModels)
  })

  it('Registry 加载完成前禁止更新媒体账号，能力校验通过后允许提交', async () => {
    const registryRequest = createDeferred<MediaModelDefinition[]>()
    listEnabledMock.mockReturnValue(registryRequest.promise)
    const account = buildMediaAccount()
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(updateAccountMock).not.toHaveBeenCalled()

    registryRequest.resolve(readyEditRegistryModels)
    await flushPromises()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
  })

  it('hydrates and updates media_config without deleting existing extra keys', async () => {
    const account = buildMediaAccount()
    account.extra = {
      allow_overages: true,
      media_config: {
        version: 1,
        provider: 'volcengine',
        models: {
          seedance: {
            enabled: true,
            upstream_model_id: 'doubao-seedance',
            async_mode: 'native',
            request_mapping: {}
          }
        }
      }
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      version: 1,
      provider: 'volcengine',
      models: {
        seedance: {
          enabled: true,
          upstream_model_id: 'doubao-seedance',
          async_mode: 'native',
          request_mapping: {}
        }
      }
    })
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      version: 1,
      provider: 'xai',
      models: {
        grok: {
          enabled: true,
          upstream_model_id: 'grok-imagine',
          async_mode: 'unsupported',
          request_mapping: {}
        }
      }
    })
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:valid', true)
    await nextTick()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({
      allow_overages: true,
      media_config: {
        version: 1,
        provider: 'xai',
        models: {
          grok: {
            enabled: true,
            upstream_model_id: 'grok-imagine',
            async_mode: 'unsupported',
            request_mapping: {}
          }
        }
      }
    })
  })

  it('blocks media account updates when provider is cleared', async () => {
    const account = buildMediaAccount()
    account.extra = {
      allow_overages: true,
      ...account.extra
    }
    updateAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      version: 1,
      provider: '   ',
      models: {}
    })
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:valid', false)
    await nextTick()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('does not fall back to the Anthropic Base URL when a media account has none', async () => {
    const account = buildMediaAccount()
    delete (account.credentials as Record<string, unknown>).base_url
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    const baseUrl = wrapper.get<HTMLInputElement>('input[placeholder="https://api.provider.example"]')
    expect(baseUrl.element.value).toBe('')

    await wrapper.get('form#edit-account-form').trigger('submit.prevent')
    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('rehydrates media_config when the same account modal is reopened', async () => {
    const account = buildMediaAccount()
    account.extra = {
      media_config: {
        adapter: 'gemini',
        native_async_mode: 'optional',
        model_overrides: { veo: { upstream_model: 'veo-upstream' } }
      }
    }
    const wrapper = mountModal(account)

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      version: 1,
      provider: 'xai',
      models: {}
    })
    await nextTick()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      version: 1,
      provider: 'gemini',
      models: {
        veo: {
          enabled: true,
          upstream_model_id: 'veo-upstream',
          async_mode: 'native',
          request_mapping: {}
        }
      }
    })
  })

  it('blocks update while media overrides are invalid', async () => {
    const account = buildMediaAccount({ extra: { media_config: { version: 1, provider: 'xai', models: {} } } })
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    await makeMediaConfigInvalid(wrapper)
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(wrapper.get('[data-test="media-duplicate-model-error"]').exists()).toBe(true)
    expect(updateAccountMock).not.toHaveBeenCalled()
  })

  it('resets invalid media state when reopened before updating', async () => {
    const account = buildMediaAccount()
    updateAccountMock.mockResolvedValue(account)
    const wrapper = mountModal(account)

    await makeMediaConfigInvalid(wrapper)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(wrapper.find('[data-test="media-duplicate-model-error"]').exists()).toBe(false)
    expect(updateAccountMock).toHaveBeenCalledTimes(1)
  })

  it.each(editExtraCases)('does not expose account-level media configuration for $name', async ({ account: overrides }) => {
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

    expect(wrapper.findComponent(MediaConfigEditor).exists()).toBe(false)
    await wrapper.get('form#edit-account-form').trigger('submit.prevent')

    expect(updateAccountMock).toHaveBeenCalledTimes(1)
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).toMatchObject({ preserved_extra: 'keep-me' })
    expect(updateAccountMock.mock.calls[0]?.[1]?.extra).not.toHaveProperty('media_config')
  })

  it.each([
    { name: 'missing', wire: undefined },
    { name: 'explicit null', wire: null },
    { name: 'malformed object', wire: { adapter: 42, native_async_mode: 'sometimes', model_overrides: [] } }
  ])('normalizes $name media_config to editor defaults', ({ wire }) => {
    const account = buildMediaAccount({ extra: { preserved_extra: 'keep-me' } })
    ;(account.extra as Record<string, unknown>).media_config = wire

    const wrapper = mountModal(account)

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      version: 1,
      provider: '',
      models: {}
    })
  })

  it('normalizes legacy wire media_config with omitted mode and overrides', () => {
    const wire: MediaAccountConfigWire = { adapter: ' gemini ' }
    const account = buildMediaAccount({ extra: { media_config: wire } })

    const wrapper = mountModal(account)

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      version: 1,
      provider: 'gemini',
      models: {}
    })
  })

  it('hydrates media_config when switching to a different account', async () => {
    const first = buildMediaAccount({
      id: 1,
      extra: {
        media_config: {
          adapter: 'gemini',
          native_async_mode: 'optional'
        }
      }
    })
    const second = buildMediaAccount({
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
      version: 1,
      provider: 'xai',
      models: {
        image: {
          enabled: true,
          upstream_model_id: 'image-upstream',
          async_mode: 'native',
          request_mapping: {}
        }
      }
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

import { describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import CreateAccountModal from '../CreateAccountModal.vue'

const { createAccountMock, checkMixedChannelRiskMock, importAgentIdentityMock } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  importAgentIdentityMock: vi.fn()
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
    showWarning: vi.fn()
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
      create: createAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      importAgentIdentity: importAgentIdentityMock
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
  getAntigravityDefaultModelMapping: vi.fn().mockResolvedValue([])
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

const ModelWhitelistSelectorStub = defineComponent({
  name: 'ModelWhitelistSelector',
  props: {
    modelValue: {
      type: Array,
      default: () => []
    }
  },
  emits: ['update:modelValue'],
  template: '<button type="button" data-testid="select-embedding-model" @click="$emit(\'update:modelValue\', [\'text-embedding-3-small\'])">model</button>'
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlow',
  props: {
    showAgentIdentityOption: Boolean
  },
  emits: ['import-agent-identity'],
  template: `
    <button
      v-if="showAgentIdentityOption"
      type="button"
      data-testid="emit-agent-identity-import"
      @click="$emit('import-agent-identity', '  {&quot;auth_mode&quot;:&quot;agentIdentity&quot;}  ')"
    >
      import
    </button>
  `
})

function mountModal() {
  return mount(CreateAccountModal, {
    props: {
      show: true,
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
        ModelWhitelistSelector: ModelWhitelistSelectorStub,
        QuotaLimitCard: true,
        OAuthAuthorizationFlow: OAuthAuthorizationFlowStub,
        ConfirmDialog: true
      }
    }
  })
}

describe('CreateAccountModal', () => {
  it('creates embedding as API key with model config and pool mode', async () => {
    createAccountMock.mockReset().mockResolvedValue({})
    checkMixedChannelRiskMock.mockReset().mockResolvedValue({ has_risk: false })
    const wrapper = mountModal()

    await wrapper.get('[data-testid="platform-embedding"]').trigger('click')
    expect(wrapper.get('[data-testid="platform-embedding"]').classes()).toContain('text-rose-600')
    expect(wrapper.text()).toContain('admin.accounts.modelRestriction')
    expect(wrapper.text()).toContain('admin.accounts.poolMode')
    expect(wrapper.text()).not.toContain('admin.accounts.customErrorCodes')

    const poolSection = wrapper
      .findAll('div.border-t')
      .find((section) => section.text().includes('admin.accounts.poolModeHint'))
    expect(poolSection).toBeDefined()
    await poolSection!.get('button').trigger('click')
    await poolSection!.get<HTMLInputElement>('input[type="number"]').setValue('99')

    const name = wrapper.get<HTMLInputElement>('[data-tour="account-form-name"]')
    await name.setValue('Embedding Key')
    await wrapper.get<HTMLInputElement>('[data-testid="create-apikey-base-url"]').setValue('http://embedding.internal:8080/v1')
    await wrapper.get<HTMLInputElement>('[data-testid="create-apikey-value"]').setValue('sk-embed')
    await wrapper.get('[data-testid="select-embedding-model"]').trigger('click')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      platform: 'embedding',
      type: 'apikey',
      credentials: {
        base_url: 'http://embedding.internal:8080/v1',
        api_key: 'sk-embed',
        model_mapping: { 'text-embedding-3-small': 'text-embedding-3-small' },
        pool_mode: true,
        pool_mode_retry_count: 10
      }
    })
    expect(Object.keys(createAccountMock.mock.calls[0]?.[0]?.credentials || {}).sort()).toEqual([
      'api_key', 'base_url', 'model_mapping', 'pool_mode', 'pool_mode_retry_count'
    ])
  })

  it('creates OpenAI API Key accounts with HTTP image response URLs by default', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })

    const wrapper = mountModal()

    await wrapper.findAll('button').find((button) => button.text().includes('OpenAI'))?.trigger('click')
    await wrapper.findAll('button').find((button) => button.text().includes('API Key'))?.trigger('click')

    const textInputs = wrapper.findAll<HTMLInputElement>('form#create-account-form input[type="text"]')
    await textInputs[0].setValue('OpenAI Key')
    await wrapper.get<HTMLInputElement>('form#create-account-form input[type="password"]').setValue('sk-test')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.openai_image_response_url_mode).toBe('http_url')
  })

  it('creates OpenAI API Key accounts with a Codex image bridge override', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })

    const wrapper = mountModal()

    await wrapper.findAll('button').find((button) => button.text().includes('OpenAI'))?.trigger('click')
    await wrapper.findAll('button').find((button) => button.text().includes('API Key'))?.trigger('click')
    await wrapper.get<HTMLSelectElement>('[data-testid="create-openai-codex-image-generation-bridge"]').setValue('enabled')

    const textInputs = wrapper.findAll<HTMLInputElement>('form#create-account-form input[type="text"]')
    await textInputs[0].setValue('OpenAI Key')
    await wrapper.get<HTMLInputElement>('form#create-account-form input[type="password"]').setValue('sk-test')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.codex_image_generation_bridge).toBe(true)
  })

  it('imports Agent Identity auth.json through the dedicated endpoint', async () => {
    importAgentIdentityMock.mockReset().mockResolvedValue({
      total: 1,
      created: 1,
      updated: 0,
      failed: 0,
      items: [{ index: 1, action: 'created', account_id: 42 }]
    })

    const wrapper = mountModal()
    await wrapper.findAll('button').find((button) => button.text().includes('OpenAI'))?.trigger('click')
    const textInputs = wrapper.findAll<HTMLInputElement>('form#create-account-form input[type="text"]')
    await textInputs[0].setValue('Agent account')
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    const flow = wrapper.getComponent(OAuthAuthorizationFlowStub)
    expect(flow.props('showAgentIdentityOption')).toBe(true)
    await wrapper.get('[data-testid="emit-agent-identity-import"]').trigger('click')

    expect(importAgentIdentityMock).toHaveBeenCalledTimes(1)
    expect(importAgentIdentityMock.mock.calls[0]?.[0]).toMatchObject({
      content: '{"auth_mode":"agentIdentity"}',
      name: 'Agent account',
      update_existing: true
    })
  })
})

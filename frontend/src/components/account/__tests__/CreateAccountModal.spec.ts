import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick, ref } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import CreateAccountModal from '../CreateAccountModal.vue'
import MediaConfigEditor from '../MediaConfigEditor.vue'
import type { MediaModelDefinition } from '@/types'

const {
  createAccountMock,
  checkMixedChannelRiskMock,
  generateAuthUrlMock,
  exchangeCodeMock,
  refreshOpenAITokenMock,
  refreshAntigravityTokenMock,
  listEnabledMock,
  authStoreState
} = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
  checkMixedChannelRiskMock: vi.fn(),
  generateAuthUrlMock: vi.fn(),
  exchangeCodeMock: vi.fn(),
  refreshOpenAITokenMock: vi.fn(),
  refreshAntigravityTokenMock: vi.fn(),
  listEnabledMock: vi.fn(),
  authStoreState: { isSimpleMode: false }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStoreState
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      create: createAccountMock,
      checkMixedChannelRisk: checkMixedChannelRiskMock,
      generateAuthUrl: generateAuthUrlMock,
      exchangeCode: exchangeCodeMock,
      refreshOpenAIToken: refreshOpenAITokenMock
    },
    settings: {
      getWebSearchEmulationConfig: vi.fn().mockResolvedValue({ enabled: false, providers: [] }),
      getSettings: vi.fn().mockResolvedValue({ account_quota_notify_enabled: false })
    },
    tlsFingerprintProfiles: {
      list: vi.fn().mockResolvedValue([])
    },
    antigravity: {
      refreshAntigravityToken: refreshAntigravityTokenMock
    },
    mediaModels: {
      listEnabled: listEnabledMock
    }
  }
}))

const readyCreateRegistryModel: MediaModelDefinition = {
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
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

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
  template: '<div />'
})

const OAuthAuthorizationFlowStub = defineComponent({
  name: 'OAuthAuthorizationFlowStub',
  emits: ['generate-url', 'cookie-auth', 'validate-refresh-token'],
  setup(_, { expose }) {
    const authCode = ref('test-auth-code')
    const oauthState = ref('test-state')
    const projectId = ref('test-project')
    const sessionKey = ref('test-session-key')
    const refreshToken = ref('test-refresh-token')
    const sessionToken = ref('')
    const inputMethod = ref('manual')
    const reset = () => {
      authCode.value = ''
      oauthState.value = ''
      projectId.value = ''
      sessionKey.value = ''
      refreshToken.value = ''
      sessionToken.value = ''
      inputMethod.value = 'manual'
    }
    expose({
      authCode,
      oauthState,
      projectId,
      sessionKey,
      refreshToken,
      sessionToken,
      inputMethod,
      reset
    })
    return {}
  },
  template: `
    <div>
      <button data-test="oauth-generate" type="button" @click="$emit('generate-url')">generate</button>
      <button data-test="oauth-refresh" type="button" @click="$emit('validate-refresh-token', 'test-refresh-token')">refresh</button>
      <button data-test="oauth-cookie" type="button" @click="$emit('cookie-auth', 'test-session-key')">cookie</button>
    </div>
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

async function selectPlatform(wrapper: ReturnType<typeof mountModal>, platform: 'OpenAI' | 'Antigravity' | 'Media') {
  await wrapper.findAll('button').find((button) => button.text().includes(platform))?.trigger('click')
}

async function fillOAuthAccountName(wrapper: ReturnType<typeof mountModal>, name: string) {
  await wrapper.get<HTMLInputElement>('form#create-account-form input[type="text"]').setValue(name)
}

async function goToOAuthStep(wrapper: ReturnType<typeof mountModal>, name: string) {
  await fillOAuthAccountName(wrapper, name)
  await wrapper.get('form#create-account-form').trigger('submit.prevent')
  await nextTick()
}

async function setMediaConfig(wrapper: ReturnType<typeof mountModal>, provider = 'volcengine') {
  wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
    version: 1,
    provider,
    models: {
      image: {
        enabled: true,
        upstream_model_id: 'image-upstream',
        async_mode: 'native',
        request_mapping: {}
      }
    }
  })
  wrapper.getComponent(MediaConfigEditor).vm.$emit('update:valid', true)
  await nextTick()
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

async function fillMinimalCreateAccountForm(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.get('[data-test="platform-media"]').trigger('click')
  const textInputs = wrapper.findAll<HTMLInputElement>('form#create-account-form input[type="text"]')
  await textInputs[0].setValue('Media Key')
  await textInputs[1].setValue('https://media.example.com')
  await wrapper.get<HTMLInputElement>('form#create-account-form input[type="password"]').setValue('sk-test')
}

describe('CreateAccountModal', () => {
  beforeEach(() => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    generateAuthUrlMock.mockReset()
    exchangeCodeMock.mockReset()
    refreshOpenAITokenMock.mockReset()
    refreshAntigravityTokenMock.mockReset()
    listEnabledMock.mockReset()
    authStoreState.isSimpleMode = false

    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    generateAuthUrlMock.mockResolvedValue({
      auth_url: 'https://auth.example/callback?state=test-state',
      session_id: 'test-session-id'
    })
    exchangeCodeMock.mockImplementation((endpoint: string) => {
      if (endpoint.includes('/admin/openai/')) {
        return Promise.resolve({
          access_token: 'openai-access',
          refresh_token: 'openai-refresh',
          expires_at: 123,
          email: 'openai@example.com'
        })
      }
      return Promise.resolve({
        access_token: 'anthropic-access',
        org_uuid: 'org-1',
        account_uuid: 'account-1',
        email_address: 'anthropic@example.com'
      })
    })
    refreshOpenAITokenMock.mockResolvedValue({
      access_token: 'openai-access',
      refresh_token: 'openai-refresh',
      expires_at: 123,
      email: 'openai@example.com'
    })
    refreshAntigravityTokenMock.mockResolvedValue({
      access_token: 'ag-access',
      refresh_token: 'ag-refresh',
      token_type: 'Bearer',
      expires_at: 123,
      project_id: 'project-1',
      email: 'ag@example.com'
    })
    listEnabledMock.mockResolvedValue([readyCreateRegistryModel])
  })

  it('简易模式隐藏媒体平台入口', () => {
    authStoreState.isSimpleMode = true

    const wrapper = mountModal()

    expect(wrapper.find('[data-test="platform-media"]').exists()).toBe(false)
  })

  it('creates account with media_config while preserving other extra fields', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    const wrapper = mountModal()

    await fillMinimalCreateAccountForm(wrapper)
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      version: 1,
      provider: '  xai  ',
      models: {
        'grok-imagine': {
          enabled: true,
          upstream_model_id: 'grok-imagine-v1',
          async_mode: 'native',
          request_mapping: {}
        }
      }
    })
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:valid', true)
    await nextTick()
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock.mock.calls[0]?.[0]).toMatchObject({
      platform: 'media',
      type: 'apikey',
      extra: {
        media_config: {
          version: 1,
          provider: 'xai'
        }
      }
    })
    expect(createAccountMock.mock.calls[0]?.[0]?.extra?.media_config?.models).toHaveProperty('grok-imagine')
  })

  it('Registry 加载完成前禁止创建媒体账号，能力校验通过后允许提交', async () => {
    const registryRequest = createDeferred<MediaModelDefinition[]>()
    listEnabledMock.mockReturnValue(registryRequest.promise)
    const wrapper = mountModal()

    await fillMinimalCreateAccountForm(wrapper)
    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      version: 1,
      provider: 'test-vendor',
      models: {
        duplicate: {
          enabled: true,
          upstream_model_id: 'duplicate-upstream',
          async_mode: 'native',
          request_mapping: {}
        }
      }
    })
    await nextTick()

    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()
    expect(createAccountMock).not.toHaveBeenCalled()

    registryRequest.resolve([readyCreateRegistryModel])
    await flushPromises()
    await wrapper.get('form#create-account-form').trigger('submit.prevent')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
  })

  it('切换平台时清空已选分组，避免提交不可见的不兼容分组', async () => {
    authStoreState.isSimpleMode = false
    const wrapper = mountModal()
    const groupSelector = wrapper.getComponent({ name: 'GroupSelector' })

    groupSelector.vm.$emit('update:modelValue', [42])
    await nextTick()
    expect(wrapper.getComponent({ name: 'GroupSelector' }).props('modelValue')).toEqual([42])

    await wrapper.get('[data-test="platform-media"]').trigger('click')

    expect(wrapper.getComponent({ name: 'GroupSelector' }).props('modelValue')).toEqual([])
  })

  it('does not expose or create media_config for text platform accounts', async () => {
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
    expect(wrapper.findComponent(MediaConfigEditor).exists()).toBe(false)
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('media_config')
  })

  it('resets media_config when the modal is reopened', async () => {
    const wrapper = mountModal()

    await fillMinimalCreateAccountForm(wrapper)
    await setMediaConfig(wrapper, 'xai')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await wrapper.get('[data-test="platform-media"]').trigger('click')

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      version: 1,
      provider: '',
      models: {}
    })
  })

  it('blocks the common create path while media overrides are invalid', async () => {
    const wrapper = mountModal()

    await fillMinimalCreateAccountForm(wrapper)
    await makeMediaConfigInvalid(wrapper)
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(wrapper.get('[data-test="media-duplicate-model-error"]').exists()).toBe(true)
    expect(createAccountMock).not.toHaveBeenCalled()
  })

  it('resets invalid media state when reopened and allows a valid create', async () => {
    const wrapper = mountModal()

    await wrapper.get('[data-test="platform-media"]').trigger('click')
    await makeMediaConfigInvalid(wrapper)
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await fillMinimalCreateAccountForm(wrapper)
    await setMediaConfig(wrapper)
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(wrapper.find('[data-test="media-duplicate-model-error"]').exists()).toBe(false)
    expect(createAccountMock).toHaveBeenCalledTimes(1)
  })

  it('merges media_config in the direct OpenAI auth-code create path', async () => {
    const wrapper = mountModal()

    await selectPlatform(wrapper, 'OpenAI')
    await goToOAuthStep(wrapper, 'OpenAI auth code')
    await wrapper.get('[data-test="oauth-generate"]').trigger('click')
    await flushPromises()
    await wrapper.findAll('button').find((button) =>
      button.text().includes('admin.accounts.oauth.completeAuth')
    )?.trigger('click')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).toMatchObject({ email: 'openai@example.com' })
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('media_config')
  })

  it('merges media_config in the direct OpenAI refresh-token create path', async () => {
    const wrapper = mountModal()

    await selectPlatform(wrapper, 'OpenAI')
    await goToOAuthStep(wrapper, 'OpenAI refresh token')
    await wrapper.get('[data-test="oauth-refresh"]').trigger('click')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).toMatchObject({ email: 'openai@example.com' })
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('media_config')
  })

  it('preserves Antigravity flags and media_config in the direct refresh-token path', async () => {
    const wrapper = mountModal()

    await selectPlatform(wrapper, 'Antigravity')
    await wrapper.findAll('label').find((label) =>
      label.text().includes('admin.accounts.mixedScheduling')
    )?.get('input').setValue(true)
    await wrapper.findAll('label').find((label) =>
      label.text().includes('admin.accounts.allowOverages')
    )?.get('input').setValue(true)
    await goToOAuthStep(wrapper, 'Antigravity refresh token')
    await wrapper.get('[data-test="oauth-refresh"]').trigger('click')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).toMatchObject({
      mixed_scheduling: true,
      allow_overages: true
    })
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('media_config')
  })

  it('merges media_config in the direct Anthropic cookie create path', async () => {
    const wrapper = mountModal()

    await goToOAuthStep(wrapper, 'Anthropic cookie')
    await wrapper.get('[data-test="oauth-cookie"]').trigger('click')
    await flushPromises()

    expect(createAccountMock).toHaveBeenCalledTimes(1)
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).toMatchObject({
      org_uuid: 'org-1',
      account_uuid: 'account-1'
    })
    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('media_config')
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
})

import { describe, expect, it, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount } from '@vue/test-utils'
import CreateAccountModal from '../CreateAccountModal.vue'
import MediaConfigEditor from '../MediaConfigEditor.vue'

const { createAccountMock, checkMixedChannelRiskMock } = vi.hoisted(() => ({
  createAccountMock: vi.fn(),
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
      create: createAccountMock,
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
        OAuthAuthorizationFlow: true,
        ConfirmDialog: true
      }
    }
  })
}

async function fillMinimalCreateAccountForm(wrapper: ReturnType<typeof mountModal>) {
  await wrapper.findAll('button').find((button) => button.text().includes('OpenAI'))?.trigger('click')
  await wrapper.findAll('button').find((button) => button.text().includes('API Key'))?.trigger('click')
  const textInputs = wrapper.findAll<HTMLInputElement>('form#create-account-form input[type="text"]')
  await textInputs[0].setValue('Media Key')
  await wrapper.get<HTMLInputElement>('form#create-account-form input[type="password"]').setValue('sk-test')
}

describe('CreateAccountModal', () => {
  it('creates account with media_config while preserving other extra fields', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    const wrapper = mountModal()

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      adapter: '  xai  ',
      native_async_mode: 'required',
      model_overrides: { 'grok-imagine': { upstream_model: 'grok-imagine-v1' } }
    })
    await nextTick()
    await fillMinimalCreateAccountForm(wrapper)
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock.mock.calls[0]?.[0]?.extra).toMatchObject({
      openai_image_response_url_mode: 'http_url',
      media_config: {
        adapter: 'xai',
        native_async_mode: 'required',
        model_overrides: { 'grok-imagine': { upstream_model: 'grok-imagine-v1' } }
      }
    })
  })

  it('does not create media_config when adapter is empty', async () => {
    createAccountMock.mockReset()
    checkMixedChannelRiskMock.mockReset()
    createAccountMock.mockResolvedValue({})
    checkMixedChannelRiskMock.mockResolvedValue({ has_risk: false })
    const wrapper = mountModal()

    await fillMinimalCreateAccountForm(wrapper)
    await wrapper.get('form#create-account-form').trigger('submit.prevent')

    expect(createAccountMock.mock.calls[0]?.[0]?.extra).not.toHaveProperty('media_config')
  })

  it('resets media_config when the modal is reopened', async () => {
    const wrapper = mountModal()

    wrapper.getComponent(MediaConfigEditor).vm.$emit('update:modelValue', {
      adapter: 'xai',
      native_async_mode: 'required',
      model_overrides: {}
    })
    await nextTick()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: '',
      native_async_mode: 'unsupported',
      model_overrides: {}
    })
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

import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import MediaConfigEditor from '../MediaConfigEditor.vue'
import type { MediaAccountConfig, MediaModelDefinition } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const listEnabledMock = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin', () => ({
  adminAPI: { mediaModels: { listEnabled: listEnabledMock } }
}))

function readyRegistryModel(
  modelID: string,
  capabilities: { sync_upstream: boolean; native_async_upstream: boolean } = {
    sync_upstream: true,
    native_async_upstream: true
  },
  id = 1
): MediaModelDefinition {
  return {
    id,
    model_id: modelID,
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
        content_fetch: false,
        ...capabilities
      },
      reason_code: ''
    }
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

const defaultRegistryModelIDs = [
  'seedance',
  'seedance-lite',
  'grok-image',
  'grok_image',
  '__proto__'
]

const initialConfig = (): MediaAccountConfig => ({
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

function mountHarness(value: MediaAccountConfig = initialConfig()) {
  return mount(defineComponent({
    components: { MediaConfigEditor },
    setup() {
      const config = ref<MediaAccountConfig>(value)
      const valid = ref(true)
      const reset = () => {
        config.value = {
          version: 1,
          provider: 'xai',
          models: {
            grok_image: {
              enabled: true,
              upstream_model_id: 'grok-imagine-image',
              async_mode: 'unsupported',
              request_mapping: {}
            }
          }
        }
      }
      return { config, valid, reset }
    },
    template: `
      <MediaConfigEditor v-model="config" @update:valid="valid = $event" />
      <output data-test="valid">{{ String(valid) }}</output>
      <button data-test="reset" type="button" @click="reset">reset</button>
    `
  }))
}

describe('MediaConfigEditor', () => {
  beforeEach(() => {
    listEnabledMock.mockReset()
    listEnabledMock.mockResolvedValue(defaultRegistryModelIDs.map((modelID, index) =>
      readyRegistryModel(modelID, undefined, index + 1)))
  })

  it('按 v1 契约发布 provider 与模型绑定，且不暴露 Adapter 字段', async () => {
    const wrapper = mountHarness({ version: 1, provider: '', models: {} })
    await flushPromises()

    expect(wrapper.find('[data-test="media-adapter"]').exists()).toBe(false)
    await wrapper.get('[data-test="media-provider"]').setValue('xai')
    await wrapper.get('[data-test="media-add-model"]').trigger('click')
    await wrapper.get('[data-test="media-model-id-0"]').setValue('grok-image')
    await wrapper.get('[data-test="media-upstream-model-0"]').setValue('grok-imagine-image')

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      version: 1,
      provider: 'xai',
      models: {
        'grok-image': {
          enabled: true,
          upstream_model_id: 'grok-imagine-image',
          async_mode: 'unsupported',
          request_mapping: {}
        }
      }
    })
    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')
  })

  it('保存原生异步能力与声明式请求映射', async () => {
    const wrapper = mountHarness()
    await flushPromises()
    await wrapper.get('[data-test="media-async-mode-0"]').setValue('unsupported')
    await wrapper.get('[data-test="media-request-mapping-0"]').setValue(JSON.stringify({
      rules: [{ source: 'size', target: 'chicun', operation: 'rename' }]
    }))

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue').models.seedance).toEqual({
      enabled: true,
      upstream_model_id: 'doubao-seedance',
      async_mode: 'unsupported',
      request_mapping: {
        rules: [{ source: 'size', target: 'chicun', operation: 'rename' }]
      }
    })
  })

  it('拒绝超出系统 Adapter 解析能力的账号执行模式', async () => {
    listEnabledMock.mockResolvedValue([
      readyRegistryModel('grok-2-image', {
        sync_upstream: true,
        native_async_upstream: false
      })
    ])
    const wrapper = mountHarness({
      version: 1,
      provider: 'xai',
      models: {
        'grok-2-image': {
          enabled: true,
          upstream_model_id: 'upstream-image',
          async_mode: 'native',
          request_mapping: {}
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-test="media-async-mode-0"] option[value="native"]')
      .attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')
    expect(wrapper.get('[data-test="media-mode-capability-error-0"]').exists()).toBe(true)
  })

  it('在 Registry 成功加载并完成能力校验前保持无效', async () => {
    const registryRequest = createDeferred<MediaModelDefinition[]>()
    listEnabledMock.mockReturnValue(registryRequest.promise)

    const wrapper = mountHarness()
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')

    registryRequest.resolve([
      readyRegistryModel('seedance', {
        sync_upstream: true,
        native_async_upstream: true
      })
    ])
    await flushPromises()

    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')
  })

  it('Registry 加载失败后继续保持无效', async () => {
    listEnabledMock.mockRejectedValue(new Error('registry unavailable'))

    const wrapper = mountHarness()
    await flushPromises()

    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')
  })

  it('对 native-only Adapter 禁用同步模式并将当前绑定判为无效', async () => {
    listEnabledMock.mockResolvedValue([
      readyRegistryModel('seedance-native', {
        sync_upstream: false,
        native_async_upstream: true
      })
    ])
    const wrapper = mountHarness({
      version: 1,
      provider: 'volcengine',
      models: {
        'seedance-native': {
          enabled: true,
          upstream_model_id: 'seedance-upstream',
          async_mode: 'unsupported',
          request_mapping: {}
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-test="media-async-mode-0"] option[value="unsupported"]')
      .attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')
    expect(wrapper.get('[data-test="media-mode-capability-error-0"]').exists()).toBe(true)
  })

  it('保留历史不可用模型供管理员禁用或移除，并将启用绑定判为无效', async () => {
    listEnabledMock.mockResolvedValue([readyRegistryModel('ready-image')])
    const wrapper = mountHarness({
      version: 1,
      provider: 'xai',
      models: {
        'ready-image': {
          enabled: true,
          upstream_model_id: 'ready-upstream',
          async_mode: 'unsupported',
          request_mapping: {}
        },
        'removed-image': {
          enabled: true,
          upstream_model_id: 'removed-upstream',
          async_mode: 'unsupported',
          request_mapping: {}
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-test="media-model-id-1"] option[value="removed-image"]').exists())
      .toBe(true)
    expect(wrapper.get('[data-test="media-model-unavailable-1"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')

    await wrapper.get('[data-test="media-model-remove-1"]').trigger('click')
    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')
  })

  it('阻止公共模型 ID 重复或模型字段缺失', async () => {
    const wrapper = mountHarness()
    await flushPromises()
    await wrapper.get('[data-test="media-add-model"]').trigger('click')
    await wrapper.get('[data-test="media-model-id-1"]').setValue('seedance')
    await wrapper.get('[data-test="media-upstream-model-1"]').setValue('another-seedance')

    expect(wrapper.get('[data-test="media-duplicate-model-error"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')

    await wrapper.get('[data-test="media-model-id-1"]').setValue('seedance-lite')
    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')
  })

  it('拒绝非法请求映射 JSON，并在修复后恢复有效状态', async () => {
    const wrapper = mountHarness()
    await flushPromises()
    await wrapper.get('[data-test="media-request-mapping-0"]').setValue('{invalid')

    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')
    expect(wrapper.get('[data-test="media-request-mapping-0"]').attributes('aria-invalid')).toBe('true')

    await wrapper.get('[data-test="media-request-mapping-0"]').setValue('{}')
    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')
  })

  it('把 __proto__ 作为普通模型键发布且不改变映射原型', async () => {
    const wrapper = mountHarness({ version: 1, provider: 'xai', models: {} })
    await flushPromises()
    await wrapper.get('[data-test="media-add-model"]').trigger('click')
    await wrapper.get('[data-test="media-model-id-0"]').setValue('__proto__')
    await wrapper.get('[data-test="media-upstream-model-0"]').setValue('safe-upstream')

    const models = wrapper.getComponent(MediaConfigEditor).props('modelValue').models
    expect(Object.hasOwn(models, '__proto__')).toBe(true)
    expect(models.__proto__.upstream_model_id).toBe('safe-upstream')
    expect(Object.getPrototypeOf(models)).toBeNull()
  })

  it('外部重置会重新水合 provider、模型和表单关联标签', async () => {
    const wrapper = mountHarness()
    await flushPromises()
    await wrapper.get('[data-test="reset"]').trigger('click')

    expect(wrapper.get<HTMLInputElement>('[data-test="media-provider"]').element.value).toBe('xai')
    expect(wrapper.get<HTMLInputElement>('[data-test="media-model-id-0"]').element.value).toBe('grok_image')
    expect(wrapper.get('label[for="media-provider"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="media-add-model"]').attributes('type')).toBe('button')
  })
})

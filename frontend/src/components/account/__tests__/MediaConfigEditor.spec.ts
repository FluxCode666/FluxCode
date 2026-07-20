import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import MediaConfigEditor from '../MediaConfigEditor.vue'
import type { MediaAccountConfig } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const registryModelIDs = vi.hoisted(() => [
  'seedance',
  'seedance-lite',
  'grok-image',
  'grok_image',
  '__proto__'
])

vi.mock('@/api/admin', () => ({
  adminAPI: {
    mediaModels: {
      listEnabled: vi.fn().mockResolvedValue(registryModelIDs.map((model_id, index) => ({
        id: index + 1,
        model_id,
        vendor: 'test-vendor',
        media_type: 'image',
        operations: ['text_to_image'],
        constraints: {},
        billing_unit: 'image',
        default_adapter: 'test-adapter',
        default_async_mode: 'unsupported',
        enabled: true,
        aliases: []
      })))
    }
  }
}))

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

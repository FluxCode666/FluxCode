import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import MediaConfigEditor from '../MediaConfigEditor.vue'
import type { MediaAccountConfig } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const initialConfig = (): MediaAccountConfig => ({
  adapter: 'gemini',
  native_async_mode: 'optional',
  model_overrides: {}
})

function mountHarness(value: MediaAccountConfig = initialConfig()) {
  return mount(defineComponent({
    components: { MediaConfigEditor },
    setup() {
      const config = ref<MediaAccountConfig>(value)
      const valid = ref(true)
      const reset = () => {
        config.value = {
          adapter: 'xai',
          native_async_mode: 'unsupported',
          model_overrides: {
            reset_model: { upstream_model: 'reset-upstream' }
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

function mountCloningHarness() {
  return mount(defineComponent({
    components: { MediaConfigEditor },
    setup() {
      const config = ref<MediaAccountConfig>(initialConfig())
      const cloneNextUpdate = ref(false)
      const handleUpdate = (value: MediaAccountConfig) => {
        config.value = cloneNextUpdate.value
          ? { ...value, model_overrides: { ...value.model_overrides } }
          : value
        cloneNextUpdate.value = false
      }
      return { config, cloneNextUpdate, handleUpdate }
    },
    template: `
      <MediaConfigEditor :model-value="config" @update:model-value="handleUpdate" />
      <button data-test="clone-next" type="button" @click="cloneNextUpdate = true">clone</button>
    `
  }))
}

describe('MediaConfigEditor', () => {
  it('通过真实父级 v-model 连续编辑默认模式和模型覆盖', async () => {
    const wrapper = mountHarness()

    await wrapper.get('[data-test="media-default-async-mode"]').setValue('required')
    await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
    await wrapper.get('[data-test="media-override-model-0"]').setValue('veo-3.1')
    await wrapper.get('[data-test="media-override-upstream-0"]').setValue('veo-3.1-generate')

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: 'gemini',
      native_async_mode: 'required',
      model_overrides: {
        'veo-3.1': { upstream_model: 'veo-3.1-generate' }
      }
    })
  })

  it('重命名和删除覆盖时保留其它行，并省略继承字段', async () => {
    const wrapper = mountHarness({
      adapter: 'gemini',
      native_async_mode: 'optional',
      model_overrides: {
        alpha: { upstream_model: 'alpha-upstream' },
        beta: { native_async_mode: 'required' }
      }
    })

    await wrapper.get('[data-test="media-override-model-0"]').setValue('alpha-renamed')
    await wrapper.get('[data-test="media-override-remove-0"]').trigger('click')

    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element.value).toBe('beta')
    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue').model_overrides).toEqual({
      beta: { native_async_mode: 'required' }
    })
  })

  it('过滤空模型名，并阻止 trim 后重复的模型名静默覆盖', async () => {
    const wrapper = mountHarness({
      adapter: 'gemini',
      native_async_mode: 'optional',
      model_overrides: {
        alpha: { upstream_model: 'alpha-upstream' },
        beta: { upstream_model: 'beta-upstream' }
      }
    })

    await wrapper.get('[data-test="media-override-model-1"]').setValue(' alpha ')

    expect(wrapper.get('[data-test="media-override-duplicate-error"]').text()).toBe(
      'admin.accounts.mediaConfig.duplicateModel'
    )
    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue').model_overrides).toHaveProperty('beta')

    await wrapper.get('[data-test="media-override-model-1"]').setValue('   ')

    expect(wrapper.find('[data-test="media-override-duplicate-error"]').exists()).toBe(false)
    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue').model_overrides).toEqual({
      alpha: { upstream_model: 'alpha-upstream' }
    })
  })

  it('重复 key 期间保留标量草稿，恢复唯一后一次发布最新值', async () => {
    const wrapper = mountHarness({
      adapter: 'gemini',
      native_async_mode: 'optional',
      model_overrides: {
        alpha: {},
        beta: {}
      }
    })

    await wrapper.get('[data-test="media-override-model-1"]').setValue(' alpha ')
    await wrapper.get('[data-test="media-adapter"]').setValue('xai-draft')
    await wrapper.get('[data-test="media-default-async-mode"]').setValue('required')
    await wrapper.get('[data-test="media-override-model-1"]').setValue('gamma')

    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue')).toEqual({
      adapter: 'xai-draft',
      native_async_mode: 'required',
      model_overrides: {
        alpha: {},
        gamma: {}
      }
    })
  })

  it('发布重复 key validity=false，恢复唯一或外部重置后 validity=true', async () => {
    const wrapper = mountHarness({
      adapter: 'gemini',
      native_async_mode: 'optional',
      model_overrides: {
        alpha: {},
        beta: {}
      }
    })

    await wrapper.get('[data-test="media-override-model-1"]').setValue('alpha')
    expect(wrapper.get('[data-test="valid"]').text()).toBe('false')

    await wrapper.get('[data-test="media-override-model-1"]').setValue('gamma')
    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')

    await wrapper.get('[data-test="media-override-model-1"]').setValue('alpha')
    await wrapper.get('[data-test="reset"]').trigger('click')
    expect(wrapper.get('[data-test="valid"]').text()).toBe('true')
  })

  it('标准 v-model 的 reactive Proxy 自身连续回流保留空行和 DOM identity', async () => {
    const wrapper = mountHarness()

    await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
    const draftInput = wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element
    const stableRowId = draftInput.id

    await wrapper.get('[data-test="media-adapter"]').setValue('xai')

    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element).toBe(draftInput)
    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element.id).toBe(stableRowId)
    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue').adapter).toBe('xai')

    await wrapper.get('[data-test="media-default-async-mode"]').setValue('required')

    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element).toBe(draftInput)
    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element.id).toBe(stableRowId)
    expect(wrapper.getComponent(MediaConfigEditor).props('modelValue').native_async_mode).toBe('required')
  })

  it('仅按对象引用识别自身回流，同值外部新对象仍会清除空草稿行', async () => {
    const wrapper = mountCloningHarness()

    await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
    await wrapper.get('[data-test="clone-next"]').trigger('click')
    await wrapper.get('[data-test="media-default-async-mode"]').setValue('required')

    expect(wrapper.find('[data-test="media-override-model-0"]').exists()).toBe(false)
  })

  it('外部重置时重新水合标量和覆盖行', async () => {
    const wrapper = mountHarness()

    await wrapper.get('[data-test="media-add-model-override"]').trigger('click')
    await wrapper.get('[data-test="media-override-model-0"]').setValue('draft-model')
    await wrapper.get('[data-test="reset"]').trigger('click')

    expect(wrapper.get<HTMLInputElement>('[data-test="media-adapter"]').element.value).toBe('xai')
    expect(wrapper.get<HTMLSelectElement>('[data-test="media-default-async-mode"]').element.value).toBe('unsupported')
    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-model-0"]').element.value).toBe('reset_model')
    expect(wrapper.get<HTMLInputElement>('[data-test="media-override-upstream-0"]').element.value).toBe('reset-upstream')
  })

  it('为原生表单控件提供关联 label 和键盘可用按钮', () => {
    const wrapper = mountHarness()

    expect(wrapper.get('label[for="media-adapter"]').exists()).toBe(true)
    expect(wrapper.get('label[for="media-default-async-mode"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="media-add-model-override"]').attributes('type')).toBe('button')
  })
})

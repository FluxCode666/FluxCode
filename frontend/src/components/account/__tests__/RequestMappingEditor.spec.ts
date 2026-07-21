import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import RequestMappingEditor from '../RequestMappingEditor.vue'
import type { MediaRequestMapping } from '@/types'

const previewRequestMappingMock = vi.hoisted(() => vi.fn())

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { mediaModels: { previewRequestMapping: previewRequestMappingMock } }
}))

function mountHarness(value: MediaRequestMapping = {}) {
  return mount(defineComponent({
    components: { RequestMappingEditor },
    setup() {
      const mapping = ref<MediaRequestMapping>(value)
      const valid = ref(true)
      return { mapping, valid }
    },
    template: `
      <RequestMappingEditor
        v-model="mapping"
        id-prefix="mapping"
        @update:valid="valid = $event"
      />
      <output data-test="mapping-value">{{ JSON.stringify(mapping) }}</output>
      <output data-test="mapping-valid">{{ String(valid) }}</output>
    `
  }))
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('RequestMappingEditor', () => {
  beforeEach(() => {
    previewRequestMappingMock.mockReset()
  })

  it('可视化编辑并保存全部五种真实规则', async () => {
    const wrapper = mountHarness({
      rules: [
        { operation: 'rename', source: 'size', target: 'image_size' },
        { operation: 'copy', source: 'prompt', target: 'input.prompt' },
        { operation: 'default', target: 'n', value: 1 },
        { operation: 'enum', source: 'quality', target: 'upstream_quality', values: { standard: 'normal' } },
        { operation: 'cast', source: 'seed', target: 'seed_number', cast: 'number' }
      ]
    })

    await wrapper.get('[data-test="mapping-default-value-2"]').setValue('2')
    await wrapper.get('[data-test="mapping-enum-target-3-0"]').setValue('balanced')
    await wrapper.get('[data-test="mapping-cast-4"]').setValue('integer')

    expect(JSON.parse(wrapper.get('[data-test="mapping-value"]').text())).toEqual({
      rules: [
        { operation: 'rename', source: 'size', target: 'image_size' },
        { operation: 'copy', source: 'prompt', target: 'input.prompt' },
        { operation: 'default', target: 'n', value: 2 },
        { operation: 'enum', source: 'quality', target: 'upstream_quality', values: { standard: 'balanced' } },
        { operation: 'cast', source: 'seed', target: 'seed_number', cast: 'integer' }
      ]
    })
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('true')
  })

  it('阻止非法路径、重复目标和非法默认值进入绑定配置', async () => {
    const wrapper = mountHarness()
    await wrapper.get('[data-test="mapping-add-rule"]').trigger('click')
    await wrapper.get('[data-test="mapping-source-0"]').setValue('size[0]')
    await wrapper.get('[data-test="mapping-target-0"]').setValue('image_size')

    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('false')
    expect(JSON.parse(wrapper.get('[data-test="mapping-value"]').text())).toEqual({})

    await wrapper.get('[data-test="mapping-source-0"]').setValue('size')
    await wrapper.get('[data-test="mapping-add-rule"]').trigger('click')
    await wrapper.get('[data-test="mapping-operation-1"]').setValue('default')
    await wrapper.get('[data-test="mapping-target-1"]').setValue('image_size')
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('false')

    await wrapper.get('[data-test="mapping-target-1"]').setValue('n')
    await wrapper.get('[data-test="mapping-default-value-1"]').setValue('{invalid')
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('false')

    await wrapper.get('[data-test="mapping-default-value-1"]').setValue('1')
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('true')
    expect(JSON.parse(wrapper.get('[data-test="mapping-value"]').text())).toEqual({
      rules: [
        { operation: 'rename', source: 'size', target: 'image_size' },
        { operation: 'default', target: 'n', value: 1 }
      ]
    })
  })

  it('按服务端规则拒绝 copy 同路径、父子路径冲突和 envelope/body 混用', async () => {
    const wrapper = mountHarness({
      rules: [{ operation: 'copy', source: 'size', target: 'size' }]
    })
    await wrapper.vm.$nextTick()

    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('false')
    expect(wrapper.text()).toContain('samePath')

    await wrapper.get('[data-test="mapping-target-0"]').setValue('input.size')
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('true')

    await wrapper.get('[data-test="mapping-add-rule"]').trigger('click')
    await wrapper.get('[data-test="mapping-operation-1"]').setValue('default')
    await wrapper.get('[data-test="mapping-target-1"]').setValue('input')
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('false')
    expect(wrapper.text()).toContain('pathConflict')

    await wrapper.get('[data-test="mapping-target-1"]').setValue('video.resolution')
    expect(wrapper.get('[data-test="mapping-valid"]').text()).toBe('false')
    expect(wrapper.text()).toContain('mixedPathStyle')
  })

  it('支持调整规则执行顺序', async () => {
    const wrapper = mountHarness({
      rules: [
        { operation: 'copy', source: 'prompt', target: 'input.prompt' },
        { operation: 'default', target: 'n', value: 1 }
      ]
    })

    await wrapper.get('[data-test="mapping-move-down-0"]').trigger('click')

    const mapping = JSON.parse(wrapper.get('[data-test="mapping-value"]').text()) as MediaRequestMapping
    expect(mapping.rules?.map((rule) => rule.operation)).toEqual(['default', 'copy'])
  })

  it('把统一下游样例和当前 mapping 发送到服务端并展示转换结果', async () => {
    const mapping: MediaRequestMapping = {
      rules: [{ operation: 'rename', source: 'size', target: 'image_size' }]
    }
    previewRequestMappingMock.mockResolvedValue({ prompt: 'hello', image_size: '1024x1024' })
    const wrapper = mountHarness(mapping)
    await wrapper.get('[data-test="mapping-sample"]').setValue(JSON.stringify({
      prompt: 'hello',
      size: '1024x1024'
    }))

    await wrapper.get('[data-test="mapping-run-preview"]').trigger('click')
    await flushPromises()

    expect(previewRequestMappingMock).toHaveBeenCalledWith(
      { prompt: 'hello', size: '1024x1024' },
      mapping
    )
    expect(wrapper.get('[data-test="mapping-preview-result"]').text()).toContain('image_size')
  })

  it('在样例非法时不请求服务端，并展示服务端校验错误', async () => {
    const wrapper = mountHarness()
    await wrapper.get('[data-test="mapping-sample"]').setValue('[]')
    await wrapper.get('[data-test="mapping-run-preview"]').trigger('click')
    expect(previewRequestMappingMock).not.toHaveBeenCalled()
    expect(wrapper.get('[data-test="mapping-preview-error"]').text()).toContain('sampleMustBeObject')

    previewRequestMappingMock.mockRejectedValue({ response: { data: { message: 'missing source size' } } })
    await wrapper.get('[data-test="mapping-sample"]').setValue('{}')
    await wrapper.get('[data-test="mapping-run-preview"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-test="mapping-preview-error"]').text()).toBe('missing source size')
  })

  it('编辑规则后不会展示过期的异步预览结果', async () => {
    const deferred = createDeferred<Record<string, unknown>>()
    previewRequestMappingMock.mockReturnValue(deferred.promise)
    const wrapper = mountHarness({
      rules: [{ operation: 'rename', source: 'size', target: 'image_size' }]
    })

    await wrapper.get('[data-test="mapping-run-preview"]').trigger('click')
    await wrapper.get('[data-test="mapping-target-0"]').setValue('dimensions')
    deferred.resolve({ image_size: 'stale' })
    await flushPromises()

    expect(wrapper.find('[data-test="mapping-preview-result"]').exists()).toBe(false)
  })
})

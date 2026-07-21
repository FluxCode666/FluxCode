import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import type { MediaAdapterResolution, MediaModelDefinitionInput } from '@/types'
import MediaModelEditor from '../MediaModelEditor.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const initialValue = (): MediaModelDefinitionInput => ({
  model_id: 'seedance-1.5-pro',
  vendor: 'bytedance',
  media_type: 'video',
  operations: ['text_to_video'],
  constraints: { video_durations: [5, 10] },
  billing_unit: 'second',
  enabled: true,
  aliases: ['seedance'],
})

const readyResolution = (): MediaAdapterResolution => ({
  status: 'ready',
  resolved_adapter: 'volcengine-seedance',
  matched_by: 'family',
  matched_family: 'seedance',
  capabilities: {
    operations: ['text_to_video'],
    sync_upstream: false,
    native_async_upstream: true,
    content_fetch: true,
  },
  reason_code: '',
})

const initialImageValue = (): MediaModelDefinitionInput => ({
  ...initialValue(),
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  aliases: [],
})

function mountVModelHarness(value = initialImageValue()) {
  return mount(defineComponent({
    components: { MediaModelEditor },
    setup() {
      const form = ref(value)
      return { form }
    },
    template: '<MediaModelEditor v-model="form" />',
  }))
}

async function typeOneCharacterAtATime(
  wrapper: ReturnType<typeof mountVModelHarness>,
  selector: string,
  value: string,
) {
  const input = wrapper.get<HTMLInputElement | HTMLTextAreaElement>(selector)
  for (const character of value) {
    await input.setValue(input.element.value + character)
  }
  return input
}

describe('MediaModelEditor', () => {
  it('只编辑业务字段并只读展示系统解析的 Adapter', async () => {
    const wrapper = mount(MediaModelEditor, {
      props: { modelValue: initialValue(), adapterResolution: readyResolution() },
    })

    expect(wrapper.find('[data-test="media-registry-adapter"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="media-registry-async-mode"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="media-adapter-resolution"]').text())
      .toContain('volcengine-seedance')
    expect(wrapper.text()).toContain('seedance')
    await wrapper.get('[data-test="media-registry-aliases"]').setValue('seedance, doubao-video')

    const emitted = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as MediaModelDefinitionInput
    expect(emitted.aliases).toEqual(['seedance', 'doubao-video'])
    expect(emitted).not.toHaveProperty('default_adapter')
    expect(emitted).not.toHaveProperty('default_async_mode')
  })

  it('新建模型时提示保存后由系统解析', () => {
    const wrapper = mount(MediaModelEditor, { props: { modelValue: initialValue() } })

    expect(wrapper.get('[data-test="media-adapter-resolution-pending"]').exists()).toBe(true)
  })

  it('切换媒体类型时重置为相符的默认能力', async () => {
    const wrapper = mount(MediaModelEditor, { props: { modelValue: initialValue() } })

    await wrapper.get('[data-test="media-registry-type-image"]').trigger('click')

    const emitted = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as MediaModelDefinitionInput
    expect(emitted.media_type).toBe('image')
    expect(emitted.operations).toEqual(['text_to_image'])
    expect(emitted.constraints.video_durations).toEqual([])
  })

  it('拒绝重复别名', async () => {
    const wrapper = mount(MediaModelEditor, { props: { modelValue: initialValue() } })

    await wrapper.get('[data-test="media-registry-aliases"]').setValue('seedance, seedance')

    expect(wrapper.get('[data-test="media-registry-validation-errors"]').exists()).toBe(true)
    expect(wrapper.emitted('update:valid')?.at(-1)?.[0]).toBe(false)
  })

  it('清空可选数字约束时规范化为零', async () => {
    const wrapper = mount(MediaModelEditor, { props: { modelValue: initialValue() } })

    await wrapper.get('#media-registry-min-fps').setValue('')

    const emitted = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as MediaModelDefinitionInput
    expect(emitted.constraints.min_fps).toBe(0)
    expect(wrapper.emitted('update:valid')?.at(-1)?.[0]).toBe(true)
  })

  it('拒绝负数和小数形式的整数约束', async () => {
    const wrapper = mount(MediaModelEditor, { props: { modelValue: initialValue() } })

    await wrapper.get('#media-registry-min-fps').setValue('-1')
    expect(wrapper.emitted('update:valid')?.at(-1)?.[0]).toBe(false)

    await wrapper.get('#media-registry-min-fps').setValue('1.5')
    expect(wrapper.emitted('update:valid')?.at(-1)?.[0]).toBe(false)
    expect(wrapper.get('[data-test="media-registry-validation-errors"]').exists()).toBe(true)
  })

  it('标准 v-model 回流时保留列表字段的逐键输入草稿', async () => {
    const wrapper = mountVModelHarness()

    const aliases = await typeOneCharacterAtATime(
      wrapper,
      '[data-test="media-registry-aliases"]',
      'seedance, doubao-video',
    )
    const imageSizes = await typeOneCharacterAtATime(
      wrapper,
      '[data-test="media-registry-image-sizes"]',
      '1024x1024, 1536x1024',
    )

    expect(aliases.element.value).toBe('seedance, doubao-video')
    expect(imageSizes.element.value).toBe('1024x1024, 1536x1024')
    expect(wrapper.getComponent(MediaModelEditor).props('modelValue')).toMatchObject({
      aliases: ['seedance', 'doubao-video'],
      constraints: { image_sizes: ['1024x1024', '1536x1024'] },
    })

    await wrapper.get('[data-test="media-registry-type-video"]').trigger('click')
    const durations = await typeOneCharacterAtATime(
      wrapper,
      '[data-test="media-registry-video-durations"]',
      '5, 10',
    )
    const resolutions = await typeOneCharacterAtATime(
      wrapper,
      '[data-test="media-registry-video-resolutions"]',
      '720p, 1080p',
    )

    expect(durations.element.value).toBe('5, 10')
    expect(resolutions.element.value).toBe('720p, 1080p')
    expect(wrapper.getComponent(MediaModelEditor).props('modelValue')).toMatchObject({
      constraints: {
        video_durations: [5, 10],
        video_resolutions: ['720p', '1080p'],
      },
    })
  })

  it('外部 modelValue 更新仍会重新水合所有列表草稿', async () => {
    const wrapper = mount(MediaModelEditor, { props: { modelValue: initialImageValue() } })
    await wrapper.get('[data-test="media-registry-aliases"]').setValue('draft,')
    await wrapper.get('[data-test="media-registry-image-sizes"]').setValue('512x512,')

    await wrapper.setProps({
      modelValue: {
        ...initialValue(),
        aliases: ['external-one', 'external-two'],
        constraints: {
          video_durations: [6, 12],
          video_resolutions: ['720p', '1080p'],
        },
      },
    })

    expect(wrapper.get<HTMLTextAreaElement>('[data-test="media-registry-aliases"]').element.value)
      .toBe('external-one, external-two')
    expect(wrapper.get<HTMLInputElement>('[data-test="media-registry-video-durations"]').element.value)
      .toBe('6, 12')
    expect(wrapper.get<HTMLInputElement>('[data-test="media-registry-video-resolutions"]').element.value)
      .toBe('720p, 1080p')
  })
})

import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import type { GroupMediaConfig } from '@/types'
import GroupMediaSettings from '../GroupMediaSettings.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const ParentHarness = defineComponent({
  components: { GroupMediaSettings },
  setup() {
    const mediaConfig = ref<GroupMediaConfig>({
      allow_image_generation: true,
      allow_video_generation: false,
      media_cross_platform_enabled: false,
    })

    return { mediaConfig }
  },
  template: `
    <GroupMediaSettings v-model="mediaConfig" />
    <output data-test="media-config">{{ JSON.stringify(mediaConfig) }}</output>
  `,
})

describe('GroupMediaSettings', () => {
  it('在标准父级 v-model 回流中连续更新字段而不丢失先前值', async () => {
    const wrapper = mount(ParentHarness)

    await wrapper.get('[data-test="allow-video-generation"]').setValue(true)
    await wrapper.get('[data-test="media-cross-platform-enabled"]').setValue(true)

    expect(wrapper.get('[data-test="media-config"]').text()).toBe(
      JSON.stringify({
        allow_image_generation: true,
        allow_video_generation: true,
        media_cross_platform_enabled: true,
      }),
    )
  })

  it('中英文提示均明确跨平台开关不改变文本请求的平台边界', () => {
    expect(zh.admin.groups.mediaCrossPlatformHint).toContain('媒体账号候选')
    expect(zh.admin.groups.mediaCrossPlatformHint).toContain('文本请求')
    expect(en.admin.groups.mediaCrossPlatformHint).toContain('media account candidates')
    expect(en.admin.groups.mediaCrossPlatformHint).toContain('text requests')
  })

  it('媒体分组隐藏旧跨平台开关', () => {
    const wrapper = mount(GroupMediaSettings, {
      props: {
        platform: 'media',
        modelValue: {
          allow_image_generation: true,
          allow_video_generation: true,
          media_cross_platform_enabled: false,
        },
      },
    })

    expect(wrapper.find('[data-test="media-cross-platform-enabled"]').exists()).toBe(false)
  })

  it('媒体分组可从 Registry 多选公共模型', async () => {
    const wrapper = mount(GroupMediaSettings, {
      props: {
        platform: 'media',
        modelValue: {
          allow_image_generation: true,
          allow_video_generation: true,
          media_cross_platform_enabled: false,
        },
        selectedModelIds: ['seedance'],
        availableModels: [
          {
            id: 1,
            model_id: 'seedance',
            vendor: 'bytedance',
            media_type: 'video',
            operations: ['text_to_video'],
            constraints: {},
            billing_unit: 'second',
            default_adapter: 'volcengine-seedance',
            default_async_mode: 'required',
            enabled: true,
            aliases: [],
          },
          {
            id: 2,
            model_id: 'grok-image',
            vendor: 'xai',
            media_type: 'image',
            operations: ['text_to_image'],
            constraints: {},
            billing_unit: 'image',
            default_adapter: 'xai-image',
            default_async_mode: 'unsupported',
            enabled: true,
            aliases: [],
          },
        ],
      },
    })

    await wrapper.get('[data-test="media-model-scope-grok-image"]').setValue(true)

    expect(wrapper.emitted('update:selectedModelIds')?.at(-1)?.[0]).toEqual([
      'seedance',
      'grok-image',
    ])
  })
})

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
})

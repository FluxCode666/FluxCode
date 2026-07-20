import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { AdminGroup } from '@/types'
import GroupSelector from '../GroupSelector.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const group = (overrides: Partial<AdminGroup> = {}): AdminGroup => ({
  id: 1,
  name: 'group',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  system_prompt: '',
  system_prompt_mode: 'inherit',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  is_fallback_group: false,
  fallback_group_id_on_invalid_request: null,
  allow_image_generation: false,
  allow_video_generation: false,
  media_cross_platform_enabled: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  sort_order: 0,
  ...overrides,
})

const mountSelector = (props: {
  platform: 'gemini' | 'codex2api' | 'antigravity' | 'media'
  groups: AdminGroup[]
  mixedScheduling?: boolean
}) =>
  mount(GroupSelector, {
    props: {
      modelValue: [],
      ...props,
    },
    global: {
      stubs: {
        GroupBadge: {
          props: ['name'],
          template: '<span>{{ name }}</span>',
        },
      },
    },
  })

describe('GroupSelector media account candidates', () => {
  it('媒体账号只显示媒体分组，文本账号也不会误选媒体分组', () => {
    const groups = [
      group({ id: 1, name: 'media-only', platform: 'media' }),
      group({ id: 2, name: 'openai-text', platform: 'openai' }),
      group({ id: 3, name: 'legacy-cross', platform: 'gemini', media_cross_platform_enabled: true }),
    ]

    const mediaWrapper = mountSelector({ platform: 'media', groups })
    expect(mediaWrapper.text()).toContain('media-only')
    expect(mediaWrapper.text()).not.toContain('openai-text')
    expect(mediaWrapper.text()).not.toContain('legacy-cross')

    const textWrapper = mountSelector({ platform: 'gemini', groups })
    expect(textWrapper.text()).not.toContain('media-only')
  })

  it('为账号管理显示同平台和显式开启的跨平台媒体分组', () => {
    const wrapper = mountSelector({
      platform: 'gemini',
      groups: [
        group({ id: 1, name: 'gemini', platform: 'gemini' }),
        group({
          id: 2,
          name: 'xai-media',
          platform: 'openai',
          media_cross_platform_enabled: true,
        }),
        group({ id: 3, name: 'openai-text', platform: 'openai' }),
      ],
    })

    expect(wrapper.text()).toContain('gemini')
    expect(wrapper.text()).toContain('xai-media')
    expect(wrapper.text()).not.toContain('openai-text')
  })

  it('保留 codex2api 到 openai 的同平台兼容映射', () => {
    const wrapper = mountSelector({
      platform: 'codex2api',
      groups: [
        group({ id: 1, name: 'openai-compatible', platform: 'openai' }),
        group({ id: 2, name: 'gemini-text', platform: 'gemini' }),
      ],
    })

    expect(wrapper.text()).toContain('openai-compatible')
    expect(wrapper.text()).not.toContain('gemini-text')
  })

  it('保留 Antigravity Mixed Scheduling 候选并叠加媒体跨平台候选', () => {
    const wrapper = mountSelector({
      platform: 'antigravity',
      mixedScheduling: true,
      groups: [
        group({ id: 1, name: 'antigravity', platform: 'antigravity' }),
        group({ id: 2, name: 'anthropic-mixed', platform: 'anthropic' }),
        group({ id: 3, name: 'gemini-mixed', platform: 'gemini' }),
        group({
          id: 4,
          name: 'openai-media',
          platform: 'openai',
          media_cross_platform_enabled: true,
        }),
        group({ id: 5, name: 'openai-text', platform: 'openai' }),
      ],
    })

    expect(wrapper.text()).toContain('antigravity')
    expect(wrapper.text()).toContain('anthropic-mixed')
    expect(wrapper.text()).toContain('gemini-mixed')
    expect(wrapper.text()).toContain('openai-media')
    expect(wrapper.text()).not.toContain('openai-text')
  })
})

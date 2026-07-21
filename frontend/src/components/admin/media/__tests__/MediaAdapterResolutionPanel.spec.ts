import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { MediaAdapterResolution } from '@/types'
import MediaAdapterResolutionPanel from '../MediaAdapterResolutionPanel.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('MediaAdapterResolutionPanel', () => {
  it('未保存的新模型显示由系统解析的等待状态', () => {
    const wrapper = mount(MediaAdapterResolutionPanel, { props: { resolution: null } })

    expect(wrapper.get('[data-test="media-adapter-resolution-pending"]').text())
      .toContain('admin.mediaModels.resolution.pending')
  })

  it('显示 family 命中的 Adapter 和全部代码能力', () => {
    const resolution = {
      status: 'ready',
      resolved_adapter: 'xai-image',
      matched_by: 'family',
      matched_family: 'grok-image',
      capabilities: {
        operations: ['text_to_image'],
        sync_upstream: true,
        native_async_upstream: false,
        content_fetch: true,
      },
      reason_code: '',
    } satisfies MediaAdapterResolution

    const wrapper = mount(MediaAdapterResolutionPanel, { props: { resolution } })

    expect(wrapper.get('[data-test="media-adapter-resolution"]').text()).toContain('xai-image')
    expect(wrapper.get('[data-status="ready"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('grok-image')
    expect(wrapper.findAll('li')).toHaveLength(3)
  })

  it('保留能力不匹配状态、实际能力和稳定原因码', () => {
    const resolution = {
      status: 'capability_mismatch',
      resolved_adapter: 'xai-image',
      matched_by: 'exact',
      matched_family: '',
      capabilities: {
        operations: ['text_to_image'],
        sync_upstream: true,
        native_async_upstream: false,
        content_fetch: false,
      },
      reason_code: 'MEDIA_ADAPTER_CAPABILITY_MISMATCH',
    } satisfies MediaAdapterResolution

    const wrapper = mount(MediaAdapterResolutionPanel, { props: { resolution } })

    expect(wrapper.get('[data-status="capability_mismatch"]').exists()).toBe(true)
    expect(wrapper.get('[role="status"]').text())
      .toContain('admin.mediaModels.resolution.reason.MEDIA_ADAPTER_CAPABILITY_MISMATCH')
    expect(wrapper.find('li').exists()).toBe(true)
  })
})

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'

import HelpTooltip from '../HelpTooltip.vue'

describe('HelpTooltip', () => {
  it('使用高于下拉面板的层级显示提示弹窗', async () => {
    const wrapper = mount(HelpTooltip, {
      props: {
        content: '说明',
      },
      attachTo: document.body,
    })

    await wrapper.trigger('mouseenter')
    await nextTick()

    expect(document.body.innerHTML).toContain('z-[100000030]')

    wrapper.unmount()
  })
})

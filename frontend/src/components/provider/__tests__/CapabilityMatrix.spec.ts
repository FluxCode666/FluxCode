import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import CapabilityMatrix from '../CapabilityMatrix.vue'

describe('CapabilityMatrix', () => {
  it('只展示后端返回的真实原生和转换路由', () => {
    const wrapper = mount(CapabilityMatrix, {
      props: {
        rows: [
          {
            provider_id: 1,
            provider_name: 'NewAPI',
            logical_model: 'deepseek-chat',
            ingress_protocol: 'chat_completions',
            upstream_protocol: 'chat_completions',
            tier: 'native',
            group_priority: 10
          },
          {
            provider_id: 1,
            provider_name: 'NewAPI',
            logical_model: 'deepseek-chat',
            ingress_protocol: 'responses',
            upstream_protocol: 'chat_completions',
            tier: 'conversion',
            adapter: 'responses_to_chat_completions',
            group_priority: 10
          }
        ]
      }
    })

    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('chat_completions')
    expect(rows[0].text()).toContain('native')
    expect(rows[1].text()).toContain('responses')
    expect(rows[1].text()).toContain('conversion')
  })
})

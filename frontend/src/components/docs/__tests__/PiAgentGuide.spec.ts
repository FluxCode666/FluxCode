import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import PiAgentGuide from '../PiAgentGuide.vue'

const CodeBlockStub = {
  props: ['code', 'label'],
  template: '<pre :data-label="label">{{ code }}</pre>'
}

function mountGuide() {
  return mount(PiAgentGuide, {
    props: {
      siteName: 'FluxCode 测试站',
      gatewayBaseUrl: 'https://gateway.example.com',
      apiEndpoint: 'https://gateway.example.com/v1',
      suggestedModelId: 'gpt-5.6-terra'
    },
    global: {
      stubs: {
        CodeBlock: CodeBlockStub
      }
    }
  })
}

describe('PiAgentGuide', () => {
  it('生成使用当前站点、端点和模型的最小 Pi 配置', () => {
    const wrapper = mountGuide()
    const configBlock = wrapper.get('pre[data-label="~/.pi/agent/models.json"]')
    const config = JSON.parse(configBlock.text())

    expect(wrapper.text()).toContain('@earendil-works/pi-coding-agent')
    expect(config.providers.fluxcode).toMatchObject({
      name: 'FluxCode 测试站',
      baseUrl: 'https://gateway.example.com/v1',
      api: 'openai-completions',
      apiKey: '$FLUXCODE_API_KEY'
    })
    expect(config.providers.fluxcode.models).toEqual([
      { id: 'gpt-5.6-terra', name: 'gpt-5.6-terra' }
    ])
  })

  it('按操作系统切换 API Key 环境变量命令', async () => {
    const wrapper = mountGuide()

    expect(wrapper.text()).toContain("$env:FLUXCODE_API_KEY = 'sk-粘贴你的 API Key'")
    await wrapper.findAll('[role="tab"]')[1].trigger('click')

    expect(wrapper.text()).toContain("export FLUXCODE_API_KEY='sk-粘贴你的 API Key'")
    expect(wrapper.findAll('[role="tab"]')[1].attributes('aria-selected')).toBe('true')
  })
})

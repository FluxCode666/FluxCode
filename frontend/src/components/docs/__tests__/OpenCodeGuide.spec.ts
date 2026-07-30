import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import OpenCodeGuide from '../OpenCodeGuide.vue'

const CodeBlockStub = {
  props: ['code', 'label'],
  template: '<pre :data-label="label">{{ code }}</pre>'
}

function mountGuide() {
  return mount(OpenCodeGuide, {
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

describe('OpenCodeGuide', () => {
  it('生成使用当前端点、模型和环境变量密钥的 OpenCode 配置', () => {
    const wrapper = mountGuide()
    const configBlock = wrapper.get('pre[data-label="~/.config/opencode/opencode.json"]')
    const config = JSON.parse(configBlock.text())

    expect(wrapper.text()).toContain('npm install -g opencode-ai@latest')
    expect(config).toMatchObject({
      $schema: 'https://opencode.ai/config.json',
      model: 'openai/gpt-5.6-terra'
    })
    expect(config.provider.openai.options).toEqual({
      baseURL: 'https://gateway.example.com/v1',
      apiKey: '{env:FLUXCODE_API_KEY}'
    })
    expect(config.provider.openai.models).toEqual({
      'gpt-5.6-terra': {
        name: 'gpt-5.6-terra',
        options: { store: false }
      }
    })
  })

  it('按操作系统切换环境变量命令与配置路径', async () => {
    const wrapper = mountGuide()

    expect(wrapper.text()).toContain("export FLUXCODE_API_KEY='sk-粘贴你的 API Key'")

    await wrapper.findAll('[role="tab"]')[1].trigger('click')

    expect(wrapper.text()).toContain('$env:FLUXCODE_API_KEY = "sk-粘贴你的 API Key"')
    expect(wrapper.findAll('pre').some((block) => block.attributes('data-label') === '%USERPROFILE%\\.config\\opencode\\opencode.json')).toBe(true)
    expect(wrapper.findAll('[role="tab"]')[1].attributes('aria-selected')).toBe('true')
  })
})

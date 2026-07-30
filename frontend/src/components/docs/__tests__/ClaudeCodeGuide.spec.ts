import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import ClaudeCodeGuide from '../ClaudeCodeGuide.vue'

const CodeBlockStub = {
  props: ['code', 'label'],
  template: '<pre :data-label="label">{{ code }}</pre>'
}

function mountGuide() {
  return mount(ClaudeCodeGuide, {
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

describe('ClaudeCodeGuide', () => {
  it('生成使用网关根地址与 Anthropic 认证令牌的配置', () => {
    const wrapper = mountGuide()
    const environmentBlock = wrapper.findAll('pre').find((block) => block.text().includes('ANTHROPIC_BASE_URL'))
    const settingsBlock = wrapper.get('pre[data-label="~/.claude/settings.json"]')
    const settings = JSON.parse(settingsBlock.text())

    expect(wrapper.text()).toContain('@anthropic-ai/claude-code')
    expect(environmentBlock?.text()).toContain('export ANTHROPIC_BASE_URL="https://gateway.example.com"')
    expect(environmentBlock?.text()).not.toContain('https://gateway.example.com/v1')
    expect(settings.env).toMatchObject({
      ANTHROPIC_BASE_URL: 'https://gateway.example.com',
      ANTHROPIC_AUTH_TOKEN: '粘贴你的 API Key',
      CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: '1',
      CLAUDE_CODE_ATTRIBUTION_HEADER: '0'
    })
  })

  it('按终端类型切换 Claude Code 环境变量命令与配置路径', async () => {
    const wrapper = mountGuide()

    await wrapper.findAll('[role="tab"]')[1].trigger('click')

    expect(wrapper.text()).toContain('set ANTHROPIC_BASE_URL=https://gateway.example.com')
    expect(wrapper.findAll('pre').some((block) => block.attributes('data-label') === '%USERPROFILE%\\.claude\\settings.json')).toBe(true)

    await wrapper.findAll('[role="tab"]')[2].trigger('click')

    expect(wrapper.text()).toContain('$env:ANTHROPIC_AUTH_TOKEN="粘贴你的 API Key"')
    expect(wrapper.findAll('[role="tab"]')[2].attributes('aria-selected')).toBe('true')
  })
})

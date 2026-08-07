import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'

const registry = vi.hoisted(() => {
  const CodexGuide = {
    props: ['siteName', 'gatewayBaseUrl', 'apiEndpoint', 'suggestedModelId'],
    template: '<div data-testid="codex-guide">{{ siteName }}|{{ gatewayBaseUrl }}|{{ apiEndpoint }}|{{ suggestedModelId }}</div>'
  }
  const PiAgentGuide = {
    props: ['siteName', 'gatewayBaseUrl', 'apiEndpoint', 'suggestedModelId'],
    template: '<div data-testid="pi-guide">{{ siteName }}|{{ gatewayBaseUrl }}|{{ apiEndpoint }}|{{ suggestedModelId }}</div>'
  }
  const ClaudeCodeGuide = {
    props: ['siteName', 'gatewayBaseUrl', 'apiEndpoint', 'suggestedModelId'],
    template: '<div data-testid="claude-code-guide">Claude Code|{{ siteName }}|{{ gatewayBaseUrl }}|{{ apiEndpoint }}</div>'
  }
  const OpenCodeGuide = {
    props: ['siteName', 'gatewayBaseUrl', 'apiEndpoint', 'suggestedModelId'],
    template: '<div data-testid="opencode-guide">OpenCode|{{ siteName }}|{{ gatewayBaseUrl }}|{{ apiEndpoint }}|{{ suggestedModelId }}</div>'
  }
  const clientDocs = [
    {
      id: 'codex',
      name: 'OpenAI Codex CLI',
      shortName: 'Codex',
      description: 'Codex 说明',
      protocol: 'Responses API',
      icon: '⌘',
      component: CodexGuide
    },
    {
      id: 'pi-agent',
      name: 'Pi Agent',
      shortName: 'Pi Agent',
      description: 'Pi 说明',
      protocol: 'Chat Completions',
      icon: 'π',
      component: PiAgentGuide
    },
    {
      id: 'claude-code',
      name: 'Claude Code CLI',
      shortName: 'Claude Code',
      description: 'Claude Code 说明',
      protocol: 'Anthropic Messages',
      icon: '◆',
      component: ClaudeCodeGuide
    },
    {
      id: 'opencode',
      name: 'OpenCode',
      shortName: 'OpenCode',
      description: 'OpenCode 说明',
      protocol: 'OpenAI Compatible',
      icon: '◈',
      component: OpenCodeGuide
    }
  ]

  return { clientDocs }
})

const { fetchPublicSettings } = vi.hoisted(() => ({
  fetchPublicSettings: vi.fn()
}))

vi.mock('@/docs/clientRegistry', () => ({
  clientDocs: registry.clientDocs,
  findClientDoc: (id: string | undefined) => registry.clientDocs.find((client) => client.id === id)
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: 'FluxCode 测试站',
    siteLogo: '',
    apiBaseUrl: 'https://gateway.example.com/api/v1',
    openaiUseKeyModelId: '',
    fetchPublicSettings
  })
}))

vi.mock('@/utils/openaiUseKeyModel', () => ({
  resolveOpenAIUseKeyModelId: () => 'gpt-5.6-terra'
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => (key === 'home.sections.docsTitle' ? '使用文档' : key)
    })
  }
})

import DocsView from '../DocsView.vue'

async function mountAt(path: string) {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: '/docs/:clientId?',
        name: 'Docs',
        component: { template: '<div />' }
      },
      {
        path: '/legal/:document',
        component: { template: '<div />' }
      }
    ]
  })

  await router.push(path)
  await router.isReady()

  const wrapper = mount(DocsView, {
    global: {
      plugins: [router],
      stubs: {
        PublicHeader: true
      }
    }
  })

  await flushPromises()
  return { wrapper, router }
}

describe('DocsView', () => {
  beforeEach(() => {
    fetchPublicSettings.mockReset()
  })

  it('保留 /docs 的默认 Codex 页面，并生成客户端入口', async () => {
    const { wrapper } = await mountAt('/docs')

    expect(wrapper.get('[data-testid="client-guide"]').text()).toContain(
      'FluxCode 测试站|https://gateway.example.com|https://gateway.example.com/v1|gpt-5.6-terra'
    )
    expect(wrapper.find('a[href="/docs/pi-agent"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/docs/claude-code"]').exists()).toBe(true)
    expect(wrapper.find('a[href="/docs/opencode"]').exists()).toBe(true)
    expect(fetchPublicSettings).toHaveBeenCalledTimes(1)
  })

  it('根据路由参数展示 Pi Agent 指南', async () => {
    const { wrapper } = await mountAt('/docs/pi-agent')

    expect(wrapper.get('[data-testid="client-guide"]').text()).toContain('https://gateway.example.com/v1')
  })

  it('根据路由参数展示 Claude Code 和 OpenCode 指南', async () => {
    const claude = await mountAt('/docs/claude-code')
    expect(claude.wrapper.get('[data-testid="client-guide"]').text()).toContain('Claude Code|FluxCode 测试站|https://gateway.example.com|https://gateway.example.com/v1')

    const opencode = await mountAt('/docs/opencode')
    expect(opencode.wrapper.get('[data-testid="client-guide"]').text()).toContain('OpenCode|FluxCode 测试站|https://gateway.example.com|https://gateway.example.com/v1|gpt-5.6-terra')
  })

  it('对未知客户端显示明确的恢复入口', async () => {
    const { wrapper } = await mountAt('/docs/not-supported')

    expect(wrapper.get('[data-testid="unknown-client-guide"]').text()).toContain('not-supported')
    expect(wrapper.get('[data-testid="unknown-client-guide"] a').attributes('href')).toBe('/docs/codex')
  })
})

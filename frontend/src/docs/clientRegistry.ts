import type { Component } from 'vue'
import ClaudeCodeGuide from '@/components/docs/ClaudeCodeGuide.vue'
import CodexGuide from '@/components/docs/CodexGuide.vue'
import OpenCodeGuide from '@/components/docs/OpenCodeGuide.vue'
import PiAgentGuide from '@/components/docs/PiAgentGuide.vue'

export type ClientDoc = {
  id: string
  name: string
  shortName: string
  description: string
  protocol: string
  icon: string
  component: Component
}

/**
 * 客户端配置文档注册表。
 * 新增客户端时，添加一个指南组件并在这里注册即可；DocsView 会自动生成卡片、侧边栏和直达地址。
 */
export const clientDocs: ClientDoc[] = [
  {
    id: 'codex',
    name: 'OpenAI Codex CLI',
    shortName: 'Codex',
    description: '使用 Responses API 配置 Codex 的本地模型提供商与认证信息。',
    protocol: 'Responses API',
    icon: '⌘',
    component: CodexGuide
  },
  {
    id: 'pi-agent',
    name: 'Pi Agent',
    shortName: 'Pi Agent',
    description: '通过 models.json 和环境变量接入 OpenAI Chat Completions。',
    protocol: 'Chat Completions',
    icon: 'π',
    component: PiAgentGuide
  },
  {
    id: 'claude-code',
    name: 'Claude Code CLI',
    shortName: 'Claude Code',
    description: '通过 Anthropic Messages 协议设置 Claude Code 的网关、认证与全局配置。',
    protocol: 'Anthropic Messages',
    icon: '◆',
    component: ClaudeCodeGuide
  },
  {
    id: 'opencode',
    name: 'OpenCode',
    shortName: 'OpenCode',
    description: '使用 OpenAI Compatible provider 与环境变量接入 OpenCode。',
    protocol: 'OpenAI Compatible',
    icon: '◈',
    component: OpenCodeGuide
  }
]

export function findClientDoc(id: string | undefined): ClientDoc | undefined {
  return clientDocs.find((client) => client.id === id)
}

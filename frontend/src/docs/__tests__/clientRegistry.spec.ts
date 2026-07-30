import { describe, expect, it } from 'vitest'
import { clientDocs, findClientDoc } from '../clientRegistry'

describe('clientRegistry', () => {
  it('注册 Claude Code 与 OpenCode 配置指南', () => {
    expect(clientDocs.map((client) => client.id)).toEqual(expect.arrayContaining(['claude-code', 'opencode']))
    expect(findClientDoc('claude-code')).toMatchObject({
      name: 'Claude Code CLI',
      protocol: 'Anthropic Messages'
    })
    expect(findClientDoc('opencode')).toMatchObject({
      name: 'OpenCode',
      protocol: 'OpenAI Compatible'
    })
  })
})

import { describe, expect, it } from 'vitest'

import {
  buildChatboxDeepLink,
  buildCherryStudioDeepLink
} from '../clientImportDeepLink'
import { DEFAULT_CCSWITCH_CLAUDE_MODEL_ID } from '../ccswitchDeepLink'

function decodeConfig(link: string, param: 'data' | 'config') {
  const encoded = new URL(link).searchParams.get(param)
  if (!encoded) throw new Error(`Missing ${param} parameter`)

  const binary = atob(encoded)
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0))
  return JSON.parse(new TextDecoder().decode(bytes))
}

const baseOptions = {
  name: '测试站点',
  baseUrl: 'https://flux.example/api/v1/',
  apiKey: 'sk-test'
}

describe('buildCherryStudioDeepLink', () => {
  it('imports an OpenAI provider with UTF-8 metadata and a normalized v1 endpoint', () => {
    const link = buildCherryStudioDeepLink({
      ...baseOptions,
      providerType: 'openai'
    })
    const config = decodeConfig(link, 'data')

    expect(link.startsWith('cherrystudio://providers/api-keys?')).toBe(true)
    expect(config).toMatchObject({
      name: '测试站点 (OpenAI)',
      type: 'openai',
      baseUrl: 'https://flux.example/api/v1',
      apiKey: 'sk-test'
    })
    expect(config.id).toMatch(/^fluxcode-openai-/)
  })

  it('uses the native Gemini v1beta endpoint for Antigravity imports', () => {
    const config = decodeConfig(buildCherryStudioDeepLink({
      ...baseOptions,
      baseUrl: 'https://flux.example/api/antigravity',
      providerType: 'gemini'
    }), 'data')

    expect(config.baseUrl).toBe('https://flux.example/api/antigravity/v1beta')
    expect(config.type).toBe('gemini')
  })
})

describe('buildChatboxDeepLink', () => {
  it('imports a custom OpenAI provider with the configured default model', () => {
    const config = decodeConfig(buildChatboxDeepLink({
      ...baseOptions,
      providerType: 'openai',
      openaiModelId: 'gpt-5.5-pro'
    }), 'config')

    expect(config).toMatchObject({
      isCustom: true,
      name: '测试站点 (OpenAI)',
      type: 'openai',
      settings: {
        apiHost: 'https://flux.example/api/v1',
        apiKey: 'sk-test',
        models: [{ modelId: 'gpt-5.5-pro', nickname: 'gpt-5.5-pro' }]
      }
    })
    expect(config.id).toMatch(/^custom-provider-fluxcode-openai-/)
  })

  it('imports a custom Anthropic provider with a usable default model', () => {
    const config = decodeConfig(buildChatboxDeepLink({
      ...baseOptions,
      providerType: 'anthropic'
    }), 'config')

    expect(config.settings.apiHost).toBe('https://flux.example/api/v1')
    expect(config.settings.models[0].modelId).toBe(DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
  })

  it('updates Chatbox built-in Gemini settings without duplicating v1beta', () => {
    const config = decodeConfig(buildChatboxDeepLink({
      ...baseOptions,
      baseUrl: 'https://flux.example/api/antigravity/v1beta',
      providerType: 'gemini'
    }), 'config')

    expect(config).toEqual({
      id: 'gemini',
      settings: {
        apiHost: 'https://flux.example/api/antigravity',
        apiKey: 'sk-test'
      }
    })
  })
})

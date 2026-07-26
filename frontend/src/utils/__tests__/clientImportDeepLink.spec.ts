import { describe, expect, it, vi } from 'vitest'

import {
  buildChatboxDeepLink,
  buildCherryStudioDeepLink,
  fetchClientImportModelIds
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

describe('fetchClientImportModelIds', () => {
  it('loads and deduplicates OpenAI-compatible models with bearer authentication', async () => {
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        data: [
          { id: 'gpt-5.5-pro' },
          { id: 'gpt-5.5-mini' },
          { id: 'gpt-5.5-pro' }
        ]
      })
    })

    const modelIds = await fetchClientImportModelIds({
      ...baseOptions,
      providerType: 'openai'
    }, fetcher as unknown as typeof fetch)

    expect(modelIds).toEqual(['gpt-5.5-pro', 'gpt-5.5-mini'])
    expect(fetcher).toHaveBeenCalledWith(
      'https://flux.example/api/v1/models',
      expect.objectContaining({
        method: 'GET',
        headers: {
          Accept: 'application/json',
          Authorization: 'Bearer sk-test'
        }
      })
    )
  })

  it('normalizes Gemini model names and uses Google API key authentication', async () => {
    const fetcher = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        models: [
          { name: 'models/gemini-2.5-pro' },
          { name: 'models/gemini-2.5-flash' }
        ]
      })
    })

    const modelIds = await fetchClientImportModelIds({
      ...baseOptions,
      baseUrl: 'https://flux.example/api/antigravity/v1beta',
      providerType: 'gemini'
    }, fetcher as unknown as typeof fetch)

    expect(modelIds).toEqual(['gemini-2.5-pro', 'gemini-2.5-flash'])
    expect(fetcher).toHaveBeenCalledWith(
      'https://flux.example/api/antigravity/v1beta/models',
      expect.objectContaining({
        headers: {
          Accept: 'application/json',
          'x-goog-api-key': 'sk-test'
        }
      })
    )
  })
})

describe('buildCherryStudioDeepLink', () => {
  it('imports an OpenAI provider with UTF-8 metadata and the available model list', () => {
    const link = buildCherryStudioDeepLink({
      ...baseOptions,
      providerType: 'openai',
      modelIds: ['gpt-5.5-pro', 'gpt-5.5-mini', 'gpt-5.5-pro']
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
    expect(config.models).toEqual([
      {
        id: 'gpt-5.5-pro',
        provider: config.id,
        name: 'gpt-5.5-pro',
        group: '测试站点 (OpenAI)'
      },
      {
        id: 'gpt-5.5-mini',
        provider: config.id,
        name: 'gpt-5.5-mini',
        group: '测试站点 (OpenAI)'
      }
    ])
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
  it('imports a custom OpenAI provider with all available models', () => {
    const config = decodeConfig(buildChatboxDeepLink({
      ...baseOptions,
      providerType: 'openai',
      modelIds: ['gpt-5.5-pro', 'gpt-5.5-mini']
    }), 'config')

    expect(config).toMatchObject({
      isCustom: true,
      name: '测试站点 (OpenAI)',
      type: 'openai',
      settings: {
        apiHost: 'https://flux.example/api/v1',
        apiKey: 'sk-test',
        models: [
          { modelId: 'gpt-5.5-pro', nickname: 'gpt-5.5-pro' },
          { modelId: 'gpt-5.5-mini', nickname: 'gpt-5.5-mini' }
        ]
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
      providerType: 'gemini',
      modelIds: ['gemini-2.5-pro', 'models/gemini-2.5-flash']
    }), 'config')

    expect(config).toEqual({
      id: 'gemini',
      settings: {
        apiHost: 'https://flux.example/api/antigravity',
        apiKey: 'sk-test',
        models: [
          { modelId: 'gemini-2.5-pro', nickname: 'gemini-2.5-pro' },
          { modelId: 'gemini-2.5-flash', nickname: 'gemini-2.5-flash' }
        ]
      }
    })
  })
})

import { describe, expect, it } from 'vitest'

import {
  DEFAULT_CCSWITCH_CLAUDE_MODEL_ID,
  DEFAULT_CCSWITCH_CODEX_MODEL_ID,
  buildCcswitchProviderDeepLink
} from '../ccswitchDeepLink'

function paramsFor(link: string) {
  return new URL(link).searchParams
}

const baseOptions = {
  name: 'FluxCode',
  homepage: 'https://flux.example',
  endpoint: 'https://flux.example',
  apiKey: 'sk-test',
  usageScript: '({ request: { url: "{{baseUrl}}/v1/usage" } })'
}

describe('buildCcswitchProviderDeepLink', () => {
  it('adds GPT-5.6 Sol as the default Codex model', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'codex',
      openaiModelId: '   '
    }))

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('model')).toBe(DEFAULT_CCSWITCH_CODEX_MODEL_ID)
    expect(params.has('opusModel')).toBe(false)
  })

  it('migrates the previous OpenAI default model for Codex imports', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'codex',
      openaiModelId: 'gpt-5.5'
    }))

    expect(params.get('model')).toBe(DEFAULT_CCSWITCH_CODEX_MODEL_ID)
  })

  it('uses configured OpenAI model id for Codex imports', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'codex',
      openaiModelId: 'gpt-5.5-pro'
    }))

    expect(params.get('model')).toBe('gpt-5.5-pro')
  })

  it('adds Claude Opus 4.7 as the Claude default and opus model', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'claude'
    }))

    expect(params.get('model')).toBe(DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
    expect(params.get('opusModel')).toBe(DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
  })

  it('does not add model fields for Gemini imports', () => {
    const params = paramsFor(buildCcswitchProviderDeepLink({
      ...baseOptions,
      app: 'gemini'
    }))

    expect(params.has('model')).toBe(false)
    expect(params.has('opusModel')).toBe(false)
  })
})

import { resolveOpenAIUseKeyModelId } from './openaiUseKeyModel'

export type CcswitchProviderApp = 'claude' | 'codex' | 'gemini'

export const DEFAULT_CCSWITCH_CLAUDE_MODEL_ID = 'claude-opus-4-7'

interface BuildCcswitchProviderDeepLinkOptions {
  app: CcswitchProviderApp
  name: string
  homepage: string
  endpoint: string
  apiKey: string
  usageScript: string
  openaiModelId?: string | null
}

export function buildCcswitchProviderDeepLink(
  options: BuildCcswitchProviderDeepLinkOptions
): string {
  const params = new URLSearchParams({
    resource: 'provider',
    app: options.app,
    name: options.name,
    homepage: options.homepage,
    endpoint: options.endpoint,
    apiKey: options.apiKey,
    configFormat: 'json',
    usageEnabled: 'true',
    usageScript: btoa(options.usageScript),
    usageAutoInterval: '30'
  })

  if (options.app === 'codex') {
    params.set('model', resolveOpenAIUseKeyModelId(options.openaiModelId))
  }

  if (options.app === 'claude') {
    params.set('model', DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
    params.set('opusModel', DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
  }

  return `ccswitch://v1/import?${params.toString()}`
}

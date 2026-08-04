import {
  DEFAULT_OPENAI_USE_KEY_MODEL_ID,
  resolveOpenAIUseKeyModelId
} from './openaiUseKeyModel'

export type CcswitchProviderApp = 'claude' | 'codex' | 'gemini'

export const DEFAULT_CCSWITCH_CLAUDE_MODEL_ID = 'claude-opus-4-7'
export const DEFAULT_CCSWITCH_CODEX_MODEL_ID = 'gpt-5.6-sol'

function resolveCcswitchCodexModelId(raw?: string | null): string {
  const modelId = resolveOpenAIUseKeyModelId(raw)
  return modelId === DEFAULT_OPENAI_USE_KEY_MODEL_ID ? DEFAULT_CCSWITCH_CODEX_MODEL_ID : modelId
}

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
    params.set('model', resolveCcswitchCodexModelId(options.openaiModelId))
  }

  if (options.app === 'claude') {
    params.set('model', DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
    params.set('opusModel', DEFAULT_CCSWITCH_CLAUDE_MODEL_ID)
  }

  return `ccswitch://v1/import?${params.toString()}`
}

export function openCcswitchDeepLink(deepLink: string): void {
  if (!deepLink.startsWith('ccswitch://')) {
    throw new TypeError('Invalid CC-Switch deep link')
  }

  const link = document.createElement('a')
  link.href = deepLink
  link.tabIndex = -1
  link.setAttribute('aria-hidden', 'true')
  link.style.display = 'none'
  document.body.appendChild(link)

  try {
    link.click()
  } finally {
    link.remove()
  }
}

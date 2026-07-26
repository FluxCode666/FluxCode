import { DEFAULT_CCSWITCH_CLAUDE_MODEL_ID } from './ccswitchDeepLink'
import { resolveOpenAIUseKeyModelId } from './openaiUseKeyModel'

export type ClientImportProviderType = 'openai' | 'anthropic' | 'gemini'

const DEFAULT_CLIENT_GEMINI_MODEL_ID = 'gemini-2.5-flash'

interface BuildClientImportDeepLinkOptions {
  name: string
  baseUrl: string
  apiKey: string
  providerType: ClientImportProviderType
  openaiModelId?: string | null
}

interface ChatboxModelConfig {
  modelId: string
  nickname: string
}

const providerLabels: Record<ClientImportProviderType, string> = {
  openai: 'OpenAI',
  anthropic: 'Claude',
  gemini: 'Gemini'
}

function encodeUtf8Base64(value: unknown): string {
  const bytes = new TextEncoder().encode(JSON.stringify(value))
  let binary = ''

  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }

  return btoa(binary)
}

function trimApiVersion(baseUrl: string): string {
  return baseUrl.trim().replace(/\/(?:v1beta|v1)\/?$/i, '').replace(/\/+$/, '')
}

function appendApiVersion(baseUrl: string, version: 'v1' | 'v1beta'): string {
  return `${trimApiVersion(baseUrl)}/${version}`
}

function hashProviderIdentity(value: string): string {
  let hash = 2166136261

  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index)
    hash = Math.imul(hash, 16777619)
  }

  return (hash >>> 0).toString(36)
}

function buildProviderIdentity(options: BuildClientImportDeepLinkOptions): string {
  return `${options.providerType}-${hashProviderIdentity(trimApiVersion(options.baseUrl).toLowerCase())}`
}

function buildProviderName(options: BuildClientImportDeepLinkOptions): string {
  const name = options.name.trim() || 'FluxCode'
  return `${name} (${providerLabels[options.providerType]})`
}

function resolveDefaultModel(options: BuildClientImportDeepLinkOptions): ChatboxModelConfig {
  switch (options.providerType) {
    case 'openai': {
      const modelId = resolveOpenAIUseKeyModelId(options.openaiModelId)
      return { modelId, nickname: modelId }
    }
    case 'gemini':
      return { modelId: DEFAULT_CLIENT_GEMINI_MODEL_ID, nickname: 'Gemini 2.5 Flash' }
    default:
      return { modelId: DEFAULT_CCSWITCH_CLAUDE_MODEL_ID, nickname: 'Claude Opus 4.7' }
  }
}

export function buildCherryStudioDeepLink(options: BuildClientImportDeepLinkOptions): string {
  const baseUrl = appendApiVersion(
    options.baseUrl,
    options.providerType === 'gemini' ? 'v1beta' : 'v1'
  )
  const config = {
    id: `fluxcode-${buildProviderIdentity(options)}`,
    name: buildProviderName(options),
    type: options.providerType,
    baseUrl,
    apiKey: options.apiKey
  }
  const params = new URLSearchParams({
    v: '1',
    data: encodeUtf8Base64(config)
  })

  return `cherrystudio://providers/api-keys?${params.toString()}`
}

export function buildChatboxDeepLink(options: BuildClientImportDeepLinkOptions): string {
  const config = options.providerType === 'gemini'
    ? {
        id: 'gemini',
        settings: {
          apiHost: trimApiVersion(options.baseUrl),
          apiKey: options.apiKey
        }
      }
    : {
        isCustom: true,
        id: `custom-provider-fluxcode-${buildProviderIdentity(options)}`,
        name: buildProviderName(options),
        type: options.providerType,
        settings: {
          apiHost: appendApiVersion(options.baseUrl, 'v1'),
          apiKey: options.apiKey,
          models: [resolveDefaultModel(options)]
        }
      }
  const params = new URLSearchParams({
    config: encodeUtf8Base64(config)
  })

  return `chatbox://provider/import?${params.toString()}`
}

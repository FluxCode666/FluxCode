import { DEFAULT_CCSWITCH_CLAUDE_MODEL_ID } from './ccswitchDeepLink'
import { resolveOpenAIUseKeyModelId } from './openaiUseKeyModel'

export type ClientImportProviderType = 'openai' | 'anthropic' | 'gemini'

const DEFAULT_CLIENT_GEMINI_MODEL_ID = 'gemini-2.5-flash'

export interface ClientImportDeepLinkOptions {
  name: string
  baseUrl: string
  apiKey: string
  providerType: ClientImportProviderType
  openaiModelId?: string | null
  modelIds?: readonly string[]
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

function buildProviderIdentity(options: ClientImportDeepLinkOptions): string {
  return `${options.providerType}-${hashProviderIdentity(trimApiVersion(options.baseUrl).toLowerCase())}`
}

function buildProviderName(options: ClientImportDeepLinkOptions): string {
  const name = options.name.trim() || 'FluxCode'
  return `${name} (${providerLabels[options.providerType]})`
}

function resolveDefaultModel(options: ClientImportDeepLinkOptions): ChatboxModelConfig {
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

function normalizeModelId(modelId: string): string {
  return modelId.trim().replace(/^models\//i, '')
}

function resolveModelIds(options: ClientImportDeepLinkOptions): string[] {
  const modelIds = Array.from(new Set(
    (options.modelIds ?? [])
      .map(normalizeModelId)
      .filter(Boolean)
  ))

  return modelIds.length > 0 ? modelIds : [resolveDefaultModel(options).modelId]
}

function resolveChatboxModels(options: ClientImportDeepLinkOptions): ChatboxModelConfig[] {
  return resolveModelIds(options).map(modelId => ({ modelId, nickname: modelId }))
}

function extractModelList(payload: unknown): unknown[] {
  if (!payload || typeof payload !== 'object') return []

  const response = payload as { data?: unknown; models?: unknown }
  if (Array.isArray(response.data)) return response.data
  if (Array.isArray(response.models)) return response.models
  return []
}

function extractModelId(model: unknown): string {
  if (typeof model === 'string') return normalizeModelId(model)
  if (!model || typeof model !== 'object') return ''

  const candidate = model as { id?: unknown; name?: unknown; model?: unknown }
  const rawModelId = [candidate.id, candidate.name, candidate.model]
    .find(value => typeof value === 'string')

  return typeof rawModelId === 'string' ? normalizeModelId(rawModelId) : ''
}

export async function fetchClientImportModelIds(
  options: Pick<ClientImportDeepLinkOptions, 'baseUrl' | 'apiKey' | 'providerType'>,
  fetcher: typeof fetch = fetch,
  timeoutMs = 10000
): Promise<string[]> {
  const apiVersion = options.providerType === 'gemini' ? 'v1beta' : 'v1'
  const endpoint = `${appendApiVersion(options.baseUrl, apiVersion)}/models`
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeoutMs)

  try {
    const headers: Record<string, string> = { Accept: 'application/json' }
    if (options.providerType === 'gemini') {
      headers['x-goog-api-key'] = options.apiKey
    } else {
      headers.Authorization = `Bearer ${options.apiKey}`
    }

    const response = await fetcher(endpoint, {
      method: 'GET',
      headers,
      signal: controller.signal
    })
    if (!response.ok) {
      throw new Error(`Model list request failed with status ${response.status}`)
    }

    const payload: unknown = await response.json()
    const modelIds = Array.from(new Set(extractModelList(payload).map(extractModelId).filter(Boolean)))
    if (modelIds.length === 0) {
      throw new Error('Model list response is empty')
    }

    return modelIds
  } finally {
    clearTimeout(timeoutId)
  }
}

export function buildCherryStudioDeepLink(options: ClientImportDeepLinkOptions): string {
  const baseUrl = appendApiVersion(
    options.baseUrl,
    options.providerType === 'gemini' ? 'v1beta' : 'v1'
  )
  const providerId = `fluxcode-${buildProviderIdentity(options)}`
  const providerName = buildProviderName(options)
  const config = {
    id: providerId,
    name: providerName,
    type: options.providerType,
    baseUrl,
    apiKey: options.apiKey,
    models: resolveModelIds(options).map(modelId => ({
      id: modelId,
      provider: providerId,
      name: modelId,
      group: providerName
    }))
  }
  const params = new URLSearchParams({
    v: '1',
    data: encodeUtf8Base64(config)
  })

  return `cherrystudio://providers/api-keys?${params.toString()}`
}

export function buildChatboxDeepLink(options: ClientImportDeepLinkOptions): string {
  const models = resolveChatboxModels(options)
  const config = options.providerType === 'gemini'
    ? {
        id: 'gemini',
        settings: {
          apiHost: trimApiVersion(options.baseUrl),
          apiKey: options.apiKey,
          models
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
          models
        }
      }
  const params = new URLSearchParams({
    config: encodeUtf8Base64(config)
  })

  return `chatbox://provider/import?${params.toString()}`
}

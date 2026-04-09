export const DEFAULT_OPENAI_USE_KEY_MODEL_ID = 'gpt-5.3-codex'

export function resolveOpenAIUseKeyModelId(raw?: string | null): string {
  const trimmed = (raw || '').trim()
  return trimmed || DEFAULT_OPENAI_USE_KEY_MODEL_ID
}

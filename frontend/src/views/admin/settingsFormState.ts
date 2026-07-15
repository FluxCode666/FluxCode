import type {
  MediaGenerationSettings,
  SystemSettings,
  UpdateSettingsRequest,
} from '@/api/admin/settings'

export type NullableMediaGenerationSettings = {
  [K in keyof MediaGenerationSettings]?: MediaGenerationSettings[K] | null
}

export function createDefaultMediaGenerationSettings(): MediaGenerationSettings {
  return {
    media_sync_wait_timeout_seconds: 240,
    media_sync_timeout_fallback_async_enabled: false,
    media_sync_timeout_billing_policy: 'penalty',
    media_sync_timeout_penalty_ratio: 0.8,
    media_video_storage_mode: 'hybrid',
    media_video_proxy_fallback_enabled: true,
  }
}

export function resolveMediaGenerationSettingsForForm(
  current: MediaGenerationSettings,
  response: NullableMediaGenerationSettings
): MediaGenerationSettings {
  return {
    media_sync_wait_timeout_seconds:
      response.media_sync_wait_timeout_seconds ?? current.media_sync_wait_timeout_seconds,
    media_sync_timeout_fallback_async_enabled:
      response.media_sync_timeout_fallback_async_enabled ??
      current.media_sync_timeout_fallback_async_enabled,
    media_sync_timeout_billing_policy:
      response.media_sync_timeout_billing_policy ?? current.media_sync_timeout_billing_policy,
    media_sync_timeout_penalty_ratio:
      response.media_sync_timeout_penalty_ratio ?? current.media_sync_timeout_penalty_ratio,
    media_video_storage_mode:
      response.media_video_storage_mode ?? current.media_video_storage_mode,
    media_video_proxy_fallback_enabled:
      response.media_video_proxy_fallback_enabled ?? current.media_video_proxy_fallback_enabled,
  }
}

export function buildMediaGenerationSettingsUpdate(
  form: MediaGenerationSettings
): MediaGenerationSettings {
  return {
    media_sync_wait_timeout_seconds: form.media_sync_wait_timeout_seconds,
    media_sync_timeout_fallback_async_enabled: form.media_sync_timeout_fallback_async_enabled,
    media_sync_timeout_billing_policy: form.media_sync_timeout_billing_policy,
    media_sync_timeout_penalty_ratio: form.media_sync_timeout_penalty_ratio,
    media_video_storage_mode: form.media_video_storage_mode,
    media_video_proxy_fallback_enabled: form.media_video_proxy_fallback_enabled,
  }
}

type CodexCLISettingKey = 'codex_cli_user_agent' | 'codex_cli_version'

const codexCLISettingKeys: CodexCLISettingKey[] = [
  'codex_cli_user_agent',
  'codex_cli_version',
]

export function applyDefinedSettingsToForm(form: object, settings: object): void {
  const target = form as Record<string, unknown>
  for (const [key, value] of Object.entries(settings)) {
    if (value !== null && value !== undefined) {
      target[key] = value
    }
  }
}

export function resolveSettingsUpdateForForm(
  updated: SystemSettings,
  payload: UpdateSettingsRequest
): SystemSettings {
  const result = { ...updated }

  for (const key of codexCLISettingKeys) {
    preserveSubmittedStringWhenResponseIsEmpty(result, payload, key)
  }

  return result
}

function preserveSubmittedStringWhenResponseIsEmpty(
  target: SystemSettings,
  payload: UpdateSettingsRequest,
  key: CodexCLISettingKey
): void {
  const submitted = payload[key]
  if (typeof submitted !== 'string') return

  const trimmedSubmitted = submitted.trim()
  if (!trimmedSubmitted) return

  const returned = target[key]
  if (typeof returned !== 'string' || returned.trim() === '') {
    target[key] = trimmedSubmitted
  }
}

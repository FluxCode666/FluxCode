import type { SystemSettings, UpdateSettingsRequest } from '@/api/admin/settings'

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

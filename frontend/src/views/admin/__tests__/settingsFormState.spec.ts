import { describe, expect, it } from 'vitest'

import type { SystemSettings, UpdateSettingsRequest } from '@/api/admin/settings'
import { resolveSettingsUpdateForForm } from '../settingsFormState'

describe('settings form state helpers', () => {
  it('keeps submitted Codex CLI values when the update response returns empty strings', () => {
    const updated = {
      codex_cli_user_agent: '',
      codex_cli_version: '',
    } as SystemSettings
    const payload: UpdateSettingsRequest = {
      codex_cli_user_agent: ' codex_cli_rs/9.8.7 ',
      codex_cli_version: ' 9.8.7 ',
    }

    const result = resolveSettingsUpdateForForm(updated, payload)

    expect(result.codex_cli_user_agent).toBe('codex_cli_rs/9.8.7')
    expect(result.codex_cli_version).toBe('9.8.7')
  })

  it('allows clearing Codex CLI values from the settings form', () => {
    const updated = {
      codex_cli_user_agent: '',
      codex_cli_version: '',
    } as SystemSettings
    const payload: UpdateSettingsRequest = {
      codex_cli_user_agent: '',
      codex_cli_version: '',
    }

    const result = resolveSettingsUpdateForForm(updated, payload)

    expect(result.codex_cli_user_agent).toBe('')
    expect(result.codex_cli_version).toBe('')
  })

  it('keeps non-empty server-normalized Codex CLI values', () => {
    const updated = {
      codex_cli_user_agent: 'codex_cli_rs/server',
      codex_cli_version: 'server',
    } as SystemSettings
    const payload: UpdateSettingsRequest = {
      codex_cli_user_agent: 'codex_cli_rs/client',
      codex_cli_version: 'client',
    }

    const result = resolveSettingsUpdateForForm(updated, payload)

    expect(result.codex_cli_user_agent).toBe('codex_cli_rs/server')
    expect(result.codex_cli_version).toBe('server')
  })
})

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

import type { SystemSettings, UpdateSettingsRequest } from '@/api/admin/settings'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import { applyDefinedSettingsToForm, resolveSettingsUpdateForForm } from '../settingsFormState'

const settingsViewSource = readFileSync(resolve(process.cwd(), 'src/views/admin/SettingsView.vue'), 'utf8')

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

  it('applies media zero and false values without replacing missing media defaults', () => {
    const form = {
      media_sync_wait_timeout_seconds: 240,
      media_sync_timeout_fallback_async_enabled: true,
      media_sync_timeout_penalty_ratio: 0.8,
    }

    applyDefinedSettingsToForm(form, {
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: false,
    })

    expect(form).toEqual({
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: false,
      media_sync_timeout_penalty_ratio: 0.8,
    })
  })

  it('wires all six media settings through the controlled card and explicit save payload', () => {
    const mediaFields = [
      'media_sync_wait_timeout_seconds',
      'media_sync_timeout_fallback_async_enabled',
      'media_sync_timeout_billing_policy',
      'media_sync_timeout_penalty_ratio',
      'media_video_storage_mode',
      'media_video_proxy_fallback_enabled',
    ]

    expect(settingsViewSource).toContain("{ key: 'media' as SettingsTab")
    expect(settingsViewSource).toContain('activeTab === \'media\'')
    expect(settingsViewSource).toContain('<MediaGenerationSettingsCard v-model="mediaGenerationSettings" />')
    for (const field of mediaFields) {
      expect(settingsViewSource).toContain(`${field}: form.${field}`)
    }
  })

  it('limits timeout penalty copy to synchronous waits submitted upstream', () => {
    expect(zh.admin.settings.mediaGeneration.timeoutBillingPolicyHint).toContain(
      '同步等待超时且已提交上游'
    )
    expect(en.admin.settings.mediaGeneration.timeoutBillingPolicyHint).toContain(
      'synchronous waiting times out after the request has already been submitted upstream'
    )
    expect(zh.admin.settings.mediaGeneration.timeoutBillingPolicyHint).toContain(
      '不适用于队列、Worker 或存储错误'
    )
    expect(en.admin.settings.mediaGeneration.timeoutBillingPolicyHint).toContain(
      'does not apply to queue, Worker, or storage errors'
    )
  })
})

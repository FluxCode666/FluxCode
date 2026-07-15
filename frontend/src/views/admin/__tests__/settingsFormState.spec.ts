import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

import type {
  MediaGenerationSettings,
  SystemSettings,
  UpdateSettingsRequest,
} from '@/api/admin/settings'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import {
  buildMediaGenerationSettingsUpdate,
  createDefaultMediaGenerationSettings,
  resolveMediaGenerationSettingsForForm,
  resolveSettingsUpdateForForm,
} from '../settingsFormState'

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

  it('creates the backend-aligned media defaults', () => {
    expect(createDefaultMediaGenerationSettings()).toEqual({
      media_sync_wait_timeout_seconds: 240,
      media_sync_timeout_fallback_async_enabled: false,
      media_sync_timeout_billing_policy: 'penalty',
      media_sync_timeout_penalty_ratio: 0.8,
      media_video_storage_mode: 'hybrid',
      media_video_proxy_fallback_enabled: true,
    })
  })

  it('loads all six media fields from a complete response', () => {
    const response: MediaGenerationSettings = {
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: true,
      media_sync_timeout_billing_policy: 'refund',
      media_sync_timeout_penalty_ratio: 0.35,
      media_video_storage_mode: 'hybrid',
      media_video_proxy_fallback_enabled: false,
    }

    expect(
      resolveMediaGenerationSettingsForForm(createDefaultMediaGenerationSettings(), response)
    ).toEqual(response)
  })

  it('keeps every current media field when its response value is missing or null', () => {
    const current: MediaGenerationSettings = {
      media_sync_wait_timeout_seconds: 17,
      media_sync_timeout_fallback_async_enabled: true,
      media_sync_timeout_billing_policy: 'refund',
      media_sync_timeout_penalty_ratio: 0.25,
      media_video_storage_mode: 'hybrid',
      media_video_proxy_fallback_enabled: false,
    }
    const fields = Object.keys(current) as Array<keyof MediaGenerationSettings>

    for (const field of fields) {
      expect(resolveMediaGenerationSettingsForForm(current, {})[field]).toBe(current[field])
      expect(resolveMediaGenerationSettingsForForm(current, { [field]: null })[field]).toBe(
        current[field]
      )
    }
  })

  it('loads media zero and false values instead of treating them as missing', () => {
    const current: MediaGenerationSettings = {
      media_sync_wait_timeout_seconds: 240,
      media_sync_timeout_fallback_async_enabled: true,
      media_sync_timeout_billing_policy: 'penalty',
      media_sync_timeout_penalty_ratio: 0.8,
      media_video_storage_mode: 'hybrid',
      media_video_proxy_fallback_enabled: true,
    }

    expect(
      resolveMediaGenerationSettingsForForm(current, {
        media_sync_wait_timeout_seconds: 0,
        media_sync_timeout_fallback_async_enabled: false,
        media_sync_timeout_penalty_ratio: 0,
        media_video_proxy_fallback_enabled: false,
      })
    ).toEqual({
      ...current,
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: false,
      media_sync_timeout_penalty_ratio: 0,
      media_video_proxy_fallback_enabled: false,
    })
  })

  it('builds an exact six-field media update payload from the form', () => {
    const fields: Array<keyof MediaGenerationSettings> = [
      'media_sync_wait_timeout_seconds',
      'media_sync_timeout_fallback_async_enabled',
      'media_sync_timeout_billing_policy',
      'media_sync_timeout_penalty_ratio',
      'media_video_storage_mode',
      'media_video_proxy_fallback_enabled',
    ]
    const form = {
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: true,
      media_sync_timeout_billing_policy: 'refund' as const,
      media_sync_timeout_penalty_ratio: 0.35,
      media_video_storage_mode: 'hybrid' as const,
      media_video_proxy_fallback_enabled: false,
      unrelated_field: 'must not be forwarded',
    }

    const payload = buildMediaGenerationSettingsUpdate(form)

    expect(Object.keys(payload)).toEqual(fields)
    expect(payload).toEqual({
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: true,
      media_sync_timeout_billing_policy: 'refund',
      media_sync_timeout_penalty_ratio: 0.35,
      media_video_storage_mode: 'hybrid',
      media_video_proxy_fallback_enabled: false,
    })
  })

  it('wires the controlled card and production media mappers into SettingsView', () => {
    expect(settingsViewSource).toContain("{ key: 'media' as SettingsTab")
    expect(settingsViewSource).toContain('activeTab === \'media\'')
    expect(settingsViewSource).toContain('<MediaGenerationSettingsCard v-model="mediaGenerationSettings" />')
    expect(settingsViewSource).toContain('...createDefaultMediaGenerationSettings()')
    expect(settingsViewSource.match(/buildMediaGenerationSettingsUpdate\(form\)/g)).toHaveLength(1)
    expect(settingsViewSource).toContain('resolveMediaGenerationSettingsForForm(form, settings)')
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

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const viewSource = readFileSync(resolve(root, 'views/admin/SettingsView.vue'), 'utf8')
const settingsApiSource = readFileSync(resolve(root, 'api/admin/settings.ts'), 'utf8')

describe('SettingsView successful request payload recording', () => {
  it('binds the dynamic switch and body limit through API types, form state, and save payload', () => {
    expect(settingsApiSource).toContain('successful_request_records_enabled: boolean')
    expect(settingsApiSource).toContain('successful_request_records_max_body_bytes: number')
    expect(settingsApiSource).toContain('successful_request_records_enabled?: boolean')
    expect(settingsApiSource).toContain('successful_request_records_max_body_bytes?: number')
    expect(viewSource).toContain('v-model="form.successful_request_records_enabled"')
    expect(viewSource).toContain('successful_request_records_enabled: false')
    expect(viewSource).toContain(
      'successful_request_records_enabled: form.successful_request_records_enabled'
    )
    expect(viewSource).toContain(
      'successful_request_records_max_body_bytes: form.successful_request_records_max_body_bytes'
    )
  })

  it('requires the encryption key and only displays the body limit while enabled', () => {
    expect(viewSource).toContain(
      ':disabled="!form.totp_encryption_key_configured && !form.successful_request_records_enabled"'
    )
    expect(viewSource).toContain('v-if="form.successful_request_records_enabled"')
    expect(viewSource).toContain('v-model.number="successfulRequestRecordsMaxBodyKB"')
    expect(viewSource).toContain('successfulRequestRecordsMaxBodyBytes < 1024')
    expect(viewSource).toContain('successfulRequestRecordsMaxBodyBytes > 16 * 1024 * 1024')
  })

  it('provides Chinese and English sensitivity warnings', () => {
    expect(zh.admin.settings.successfulRequestRecords.sensitiveWarning).toContain('当前不做脱敏')
    expect(en.admin.settings.successfulRequestRecords.sensitiveWarning).toContain('without redaction')
  })
})

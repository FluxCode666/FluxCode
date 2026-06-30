import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const settingsViewSource = readFileSync(
  resolve(__dirname, '../SettingsView.vue'),
  'utf8'
)

describe('SettingsView generated image storage settings', () => {
  it('renders generated image storage source controls and Qiniu fields', () => {
    expect(settingsViewSource).toContain('data-test="generated-image-storage"')
    expect(settingsViewSource).toContain('generated_image_storage_source')
    expect(settingsViewSource).toContain('generated_image_storage_config_source')
    expect(settingsViewSource).toContain('qiniu_access_key')
    expect(settingsViewSource).toContain('qiniu_secret_key')
    expect(settingsViewSource).toContain('qiniu_secret_key_configured')
    expect(settingsViewSource).toContain('qiniu_bucket')
    expect(settingsViewSource).toContain('qiniu_cdn_domain')
    expect(settingsViewSource).toContain('qiniu_prefix')
    expect(settingsViewSource).toContain('qiniu_use_https')
    expect(settingsViewSource).toContain('qiniu_upload_timeout_seconds')
    expect(settingsViewSource).toContain('qiniu_token_ttl_seconds')
  })

  it('provides Chinese and English copy for DB and Qiniu data sources', () => {
    expect(zh.admin.settings.generatedImageStorage.title).toBe('生图存储')
    expect(zh.admin.settings.generatedImageStorage.configSource).toBe('配置数据源')
    expect(zh.admin.settings.generatedImageStorage.useSource).toBe('使用数据源')
    expect(zh.admin.settings.generatedImageStorage.sources.db).toBe('DB')
    expect(zh.admin.settings.generatedImageStorage.sources.qiniu).toBe('七牛云')
    expect(en.admin.settings.generatedImageStorage.title).toBe('Generated Image Storage')
    expect(en.admin.settings.generatedImageStorage.configSource).toBe('Configuration source')
    expect(en.admin.settings.generatedImageStorage.useSource).toBe('Active source')
    expect(en.admin.settings.generatedImageStorage.sources.db).toBe('DB')
    expect(en.admin.settings.generatedImageStorage.sources.qiniu).toBe('Qiniu Cloud')
  })
})

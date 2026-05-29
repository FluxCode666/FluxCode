import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const settingsViewSource = readFileSync(
  resolve(__dirname, '../SettingsView.vue'),
  'utf8'
)

describe('SettingsView system prompt placement', () => {
  it('exposes platform system prompt configuration as its own settings tab', () => {
    expect(settingsViewSource).toContain("'systemPrompt'")
    expect(settingsViewSource).toContain("key: 'systemPrompt'")
    expect(settingsViewSource).toContain("activeTab === 'systemPrompt'")
    expect(zh.admin.settings.tabs.systemPrompt).toBe('系统提示词')
  })

  it('explains platform system prompt priority in system settings', () => {
    expect(zh.admin.settings.systemPrompt.description).toContain('API Key > 分组 > 系统配置')
    expect(zh.admin.settings.systemPrompt.platformDescription).not.toContain('API Key > 分组 > 系统配置')
    expect(en.admin.settings.systemPrompt.description).toContain('API Key > Group > System Settings')
    expect(en.admin.settings.systemPrompt.platformDescription).not.toContain('API Key > Group > System Settings')
  })

  it('renders platform names as titles without per-platform subtitles', () => {
    expect(settingsViewSource).toContain(':title="platform.label"')
    expect(settingsViewSource).not.toContain("admin.settings.systemPrompt.platformDescription")
    expect(settingsViewSource).toContain(":mode-label=\"t('systemPrompt.modeLabel')\"")
    expect(settingsViewSource).toContain(":prompt-label=\"t('systemPrompt.promptLabel')\"")
  })
})

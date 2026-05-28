import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

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
})

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const viewSource = readFileSync(resolve(root, 'views/admin/SettingsView.vue'), 'utf8')
const settingsApiSource = readFileSync(resolve(root, 'api/admin/settings.ts'), 'utf8')

describe('SettingsView Codex image generation bridge setting', () => {
  it('binds the global bridge switch into API types, form state, and save payload', () => {
    expect(settingsApiSource).toContain('codex_image_generation_bridge_enabled: boolean')
    expect(settingsApiSource).toContain('codex_image_generation_bridge_enabled?: boolean')
    expect(viewSource).toContain('v-model="form.codex_image_generation_bridge_enabled"')
    expect(viewSource).toContain('codex_image_generation_bridge_enabled: false')
    expect(viewSource).toContain(
      'codex_image_generation_bridge_enabled: form.codex_image_generation_bridge_enabled'
    )
  })

  it('provides Chinese and English copy for the bridge switch', () => {
    expect(zh.admin.settings.codexCLIUA.imageGenerationBridge).toBe('Codex 生图 Bridge')
    expect(en.admin.settings.codexCLIUA.imageGenerationBridge).toBe('Codex image generation bridge')
  })
})

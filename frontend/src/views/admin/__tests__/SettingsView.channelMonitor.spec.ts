import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../SettingsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('SettingsView channel monitor controls', () => {
  it('binds channel monitor settings into the form and save payload', () => {
    expect(viewSource).toContain("t('admin.settings.channelMonitor.title')")
    expect(viewSource).toContain("t('admin.settings.channelMonitor.description')")
    expect(viewSource).toContain('v-model="form.channel_monitor_enabled"')
    expect(viewSource).toContain('form.channel_monitor_default_interval_seconds')
    expect(viewSource).toContain('channel_monitor_enabled: form.channel_monitor_enabled')
    expect(viewSource).toContain(
      'channel_monitor_default_interval_seconds: form.channel_monitor_default_interval_seconds'
    )
  })

  it('binds OpenAI use-key model setting into the form and save payload', () => {
    expect(viewSource).toContain("t('admin.settings.codexCLIUA.openaiUseKeyModelId')")
    expect(viewSource).toContain('v-model="form.openai_use_key_model_id"')
    expect(viewSource).toContain("openai_use_key_model_id: 'gpt-5.5'")
    expect(viewSource).toContain('openai_use_key_model_id: form.openai_use_key_model_id')
  })
})

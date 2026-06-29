import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../SettingsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('SettingsView dashboard fireworks controls', () => {
  it('binds dashboard fireworks settings into the form and save payload', () => {
    expect(viewSource).toContain("t('admin.settings.dashboardFireworks.title')")
    expect(viewSource).toContain("t('admin.settings.dashboardFireworks.description')")
    expect(viewSource).toContain('v-model="form.dashboard_fireworks_enabled"')
    expect(viewSource).toContain('v-model.number="form.dashboard_fireworks_threshold"')
    expect(viewSource).toContain('dashboard_fireworks_enabled: form.dashboard_fireworks_enabled')
    expect(viewSource).toContain('dashboard_fireworks_threshold: form.dashboard_fireworks_threshold')
  })
})

import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('AppSidebar channel monitor menu', () => {
  it('gates user and admin channel monitor entries behind settings', () => {
    expect(componentSource).toContain('appStore.channelMonitorEnabled')
    expect(componentSource).toContain("path: '/monitor'")
    expect(componentSource).toContain("label: t('nav.channelStatus'")

    expect(componentSource).toContain('adminSettingsStore.channelMonitorEnabled')
    expect(componentSource).toContain("path: '/admin/channels/monitor'")
    expect(componentSource).toContain("label: t('nav.channelMonitor'")
  })
})

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const keysViewSource = readFileSync(resolve(__dirname, '../KeysView.vue'), 'utf8')

describe('KeysView client import menu', () => {
  it('uses one import trigger and exposes all supported clients in a menu', () => {
    const actionsStart = keysViewSource.indexOf('<template #cell-actions')
    const actionsEnd = keysViewSource.indexOf('<!-- Toggle Status Button -->', actionsStart)
    const actionsSource = keysViewSource.slice(actionsStart, actionsEnd)

    expect(actionsSource).toContain('aria-haspopup="menu"')
    expect(actionsSource).toContain("t('common.import')")
    expect(actionsSource.match(/name="upload"/g)).toHaveLength(1)
    expect(actionsSource).not.toContain("@click=\"importToClient(row, 'ccswitch')\"")

    expect(keysViewSource).toContain("selectClientImportTarget('ccswitch')")
    expect(keysViewSource).toContain("selectClientImportTarget('cherryStudio')")
    expect(keysViewSource).toContain("selectClientImportTarget('chatbox')")
    expect(keysViewSource).toContain('await fetchClientImportModelIds(options)')
    expect(keysViewSource).toContain('const importOptions = { ...options, modelIds }')
    expect(keysViewSource).toContain('openCcswitchDeepLink(deeplink)')
  })

  it('supports hover, keyboard navigation, and a teleported unclipped menu', () => {
    expect(keysViewSource).toContain('@mouseenter="openClientImportMenu(row)"')
    expect(keysViewSource).toContain('@keydown.arrow-down.prevent="openClientImportMenu(row, true)"')
    expect(keysViewSource).toContain('@keydown.esc.stop="closeClientImportMenu(true)"')
    expect(keysViewSource).toContain('<Teleport to="body">')
    expect(keysViewSource).toContain('hasClientImportTargets(clientImportMenuRow)')
    expect(keysViewSource).toContain('role="menu"')
    expect(keysViewSource).toContain('role="menuitem"')
    expect(keysViewSource).toContain('focusAdjacentClientImportMenuItem')
  })
})

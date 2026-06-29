import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../DashboardView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('user DashboardView fireworks integration', () => {
  it('renders dashboard fireworks only through the once-per-day trigger helper', () => {
    expect(viewSource).toContain('<DashboardFireworks')
    expect(viewSource).toContain('shouldTriggerDashboardFireworks')
    expect(viewSource).toContain('markDashboardFireworksShown')
    expect(viewSource).toContain('today_actual_cost')
    expect(viewSource).toContain('isMobileDevice')
  })

  it('keeps the fireworks layer mounted long enough for the delayed demo physics arc', () => {
    expect(viewSource).toContain('const DASHBOARD_FIREWORKS_DURATION_MS = 5200')
    expect(viewSource).toContain('window.setTimeout(() => { showDashboardFireworks.value = false }, DASHBOARD_FIREWORKS_DURATION_MS)')
  })
})

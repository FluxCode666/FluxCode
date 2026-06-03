import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../MonitorCardGrid.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('MonitorCardGrid layout', () => {
  it('keeps the console grid on the original fixed breakpoints by default', () => {
    expect(componentSource).toContain('monitor-card-grid')
    expect(componentSource).toContain('grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4')
    expect(componentSource).toContain("variant?: 'console' | 'public'")
  })

  it('supports a bounded public grid for the website status page', () => {
    expect(componentSource).toContain('publicCardGridStyle')
    expect(componentSource).toContain('minmax(min(100%, 300px), 320px)')
  })
})

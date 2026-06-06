import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../MonitorTimeline.vue')
const componentSource = readFileSync(componentPath, 'utf8')

describe('MonitorTimeline layout', () => {
  it('keeps sixty buckets inside the card width', () => {
    expect(componentSource).not.toContain('min-w-[3px]')
    expect(componentSource).toContain('gridTemplateColumns')
    expect(componentSource).toContain('minmax(0, 1fr)')
  })
})

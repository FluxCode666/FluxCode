import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../ChannelsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('ChannelsView capabilities mapping', () => {
  it('threads capabilities through pricing form creation and API mapping', () => {
    expect(viewSource).toContain('capabilities: []')
    expect(viewSource).toContain('capabilities: normalizeCapabilities(entry.capabilities)')
    expect(viewSource).toContain('capabilities: normalizeCapabilities(p.capabilities)')
  })
})

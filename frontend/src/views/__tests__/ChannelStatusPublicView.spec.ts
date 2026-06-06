import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../ChannelStatusPublicView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('ChannelStatusPublicView layout', () => {
  it('uses the bounded public card grid variant', () => {
    expect(viewSource).toContain('<ChannelStatusContent variant="public" />')
  })
})

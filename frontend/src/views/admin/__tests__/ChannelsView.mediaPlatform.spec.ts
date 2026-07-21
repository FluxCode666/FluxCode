import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../ChannelsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('ChannelsView media platform', () => {
  it('exposes media after the existing text platforms', () => {
    expect(viewSource).toContain(
      "const platformOrder: GroupPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'media']"
    )
  })
})

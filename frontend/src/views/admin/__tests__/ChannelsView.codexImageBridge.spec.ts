import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../ChannelsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('ChannelsView Codex image generation bridge override', () => {
  it('binds the OpenAI channel override to features_config', () => {
    expect(viewSource).toContain('data-testid="channel-openai-codex-image-generation-bridge"')
    expect(viewSource).toContain('codex_image_generation_bridge_mode')
    expect(viewSource).toContain('featuresConfig.codex_image_generation_bridge')
    expect(viewSource).toContain('resolveCodexImageGenerationBridgeMode')
  })
})

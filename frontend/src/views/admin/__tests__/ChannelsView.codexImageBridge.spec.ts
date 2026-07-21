import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import {
  formToChannelAPI,
  resolveCodexImageGenerationBridgeMode,
  type PlatformSection,
} from '@/components/admin/channel/pricingMappings'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../ChannelsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('ChannelsView Codex image generation bridge override', () => {
  it('binds the OpenAI channel override control', () => {
    expect(viewSource).toContain('data-testid="channel-openai-codex-image-generation-bridge"')
    expect(viewSource).toContain('v-model="section.codex_image_generation_bridge_mode"')
    expect(viewSource).toContain('formToChannelAPI')
  })

  it('converts the OpenAI override to and from features_config', () => {
    const section: PlatformSection = {
      platform: 'openai',
      enabled: true,
      collapsed: false,
      group_ids: [1],
      model_mapping: {},
      model_pricing: [],
      web_search_emulation: false,
      codex_image_generation_bridge_mode: 'enabled',
      account_stats_pricing_rules: [],
    }

    const payload = formToChannelAPI([section], { preserved: true })

    expect(payload.features_config).toEqual({
      preserved: true,
      codex_image_generation_bridge: { openai: true },
    })
    expect(resolveCodexImageGenerationBridgeMode(
      payload.features_config.codex_image_generation_bridge,
      'openai'
    )).toBe('enabled')
  })
})

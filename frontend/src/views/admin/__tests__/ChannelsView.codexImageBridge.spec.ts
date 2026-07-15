import { describe, expect, it } from 'vitest'

import type { Channel } from '@/api/admin/channels'
import {
  apiToPlatformSections,
  formToChannelAPI,
  resolveCodexImageGenerationBridgeMode,
  type CodexImageGenerationBridgeMode,
  type PlatformSection,
} from '@/components/admin/channel/pricingMappings'
import type { AdminGroup } from '@/types'

const openAIGroup: AdminGroup = {
  id: 1,
  name: 'OpenAI A',
  platform: 'openai',
  sort_order: 1,
  created_at: '',
  updated_at: '',
}

function createOpenAISection(mode: CodexImageGenerationBridgeMode): PlatformSection {
  return {
    platform: 'openai',
    enabled: true,
    collapsed: false,
    group_ids: [openAIGroup.id],
    model_mapping: {},
    model_pricing: [],
    web_search_emulation: false,
    codex_image_generation_bridge_mode: mode,
    account_stats_pricing_rules: [],
  }
}

function createChannel(featuresConfig: Record<string, unknown>): Channel {
  return {
    id: 1,
    name: 'OpenAI channel',
    description: '',
    status: 'active',
    billing_model_source: 'channel_mapped',
    restrict_models: false,
    features_config: featuresConfig,
    group_ids: [openAIGroup.id],
    model_pricing: [],
    model_mapping: {},
    apply_pricing_to_account_stats: false,
    account_stats_pricing_rules: [],
    created_at: '',
    updated_at: '',
  }
}

describe('channel Codex image generation bridge mapping', () => {
  it.each([
    [true, 'enabled'],
    [false, 'disabled'],
    [{ openai: true }, 'enabled'],
    [{ openai: false }, 'disabled'],
    [{ anthropic: true }, 'inherit'],
    [undefined, 'inherit'],
  ] as const)('resolves %j to %s', (value, expectedMode) => {
    expect(resolveCodexImageGenerationBridgeMode(value, 'openai')).toBe(expectedMode)
  })

  it.each([
    ['enabled', true],
    ['disabled', false],
  ] as const)('saves the %s override without dropping other feature flags', (mode, expectedValue) => {
    const payload = formToChannelAPI(
      [createOpenAISection(mode)],
      { preserved_feature: 'preserved' },
    )

    expect(payload.features_config).toEqual({
      preserved_feature: 'preserved',
      codex_image_generation_bridge: { openai: expectedValue },
    })
  })

  it('removes the channel override when the mode inherits the system setting', () => {
    const payload = formToChannelAPI(
      [createOpenAISection('inherit')],
      {
        preserved_feature: 'preserved',
        codex_image_generation_bridge: { openai: true },
      },
    )

    expect(payload.features_config).toEqual({ preserved_feature: 'preserved' })
  })

  it.each([
    [{ codex_image_generation_bridge: { openai: true } }, 'enabled'],
    [{ codex_image_generation_bridge: { openai: false } }, 'disabled'],
    [{}, 'inherit'],
  ] as const)('preserves the %s mode through API-to-form-to-API conversion', (featuresConfig, expectedMode) => {
    const [section] = apiToPlatformSections(
      createChannel(featuresConfig),
      [openAIGroup],
      ['openai'],
    )

    expect(section.codex_image_generation_bridge_mode).toBe(expectedMode)

    const payload = formToChannelAPI([section], featuresConfig)
    if (expectedMode === 'inherit') {
      expect(payload.features_config).not.toHaveProperty('codex_image_generation_bridge')
    } else {
      expect(payload.features_config.codex_image_generation_bridge).toEqual(
        featuresConfig.codex_image_generation_bridge,
      )
    }
  })
})

import { describe, expect, it } from 'vitest'

import type { Channel, AccountStatsPricingRule } from '@/api/admin/channels'
import type { AdminGroup } from '@/types'
import {
  accountStatsRulesToAPI,
  apiToPlatformSections,
  distributeRulesToPlatformSections,
  formToChannelAPI,
  type PlatformSection,
} from '@/components/admin/channel/pricingMappings'

const groups: AdminGroup[] = [
  { id: 1, name: 'Anthropic A', platform: 'anthropic', sort_order: 1, created_at: '', updated_at: '' },
  { id: 2, name: 'OpenAI A', platform: 'openai', sort_order: 2, created_at: '', updated_at: '' },
]

const baseSections: PlatformSection[] = [
  {
    platform: 'anthropic',
    enabled: true,
    collapsed: false,
    group_ids: [1],
    model_mapping: {},
    model_pricing: [
      {
        models: ['claude-sonnet-4'],
        capabilities: ['streaming', 'tools', 'tools'],
        billing_mode: 'token',
        input_price: 3,
        output_price: 15,
        cache_write_price: null,
        cache_read_price: null,
        image_output_price: null,
        per_request_price: null,
        intervals: []
      }
    ],
    web_search_emulation: false,
    codex_image_generation_bridge_mode: 'inherit',
    account_stats_pricing_rules: [
      {
        name: 'rule-a',
        group_ids: [1],
        account_ids: [101],
        pricing: [
          {
            models: ['claude-3-5-haiku'],
            capabilities: ['prompt_cache', 'prompt_cache', 'streaming'],
            billing_mode: 'per_request',
            input_price: null,
            output_price: null,
            cache_write_price: null,
            cache_read_price: null,
            image_output_price: null,
            per_request_price: 0.2,
            intervals: []
          }
        ]
      }
    ]
  }
]

const channelFixture: Channel = {
  id: 9,
  name: 'capability-channel',
  description: 'test',
  status: 'active',
  billing_model_source: 'channel_mapped',
  restrict_models: false,
  features_config: {},
  group_ids: [1],
  model_pricing: [
    {
      platform: 'anthropic',
      models: ['claude-sonnet-4'],
      capabilities: ['streaming', 'tools'],
      billing_mode: 'token',
      input_price: 0.000003,
      output_price: 0.000015,
      cache_write_price: null,
      cache_read_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    }
  ],
  model_mapping: {},
  apply_pricing_to_account_stats: true,
  account_stats_pricing_rules: [
    {
      name: 'rule-a',
      group_ids: [1],
      account_ids: [101],
      pricing: [
        {
          platform: 'anthropic',
          models: ['claude-3-5-haiku'],
          capabilities: ['prompt_cache', 'streaming'],
          billing_mode: 'per_request',
          input_price: null,
          output_price: null,
          cache_write_price: null,
          cache_read_price: null,
          image_output_price: null,
          per_request_price: 0.2,
          intervals: []
        }
      ]
    }
  ],
  created_at: '',
  updated_at: ''
}

describe('ChannelsView capabilities mapping', () => {
  it('round-trips embedding explicit zero input prices and strips unsupported price fields', () => {
    const embeddingGroups: AdminGroup[] = [
      { id: 3, name: 'Embedding A', platform: 'embedding', sort_order: 3, created_at: '', updated_at: '' }
    ]
    const embeddingChannel: Channel = {
      ...channelFixture,
      group_ids: [3],
      model_pricing: [{
        platform: 'embedding',
        models: ['embed-model'],
        capabilities: ['streaming'],
        billing_mode: 'per_request',
        input_price: 0,
        output_price: 0.000015,
        cache_write_price: 0.000001,
        cache_read_price: 0.000002,
        image_output_price: 0.25,
        per_request_price: 0.5,
        intervals: [{
          min_tokens: 0,
          max_tokens: 1000,
          tier_label: 'disabled',
          input_price: 0,
          output_price: 0.000015,
          cache_write_price: 0.000001,
          cache_read_price: 0.000002,
          per_request_price: 0.5,
          sort_order: 0
        }]
      }]
    }

    const sections = apiToPlatformSections(embeddingChannel, embeddingGroups, ['embedding'])
    expect(sections[0].model_pricing[0].input_price).toBe(0)
    expect(sections[0].model_pricing[0].intervals[0].input_price).toBe(0)

    const payload = formToChannelAPI(sections, {})
    expect(payload.model_pricing[0]).toMatchObject({
      platform: 'embedding',
      billing_mode: 'token',
      input_price: 0,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: [{
        input_price: 0,
        output_price: null,
        cache_write_price: null,
        cache_read_price: null,
        per_request_price: null
      }]
    })
  })

  it('includes capabilities in channel-level and account-stats save payloads', () => {
    const channelPayload = formToChannelAPI(baseSections, {})
    const rulesPayload = accountStatsRulesToAPI(baseSections)

    expect(channelPayload.model_pricing).toEqual([
      expect.objectContaining({
        models: ['claude-sonnet-4'],
        capabilities: ['streaming', 'tools']
      })
    ])

    expect(rulesPayload).toEqual([
      expect.objectContaining({
        pricing: [
          expect.objectContaining({
            models: ['claude-3-5-haiku'],
            capabilities: ['prompt_cache', 'streaming']
          })
        ]
      })
    ])
  })

  it('preserves capabilities when converting API payloads back into form sections and distributing rules', () => {
    const sections = apiToPlatformSections(channelFixture, groups, ['anthropic', 'openai'])
    distributeRulesToPlatformSections(sections, channelFixture.account_stats_pricing_rules as AccountStatsPricingRule[], groups)

    expect(sections[0].model_pricing[0].capabilities).toEqual(['streaming', 'tools'])
    expect(sections[0].account_stats_pricing_rules[0].pricing[0].capabilities).toEqual(['prompt_cache', 'streaming'])
  })
})

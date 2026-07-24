import type { AccountStatsPricingRule, Channel, ChannelModelPricing } from '@/api/admin/channels'
import type { AdminGroup, GroupPlatform } from '@/types'

import type { PricingFormEntry } from './types'
import { apiIntervalsToForm, formIntervalsToAPI, mTokToPerToken, normalizeCapabilities, perTokenToMTok } from './types'

export type CodexImageGenerationBridgeMode = 'inherit' | 'enabled' | 'disabled'

export interface FormPricingRule {
  name: string
  group_ids: number[]
  account_ids: number[]
  pricing: PricingFormEntry[]
}

export interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
  web_search_emulation: boolean
  codex_image_generation_bridge_mode: CodexImageGenerationBridgeMode
  account_stats_pricing_rules: FormPricingRule[]
}

export function resolveCodexImageGenerationBridgeMode(
  value: unknown,
  platform: GroupPlatform
): CodexImageGenerationBridgeMode {
  if (typeof value === 'boolean') {
    return value ? 'enabled' : 'disabled'
  }
  if (value && typeof value === 'object') {
    const platformValue = (value as Record<string, unknown>)[platform]
    if (typeof platformValue === 'boolean') {
      return platformValue ? 'enabled' : 'disabled'
    }
  }
  return 'inherit'
}

function pricingEntryToAPI(entry: PricingFormEntry, platform: GroupPlatform): ChannelModelPricing {
  const isEmbedding = platform === 'embedding'
  return {
    platform,
    models: entry.models,
    capabilities: normalizeCapabilities(entry.capabilities),
    billing_mode: isEmbedding ? 'token' : entry.billing_mode,
    input_price: mTokToPerToken(entry.input_price),
    output_price: isEmbedding ? null : mTokToPerToken(entry.output_price),
    cache_write_price: isEmbedding ? null : mTokToPerToken(entry.cache_write_price),
    cache_read_price: isEmbedding ? null : mTokToPerToken(entry.cache_read_price),
    image_output_price: isEmbedding ? null : mTokToPerToken(entry.image_output_price),
    per_request_price: isEmbedding ? null : entry.per_request_price != null && entry.per_request_price !== '' ? Number(entry.per_request_price) : null,
    intervals: formIntervalsToAPI(entry.intervals || []).map(interval => isEmbedding ? {
      ...interval, output_price: null, cache_write_price: null, cache_read_price: null, per_request_price: null
    } : interval)
  }
}

export function accountStatsRulesToAPI(sections: PlatformSection[]): AccountStatsPricingRule[] {
  const rules: AccountStatsPricingRule[] = []
  for (const section of sections) {
    if (!section.enabled) continue
    for (const rule of section.account_stats_pricing_rules) {
      rules.push({
        name: rule.name,
        group_ids: rule.group_ids,
        account_ids: rule.account_ids,
        pricing: rule.pricing
          .filter(p => p.models.length > 0)
          .map(p => pricingEntryToAPI(p, section.platform))
      })
    }
  }
  return rules
}

export function formToChannelAPI(
  sections: PlatformSection[],
  existingFeaturesConfig?: Record<string, unknown>
): { group_ids: number[], model_pricing: ChannelModelPricing[], model_mapping: Record<string, Record<string, string>>, features_config: Record<string, unknown> } {
  const group_ids: number[] = []
  const model_pricing: ChannelModelPricing[] = []
  const model_mapping: Record<string, Record<string, string>> = {}
  const featuresConfig: Record<string, unknown> = existingFeaturesConfig
    ? { ...existingFeaturesConfig }
    : {}

  for (const section of sections) {
    if (!section.enabled) continue
    group_ids.push(...section.group_ids)

    if (Object.keys(section.model_mapping).length > 0) {
      model_mapping[section.platform] = { ...section.model_mapping }
    }

    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      model_pricing.push(pricingEntryToAPI(entry, section.platform))
    }
  }

  const wsEmulation: Record<string, boolean> = {}
  for (const section of sections) {
    if (!section.enabled) continue
    if (section.platform === 'anthropic') {
      wsEmulation[section.platform] = !!section.web_search_emulation
    }
  }
  if (Object.keys(wsEmulation).length > 0) {
    featuresConfig.web_search_emulation = wsEmulation
  } else {
    delete featuresConfig.web_search_emulation
  }

  const codexBridge: Record<string, boolean> = {}
  for (const section of sections) {
    if (!section.enabled || section.platform !== 'openai') continue
    if (section.codex_image_generation_bridge_mode === 'enabled') {
      codexBridge[section.platform] = true
    } else if (section.codex_image_generation_bridge_mode === 'disabled') {
      codexBridge[section.platform] = false
    }
  }
  if (Object.keys(codexBridge).length > 0) {
    featuresConfig.codex_image_generation_bridge = codexBridge
  } else {
    delete featuresConfig.codex_image_generation_bridge
  }

  return { group_ids, model_pricing, model_mapping, features_config: featuresConfig }
}

export function apiToPlatformSections(
  channel: Channel,
  allGroups: AdminGroup[],
  platformOrder: GroupPlatform[]
): PlatformSection[] {
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const g of allGroups) {
    groupPlatformMap.set(g.id, g.platform)
  }

  const activePlatforms = new Set<GroupPlatform>()
  for (const gid of channel.group_ids || []) {
    const p = groupPlatformMap.get(gid)
    if (p) activePlatforms.add(p)
  }
  for (const p of channel.model_pricing || []) {
    if (p.platform) activePlatforms.add(p.platform as GroupPlatform)
  }
  for (const p of Object.keys(channel.model_mapping || {})) {
    if (platformOrder.includes(p as GroupPlatform)) activePlatforms.add(p as GroupPlatform)
  }

  const sections: PlatformSection[] = []
  for (const platform of platformOrder) {
    if (!activePlatforms.has(platform)) continue

    const groupIds = (channel.group_ids || []).filter(gid => groupPlatformMap.get(gid) === platform)
    const mapping = (channel.model_mapping || {})[platform] || {}
    const pricing = (channel.model_pricing || [])
      .filter(p => (p.platform || 'anthropic') === platform)
      .map(p => ({
        models: p.models || [],
        capabilities: normalizeCapabilities(p.capabilities),
        billing_mode: platform === 'embedding' ? 'token' : p.billing_mode,
        input_price: perTokenToMTok(p.input_price),
        output_price: perTokenToMTok(p.output_price),
        cache_write_price: perTokenToMTok(p.cache_write_price),
        cache_read_price: perTokenToMTok(p.cache_read_price),
        image_output_price: perTokenToMTok(p.image_output_price),
        per_request_price: p.per_request_price,
        intervals: apiIntervalsToForm(p.intervals || [])
      } as PricingFormEntry))

    const fc = channel.features_config
    const wsEmulation = fc?.web_search_emulation as Record<string, boolean> | undefined
    const webSearchEnabled = wsEmulation?.[platform] === true
    const codexImageGenerationBridgeMode = resolveCodexImageGenerationBridgeMode(
      fc?.codex_image_generation_bridge,
      platform
    )

    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids: groupIds,
      model_mapping: { ...mapping },
      model_pricing: pricing,
      web_search_emulation: webSearchEnabled,
      codex_image_generation_bridge_mode: codexImageGenerationBridgeMode,
      account_stats_pricing_rules: [],
    })
  }

  return sections
}

export function distributeRulesToPlatformSections(
  sections: PlatformSection[],
  apiRules: AccountStatsPricingRule[],
  allGroups: AdminGroup[]
) {
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const g of allGroups) {
    groupPlatformMap.set(g.id, g.platform)
  }

  for (const apiRule of apiRules) {
    const platforms = new Set<GroupPlatform>()
    for (const gid of apiRule.group_ids || []) {
      const p = groupPlatformMap.get(gid)
      if (p) platforms.add(p)
    }
    if (platforms.size === 0 && apiRule.pricing?.length > 0) {
      const p = apiRule.pricing[0].platform as GroupPlatform | undefined
      if (p) platforms.add(p)
    }
    const targetPlatform = platforms.size >= 1 ? [...platforms][0] : null
    if (!targetPlatform) continue

    const section = sections.find(s => s.platform === targetPlatform)
    if (!section) continue

    const formRule: FormPricingRule = {
      name: apiRule.name || '',
      group_ids: [...(apiRule.group_ids || [])],
      account_ids: [...(apiRule.account_ids || [])],
      pricing: (apiRule.pricing || []).map(p => ({
        models: [...(p.models || [])],
        capabilities: normalizeCapabilities(p.capabilities),
        billing_mode: p.billing_mode,
        input_price: perTokenToMTok(p.input_price),
        output_price: perTokenToMTok(p.output_price),
        cache_write_price: perTokenToMTok(p.cache_write_price),
        cache_read_price: perTokenToMTok(p.cache_read_price),
        image_output_price: perTokenToMTok(p.image_output_price),
        per_request_price: p.per_request_price,
        intervals: apiIntervalsToForm(p.intervals || [])
      } as PricingFormEntry))
    }
    section.account_stats_pricing_rules.push(formRule)
  }
}

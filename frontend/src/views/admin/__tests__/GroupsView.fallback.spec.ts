import { describe, expect, it } from 'vitest'
import type { AdminGroup } from '@/types'
import {
  buildFallbackTargetOptions,
  buildFallbackTargetOptionsForEdit,
  canEnableFallbackGroup,
  isApiKeyBindableGroup,
} from '../GroupsView.fallback'

const baseGroup = (overrides: Partial<AdminGroup>): AdminGroup => ({
  id: 1,
  name: 'group',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  system_prompt: '',
  system_prompt_mode: 'inherit',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  claude_code_only: false,
  fallback_group_id: null,
  fallback_group_id_on_invalid_request: null,
  is_fallback_group: false,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  sort_order: 0,
  ...overrides,
})

describe('group fallback helpers', () => {
  it('only allows openai and anthropic standard groups to enable fallback flag', () => {
    expect(
      canEnableFallbackGroup(
        baseGroup({ platform: 'openai', subscription_type: 'standard' }),
      ),
    ).toBe(true)
    expect(
      canEnableFallbackGroup(
        baseGroup({ platform: 'anthropic', subscription_type: 'standard' }),
      ),
    ).toBe(true)
    expect(
      canEnableFallbackGroup(
        baseGroup({ platform: 'gemini', subscription_type: 'standard' }),
      ),
    ).toBe(false)
    expect(
      canEnableFallbackGroup(
        baseGroup({ platform: 'openai', subscription_type: 'subscription' }),
      ),
    ).toBe(false)
  })

  it('builds same-platform fallback targets only', () => {
    const groups = [
      baseGroup({ id: 1, platform: 'openai', is_fallback_group: false }),
      baseGroup({ id: 2, platform: 'openai', is_fallback_group: true }),
      baseGroup({ id: 3, platform: 'anthropic', is_fallback_group: true }),
      baseGroup({ id: 4, platform: 'openai', is_fallback_group: true, status: 'inactive' }),
      baseGroup({ id: 5, platform: 'openai', is_fallback_group: true, subscription_type: 'subscription' }),
      baseGroup({ id: 6, platform: 'openai', is_fallback_group: true, fallback_group_id: 9 }),
    ]

    expect(
      buildFallbackTargetOptions(
        groups,
        baseGroup({ id: 1, platform: 'openai' }),
      ).map((option) => option.value),
    ).toEqual([null, 2])
  })

  it('recomputes edit fallback targets when platform changes', () => {
    const groups = [
      baseGroup({ id: 1, platform: 'anthropic', is_fallback_group: false }),
      baseGroup({ id: 2, name: 'openai-fallback', platform: 'openai', is_fallback_group: true }),
      baseGroup({ id: 3, name: 'anthropic-fallback', platform: 'anthropic', is_fallback_group: true }),
    ]

    expect(
      buildFallbackTargetOptionsForEdit(groups, 1, 'anthropic').map(
        (option) => option.value,
      ),
    ).toEqual([null, 3])

    expect(
      buildFallbackTargetOptionsForEdit(groups, 1, 'openai').map(
        (option) => option.value,
      ),
    ).toEqual([null, 2])
  })

  it('api key binding excludes fallback groups', () => {
    expect(isApiKeyBindableGroup(baseGroup({ is_fallback_group: false }))).toBe(
      true,
    )
    expect(isApiKeyBindableGroup(baseGroup({ is_fallback_group: true }))).toBe(
      false,
    )
  })
})

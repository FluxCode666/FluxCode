import { describe, expect, it } from 'vitest'

import type { AdminGroup, Group } from '@/types'
import {
  filterAssignableAllowedGroups,
  filterRebindableApiKeyGroups,
  isAllowedGroupAssignable,
  isApiKeyBindableGroup,
} from '../apiKeyGroupSelection'

const baseGroup = (overrides: Partial<Group> = {}): Group => ({
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
  is_fallback_group: false,
  fallback_group_id_on_invalid_request: null,
  require_oauth_only: false,
  require_privacy_set: false,
  created_at: '',
  updated_at: '',
  ...overrides,
})

const baseAdminGroup = (overrides: Partial<AdminGroup> = {}): AdminGroup => ({
  ...baseGroup(),
  model_routing: null,
  model_routing_enabled: false,
  mcp_xml_inject: true,
  sort_order: 0,
  ...overrides,
})

describe('api key group selection helpers', () => {
  it('treats fallback groups as not bindable', () => {
    expect(isApiKeyBindableGroup(baseGroup())).toBe(true)
    expect(isApiKeyBindableGroup(baseGroup({ is_fallback_group: true }))).toBe(
      false,
    )
  })

  it('only allows active standard non-fallback groups for allowed-groups assignment', () => {
    expect(isAllowedGroupAssignable(baseGroup())).toBe(true)
    expect(
      isAllowedGroupAssignable(baseGroup({ status: 'inactive' })),
    ).toBe(false)
    expect(
      isAllowedGroupAssignable(baseGroup({ subscription_type: 'subscription' })),
    ).toBe(false)
    expect(
      isAllowedGroupAssignable(baseGroup({ is_fallback_group: true })),
    ).toBe(false)
  })

  it('filters fallback groups out of admin group lists', () => {
    expect(
      filterAssignableAllowedGroups([
        baseGroup({ id: 1 }),
        baseGroup({ id: 2, is_fallback_group: true }),
        baseGroup({ id: 3, status: 'inactive' }),
      ]).map((group) => group.id),
    ).toEqual([1])

    expect(
      filterRebindableApiKeyGroups([
        baseAdminGroup({ id: 1 }),
        baseAdminGroup({ id: 2, is_fallback_group: true }),
      ]).map((group) => group.id),
    ).toEqual([1])
  })
})

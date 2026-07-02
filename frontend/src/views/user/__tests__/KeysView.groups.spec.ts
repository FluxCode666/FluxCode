import { describe, expect, it } from 'vitest'

import { isSelectableApiKeyGroup } from '../KeysView.groups'

describe('KeysView group selection', () => {
  it('excludes fallback groups from api key selection', () => {
    expect(isSelectableApiKeyGroup({ is_fallback_group: false })).toBe(true)
    expect(isSelectableApiKeyGroup({ is_fallback_group: true })).toBe(false)
  })
})

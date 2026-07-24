import { describe, expect, it } from 'vitest'
import { platformBadgeClass, platformLabel, platformIconClass } from '@/utils/platformColors'

describe('embedding platform presentation', () => {
  it('has an explicit label and non-default classes', () => {
    expect(platformLabel('embedding')).toBe('Embedding')
    expect(platformBadgeClass('embedding')).not.toContain('slate')
    expect(platformIconClass('embedding')).not.toContain('primary')
  })
})

import { describe, expect, it } from 'vitest'
import { requestTypeToLegacyStream, resolveUsageRequestType } from '@/utils/usageRequestType'

describe('usageRequestType embedding', () => {
  it('recognizes embedding and never maps it to streaming', () => {
    expect(resolveUsageRequestType({ request_type: 'embedding', stream: true })).toBe('embedding')
    expect(requestTypeToLegacyStream('embedding')).toBe(false)
  })
})

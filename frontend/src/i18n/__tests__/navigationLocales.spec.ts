import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('navigation locale keys', () => {
  it('contains admin pricing and pool monitor labels in zh', () => {
    expect(zh.nav.pricingPlans).toBe('定价方案')
    expect(zh.nav.poolMonitor).toBe('号池监控')
  })

  it('contains admin pricing and pool monitor labels in en', () => {
    expect(en.nav.pricingPlans).toBe('Pricing Plans')
    expect(en.nav.poolMonitor).toBe('Pool Monitor')
  })
})

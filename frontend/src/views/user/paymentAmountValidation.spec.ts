import { describe, expect, it } from 'vitest'

import {
  amountFitsMethodLimit,
  resolveRechargeMaxAmount,
  resolveRechargeMinAmount,
  validateRechargeAmount,
} from './paymentAmountValidation'

describe('paymentAmountValidation', () => {
  it('intersects config max with method max for recharge input', () => {
    expect(resolveRechargeMaxAmount(1000, 5000)).toBe(1000)
    expect(resolveRechargeMaxAmount(0, 5000)).toBe(5000)
    expect(resolveRechargeMaxAmount(1000, 0)).toBe(1000)
  })

  it('uses the stricter lower bound for recharge input', () => {
    expect(resolveRechargeMinAmount(5, 10)).toBe(10)
    expect(resolveRechargeMinAmount(5, 0)).toBe(5)
    expect(resolveRechargeMinAmount(0, 10)).toBe(10)
  })

  it('prioritizes configured recharge max before method availability checks', () => {
    const result = validateRechargeAmount({
      amount: 1200,
      configMin: 1,
      configMax: 1000,
      methodGlobalMin: 1,
      methodGlobalMax: 5000,
      methods: {
        alipay: { single_min: 1, single_max: 5000, daily_limit: 0, daily_used: 0, daily_remaining: 0, fee_rate: 0, available: true },
      },
      selectedMethod: 'alipay',
    })

    expect(result).toEqual({ code: 'config_max', max: 1000 })
  })

  it('returns selected method max when overall amount is valid but current method is narrower', () => {
    const result = validateRechargeAmount({
      amount: 800,
      configMin: 1,
      configMax: 1000,
      methodGlobalMin: 1,
      methodGlobalMax: 1000,
      methods: {
        alipay: { single_min: 1, single_max: 500, daily_limit: 0, daily_used: 0, daily_remaining: 0, fee_rate: 0, available: true },
        wxpay: { single_min: 1, single_max: 1000, daily_limit: 0, daily_used: 0, daily_remaining: 0, fee_rate: 0, available: true },
      },
      selectedMethod: 'alipay',
    })

    expect(result).toEqual({ code: 'selected_method_max', max: 500 })
    expect(amountFitsMethodLimit(800, {
      single_min: 1,
      single_max: 1000,
      daily_limit: 0,
      daily_used: 0,
      daily_remaining: 0,
      fee_rate: 0,
      available: true,
    })).toBe(true)
  })
})

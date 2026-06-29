import { beforeEach, describe, expect, it } from 'vitest'

import {
  getDashboardFireworksStorageKey,
  markDashboardFireworksShown,
  shouldTriggerDashboardFireworks,
} from '../dashboardFireworks'

describe('dashboard fireworks trigger', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('triggers once per user per day on desktop when spending is over the threshold', () => {
    const options = {
      enabled: true,
      threshold: 20,
      todayActualCost: 20.01,
      isMobile: false,
      userId: 7,
      dateKey: '2026-06-28',
      storage: localStorage,
    }

    expect(shouldTriggerDashboardFireworks(options)).toBe(true)

    markDashboardFireworksShown(options)

    expect(shouldTriggerDashboardFireworks(options)).toBe(false)
    expect(localStorage.getItem(getDashboardFireworksStorageKey(7, '2026-06-28'))).toBe('1')
  })

  it('does not trigger on H5, when disabled, or when spending is not over the threshold', () => {
    const base = {
      enabled: true,
      threshold: 20,
      todayActualCost: 20.01,
      isMobile: false,
      userId: 7,
      dateKey: '2026-06-28',
      storage: localStorage,
    }

    expect(shouldTriggerDashboardFireworks({ ...base, isMobile: true })).toBe(false)
    expect(shouldTriggerDashboardFireworks({ ...base, enabled: false })).toBe(false)
    expect(shouldTriggerDashboardFireworks({ ...base, todayActualCost: 20 })).toBe(false)
  })
})

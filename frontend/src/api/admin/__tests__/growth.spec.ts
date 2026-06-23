import { beforeEach, describe, expect, it, vi } from 'vitest'

import growthAPI from '../growth'
import { apiClient } from '../../client'

vi.mock('../../client', () => ({
  apiClient: {
    get: vi.fn()
  }
}))

describe('admin growth API', () => {
  beforeEach(() => {
    vi.mocked(apiClient.get).mockReset()
    vi.mocked(apiClient.get).mockResolvedValue({ data: {} })
  })

  it('uses one independent endpoint per dashboard chart', async () => {
    const params = {
      start_date: '2026-05-01',
      end_date: '2026-05-30',
      granularity: 'day' as const
    }

    await growthAPI.getOverview(params)
    await growthAPI.getUserTrend(params)
    await growthAPI.getUserSources(params)
    await growthAPI.getSourcePaymentRates(params)
    await growthAPI.getRetentionMatrix(params)
    await growthAPI.getRetentionTrend(params)
    await growthAPI.getPaymentFunnel(params)
    await growthAPI.getPaymentPlans(params)
    await growthAPI.getFirstPayment(params)
    await growthAPI.getFeatureRanking(params)
    await growthAPI.getSessionMetrics(params)
    await growthAPI.getAudienceDevices(params)
    await growthAPI.getAudienceOS(params)
    await growthAPI.getAudienceBrowsers(params)
    await growthAPI.getAudienceClients(params)

    expect(vi.mocked(apiClient.get).mock.calls).toEqual([
      ['/admin/growth/overview', { params }],
      ['/admin/growth/users/trend', { params }],
      ['/admin/growth/users/sources', { params }],
      ['/admin/growth/users/source-payment-rates', { params }],
      ['/admin/growth/retention/matrix', { params }],
      ['/admin/growth/retention/trend', { params }],
      ['/admin/growth/payments/funnel', { params }],
      ['/admin/growth/payments/plans', { params }],
      ['/admin/growth/payments/first-payment', { params }],
      ['/admin/growth/features/ranking', { params }],
      ['/admin/growth/features/session-metrics', { params }],
      ['/admin/growth/audience/devices', { params }],
      ['/admin/growth/audience/os', { params }],
      ['/admin/growth/audience/browsers', { params }],
      ['/admin/growth/audience/clients', { params }]
    ])
  })
})

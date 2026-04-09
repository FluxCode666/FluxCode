/**
 * Public pricing plans endpoints
 */

import { apiClient } from './client'
import type { PricingPlanGroup } from '@/types'

export async function listPublicPlanGroups(options?: { signal?: AbortSignal }): Promise<PricingPlanGroup[]> {
  const { data } = await apiClient.get<PricingPlanGroup[]>('/pricing/plan-groups', {
    signal: options?.signal
  })
  return data
}

export const pricingPlansAPI = {
  listPublicPlanGroups
}

export default pricingPlansAPI


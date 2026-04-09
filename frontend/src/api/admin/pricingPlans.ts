/**
 * Admin pricing plans endpoints
 */

import { apiClient } from '../client'
import type { PricingPlan, PricingPlanContactMethod, PricingPlanGroup } from '@/types'

export interface CreatePricingPlanGroupRequest {
  name: string
  description?: string | null
  sort_order?: number
  status?: 'active' | 'inactive'
}

export interface UpdatePricingPlanGroupRequest {
  name?: string
  description?: string
  sort_order?: number
  status?: 'active' | 'inactive'
}

export interface CreatePricingPlanRequest {
  group_id: number
  name: string
  description?: string | null
  icon_url?: string | null
  badge_text?: string | null
  tagline?: string | null
  price_amount?: number | null
  price_currency?: string
  price_period?: string
  price_text?: string | null
  features?: string[]
  contact_methods?: PricingPlanContactMethod[]
  is_featured?: boolean
  sort_order?: number
  status?: 'active' | 'inactive'
}

export interface UpdatePricingPlanRequest {
  group_id?: number
  name?: string
  description?: string
  icon_url?: string | null
  badge_text?: string | null
  tagline?: string | null
  price_amount?: number | null
  price_currency?: string
  price_period?: string
  price_text?: string
  features?: string[]
  contact_methods?: PricingPlanContactMethod[]
  is_featured?: boolean
  sort_order?: number
  status?: 'active' | 'inactive'
}

export async function listPlanGroups(options?: { signal?: AbortSignal }): Promise<PricingPlanGroup[]> {
  const { data } = await apiClient.get<PricingPlanGroup[]>('/admin/pricing/plan-groups', {
    signal: options?.signal
  })
  return data
}

export async function createPlanGroup(payload: CreatePricingPlanGroupRequest): Promise<PricingPlanGroup> {
  const { data } = await apiClient.post<PricingPlanGroup>('/admin/pricing/plan-groups', payload)
  return data
}

export async function updatePlanGroup(
  id: number,
  payload: UpdatePricingPlanGroupRequest
): Promise<PricingPlanGroup> {
  const { data } = await apiClient.put<PricingPlanGroup>(`/admin/pricing/plan-groups/${id}`, payload)
  return data
}

export async function deletePlanGroup(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/pricing/plan-groups/${id}`)
  return data
}

export async function createPlan(payload: CreatePricingPlanRequest): Promise<PricingPlan> {
  const { data } = await apiClient.post<PricingPlan>('/admin/pricing/plans', payload)
  return data
}

export async function updatePlan(id: number, payload: UpdatePricingPlanRequest): Promise<PricingPlan> {
  const { data } = await apiClient.put<PricingPlan>(`/admin/pricing/plans/${id}`, payload)
  return data
}

export async function deletePlan(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/pricing/plans/${id}`)
  return data
}

export const pricingPlansAPI = {
  listPlanGroups,
  createPlanGroup,
  updatePlanGroup,
  deletePlanGroup,
  createPlan,
  updatePlan,
  deletePlan
}

export default pricingPlansAPI

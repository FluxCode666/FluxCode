/**
 * Admin Promotions API endpoints
 */

import { apiClient } from '../client'
import type {
  Promotion,
  PromotionUsage,
  CreatePromotionRequest,
  UpdatePromotionRequest,
  PromotionStatus,
  BasePaginationResponse
} from '@/types'

export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    promotion_type?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: { signal?: AbortSignal }
): Promise<BasePaginationResponse<Promotion>> {
  const { data } = await apiClient.get<BasePaginationResponse<Promotion>>('/admin/promotions', {
    params: { page, page_size: pageSize, ...filters },
    signal: options?.signal
  })
  return data
}

export async function getById(id: number): Promise<Promotion> {
  const { data } = await apiClient.get<Promotion>(`/admin/promotions/${id}`)
  return data
}

export async function create(request: CreatePromotionRequest): Promise<Promotion> {
  const { data } = await apiClient.post<Promotion>('/admin/promotions', request)
  return data
}

export async function update(id: number, request: UpdatePromotionRequest): Promise<Promotion> {
  const { data } = await apiClient.put<Promotion>(`/admin/promotions/${id}`, request)
  return data
}

export async function remove(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(`/admin/promotions/${id}`)
  return data
}

export async function setStatus(id: number, status: PromotionStatus): Promise<Promotion> {
  const { data } = await apiClient.post<Promotion>(`/admin/promotions/${id}/status`, { status })
  return data
}

export async function getUsages(
  id: number,
  page: number = 1,
  pageSize: number = 20
): Promise<BasePaginationResponse<PromotionUsage>> {
  const { data } = await apiClient.get<BasePaginationResponse<PromotionUsage>>(
    `/admin/promotions/${id}/usages`,
    { params: { page, page_size: pageSize } }
  )
  return data
}

const promotionsAPI = {
  list,
  getById,
  create,
  update,
  remove,
  delete: remove,
  setStatus,
  getUsages
}

export default promotionsAPI

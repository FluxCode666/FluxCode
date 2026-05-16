import { apiClient } from '../client'
import type { BasePaginationResponse, SalesCommissionRecord, SalesCommissionSettlement, SalesCommissionSummary } from '@/types'

export interface SalesCommissionRecordParams {
  page?: number
  page_size?: number
  sales_user_id?: number
  referee_user_id?: number
  payment_order_id?: number
  status?: string
}

export async function listSummaries(params?: { page?: number; page_size?: number; search?: string }): Promise<BasePaginationResponse<SalesCommissionSummary>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionSummary>>('/admin/sales-commissions/summary', { params })
  return data
}

export async function listRecords(params?: SalesCommissionRecordParams): Promise<BasePaginationResponse<SalesCommissionRecord>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionRecord>>('/admin/sales-commissions/records', { params })
  return data
}

export async function listSettlements(params?: { page?: number; page_size?: number; sales_user_id?: number }): Promise<BasePaginationResponse<SalesCommissionSettlement>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionSettlement>>('/admin/sales-commissions/settlements', { params })
  return data
}

export async function createSettlement(payload: { sales_user_id: number; amount_cny: number; note?: string }): Promise<SalesCommissionSettlement> {
  const { data } = await apiClient.post<SalesCommissionSettlement>('/admin/sales-commissions/settlements', payload)
  return data
}

export default { listSummaries, listRecords, listSettlements, createSettlement }

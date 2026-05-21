import { apiClient } from '../client'
import type {
  BasePaginationResponse,
  SalesCommissionOverview,
  SalesCommissionOverviewRangeKey,
  SalesCommissionRecord,
  SalesCommissionSettlement,
  SalesCommissionSummary
} from '@/types'

export interface SalesCommissionRecordParams {
  page?: number
  page_size?: number
  sales_user_id?: number
  referee_user_id?: number
  payment_order_id?: number
  status?: string
}

export interface SalesCommissionOverviewParams {
  range?: SalesCommissionOverviewRangeKey
  start?: string // YYYY-MM-DD（仅 range=custom 时使用）
  end?: string
}

export async function getOverview(params?: SalesCommissionOverviewParams): Promise<SalesCommissionOverview> {
  const { data } = await apiClient.get<SalesCommissionOverview>('/admin/sales-commissions/overview', { params })
  return data
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

export default { getOverview, listSummaries, listRecords, listSettlements }

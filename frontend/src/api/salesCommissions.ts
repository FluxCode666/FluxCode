import { apiClient } from './client'
import type { BasePaginationResponse, SalesCommissionRecord, SalesCommissionSummary } from '@/types'

export async function getSummary(): Promise<SalesCommissionSummary> {
  const { data } = await apiClient.get<SalesCommissionSummary>('/sales-commissions/summary')
  return data
}

export async function listRecords(params?: { page?: number; page_size?: number; status?: string }): Promise<BasePaginationResponse<SalesCommissionRecord>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionRecord>>('/sales-commissions/records', { params })
  return data
}

export const salesCommissionsAPI = { getSummary, listRecords }
export default salesCommissionsAPI

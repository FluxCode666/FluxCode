import { apiClient } from './client'
import type {
  BasePaginationResponse,
  SalesCommissionMonthlyProgress,
  SalesCommissionRecord,
  SalesCommissionSummary
} from '@/types'

export async function getSummary(): Promise<SalesCommissionSummary> {
  const { data } = await apiClient.get<SalesCommissionSummary>('/sales-commissions/summary')
  return data
}

export async function listRecords(params?: { page?: number; page_size?: number; status?: string }): Promise<BasePaginationResponse<SalesCommissionRecord>> {
  const { data } = await apiClient.get<BasePaginationResponse<SalesCommissionRecord>>('/sales-commissions/records', { params })
  return data
}

export async function getMonthlyProgress(): Promise<SalesCommissionMonthlyProgress | null> {
  const { data } = await apiClient.get<SalesCommissionMonthlyProgress | null>('/sales-commissions/monthly-progress')
  return data
}

export const salesCommissionsAPI = { getSummary, listRecords, getMonthlyProgress }
export default salesCommissionsAPI

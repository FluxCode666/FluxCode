import { apiClient } from '../client'
import type {
  BasePaginationResponse,
  SalesCommissionOverview,
  SalesCommissionOverviewRangeKey,
  SalesCommissionRecomputeResult,
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
  /**
   * 按 created_at 的排序方向。
   *
   * - `'desc'`（默认）：最新记录在前，符合大多数管理后台的使用直觉
   * - `'asc'`：最旧记录在前
   *
   * 后端 service 会归一化任意非法值为 `'desc'`，故省略该字段时等价于 `'desc'`。
   */
  sort_order?: 'asc' | 'desc'
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

/**
 * 兜底重算「应当存在但目前缺失」的销售佣金记录。
 *
 * 后端会扫描所有满足条件的已支付余额充值订单，对每条复用 HandleBalanceRechargeCompleted
 * 路径补写 sales_commission_records；依赖 partial unique 索引保证幂等，重复点击安全。
 *
 * @param payload.limit 单次扫描候选订单上限。<=0 时后端用默认值 500，最大 2000。
 */
export async function recomputeMissingCommissions(
  payload: { limit?: number } = {}
): Promise<SalesCommissionRecomputeResult> {
  const { data } = await apiClient.post<SalesCommissionRecomputeResult>(
    '/admin/sales-commissions/recompute',
    payload
  )
  return data
}

export async function createSettlement(
  payload: { sales_user_id: number; amount_cny: number; note?: string }
): Promise<SalesCommissionSettlement> {
  const { data } = await apiClient.post<SalesCommissionSettlement>(
    '/admin/sales-commissions/settlements',
    payload
  )
  return data
}

export default {
  getOverview,
  listSummaries,
  listRecords,
  listSettlements,
  createSettlement,
  recomputeMissingCommissions
}

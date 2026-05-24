/**
 * Referral API endpoints (user-facing)
 * Handles referral info, code generation, invite list, and gift balance queries
 */

import { apiClient } from './client'

// ==================== Types ====================

export interface ReferralInfo {
  referral_code: string
  enabled: boolean
  total_invites: number
  completed_invites: number
  total_earned: number
  gift_balance_remaining: number
  gift_balance_total_granted: number
  gift_balance_total_used: number
  gift_balance_total_expired: number
  invitee_reward: number
  inviter_reward: number
  max_invites: number
  ongoing_reward_enabled: boolean
  ongoing_reward_type: string // 'fixed' | 'percentage'
  ongoing_reward_value: number
  ongoing_reward_max_count: number
  ongoing_reward_duration_days: number
  reward_expiry_days: number
}

export interface ReferralInvite {
  id: number
  referrer_id: number
  referee_id: number
  referee_email?: string
  referee_username?: string
  referral_code: string
  status: string
  invitee_reward_amount: number
  inviter_reward_amount: number
  ongoing_reward_count: number
  ongoing_reward_total: number
  invitee_rewarded_at?: string
  inviter_rewarded_at?: string
  created_at: string
}

export interface GiftBalanceRecord {
  id: number
  user_id: number
  amount: number
  remaining: number
  source: string
  source_ref_id?: number | null
  note?: string
  expires_at?: string | null
  created_at: string
  updated_at: string
}

export interface GiftBalanceSummary {
  total_granted: number
  total_remaining: number
  total_used: number
  total_expired: number
}

export interface GiftBalanceOverview {
  gift_balance_remaining: number
  next_expiry_at: string | null
  next_expiry_amount: number
}

export interface ReferralTrendPoint {
  date: string
  invitations: number
  completions: number
  rewards_total: number
}

export interface ReferralTrendResponse {
  period: string
  data: ReferralTrendPoint[]
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
}

// ==================== API Functions ====================

/**
 * Get current user's referral info (code, stats, config)
 */
export async function getReferralInfo(): Promise<ReferralInfo> {
  const { data } = await apiClient.get<ReferralInfo>('/referral/info')
  return data
}

/**
 * Generate a referral code for the current user
 */
export async function generateReferralCode(): Promise<{ referral_code: string }> {
  const { data } = await apiClient.post<{ referral_code: string }>('/referral/generate-code')
  return data
}

/**
 * Get current user's referral invites list (paginated)
 */
export async function getMyReferrals(params?: {
  page?: number
  page_size?: number
}): Promise<PaginatedResponse<ReferralInvite>> {
  const { data } = await apiClient.get<PaginatedResponse<ReferralInvite>>('/referral/invites', { params })
  return data
}

/**
 * Get current user's gift balance records (paginated)
 */
export async function getMyGiftBalanceRecords(params?: {
  page?: number
  page_size?: number
}): Promise<PaginatedResponse<GiftBalanceRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<GiftBalanceRecord>>('/referral/gift-balance', { params })
  return data
}

/**
 * Get current user's total remaining gift balance
 */
export async function getGiftBalanceRemaining(): Promise<{ remaining: number }> {
  const { data } = await apiClient.get<{ remaining: number }>('/referral/gift-balance/remaining')
  return data
}

/**
 * Get current user's gift balance summary (granted/used/remaining/expired)
 */
export async function getGiftBalanceSummary(): Promise<GiftBalanceSummary> {
  const { data } = await apiClient.get<GiftBalanceSummary>('/referral/gift-balance/summary')
  return data
}

/**
 * Get current user's referral trend (daily invitations / completions / rewards)
 */
export async function getMyReferralStats(days: number = 30): Promise<ReferralTrendResponse> {
  const { data } = await apiClient.get<ReferralTrendResponse>('/referral/stats', { params: { days } })
  return data
}

/**
 * Get current user's gift balance overview (remaining + next expiry)
 */
export async function getGiftBalanceOverview(): Promise<GiftBalanceOverview> {
  const { data } = await apiClient.get<GiftBalanceOverview>('/referral/gift-balance/overview')
  return data
}

export const referralAPI = {
  getReferralInfo,
  generateReferralCode,
  getMyReferrals,
  getMyGiftBalanceRecords,
  getGiftBalanceRemaining,
  getGiftBalanceSummary,
  getGiftBalanceOverview,
  getMyReferralStats,
}

export default referralAPI

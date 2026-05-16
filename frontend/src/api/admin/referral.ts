/**
 * Admin Referral API endpoints
 * Handles referral management, config, stats, leaderboard, dashboard, batch grants
 */

import { apiClient } from '../client'

// ==================== Types ====================

export interface ReferralStats {
  total_referrals: number
  completed_referrals: number
  total_gift_granted: number
  total_gift_used: number
  total_gift_remaining: number
  active_referrers_count: number
}

export interface ReferralConfig {
  referral_enabled: boolean
  referral_invitee_reward: number
  referral_inviter_reward: number
  referral_max_invites: number
  referral_reward_expiry_days: number
  referral_gift_balance_expiry_days: number
  referral_ongoing_reward_enabled: boolean
  referral_ongoing_reward_type: string // 'fixed' | 'percentage'
  referral_ongoing_reward_value: number
  referral_ongoing_reward_max_count: number
  referral_ongoing_reward_duration_days: number
}

export interface ReferralListItem {
  id: number
  referrer_id: number
  referrer_email?: string
  referrer_is_sales?: boolean
  referee_id: number
  referee_email?: string
  referee_username?: string
  referral_code: string
  status: string
  invitee_reward_amount: number
  inviter_reward_amount: number
  ongoing_reward_count: number
  ongoing_reward_total: number
  created_at: string
  invitee_rewarded_at?: string
  inviter_rewarded_at?: string
}

export interface LeaderboardItem {
  user_id: number
  email: string
  username: string
  referral_code: string
  invite_count: number
  total_reward: number
}

export interface ReferralFunnel {
  total_referrals: number
  registrations: number
  first_recharges: number
  conversion_rate: number
}

export interface ReferralTrendPoint {
  date: string
  invitations: number
  completions: number
  rewards_total: number
}

export interface AdminReferralDashboard {
  funnel: ReferralFunnel
  trend: ReferralTrendPoint[]
  summary: ReferralStats
}

export interface UserReferralConfig {
  user_id: number
  invitee_reward_amount?: number | null
  inviter_reward_amount?: number | null
  max_invites?: number | null
  reward_expiry_days?: number | null
  ongoing_reward_enabled?: boolean | null
  ongoing_reward_type?: string | null
  ongoing_reward_value?: number | null
  ongoing_reward_max_count?: number | null
  ongoing_reward_duration_days?: number | null
  notes?: string
  created_at?: string
  updated_at?: string
}

export interface EffectiveReferralConfig {
  enabled: boolean
  invitee_reward_amount: number
  inviter_reward_amount: number
  max_invites: number
  reward_expiry_days: number
  ongoing_reward_enabled: boolean
  ongoing_reward_type: string
  ongoing_reward_value: number
  ongoing_reward_max_count: number
  ongoing_reward_duration_days: number
}

export interface UserConfigResponse {
  has_custom_config: boolean
  config: UserReferralConfig | null
  effective: EffectiveReferralConfig
}

export interface UserConfigPayload {
  invitee_reward?: number | null
  inviter_reward?: number | null
  max_invites?: number | null
  reward_expiry_days?: number | null
  ongoing_reward_enabled?: boolean | null
  ongoing_reward_type?: string
  ongoing_reward_value?: number | null
  ongoing_reward_max_count?: number | null
  ongoing_reward_duration_days?: number | null
  notes?: string
}

// ==================== API Functions ====================

export async function getStats(): Promise<ReferralStats> {
  const { data } = await apiClient.get<ReferralStats>('/admin/referral/stats')
  return data
}

export async function getDashboard(days: number = 30): Promise<AdminReferralDashboard> {
  const { data } = await apiClient.get<AdminReferralDashboard>('/admin/referral/dashboard', {
    params: { days }
  })
  return data
}

export async function getConfig(): Promise<ReferralConfig> {
  const { data } = await apiClient.get<ReferralConfig>('/admin/referral/config')
  return data
}

export async function updateConfig(config: Partial<ReferralConfig>): Promise<{ message: string }> {
  const { data } = await apiClient.put<{ message: string }>('/admin/referral/config', config)
  return data
}

export async function listReferrals(params?: {
  page?: number
  page_size?: number
  status?: string
  search?: string
}): Promise<{ items: ReferralListItem[]; total: number; page: number }> {
  const { data } = await apiClient.get<{ items: ReferralListItem[]; total: number; page: number }>(
    '/admin/referral/list',
    { params }
  )
  return data
}

export async function getLeaderboard(params?: {
  limit?: number
  period?: 'all_time' | 'this_month' | 'this_week'
}): Promise<LeaderboardItem[]> {
  const { data } = await apiClient.get<LeaderboardItem[]>('/admin/referral/leaderboard', { params })
  return data
}

export async function grantGiftBalance(request: {
  user_id: number
  amount: number
  expiry_days?: number
  notes?: string
}): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    '/admin/referral/grant-gift-balance',
    request
  )
  return data
}

export async function markReferralCompleted(
  id: number,
  request: {
    notes: string
    order_pay_amount_cny?: number
    order_credited_amount?: number
  }
): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(
    `/admin/referral/${id}/mark-completed`,
    request
  )
  return data
}

export async function batchGrantGiftBalance(request: {
  target: 'all' | 'selected'
  user_ids?: number[]
  amount: number
  expiry_days?: number
  notes?: string
}): Promise<{ granted_count: number }> {
  const { data } = await apiClient.post<{ granted_count: number }>(
    '/admin/referral/grant-batch',
    request
  )
  return data
}

export async function getUserConfig(userId: number): Promise<UserConfigResponse> {
  const { data } = await apiClient.get<UserConfigResponse>(`/admin/referral/user-config/${userId}`)
  return data
}

export async function upsertUserConfig(
  userId: number,
  config: UserConfigPayload
): Promise<{ message: string }> {
  const { data } = await apiClient.put<{ message: string }>(
    `/admin/referral/user-config/${userId}`,
    config
  )
  return data
}

export async function deleteUserConfig(userId: number): Promise<{ message: string }> {
  const { data } = await apiClient.delete<{ message: string }>(
    `/admin/referral/user-config/${userId}`
  )
  return data
}

const adminReferralAPI = {
  getStats,
  getDashboard,
  getConfig,
  updateConfig,
  listReferrals,
  getLeaderboard,
  grantGiftBalance,
  markReferralCompleted,
  batchGrantGiftBalance,
  getUserConfig,
  upsertUserConfig,
  deleteUserConfig,
}

export default adminReferralAPI

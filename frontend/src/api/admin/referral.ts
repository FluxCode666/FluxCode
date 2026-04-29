/**
 * Admin Referral API endpoints
 * Handles referral management, config, stats, leaderboard
 */

import { apiClient } from '../client'

// ==================== Types ====================

export interface ReferralStats {
  total_referrals: number
  completed_referrals: number
  total_gift_balance_issued: number
  total_gift_balance_remaining: number
  total_gift_balance_consumed: number
}

export interface ReferralConfig {
  referral_enabled: boolean
  referral_invitee_reward: number
  referral_inviter_reward: number
  referral_inviter_reward_type: string
  referral_gift_balance_expiry_days: number
  referral_max_invites: number
  referral_ongoing_reward_enabled: boolean
  referral_ongoing_reward_value: number
  referral_ongoing_reward_type: string
  referral_ongoing_reward_max_count: number
}

export interface ReferralListItem {
  id: number
  referrer_id: number
  referrer_email: string
  referee_id: number
  referee_email: string
  referral_code: string
  status: string
  invitee_reward_amount: number
  inviter_reward_amount: number
  ongoing_reward_count: number
  ongoing_reward_total: number
  created_at: string
  inviter_rewarded_at?: string
}

export interface LeaderboardItem {
  user_id: number
  email: string
  total_invites: number
  completed_invites: number
  total_earned: number
}

export interface UserReferralConfig {
  user_id: number
  invitee_reward?: number
  inviter_reward?: number
  inviter_reward_type?: string
  max_invites?: number
  ongoing_reward_value?: number
  ongoing_reward_type?: string
  ongoing_reward_max_count?: number
  created_at?: string
  updated_at?: string
}

// ==================== API Functions ====================

export async function getStats(): Promise<ReferralStats> {
  const { data } = await apiClient.get<ReferralStats>('/admin/referral/stats')
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
}): Promise<{ items: ReferralListItem[]; total: number }> {
  const { data } = await apiClient.get<{ items: ReferralListItem[]; total: number }>(
    '/admin/referral/list',
    { params }
  )
  return data
}

export async function getLeaderboard(limit: number = 10): Promise<LeaderboardItem[]> {
  const { data } = await apiClient.get<LeaderboardItem[]>('/admin/referral/leaderboard', {
    params: { limit }
  })
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

export async function getUserConfig(userId: number): Promise<UserReferralConfig> {
  const { data } = await apiClient.get<UserReferralConfig>(`/admin/referral/user-config/${userId}`)
  return data
}

export async function upsertUserConfig(
  userId: number,
  config: Partial<UserReferralConfig>
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
  getConfig,
  updateConfig,
  listReferrals,
  getLeaderboard,
  grantGiftBalance,
  getUserConfig,
  upsertUserConfig,
  deleteUserConfig
}

export default adminReferralAPI

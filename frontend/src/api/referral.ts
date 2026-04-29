/**
 * Referral API endpoints (user-facing)
 * Handles referral info, code generation, invite list, and gift balance queries
 */

import { apiClient } from './client'

// ==================== Types ====================

export interface ReferralInfo {
  referral_code: string
  total_invites: number
  completed_invites: number
  total_earned: number
  gift_balance_remaining: number
}

export interface ReferralInvite {
  id: number
  referee_email: string
  status: string
  invitee_reward_amount: number
  inviter_reward_amount: number
  ongoing_reward_count: number
  ongoing_reward_total: number
  created_at: string
  inviter_rewarded_at?: string
}

export interface GiftBalanceRecord {
  id: number
  amount: number
  remaining: number
  source: string
  expires_at?: string
  created_at: string
}

// ==================== API Functions ====================

/**
 * Get current user's referral info (code, stats)
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
 * Get current user's referral invites list
 */
export async function getMyReferrals(): Promise<ReferralInvite[]> {
  const { data } = await apiClient.get<ReferralInvite[]>('/referral/invites')
  return data
}

/**
 * Get current user's gift balance records
 */
export async function getMyGiftBalanceRecords(): Promise<GiftBalanceRecord[]> {
  const { data } = await apiClient.get<GiftBalanceRecord[]>('/referral/gift-balance')
  return data
}

/**
 * Get current user's total remaining gift balance
 */
export async function getGiftBalanceRemaining(): Promise<{ remaining: number }> {
  const { data } = await apiClient.get<{ remaining: number }>('/referral/gift-balance/remaining')
  return data
}

export const referralAPI = {
  getReferralInfo,
  generateReferralCode,
  getMyReferrals,
  getMyGiftBalanceRecords,
  getGiftBalanceRemaining
}

export default referralAPI

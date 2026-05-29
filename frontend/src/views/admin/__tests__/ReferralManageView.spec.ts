import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ReferralManageView from '../ReferralManageView.vue'

const {
  getDashboard,
  getConfig,
  listReferrals,
  getLeaderboard,
  markReferralCompleted,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  getConfig: vi.fn(),
  listReferrals: vi.fn(),
  getLeaderboard: vi.fn(),
  markReferralCompleted: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin/referral', () => ({
  default: {
    getDashboard,
    getConfig,
    listReferrals,
    getLeaderboard,
    markReferralCompleted,
    updateConfig: vi.fn(),
    grantGiftBalance: vi.fn(),
    batchGrantGiftBalance: vi.fn(),
    getUserConfig: vi.fn(),
    upsertUserConfig: vi.fn(),
    deleteUserConfig: vi.fn()
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'en' },
      setLocaleMessage: vi.fn()
    }
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      if (key === 'adminReferral.batchSuccess' && params) {
        return `batch success ${params.count}`
      }
      return key
    }
  })
}))

describe('ReferralManageView', () => {
  beforeEach(() => {
    getDashboard.mockReset()
    getConfig.mockReset()
    listReferrals.mockReset()
    getLeaderboard.mockReset()
    markReferralCompleted.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    getDashboard.mockResolvedValue({
      funnel: { registrations: 1, first_recharges: 0, conversion_rate: 0 },
      trend: [],
      summary: { total_referrals: 1, completed_referrals: 0, total_gift_granted: 0, total_gift_remaining: 0 }
    })
    getConfig.mockResolvedValue({
      referral_enabled: true,
      referral_invitee_reward_enabled: true,
      referral_invitee_reward: 10,
      referral_inviter_reward: 20,
      referral_max_invites: 0,
      referral_reward_expiry_days: 7,
      referral_gift_balance_expiry_days: 7,
      referral_ongoing_reward_enabled: false,
      referral_ongoing_reward_type: 'fixed',
      referral_ongoing_reward_value: 0,
      referral_ongoing_reward_max_count: 0,
      referral_ongoing_reward_duration_days: 0,
      referral_sales_enabled: false,
      referral_sales_invitee_reward_enabled: false,
      referral_sales_invitee_reward: 0,
      referral_sales_invitee_ongoing_reward_enabled: false,
      referral_sales_invitee_ongoing_reward_type: 'fixed',
      referral_sales_invitee_ongoing_reward_value: 0,
      referral_sales_invitee_ongoing_reward_max_count: 0,
      referral_sales_invitee_ongoing_reward_duration_days: 0,
    })
    listReferrals.mockResolvedValue({
      items: [{
        id: 1,
        referrer_id: 12,
        referrer_email: 'referrer@example.com',
        referrer_is_sales: false,
        referee_id: 34,
        referee_email: 'buyer@example.com',
        referral_code: 'ABC123',
        status: 'pending',
        invitee_reward_amount: 10,
        invitee_first_charge_reward_amount: 2.5,
        inviter_reward_amount: 20,
        ongoing_reward_count: 0,
        ongoing_reward_total: 0,
        invitee_ongoing_reward_total: 4,
        created_at: '2026-05-14T00:00:00Z',
      }],
      total: 1,
      page: 1,
    })
    getLeaderboard.mockResolvedValue([])
    markReferralCompleted.mockResolvedValue({ message: 'ok' })
    vi.stubGlobal('prompt', vi.fn(() => 'webhook missed'))
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  it('renders referrer id and email and marks pending referrals as completed', async () => {
    const wrapper = mount(ReferralManageView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Select: { props: ['options', 'modelValue'], template: '<select />' },
          Line: true,
        }
      }
    })

    await flushPromises()

    const tabs = wrapper.findAll('button')
    const listTab = tabs.find((button) => button.text() === 'adminReferral.tabList')
    expect(listTab).toBeTruthy()
    await listTab!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('#12')
    expect(wrapper.text()).toContain('referrer@example.com')
    expect(wrapper.text()).toContain('buyer@example.com')
    expect(wrapper.text()).toContain('$16.50')
    expect(wrapper.text()).toContain('adminReferral.manualComplete')

    const manualCompleteButton = wrapper.findAll('button').find((button) => button.text() === 'adminReferral.manualComplete')
    expect(manualCompleteButton).toBeTruthy()
    await manualCompleteButton!.trigger('click')
    await flushPromises()

    expect(markReferralCompleted).toHaveBeenCalledWith(1, { notes: 'webhook missed' })
    expect(showSuccess).toHaveBeenCalledWith('adminReferral.manualCompleteSuccess')
  })
})

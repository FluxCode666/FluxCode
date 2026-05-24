import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SalesCommissionsView from '../SalesCommissionsView.vue'

const mocks = vi.hoisted(() => ({
  getSummary: vi.fn().mockResolvedValue({ total_commission_cny: 1, frozen_cny: 0.8, unlocked_cny: 0.2, settleable_cny: 0.2, settled_cny: 0 }),
  listRecords: vi.fn().mockResolvedValue({
    items: [{ id: 1, referee_email: 'buyer@example.com', commission_total_cny: 1, frozen_cny: 0.8, unlocked_cny: 0.2, settled_cny: 0, settleable_cny: 0.2, status: 'partial_unlocked', created_at: '2026-05-05T00:00:00Z' }],
    total: 1,
    page: 1,
    page_size: 20
  }),
  getMonthlyProgress: vi.fn().mockResolvedValue({
    sales_user_id: 10,
    commission_month: '2026-05-01T00:00:00Z',
    commission_mode: 'tiered',
    fixed_commission_rate: 0,
    min_monthly_sales_cny: 100,
    tiers: [
      { month_sales_from_cny: 0, month_sales_to_cny: 200, commission_rate: 5, sort_order: 1 },
      { month_sales_from_cny: 200, commission_rate: 10, sort_order: 2 }
    ],
    monthly_sales_cny: 150,
    monthly_commission_cny: 7.5,
    threshold_met: true,
    to_threshold_cny: 0,
    current_tier_index: 0,
    next_tier_index: 1,
    to_next_tier_cny: 50,
    next_tier_rate: 10,
    snapshot_frozen: true
  })
}))

vi.mock('@/api/salesCommissions', () => ({
  salesCommissionsAPI: {
    getSummary: mocks.getSummary,
    listRecords: mocks.listRecords,
    getMonthlyProgress: mocks.getMonthlyProgress
  },
  default: {
    getSummary: mocks.getSummary,
    listRecords: mocks.listRecords,
    getMonthlyProgress: mocks.getMonthlyProgress
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { locale: { value: 'en' }, setLocaleMessage: vi.fn() } }),
  useI18n: () => ({ t: (key: string) => key })
}))

describe('User SalesCommissionsView', () => {
  it('loads current sales commission data', async () => {
    const wrapper = mount(SalesCommissionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: { props: ['data'], template: '<div><span v-for="row in data" :key="row.id">{{ row.referee_email }}</span></div>' },
          Pagination: true,
          Select: { template: '<div />' }
        }
      }
    })
    await flushPromises()
    expect(mocks.getSummary).toHaveBeenCalled()
    expect(mocks.getMonthlyProgress).toHaveBeenCalled()
    expect(wrapper.text()).toContain('buyer@example.com')
    // 进度卡片：当月销售额 + 下一档佣金率提示
    expect(wrapper.find('[data-testid="sales-commission-monthly-progress"]').exists()).toBe(true)
    const progressText = wrapper.find('[data-testid="sales-commission-monthly-progress"]').text()
    expect(progressText).toContain('150.00')
    expect(progressText).toContain('10.00') // next tier rate
  })
})

import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SalesCommissionsView from '../SalesCommissionsView.vue'

const overviewFixture = {
  range: {
    key: 'this_month',
    start: '2026-05-01T00:00:00+08:00',
    end: '2026-05-31T23:59:59+08:00'
  },
  kpi: {
    related_order_amount_cny: 12345.67,
    commission_total_cny: 1000,
    frozen_cny: 800,
    settleable_cny: 150,
    settled_cny: 50,
    active_sales_users: 7,
    threshold_met_users: 3,
    avg_commission_rate: 8.1
  },
  monthly_trend: Array.from({ length: 12 }, (_, i) => ({
    month: `2025-${String((i + 6) % 12 + 1).padStart(2, '0')}-01T00:00:00Z`,
    related_order_amount_cny: 100 * (i + 1),
    commission_total_cny: 10 * (i + 1)
  })),
  top_sales: [
    { sales_user_id: 1, sales_email: 'alpha@example.com', sales_username: 'alpha', related_order_amount_cny: 500, commission_total_cny: 60 },
    { sales_user_id: 2, sales_email: 'bravo@example.com', sales_username: 'bravo', related_order_amount_cny: 500, commission_total_cny: 40 }
  ],
  status_breakdown: { frozen_cny: 800, settleable_cny: 150, settled_cny: 50 },
  mode_breakdown: { fixed_records: 2, tiered_records: 5, fixed_commission_cny: 300, tiered_commission_cny: 700 }
}

const mocks = vi.hoisted(() => ({
  getOverview: vi.fn().mockResolvedValue({
    range: { key: 'this_month', start: '2026-05-01T00:00:00+08:00', end: '2026-05-31T23:59:59+08:00' },
    kpi: {
      related_order_amount_cny: 12345.67, commission_total_cny: 1000,
      frozen_cny: 800, settleable_cny: 150, settled_cny: 50,
      active_sales_users: 7, threshold_met_users: 3, avg_commission_rate: 8.1
    },
    monthly_trend: [],
    top_sales: [
      { sales_user_id: 1, sales_email: 'alpha@example.com', sales_username: 'alpha', related_order_amount_cny: 500, commission_total_cny: 60 }
    ],
    status_breakdown: { frozen_cny: 800, settleable_cny: 150, settled_cny: 50 },
    mode_breakdown: { fixed_records: 2, tiered_records: 5, fixed_commission_cny: 300, tiered_commission_cny: 700 }
  }),
  listSummaries: vi.fn().mockResolvedValue({
    items: [{ sales_user_id: 1, sales_email: 'alpha@example.com', sales_username: 'alpha', total_commission_cny: 60, frozen_cny: 40, unlocked_cny: 20, settleable_cny: 10, settled_cny: 5, records_count: 4 }],
    total: 1, page: 1, page_size: 20
  }),
  listRecords: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }),
  listSettlements: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    salesCommissions: {
      getOverview: mocks.getOverview,
      listSummaries: mocks.listSummaries,
      listRecords: mocks.listRecords,
      listSettlements: mocks.listSettlements
    }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { locale: { value: 'en' }, setLocaleMessage: vi.fn() } }),
  useI18n: () => ({ t: (key: string, params?: any) => params ? `${key}:${JSON.stringify(params)}` : key })
}))

describe('Admin SalesCommissionsView', () => {
  it('loads overview and summary on mount, renders KPIs and Top10', async () => {
    const wrapper = mount(SalesCommissionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: { props: ['data'], template: '<div><span v-for="row in data" :key="row.sales_user_id ?? row.id">{{ row.sales_email }}</span></div>' },
          Pagination: true,
          Select: { template: '<div />' },
          // chart 组件 mock 成纯结构容器，避免在 vitest 里跑 chart.js / canvas
          MonthlyTrendChart: { props: ['trend'], template: '<div data-testid="mock-monthly-trend">trend:{{ trend.length }}</div>' },
          StatusBreakdownChart: { props: ['data'], template: '<div data-testid="mock-status-breakdown" />' },
          TopSalesChart: { props: ['items'], template: '<div data-testid="mock-top-sales">top:{{ items.length }}</div>' },
          ModeBreakdownChart: { props: ['data'], template: '<div data-testid="mock-mode-breakdown" />' }
        }
      }
    })
    await flushPromises()
    expect(mocks.getOverview).toHaveBeenCalledWith({ range: 'this_month' })
    expect(mocks.listSummaries).toHaveBeenCalled()
    // KPI grid 渲染
    expect(wrapper.find('[data-testid="sales-commission-kpi-commission_total"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('¥1000.00')
    // 销售汇总表渲染
    expect(wrapper.text()).toContain('alpha@example.com')
    // chart 组件 mock 接收到 props
    expect(wrapper.find('[data-testid="mock-top-sales"]').text()).toBe('top:1')
    // 明细折叠区默认不应该触发 records / settlements 请求
    expect(mocks.listRecords).not.toHaveBeenCalled()
    expect(mocks.listSettlements).not.toHaveBeenCalled()
  })

  it('reissues overview request when range changes', async () => {
    const wrapper = mount(SalesCommissionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: { template: '<div />' },
          Pagination: true,
          Select: { template: '<div />' },
          MonthlyTrendChart: true,
          StatusBreakdownChart: true,
          TopSalesChart: true,
          ModeBreakdownChart: true
        }
      }
    })
    await flushPromises()
    mocks.getOverview.mockClear()
    // RangePicker 触发：组件内部触发 update:modelValue → wrapper.vm.range 更新
    ;(wrapper.vm as any).range = { key: 'last_30d' }
    await flushPromises()
    expect(mocks.getOverview).toHaveBeenCalledWith({ range: 'last_30d' })
  })
})

// 让 fixture 被引用，避免 ts unused warning。
void overviewFixture

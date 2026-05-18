import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import SalesCommissionsView from '../SalesCommissionsView.vue'

const mocks = vi.hoisted(() => ({
  listSummaries: vi.fn().mockResolvedValue({
    items: [{ sales_user_id: 1, sales_email: 'sales@example.com', total_commission_cny: 1, frozen_cny: 0.8, unlocked_cny: 0.2, settleable_cny: 0.2, settled_cny: 0, records_count: 1 }],
    total: 1,
    page: 1,
    page_size: 20
  }),
  listRecords: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }),
  listSettlements: vi.fn().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 }),
  createSettlement: vi.fn().mockResolvedValue({})
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    salesCommissions: {
      listSummaries: mocks.listSummaries,
      listRecords: mocks.listRecords,
      listSettlements: mocks.listSettlements,
      createSettlement: mocks.createSettlement
    }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { locale: { value: 'en' }, setLocaleMessage: vi.fn() } }),
  useI18n: () => ({ t: (key: string) => key })
}))

describe('Admin SalesCommissionsView', () => {
  it('loads and renders sales commission summary', async () => {
    const wrapper = mount(SalesCommissionsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          DataTable: { props: ['data'], template: '<div><span v-for="row in data" :key="row.sales_user_id">{{ row.sales_email }}</span></div>' },
          Pagination: true,
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' },
          Select: { template: '<div />' },
          Icon: true
        }
      }
    })
    await flushPromises()
    expect(mocks.listSummaries).toHaveBeenCalled()
    expect(wrapper.text()).toContain('sales@example.com')
  })
})

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
  })
}))

vi.mock('@/api/salesCommissions', () => ({
  salesCommissionsAPI: { getSummary: mocks.getSummary, listRecords: mocks.listRecords },
  default: { getSummary: mocks.getSummary, listRecords: mocks.listRecords }
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
          Pagination: true
        }
      }
    })
    await flushPromises()
    expect(mocks.getSummary).toHaveBeenCalled()
    expect(wrapper.text()).toContain('buyer@example.com')
  })
})

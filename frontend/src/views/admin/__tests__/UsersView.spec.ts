import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsersView from '../UsersView.vue'

const {
  usersList,
  getAllGroups,
  listEnabledDefinitions,
  getBatchUserAttributes,
  getBatchUsersUsage,
  showError,
} = vi.hoisted(() => ({
  usersList: vi.fn(),
  getAllGroups: vi.fn(),
  listEnabledDefinitions: vi.fn(),
  getBatchUserAttributes: vi.fn(),
  getBatchUsersUsage: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: usersList,
      getById: vi.fn(),
    },
    groups: {
      getAll: getAllGroups,
    },
    userAttributes: {
      listEnabledDefinitions,
      getBatchUserAttributes,
    },
    dashboard: {
      getBatchUsersUsage,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const AppLayoutStub = { template: '<div><slot /></div>' }
const TablePageLayoutStub = {
  template: '<div><slot name="filters" /><slot name="actions" /><slot name="table" /><slot name="pagination" /><slot /></div>',
}
const DataTableStub = {
  props: ['data'],
  template: `
    <div>
      <slot
        v-if="data && data.length"
        name="cell-balance"
        :row="data[0]"
        :value="data[0].balance"
      />
    </div>
  `,
}

describe('UsersView', () => {
  beforeEach(() => {
    usersList.mockReset()
    getAllGroups.mockReset()
    listEnabledDefinitions.mockReset()
    getBatchUserAttributes.mockReset()
    getBatchUsersUsage.mockReset()
    showError.mockReset()
    localStorage.clear()

    listEnabledDefinitions.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    getBatchUserAttributes.mockResolvedValue({ attributes: {} })
    getBatchUsersUsage.mockResolvedValue({ stats: {} })
    usersList.mockResolvedValue({
      items: [{
        id: 7,
        email: 'gifted@example.com',
        username: 'gifted',
        role: 'user',
        balance: 10,
        gift_balance_remaining: 2.5,
        concurrency: 1,
        status: 'active',
        allowed_groups: null,
        is_sales: false,
        sales_commission_rate: 0,
        balance_notify_enabled: false,
        balance_notify_threshold: null,
        balance_notify_extra_emails: [],
        notes: '',
        created_at: '2026-06-01T00:00:00Z',
        updated_at: '2026-06-01T00:00:00Z',
      }],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1,
    })
  })

  it('用户列表余额展示包含赠送余额', async () => {
    const wrapper = mount(UsersView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          TablePageLayout: TablePageLayoutStub,
          DataTable: DataTableStub,
          Pagination: true,
          ConfirmDialog: true,
          EmptyState: true,
          GroupBadge: true,
          Select: true,
          Icon: true,
          UserAttributesConfigModal: true,
          UserConcurrencyCell: true,
          UserCreateModal: true,
          UserEditModal: true,
          UserApiKeysModal: true,
          UserAllowedGroupsModal: true,
          UserBalanceModal: true,
          UserBalanceHistoryModal: true,
          UserAuditLogModal: true,
          GroupReplaceModal: true,
        },
      },
    })

    await flushPromises()
    await flushPromises()

    expect(wrapper.text()).toContain('$12.50')
    expect(wrapper.text()).toContain('$10.00 + $2.50')
  })
})

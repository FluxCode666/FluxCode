import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listAccountsWithEtag,
  getBatchTodayStats,
  listAllProxies,
  listAllGroups,
  refreshCredentials,
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listAccountsWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  listAllProxies: vi.fn(),
  listAllGroups: vi.fn(),
  refreshCredentials: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag: listAccountsWithEtag,
      getBatchTodayStats,
      batchClearError: vi.fn().mockResolvedValue({ success: 0, failed: 0 }),
      batchRefresh: vi.fn().mockResolvedValue({ success: 0, failed: 0 }),
      bulkUpdate: vi.fn().mockResolvedValue({}),
      exportData: vi.fn(),
      refreshCredentials,
      recoverState: vi.fn(),
      resetAccountQuota: vi.fn(),
      getAvailableModels: vi.fn().mockResolvedValue([])
    },
    proxies: {
      getAll: listAllProxies,
      getAllWithCount: listAllProxies
    },
    groups: {
      getAll: listAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isSimpleMode: false
  })
}))


vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useIntervalFn: () => ({
      pause: vi.fn(),
      resume: vi.fn()
    })
  }
})

const AccountTableFiltersStub = defineComponent({
  props: {
    filters: {
      type: Object,
      required: true
    }
  },
  emits: ['update:filters', 'change', 'update:searchQuery'],
  template: `
    <button
      data-test="apply-filters"
      @click="$emit('update:filters', {
        ...filters,
        schedulable_status: 'manual_unschedulable',
        model: 'gpt-5.6-sol',
        proxy_ids: [2, 5],
        created_start_date: '2026-02-01',
        created_end_date: '2026-02-03'
      }); $emit('change')"
    >
      apply
    </button>
  `
})

const DataTableStub = defineComponent({
  props: {
    data: {
      type: Array,
      default: () => []
    }
  },
  emits: ['sort'],
  template: `
    <div>
      <slot v-if="data.length" name="cell-name" :row="data[0]" :value="data[0].name" />
      <button data-test="sort" @click="$emit('sort', 'created_at', 'desc')">sort</button>
    </div>
  `
})

describe('admin AccountsView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listAccounts.mockReset()
    listAccountsWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    listAllProxies.mockReset()
    listAllGroups.mockReset()
    refreshCredentials.mockReset()
    showError.mockReset()
    showSuccess.mockReset()

    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0
    })
    listAccountsWithEtag.mockResolvedValue({
      notModified: false,
      etag: null,
      data: {
        items: [],
        total: 0,
        pages: 0
      }
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    listAllProxies.mockResolvedValue([])
    listAllGroups.mockResolvedValue([])
  })

  it('forwards advanced filters and sort state to the accounts api', async () => {
    listAccounts.mockResolvedValue({
      items: [{ id: 42, name: 'account-by-id' }],
      total: 1,
      pages: 1
    })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          AccountTableFilters: AccountTableFiltersStub,
          AccountTableActions: {
            template: '<div><slot name="beforeCreate" /><slot name="after" /></div>'
          },
          AccountBulkActionsBar: true,
          DataTable: DataTableStub,
          Pagination: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          AccountActionMenu: true,
          SyncFromCrsModal: true,
          ImportDataModal: true,
          BulkEditAccountModal: true,
          TempUnschedStatusModal: true,
          ConfirmDialog: true,
          ErrorPassthroughRulesModal: true,
          AccountStatusIndicator: true,
          AccountUsageCell: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountCapacityCell: true,
          PlatformTypeBadge: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    expect(listAccounts).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[data-test="account-id"]').text()).toBe('#42')

    await wrapper.get('[data-test="apply-filters"]').trigger('click')
    await vi.advanceTimersByTimeAsync(350)
    await flushPromises()

    await wrapper.get('[data-test="sort"]').trigger('click')
    await flushPromises()

    expect(listAccounts).toHaveBeenLastCalledWith(
      1,
      expect.any(Number),
      expect.objectContaining({
        schedulable_status: 'manual_unschedulable',
        model: 'gpt-5.6-sol',
        proxy_ids: [2, 5],
        created_start_date: '2026-02-01',
        created_end_date: '2026-02-03',
        sort_by: 'created_at',
        sort_order: 'desc'
      }),
      expect.objectContaining({
        signal: expect.any(AbortSignal)
      })
    )
  })

  it('shows backend error message when manual token refresh fails', async () => {
    const message = 'OpenAI OAuth 会话已结束，请重新授权该账号。'
    refreshCredentials.mockRejectedValue({
      status: 400,
      reason: 'OPENAI_OAUTH_SESSION_TERMINATED',
      message
    })

    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          AccountTableFilters: AccountTableFiltersStub,
          AccountTableActions: {
            template: '<div><slot name="beforeCreate" /><slot name="after" /></div>'
          },
          AccountBulkActionsBar: true,
          DataTable: DataTableStub,
          Pagination: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          AccountActionMenu: defineComponent({
            emits: ['refresh-token'],
            template: '<button data-test="refresh-token" @click="$emit(\'refresh-token\', { id: 42 })">refresh</button>'
          }),
          SyncFromCrsModal: true,
          ImportDataModal: true,
          BulkEditAccountModal: true,
          TempUnschedStatusModal: true,
          ConfirmDialog: true,
          ErrorPassthroughRulesModal: true,
          AccountStatusIndicator: true,
          AccountUsageCell: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountCapacityCell: true,
          PlatformTypeBadge: true,
          Icon: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-test="refresh-token"]').trigger('click')
    await flushPromises()

    expect(refreshCredentials).toHaveBeenCalledWith(42)
    expect(showError).toHaveBeenCalledWith(message)
  })

  it('moves account import and export actions into the more menu', async () => {
    const wrapper = mount(AccountsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          TablePageLayout: {
            template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
          },
          AccountTableFilters: AccountTableFiltersStub,
          AccountTableActions: {
            methods: {
              close: vi.fn()
            },
            template: `
              <div>
                <div data-test="main-actions">
                  <slot name="beforeCreate" />
                </div>
                <div data-test="more-actions">
                  <slot name="more" :close="close" item-class="more-item" />
                </div>
              </div>
            `
          },
          AccountBulkActionsBar: true,
          DataTable: DataTableStub,
          Pagination: true,
          CreateAccountModal: true,
          EditAccountModal: true,
          ReAuthAccountModal: true,
          AccountTestModal: true,
          AccountStatsModal: true,
          ScheduledTestsPanel: true,
          AccountActionMenu: true,
          SyncFromCrsModal: true,
          ImportDataModal: true,
          BulkEditAccountModal: true,
          TempUnschedStatusModal: true,
          ConfirmDialog: true,
          ErrorPassthroughRulesModal: true,
          TLSFingerprintProfilesModal: true,
          AccountStatusIndicator: true,
          AccountUsageCell: true,
          AccountTodayStatsCell: true,
          AccountGroupsCell: true,
          AccountCapacityCell: true,
          PlatformTypeBadge: true,
          Icon: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.get('[data-test="main-actions"]').text()).not.toContain('admin.accounts.dataImport')
    expect(wrapper.get('[data-test="main-actions"]').text()).not.toContain('admin.accounts.dataExport')
    expect(wrapper.get('[data-test="more-actions"]').text()).toContain('admin.accounts.dataImport')
    expect(wrapper.get('[data-test="more-actions"]').text()).toContain('admin.accounts.dataExport')
  })
})

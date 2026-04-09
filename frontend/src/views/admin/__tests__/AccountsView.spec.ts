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
  showError,
  showSuccess
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listAccountsWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  listAllProxies: vi.fn(),
  listAllGroups: vi.fn(),
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
      refreshCredentials: vi.fn(),
      recoverState: vi.fn(),
      resetAccountQuota: vi.fn(),
      getAvailableModels: vi.fn().mockResolvedValue([])
    },
    proxies: {
      getAll: listAllProxies
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

vi.mock('@/composables/useSwipeSelect', () => ({
  useSwipeSelect: vi.fn()
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
  emits: ['sort'],
  template: `
    <div>
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
})

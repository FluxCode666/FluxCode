import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UsageView from '../UsageView.vue'

const { list, getStats, getSnapshotV2, getModelStats, getById, adminUsageList, saveAs, xlsxMock } = vi.hoisted(() => {
  vi.stubGlobal('localStorage', {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  })

  const xlsxState = {
    worksheet: { rows: [] as unknown[][] },
    aoa_to_sheet: vi.fn((rows: unknown[][]) => {
      xlsxState.worksheet = { rows: [...rows] }
      return xlsxState.worksheet
    }),
    sheet_add_aoa: vi.fn((worksheet: { rows: unknown[][] }, rows: unknown[][]) => {
      worksheet.rows.push(...rows)
    }),
    book_new: vi.fn(() => ({})),
    book_append_sheet: vi.fn(),
    write: vi.fn(() => new ArrayBuffer(8)),
  }

  return {
    list: vi.fn(),
    getStats: vi.fn(),
    getSnapshotV2: vi.fn(),
    getModelStats: vi.fn(),
    getById: vi.fn(),
    adminUsageList: vi.fn(),
    saveAs: vi.fn(),
    xlsxMock: xlsxState,
  }
})

const messages: Record<string, string> = {
  'admin.dashboard.timeRange': 'Time Range',
  'admin.dashboard.day': 'Day',
  'admin.dashboard.hour': 'Hour',
  'admin.usage.failedToLoadUser': 'Failed to load user',
  'admin.usage.traceId': 'Trace ID',
  'admin.usage.upstreamRequestId': 'Upstream Request ID',
  'admin.usage.requestId': 'Request ID',
  'usage.exportSuccess': 'Export Success',
  'usage.time': 'Time',
  'admin.usage.user': 'User',
  'usage.apiKeyFilter': 'API Key',
  'admin.usage.account': 'Account',
  'usage.model': 'Model',
  'usage.upstreamModel': 'Upstream Model',
  'usage.reasoningEffort': 'Reasoning Effort',
  'admin.usage.group': 'Group',
  'usage.inboundEndpoint': 'Inbound Endpoint',
  'usage.upstreamEndpoint': 'Upstream Endpoint',
  'usage.type': 'Type',
  'admin.usage.inputTokens': 'Input Tokens',
  'admin.usage.outputTokens': 'Output Tokens',
  'admin.usage.cacheReadTokens': 'Cache Read Tokens',
  'admin.usage.cacheCreationTokens': 'Cache Creation Tokens',
  'admin.usage.inputCost': 'Input Cost',
  'admin.usage.outputCost': 'Output Cost',
  'admin.usage.cacheReadCost': 'Cache Read Cost',
  'admin.usage.cacheCreationCost': 'Cache Creation Cost',
  'usage.rate': 'Rate',
  'usage.accountMultiplier': 'Account Multiplier',
  'usage.original': 'Original',
  'usage.userBilled': 'User Billed',
  'usage.accountBilled': 'Account Billed',
  'usage.firstToken': 'First Token',
  'usage.duration': 'Duration',
  'usage.userAgent': 'User Agent',
  'admin.usage.ipAddress': 'IP Address',
}

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

vi.mock('@/api/admin', () => ({
  adminAPI: {
    usage: {
      list,
      getStats,
    },
    dashboard: {
      getModelStats,
      getSnapshotV2,
    },
    users: {
      getById,
    },
  },
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: {
    list: adminUsageList,
  },
}))

vi.mock('file-saver', () => ({
  saveAs,
}))

vi.mock('xlsx', () => ({
  utils: {
    aoa_to_sheet: xlsxMock.aoa_to_sheet,
    sheet_add_aoa: xlsxMock.sheet_add_aoa,
    book_new: xlsxMock.book_new,
    book_append_sheet: xlsxMock.book_append_sheet,
  },
  write: xlsxMock.write,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/utils/format', () => ({
  formatReasoningEffort: (value: string | null | undefined) => value ?? '-',
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => messages[key] ?? key,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  })
}))

const AppLayoutStub = { template: '<div><slot /></div>' }
const UsageFiltersStub = {
  emits: ['export'],
  template: '<div><button data-test="export" @click="$emit(\'export\')">export</button><slot name="after-reset" /></div>',
}
const UsageTableStub = {
  props: ['columns'],
  template: `
    <div data-test="usage-table">
      <span
        v-for="column in columns"
        :key="column.key"
        class="column-key"
        :data-class="column.class || ''"
      >{{ column.key }}</span>
    </div>
  `,
}
const ModelDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="model-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}
const GroupDistributionChartStub = {
  props: ['metric'],
  emits: ['update:metric'],
  template: `
    <div data-test="group-chart">
      <span class="metric">{{ metric }}</span>
      <button class="switch-metric" @click="$emit('update:metric', 'actual_cost')">switch</button>
    </div>
  `,
}

describe('admin UsageView distribution metric toggles', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    list.mockReset()
    getStats.mockReset()
    getSnapshotV2.mockReset()
    getModelStats.mockReset()
    getById.mockReset()
    adminUsageList.mockReset()
    saveAs.mockReset()
    xlsxMock.aoa_to_sheet.mockClear()
    xlsxMock.sheet_add_aoa.mockClear()
    xlsxMock.book_new.mockClear()
    xlsxMock.book_append_sheet.mockClear()
    xlsxMock.write.mockClear()

    list.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
    })
    getStats.mockResolvedValue({
      total_requests: 0,
      total_input_tokens: 0,
      total_output_tokens: 0,
      total_cache_tokens: 0,
      total_tokens: 0,
      total_cost: 0,
      total_actual_cost: 0,
      average_duration_ms: 0,
    })
    getSnapshotV2.mockResolvedValue({
      trend: [],
      models: [],
      groups: [],
    })
    getModelStats.mockResolvedValue({
      models: [],
    })
    adminUsageList.mockResolvedValue({
      items: [],
      total: 0,
    })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps model and group metric toggles independent without refetching chart data', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)
    expect(getSnapshotV2).toHaveBeenCalledWith(expect.objectContaining({
      start_date: formatLocalDate(yesterday),
      end_date: formatLocalDate(now),
      granularity: 'hour'
    }))

    const modelChart = wrapper.find('[data-test="model-chart"]')
    const groupChart = wrapper.find('[data-test="group-chart"]')

    expect(modelChart.find('.metric').text()).toBe('tokens')
    expect(groupChart.find('.metric').text()).toBe('tokens')

    await modelChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('tokens')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)

    await groupChart.find('.switch-metric').trigger('click')
    await flushPromises()

    expect(modelChart.find('.metric').text()).toBe('actual_cost')
    expect(groupChart.find('.metric').text()).toBe('actual_cost')
    expect(getSnapshotV2).toHaveBeenCalledTimes(1)
  })

  it('passes trace id and upstream request id columns to the usage table', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    const columnKeys = wrapper.findAll('.column-key').map((node) => node.text())
    expect(columnKeys).toContain('trace_id')
    expect(columnKeys).toContain('request_id')
  })

  it('applies wrapped width classes to long text usage columns', async () => {
    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    const columns = wrapper.findAll('.column-key')
    const traceColumn = columns.find((node) => node.text() === 'trace_id')
    const requestColumn = columns.find((node) => node.text() === 'request_id')
    const modelColumn = columns.find((node) => node.text() === 'model')

    expect(traceColumn?.attributes('data-class')).toContain('whitespace-normal')
    expect(traceColumn?.attributes('data-class')).toContain('min-w-[15rem]')
    expect(requestColumn?.attributes('data-class')).toContain('whitespace-normal')
    expect(requestColumn?.attributes('data-class')).toContain('min-w-[15rem]')
    expect(modelColumn?.attributes('data-class')).toContain('whitespace-normal')
    expect(modelColumn?.attributes('data-class')).toContain('min-w-[16rem]')
  })

  it('exports trace id before upstream request id in usage rows', async () => {
    adminUsageList.mockResolvedValueOnce({
      items: [
        {
          created_at: '2026-05-13T12:00:00Z',
          trace_id: 'trace-export-1',
          request_id: 'req-upstream-export-1',
          model: 'claude-sonnet-4',
          upstream_model: 'claude-sonnet-4-20250514',
          reasoning_effort: null,
          inbound_endpoint: '/v1/chat/completions',
          upstream_endpoint: '/messages',
          input_tokens: 10,
          output_tokens: 20,
          cache_read_tokens: 0,
          cache_creation_tokens: 0,
          input_cost: 0.001,
          output_cost: 0.002,
          cache_read_cost: 0,
          cache_creation_cost: 0,
          rate_multiplier: 1,
          account_rate_multiplier: 1,
          total_cost: 0.003,
          actual_cost: 0.003,
          account_stats_cost: 0.003,
          first_token_ms: 100,
          duration_ms: 200,
          user_agent: 'Mozilla/5.0',
          ip_address: '127.0.0.1',
          user: { email: 'admin@example.com' },
          api_key: { name: 'key-a' },
          account: { name: 'account-a' },
          group: { name: 'group-a' },
          stream: false,
        },
      ],
      total: 1,
    })

    const wrapper = mount(UsageView, {
      global: {
        stubs: {
          AppLayout: AppLayoutStub,
          UsageStatsCards: true,
          UsageFilters: UsageFiltersStub,
          UsageTable: UsageTableStub,
          UsageExportProgress: true,
          UsageCleanupDialog: true,
          UserBalanceHistoryModal: true,
          Pagination: true,
          Select: true,
          DateRangePicker: true,
          Icon: true,
          TokenUsageTrend: true,
          ModelDistributionChart: ModelDistributionChartStub,
          GroupDistributionChart: GroupDistributionChartStub,
        },
      },
    })

    vi.advanceTimersByTime(120)
    await flushPromises()

    await wrapper.find('[data-test="export"]').trigger('click')
    await flushPromises()

    expect(xlsxMock.aoa_to_sheet).toHaveBeenCalled()
    const headerRow = xlsxMock.aoa_to_sheet.mock.calls[0][0][0]
    expect(headerRow).toContain('Trace ID')
    expect(headerRow).toContain('Upstream Request ID')
    expect(headerRow.indexOf('Trace ID')).toBeLessThan(headerRow.indexOf('Upstream Request ID'))

    const exportedRow = xlsxMock.sheet_add_aoa.mock.calls[0][1][0]
    expect(exportedRow).toContain('trace-export-1')
    expect(exportedRow).toContain('req-upstream-export-1')
    expect(exportedRow.indexOf('trace-export-1')).toBeLessThan(exportedRow.indexOf('req-upstream-export-1'))
    expect(saveAs).toHaveBeenCalledTimes(1)
  })
})

<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('salesCommissions.adminDescription') }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <RangePicker v-model="range" />
          <button type="button" class="btn btn-secondary" :disabled="loadingOverview" @click="loadOverview">
            {{ t('common.refresh') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="recomputing"
            :title="t('salesCommissions.recompute.tooltip')"
            @click="showRecomputeDialog = true"
          >
            {{ recomputing ? t('salesCommissions.recompute.running') : t('salesCommissions.recompute.button') }}
          </button>
        </div>
      </div>

      <div v-if="loadingOverview && !overview" class="rounded-lg border border-dashed border-gray-200 p-10 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400">
        {{ t('common.loading') }}…
      </div>

      <template v-else-if="overview">
        <OverviewKPIGrid :kpi="overview.kpi" />

        <div class="grid gap-4 lg:grid-cols-2">
          <MonthlyTrendChart :trend="overview.monthly_trend" />
          <StatusBreakdownChart :data="overview.status_breakdown" />
          <TopSalesChart :items="overview.top_sales" />
          <ModeBreakdownChart :data="overview.mode_breakdown" />
        </div>
      </template>

      <SummaryTable
        ref="summaryTableRef"
        :items="summaries"
        :loading="loadingSummaries"
        :pagination="summaryPagination"
        :search="summarySearch"
        @update:page="setSummaryPage"
        @update:search="setSummarySearch"
        @settle="onSettle"
      />

      <DetailsCollapsible
        :records="records"
        :settlements="settlements"
        :loading-records="loadingRecords"
        :loading-settlements="loadingSettlements"
        :record-pagination="recordPagination"
        :settlement-pagination="settlementPagination"
        :record-filters="recordFilters"
        :record-sort-order="recordSortOrder"
        @update:record-filters="updateRecordFilters"
        @update:record-page="setRecordPage"
        @update:record-sort="setRecordSortOrder"
        @update:settlement-page="setSettlementPage"
        @expand="ensureDetailsLoaded"
      />

      <ConfirmDialog
        :show="showRecomputeDialog"
        :title="t('salesCommissions.recompute.confirmTitle')"
        :message="t('salesCommissions.recompute.confirmMessage')"
        :confirm-text="t('salesCommissions.recompute.confirmButton')"
        @confirm="onConfirmRecompute"
        @cancel="showRecomputeDialog = false"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import OverviewKPIGrid from '@/components/admin/salesCommissions/OverviewKPIGrid.vue'
import MonthlyTrendChart from '@/components/admin/salesCommissions/MonthlyTrendChart.vue'
import StatusBreakdownChart from '@/components/admin/salesCommissions/StatusBreakdownChart.vue'
import TopSalesChart from '@/components/admin/salesCommissions/TopSalesChart.vue'
import ModeBreakdownChart from '@/components/admin/salesCommissions/ModeBreakdownChart.vue'
import SummaryTable from '@/components/admin/salesCommissions/SummaryTable.vue'
import DetailsCollapsible, { type RecordFilters } from '@/components/admin/salesCommissions/DetailsCollapsible.vue'
import RangePicker, { type RangeSelection } from '@/components/admin/salesCommissions/RangePicker.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import type {
  SalesCommissionOverview,
  SalesCommissionRecord,
  SalesCommissionSettlement,
  SalesCommissionSummary
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const summaryTableRef = ref<InstanceType<typeof SummaryTable> | null>(null)

const overview = ref<SalesCommissionOverview | null>(null)
const loadingOverview = ref(false)

const range = ref<RangeSelection>({ key: 'this_month' })
watch(range, () => { void loadOverview() }, { deep: true })

const summaries = ref<SalesCommissionSummary[]>([])
const loadingSummaries = ref(false)
const summaryPagination = reactive({ page: 1, page_size: 20, total: 0 })
const summarySearch = ref('')

const records = ref<SalesCommissionRecord[]>([])
const loadingRecords = ref(false)
const recordPagination = reactive({ page: 1, page_size: 20, total: 0 })
const recordFilters = reactive<RecordFilters>({
  salesUserID: '',
  refereeUserID: '',
  paymentOrderID: '',
  status: ''
})
// 佣金明细排序方向，初始为 desc（最新在前）。点击列头会切换并触发 loadRecords。
const recordSortOrder = ref<'asc' | 'desc'>('desc')
const detailsTouched = ref(false)

const settlements = ref<SalesCommissionSettlement[]>([])
const loadingSettlements = ref(false)
const settlementPagination = reactive({ page: 1, page_size: 20, total: 0 })

async function loadOverview() {
  loadingOverview.value = true
  try {
    const params: Record<string, string> = { range: range.value.key }
    if (range.value.key === 'custom') {
      if (!range.value.start || !range.value.end) {
        loadingOverview.value = false
        return
      }
      params.start = range.value.start
      params.end = range.value.end
    }
    overview.value = await adminAPI.salesCommissions.getOverview(params as any)
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingOverview.value = false
  }
}

async function loadSummaries() {
  loadingSummaries.value = true
  try {
    const res = await adminAPI.salesCommissions.listSummaries({
      page: summaryPagination.page,
      page_size: summaryPagination.page_size,
      search: summarySearch.value || undefined
    })
    summaries.value = res.items || []
    summaryPagination.total = res.total || 0
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingSummaries.value = false
  }
}

function setSummaryPage(page: number) {
  summaryPagination.page = page
  void loadSummaries()
}

function setSummarySearch(value: string) {
  summarySearch.value = value
  summaryPagination.page = 1
  void loadSummaries()
}

async function loadRecords() {
  loadingRecords.value = true
  try {
    const res = await adminAPI.salesCommissions.listRecords({
      page: recordPagination.page,
      page_size: recordPagination.page_size,
      sales_user_id: recordFilters.salesUserID ? Number(recordFilters.salesUserID) : undefined,
      referee_user_id: recordFilters.refereeUserID ? Number(recordFilters.refereeUserID) : undefined,
      payment_order_id: recordFilters.paymentOrderID ? Number(recordFilters.paymentOrderID) : undefined,
      status: recordFilters.status || undefined,
      sort_order: recordSortOrder.value
    })
    records.value = res.items || []
    recordPagination.total = res.total || 0
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingRecords.value = false
  }
}

function setRecordPage(page: number) {
  recordPagination.page = page
  void loadRecords()
}

function updateRecordFilters(value: RecordFilters) {
  Object.assign(recordFilters, value)
  recordPagination.page = 1
  void loadRecords()
}

// 点击 created_at 列头切换排序方向：回到第一页（避免越界）+ 立即 reload。
function setRecordSortOrder(order: 'asc' | 'desc') {
  if (recordSortOrder.value === order) return
  recordSortOrder.value = order
  recordPagination.page = 1
  void loadRecords()
}

async function loadSettlements() {
  loadingSettlements.value = true
  try {
    const res = await adminAPI.salesCommissions.listSettlements({
      page: settlementPagination.page,
      page_size: settlementPagination.page_size
    })
    settlements.value = res.items || []
    settlementPagination.total = res.total || 0
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingSettlements.value = false
  }
}

function setSettlementPage(page: number) {
  settlementPagination.page = page
  void loadSettlements()
}

// 明细折叠区是 lazy 加载，避免页面打开就发起 records / settlements 请求。
function ensureDetailsLoaded() {
  if (detailsTouched.value) return
  detailsTouched.value = true
  void loadRecords()
  void loadSettlements()
}

async function onSettle(payload: { sales_user_id: number; amount_cny: number; note: string }) {
  try {
    await adminAPI.salesCommissions.createSettlement(payload)
    appStore.showSuccess(t('salesCommissions.settleSuccess'))
    summaryTableRef.value?.closeSettleDialog()
    void loadSummaries()
    if (detailsTouched.value) {
      void loadRecords()
      void loadSettlements()
    }
  } catch (error: any) {
    const reason = error?.reason || error?.code || ''
    if (reason === 'SALES_COMMISSION_NO_SETTLEABLE') {
      appStore.showError(t('salesCommissions.settleErrNoSettleable'))
    } else if (reason === 'SALES_COMMISSION_SETTLE_AMOUNT_EXCEEDS') {
      appStore.showError(t('salesCommissions.settleErrAmountExceeds'))
    } else {
      appStore.showError(error?.message || t('salesCommissions.settleFailed'))
    }
  } finally {
    if (summaryTableRef.value) {
      summaryTableRef.value.settling = false
    }
  }
}

// "重算缺失佣金" 兜底按钮：扫描 status=completed 的余额充值订单中
// 那些应当存在销售佣金记录、但目前 sales_commission_records 里却没有的，
// 复用 HandleBalanceRechargeCompleted 路径补写。后端幂等，重复点击安全。
const showRecomputeDialog = ref(false)
const recomputing = ref(false)

async function onConfirmRecompute() {
  showRecomputeDialog.value = false
  if (recomputing.value) return
  recomputing.value = true
  try {
    const res = await adminAPI.salesCommissions.recomputeMissingCommissions()
    const summary = t('salesCommissions.recompute.resultSummary', {
      scanned: res.scanned,
      processed: res.processed,
      failed: res.failed
    })
    if (res.failed > 0) {
      appStore.showWarning(summary)
    } else if (res.processed > 0) {
      appStore.showSuccess(summary)
    } else {
      appStore.showInfo(t('salesCommissions.recompute.noMissing'))
    }
    // 重算后立即刷新概览 / 汇总，让管理员看到 frozen / total 的变化。
    void loadOverview()
    void loadSummaries()
    if (detailsTouched.value) {
      void loadRecords()
    }
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.recompute.failed'))
  } finally {
    recomputing.value = false
  }
}

onMounted(() => {
  void loadOverview()
  void loadSummaries()
})
</script>

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
        :items="summaries"
        :loading="loadingSummaries"
        :pagination="summaryPagination"
        :search="summarySearch"
        @update:page="setSummaryPage"
        @update:search="setSummarySearch"
      />

      <DetailsCollapsible
        :records="records"
        :settlements="settlements"
        :loading-records="loadingRecords"
        :loading-settlements="loadingSettlements"
        :record-pagination="recordPagination"
        :settlement-pagination="settlementPagination"
        :record-filters="recordFilters"
        @update:record-filters="updateRecordFilters"
        @update:record-page="setRecordPage"
        @update:settlement-page="setSettlementPage"
        @expand="ensureDetailsLoaded"
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
import type {
  SalesCommissionOverview,
  SalesCommissionRecord,
  SalesCommissionSettlement,
  SalesCommissionSummary
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

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
      status: recordFilters.status || undefined
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

onMounted(() => {
  void loadOverview()
  void loadSummaries()
})
</script>

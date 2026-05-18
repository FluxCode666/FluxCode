<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('salesCommissions.adminDescription') }}</p>
        </div>
        <div class="flex gap-2">
          <input v-model="search" type="search" class="input w-full sm:w-64" :placeholder="t('salesCommissions.searchSales')" @keyup.enter="loadSummaries" />
          <button type="button" class="btn btn-secondary" @click="loadAll">{{ t('common.refresh') }}</button>
        </div>
      </div>

      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="item in totalCards" :key="item.label" class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ item.label }}</div>
          <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ formatCNY(item.value) }}</div>
        </div>
      </div>

      <section class="space-y-3">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.summaries') }}</h2>
        </div>
        <DataTable :columns="summaryColumns" :data="summaries" :loading="loadingSummaries">
          <template #cell-sales_email="{ row }">
            <div>
              <div class="font-medium">{{ row.sales_email || '-' }}</div>
              <div class="text-xs text-gray-500">{{ row.sales_username || `#${row.sales_user_id}` }}</div>
            </div>
          </template>
          <template #cell-total_commission_cny="{ row }">{{ formatCNY(row.total_commission_cny) }}</template>
          <template #cell-frozen_cny="{ row }">{{ formatCNY(row.frozen_cny) }}</template>
          <template #cell-settleable_cny="{ row }">{{ formatCNY(row.settleable_cny) }}</template>
          <template #cell-actions="{ row }">
            <div class="flex flex-wrap gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="focusSales(row)">
                {{ t('salesCommissions.viewDetails') }}
              </button>
              <button type="button" class="btn btn-primary btn-sm" :disabled="row.settleable_cny <= 0" @click="openSettlement(row)">
                {{ t('salesCommissions.createSettlement') }}
              </button>
            </div>
          </template>
        </DataTable>
        <Pagination v-if="summaryPagination.total > 0" :page="summaryPagination.page" :total="summaryPagination.total" :page-size="summaryPagination.page_size" @update:page="setSummaryPage" />
      </section>

      <section class="space-y-3">
        <div class="flex flex-col gap-3">
          <div class="flex items-center justify-between">
            <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.records') }}</h2>
          </div>
          <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-[repeat(4,minmax(0,1fr))_auto_auto]">
            <input v-model.trim="recordFilters.salesUserID" type="number" min="1" class="input" :placeholder="t('salesCommissions.salesUserId')" @keyup.enter="submitRecordFilters" />
            <input v-model.trim="recordFilters.refereeUserID" type="number" min="1" class="input" :placeholder="t('salesCommissions.refereeUserId')" @keyup.enter="submitRecordFilters" />
            <input v-model.trim="recordFilters.paymentOrderID" type="number" min="1" class="input" :placeholder="t('salesCommissions.paymentOrderId')" @keyup.enter="submitRecordFilters" />
            <Select v-model="recordFilters.status" :options="recordStatusOptions" />
            <button type="button" class="btn btn-secondary" @click="resetRecordFilters">{{ t('common.reset') }}</button>
            <button type="button" class="btn btn-primary" @click="submitRecordFilters">{{ t('common.apply') }}</button>
          </div>
        </div>
        <DataTable :columns="recordColumns" :data="records" :loading="loadingRecords">
          <template #cell-sales_email="{ row }">
            <div>
              <div class="font-medium">{{ row.sales_email || '-' }}</div>
              <div class="text-xs text-gray-500">{{ row.sales_username || `#${row.sales_user_id}` }}</div>
            </div>
          </template>
          <template #cell-referee_email="{ row }">{{ row.referee_email || `#${row.referee_user_id}` }}</template>
          <template #cell-payment_order_id="{ row }">
            <div>
              <div class="font-medium">{{ row.payment_order_id ? `#${row.payment_order_id}` : '-' }}</div>
              <div v-if="row.payment_order_status || row.note" class="text-xs text-gray-500">
                {{ row.payment_order_status || row.note }}
              </div>
            </div>
          </template>
          <template #cell-commission_rate="{ row }">{{ formatPercent(row.commission_rate) }}</template>
          <template #cell-commission_context="{ row }">
            <div class="space-y-1">
              <div class="text-sm font-medium text-gray-900 dark:text-gray-100">{{ formatCommissionMode(row.commission_mode) }}</div>
              <div class="text-xs text-gray-500">{{ t('salesCommissions.commissionMonth') }}: {{ formatMonth(row.commission_month) }}</div>
              <div class="text-xs text-gray-500">{{ t('salesCommissions.monthlySales') }}: {{ formatCNY(row.monthly_sales_before_cny) }} -> {{ formatCNY(row.monthly_sales_after_cny) }}</div>
            </div>
          </template>
          <template #cell-commission_total_cny="{ row }">{{ formatCNY(row.commission_total_cny) }}</template>
          <template #cell-settleable_cny="{ row }">{{ formatCNY(row.settleable_cny) }}</template>
          <template #cell-settled_cny="{ row }">{{ formatCNY(row.settled_cny) }}</template>
          <template #cell-status="{ row }">
            <span :class="['badge', getStatusBadgeClass(row.status)]">{{ formatStatus(row.status) }}</span>
          </template>
          <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
        </DataTable>
        <Pagination v-if="recordPagination.total > 0" :page="recordPagination.page" :total="recordPagination.total" :page-size="recordPagination.page_size" @update:page="setRecordPage" />
      </section>

      <section class="space-y-3">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.settlements') }}</h2>
          <div class="flex flex-wrap gap-2">
            <input v-model.trim="settlementFilters.salesUserID" type="number" min="1" class="input w-full sm:w-44" :placeholder="t('salesCommissions.salesUserId')" @keyup.enter="submitSettlementFilters" />
            <button type="button" class="btn btn-secondary" @click="resetSettlementFilters">{{ t('common.reset') }}</button>
            <button type="button" class="btn btn-primary" @click="submitSettlementFilters">{{ t('common.apply') }}</button>
          </div>
        </div>
        <DataTable :columns="settlementColumns" :data="settlements" :loading="loadingSettlements">
          <template #cell-sales_email="{ row }">
            <div class="font-medium">{{ row.sales_email || `#${row.sales_user_id}` }}</div>
          </template>
          <template #cell-amount_cny="{ row }">{{ formatCNY(row.amount_cny) }}</template>
          <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
        </DataTable>
        <Pagination v-if="settlementPagination.total > 0" :page="settlementPagination.page" :total="settlementPagination.total" :page-size="settlementPagination.page_size" @update:page="setSettlementPage" />
      </section>
    </div>

    <BaseDialog :show="settlementDialog.open" :title="t('salesCommissions.createSettlement')" width="normal" @close="closeSettlement">
      <div class="space-y-4">
        <div>
          <label class="input-label">{{ t('salesCommissions.salesUser') }}</label>
          <input v-model.number="settlementDialog.salesUserID" type="number" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('salesCommissions.amount') }}</label>
          <input v-model.number="settlementDialog.amount" type="number" min="0.01" step="0.01" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('salesCommissions.note') }}</label>
          <textarea v-model="settlementDialog.note" rows="3" class="input"></textarea>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeSettlement">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="submittingSettlement" @click="submitSettlement">{{ t('common.confirm') }}</button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import type { SalesCommissionRecord, SalesCommissionSettlement, SalesCommissionSummary } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const search = ref('')
const summaries = ref<SalesCommissionSummary[]>([])
const records = ref<SalesCommissionRecord[]>([])
const settlements = ref<SalesCommissionSettlement[]>([])
const loadingSummaries = ref(false)
const loadingRecords = ref(false)
const loadingSettlements = ref(false)
const submittingSettlement = ref(false)

const summaryPagination = reactive({ page: 1, page_size: 20, total: 0 })
const recordPagination = reactive({ page: 1, page_size: 20, total: 0 })
const settlementPagination = reactive({ page: 1, page_size: 20, total: 0 })

const recordFilters = reactive({
  salesUserID: '',
  refereeUserID: '',
  paymentOrderID: '',
  status: ''
})

const settlementFilters = reactive({
  salesUserID: ''
})

const settlementDialog = reactive({ open: false, salesUserID: 0, amount: 0, note: '' })

const recordStatusOptions = computed(() => [
  { value: '', label: t('salesCommissions.allStatuses') },
  { value: 'frozen', label: t('salesCommissions.statuses.frozen') },
  { value: 'partial_unlocked', label: t('salesCommissions.statuses.partial_unlocked') },
  { value: 'unlocked', label: t('salesCommissions.statuses.unlocked') },
  { value: 'settled', label: t('salesCommissions.statuses.settled') },
  { value: 'settlement_blocked', label: t('salesCommissions.statuses.settlement_blocked') }
])

const summaryColumns = [
  { key: 'sales_email', label: t('salesCommissions.salesUser') },
  { key: 'total_commission_cny', label: t('salesCommissions.totalCommission') },
  { key: 'frozen_cny', label: t('salesCommissions.frozen') },
  { key: 'settleable_cny', label: t('salesCommissions.settleable') },
  { key: 'records_count', label: t('salesCommissions.records') },
  { key: 'actions', label: t('common.actions') }
]

const recordColumns = [
  { key: 'sales_email', label: t('salesCommissions.salesUser') },
  { key: 'referee_email', label: t('salesCommissions.refereeUser') },
  { key: 'payment_order_id', label: t('salesCommissions.paymentOrder') },
  { key: 'commission_rate', label: t('salesCommissions.commissionRate') },
  { key: 'commission_context', label: t('salesCommissions.commissionContext') },
  { key: 'commission_total_cny', label: t('salesCommissions.totalCommission') },
  { key: 'settleable_cny', label: t('salesCommissions.settleable') },
  { key: 'settled_cny', label: t('salesCommissions.settled') },
  { key: 'status', label: t('salesCommissions.status') },
  { key: 'created_at', label: t('salesCommissions.createdAt') }
]

const settlementColumns = [
  { key: 'sales_email', label: t('salesCommissions.salesUser') },
  { key: 'amount_cny', label: t('salesCommissions.amount') },
  { key: 'note', label: t('salesCommissions.note') },
  { key: 'created_at', label: t('salesCommissions.createdAt') }
]

const totals = computed(() => summaries.value.reduce((acc, item) => {
  acc.frozen += item.frozen_cny || 0
  acc.unlocked += item.unlocked_cny || 0
  acc.settleable += item.settleable_cny || 0
  acc.settled += item.settled_cny || 0
  return acc
}, { frozen: 0, unlocked: 0, settleable: 0, settled: 0 }))

const totalCards = computed(() => [
  { label: t('salesCommissions.frozen'), value: totals.value.frozen },
  { label: t('salesCommissions.unlocked'), value: totals.value.unlocked },
  { label: t('salesCommissions.settleable'), value: totals.value.settleable },
  { label: t('salesCommissions.settled'), value: totals.value.settled }
])

async function loadSummaries() {
  loadingSummaries.value = true
  try {
    const res = await adminAPI.salesCommissions.listSummaries({ page: summaryPagination.page, page_size: summaryPagination.page_size, search: search.value })
    summaries.value = res.items || []
    summaryPagination.total = res.total || 0
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingSummaries.value = false
  }
}

async function loadRecords() {
  loadingRecords.value = true
  try {
    const res = await adminAPI.salesCommissions.listRecords({
      page: recordPagination.page,
      page_size: recordPagination.page_size,
      sales_user_id: parseOptionalNumber(recordFilters.salesUserID),
      referee_user_id: parseOptionalNumber(recordFilters.refereeUserID),
      payment_order_id: parseOptionalNumber(recordFilters.paymentOrderID),
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

async function loadSettlements() {
  loadingSettlements.value = true
  try {
    const res = await adminAPI.salesCommissions.listSettlements({
      page: settlementPagination.page,
      page_size: settlementPagination.page_size,
      sales_user_id: parseOptionalNumber(settlementFilters.salesUserID)
    })
    settlements.value = res.items || []
    settlementPagination.total = res.total || 0
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingSettlements.value = false
  }
}

function loadAll() {
  void Promise.all([loadSummaries(), loadRecords(), loadSettlements()])
}

function setSummaryPage(page: number) {
  summaryPagination.page = page
  void loadSummaries()
}

function setRecordPage(page: number) {
  recordPagination.page = page
  void loadRecords()
}

function setSettlementPage(page: number) {
  settlementPagination.page = page
  void loadSettlements()
}

function submitRecordFilters() {
  recordPagination.page = 1
  void loadRecords()
}

function resetRecordFilters() {
  recordFilters.salesUserID = ''
  recordFilters.refereeUserID = ''
  recordFilters.paymentOrderID = ''
  recordFilters.status = ''
  submitRecordFilters()
}

function submitSettlementFilters() {
  settlementPagination.page = 1
  void loadSettlements()
}

function resetSettlementFilters() {
  settlementFilters.salesUserID = ''
  submitSettlementFilters()
}

function focusSales(row: SalesCommissionSummary) {
  const id = String(row.sales_user_id)
  recordFilters.salesUserID = id
  settlementFilters.salesUserID = id
  recordPagination.page = 1
  settlementPagination.page = 1
  void Promise.all([loadRecords(), loadSettlements()])
}

function openSettlement(row: SalesCommissionSummary) {
  settlementDialog.salesUserID = row.sales_user_id
  settlementDialog.amount = row.settleable_cny
  settlementDialog.note = ''
  settlementDialog.open = true
}

function closeSettlement() {
  settlementDialog.open = false
}

async function submitSettlement() {
  if (settlementDialog.salesUserID <= 0 || settlementDialog.amount <= 0) {
    appStore.showError(t('salesCommissions.invalidSettlement'))
    return
  }
  submittingSettlement.value = true
  try {
    await adminAPI.salesCommissions.createSettlement({
      sales_user_id: settlementDialog.salesUserID,
      amount_cny: settlementDialog.amount,
      note: settlementDialog.note
    })
    appStore.showSuccess(t('salesCommissions.settlementCreated'))
    closeSettlement()
    loadAll()
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.settlementFailed'))
  } finally {
    submittingSettlement.value = false
  }
}

function formatCNY(value: number | undefined) {
  return `¥${Number(value || 0).toFixed(2)}`
}

function formatPercent(value: number | undefined) {
  return `${Number(value || 0).toFixed(2)}%`
}

function parseOptionalNumber(value: string) {
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined
}

function formatStatus(status: string | undefined) {
  if (!status) return '-'
  return t(`salesCommissions.statuses.${status}`)
}

function getStatusBadgeClass(status: string | undefined) {
  switch (status) {
    case 'settled':
      return 'badge-success'
    case 'unlocked':
    case 'partial_unlocked':
      return 'badge-primary'
    case 'settlement_blocked':
      return 'badge-warning'
    default:
      return 'badge-gray'
  }
}

function formatCommissionMode(mode: SalesCommissionRecord['commission_mode']) {
  return mode === 'tiered'
    ? t('admin.users.sales.modeTiered')
    : t('admin.users.sales.modeFixed')
}

function formatMonth(value: string | undefined) {
  if (!value) return '-'
  return new Date(value).toLocaleDateString(undefined, { year: 'numeric', month: 'short' })
}

function formatDate(value: string | undefined) {
  return value ? new Date(value).toLocaleString() : '-'
}

onMounted(loadAll)
</script>

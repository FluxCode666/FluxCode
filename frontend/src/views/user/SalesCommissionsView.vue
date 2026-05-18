<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex items-end justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('salesCommissions.userDescription') }}</p>
        </div>
        <button type="button" class="btn btn-secondary" @click="loadAll">{{ t('common.refresh') }}</button>
      </div>

      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="item in summaryCards" :key="item.label" class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
          <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ item.label }}</div>
          <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ formatCNY(item.value) }}</div>
        </div>
      </div>

      <section class="space-y-3">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.records') }}</h2>
          <div class="w-full sm:w-52">
            <Select v-model="statusFilter" :options="statusOptions" @change="applyStatusFilter" />
          </div>
        </div>
        <DataTable :columns="recordColumns" :data="records" :loading="loadingRecords">
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
          <template #cell-frozen_cny="{ row }">{{ formatCNY(row.frozen_cny) }}</template>
          <template #cell-unlocked_cny="{ row }">{{ formatCNY(row.unlocked_cny) }}</template>
          <template #cell-settleable_cny="{ row }">{{ formatCNY(row.settleable_cny) }}</template>
          <template #cell-status="{ row }">
            <span :class="['badge', getStatusBadgeClass(row.status)]">{{ formatStatus(row.status) }}</span>
          </template>
          <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
        </DataTable>
        <Pagination v-if="pagination.total > 0" :page="pagination.page" :total="pagination.total" :page-size="pagination.page_size" @update:page="setPage" />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { salesCommissionsAPI } from '@/api/salesCommissions'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import type { SalesCommissionRecord, SalesCommissionSummary } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const summary = ref<Partial<SalesCommissionSummary>>({})
const records = ref<SalesCommissionRecord[]>([])
const loadingSummary = ref(false)
const loadingRecords = ref(false)
const pagination = reactive({ page: 1, page_size: 20, total: 0 })
const statusFilter = ref('')

const statusOptions = computed(() => [
  { value: '', label: t('salesCommissions.allStatuses') },
  { value: 'frozen', label: t('salesCommissions.statuses.frozen') },
  { value: 'partial_unlocked', label: t('salesCommissions.statuses.partial_unlocked') },
  { value: 'unlocked', label: t('salesCommissions.statuses.unlocked') },
  { value: 'settled', label: t('salesCommissions.statuses.settled') },
  { value: 'settlement_blocked', label: t('salesCommissions.statuses.settlement_blocked') }
])

const recordColumns = [
  { key: 'referee_email', label: t('salesCommissions.refereeUser') },
  { key: 'payment_order_id', label: t('salesCommissions.paymentOrder') },
  { key: 'commission_rate', label: t('salesCommissions.commissionRate') },
  { key: 'commission_context', label: t('salesCommissions.commissionContext') },
  { key: 'commission_total_cny', label: t('salesCommissions.totalCommission') },
  { key: 'frozen_cny', label: t('salesCommissions.frozen') },
  { key: 'unlocked_cny', label: t('salesCommissions.unlocked') },
  { key: 'settleable_cny', label: t('salesCommissions.settleable') },
  { key: 'status', label: t('salesCommissions.status') },
  { key: 'created_at', label: t('salesCommissions.createdAt') }
]

const summaryCards = computed(() => [
  { label: t('salesCommissions.totalCommission'), value: summary.value.total_commission_cny || 0 },
  { label: t('salesCommissions.frozen'), value: summary.value.frozen_cny || 0 },
  { label: t('salesCommissions.settleable'), value: summary.value.settleable_cny || 0 },
  { label: t('salesCommissions.settled'), value: summary.value.settled_cny || 0 }
])

async function loadSummary() {
  loadingSummary.value = true
  try {
    summary.value = await salesCommissionsAPI.getSummary()
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingSummary.value = false
  }
}

async function loadRecords() {
  loadingRecords.value = true
  try {
    const res = await salesCommissionsAPI.listRecords({
      page: pagination.page,
      page_size: pagination.page_size,
      status: statusFilter.value || undefined
    })
    records.value = res.items || []
    pagination.total = res.total || 0
  } catch (error: any) {
    appStore.showError(error?.message || t('salesCommissions.loadFailed'))
  } finally {
    loadingRecords.value = false
  }
}

function loadAll() {
  void Promise.all([loadSummary(), loadRecords()])
}

function setPage(page: number) {
  pagination.page = page
  void loadRecords()
}

function applyStatusFilter() {
  pagination.page = 1
  void loadRecords()
}

function formatCNY(value: number | undefined) {
  return `¥${Number(value || 0).toFixed(2)}`
}

function formatPercent(value: number | undefined) {
  return `${Number(value || 0).toFixed(2)}%`
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

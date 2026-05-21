<template>
  <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900" data-testid="sales-commission-details">
    <button
      type="button"
      class="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
      @click="open = !open"
    >
      <div>
        <div class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('salesCommissions.detailsSection') }}
        </div>
        <div class="mt-0.5 text-xs text-gray-500 dark:text-dark-400">
          {{ open ? t('salesCommissions.hideDetails') : t('salesCommissions.showDetails') }}
        </div>
      </div>
      <span class="text-gray-400">{{ open ? '−' : '+' }}</span>
    </button>

    <div v-if="open" class="border-t border-gray-200 px-4 py-4 dark:border-dark-700">
      <div class="space-y-6">
        <!-- 佣金明细 -->
        <div class="space-y-3">
          <div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end sm:justify-between">
            <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.records') }}</h3>
            <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-[repeat(4,minmax(0,1fr))_auto_auto]">
              <input v-model.trim="recordFiltersLocal.salesUserID" type="number" min="1" class="input" :placeholder="t('salesCommissions.salesUserId')" @keyup.enter="submitRecordFilters" />
              <input v-model.trim="recordFiltersLocal.refereeUserID" type="number" min="1" class="input" :placeholder="t('salesCommissions.refereeUserId')" @keyup.enter="submitRecordFilters" />
              <input v-model.trim="recordFiltersLocal.paymentOrderID" type="number" min="1" class="input" :placeholder="t('salesCommissions.paymentOrderId')" @keyup.enter="submitRecordFilters" />
              <Select v-model="recordFiltersLocal.status" :options="recordStatusOptions" />
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
            <template #cell-payment_order_id="{ row }">{{ row.payment_order_id ? `#${row.payment_order_id}` : '-' }}</template>
            <template #cell-commission_rate="{ row }">{{ formatPercent(row.commission_rate) }}</template>
            <template #cell-commission_total_cny="{ row }">{{ formatCNY(row.commission_total_cny) }}</template>
            <template #cell-settleable_cny="{ row }">{{ formatCNY(row.settleable_cny) }}</template>
            <template #cell-settled_cny="{ row }">{{ formatCNY(row.settled_cny) }}</template>
            <template #cell-status="{ row }">
              <span :class="['badge', statusBadgeClass(row.status)]">{{ statusLabel(row.status) }}</span>
            </template>
            <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
          </DataTable>
          <Pagination v-if="recordPagination.total > 0" :page="recordPagination.page" :total="recordPagination.total" :page-size="recordPagination.page_size" @update:page="(p) => emit('update:recordPage', p)" />
        </div>

        <!-- 结算审计 -->
        <div class="space-y-3">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.settlements') }}</h3>
          <DataTable :columns="settlementColumns" :data="settlements" :loading="loadingSettlements">
            <template #cell-sales_email="{ row }">
              <div class="font-medium">{{ row.sales_email || `#${row.sales_user_id}` }}</div>
            </template>
            <template #cell-amount_cny="{ row }">{{ formatCNY(row.amount_cny) }}</template>
            <template #cell-created_at="{ row }">{{ formatDate(row.created_at) }}</template>
          </DataTable>
          <Pagination v-if="settlementPagination.total > 0" :page="settlementPagination.page" :total="settlementPagination.total" :page-size="settlementPagination.page_size" @update:page="(p) => emit('update:settlementPage', p)" />
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import type { SalesCommissionRecord, SalesCommissionSettlement } from '@/types'

export interface RecordFilters {
  salesUserID: string
  refereeUserID: string
  paymentOrderID: string
  status: string
}

const props = defineProps<{
  records: SalesCommissionRecord[]
  settlements: SalesCommissionSettlement[]
  loadingRecords: boolean
  loadingSettlements: boolean
  recordPagination: { page: number; page_size: number; total: number }
  settlementPagination: { page: number; page_size: number; total: number }
  recordFilters: RecordFilters
}>()

const emit = defineEmits<{
  (e: 'update:recordFilters', value: RecordFilters): void
  (e: 'update:recordPage', page: number): void
  (e: 'update:settlementPage', page: number): void
  (e: 'expand'): void
}>()

const { t } = useI18n()
const open = ref(false)
watch(open, value => { if (value) emit('expand') })

const recordFiltersLocal = reactive<RecordFilters>({ ...props.recordFilters })
watch(() => props.recordFilters, value => { Object.assign(recordFiltersLocal, value) }, { deep: true })

function submitRecordFilters() {
  emit('update:recordFilters', { ...recordFiltersLocal })
}

function resetRecordFilters() {
  recordFiltersLocal.salesUserID = ''
  recordFiltersLocal.refereeUserID = ''
  recordFiltersLocal.paymentOrderID = ''
  recordFiltersLocal.status = ''
  submitRecordFilters()
}

const recordStatusOptions = computed(() => [
  { value: '', label: t('salesCommissions.allStatuses') },
  { value: 'frozen', label: t('salesCommissions.statuses.frozen') },
  { value: 'partial_unlocked', label: t('salesCommissions.statuses.partial_unlocked') },
  { value: 'unlocked', label: t('salesCommissions.statuses.unlocked') },
  { value: 'settled', label: t('salesCommissions.statuses.settled') },
  { value: 'settlement_blocked', label: t('salesCommissions.statuses.settlement_blocked') }
])

const recordColumns = computed(() => [
  { key: 'sales_email', label: t('salesCommissions.salesUser') },
  { key: 'referee_email', label: t('salesCommissions.refereeUser') },
  { key: 'payment_order_id', label: t('salesCommissions.paymentOrder') },
  { key: 'commission_rate', label: t('salesCommissions.commissionRate') },
  { key: 'commission_total_cny', label: t('salesCommissions.totalCommission') },
  { key: 'settleable_cny', label: t('salesCommissions.settleable') },
  { key: 'settled_cny', label: t('salesCommissions.settled') },
  { key: 'status', label: t('salesCommissions.status') },
  { key: 'created_at', label: t('salesCommissions.createdAt') }
])

const settlementColumns = computed(() => [
  { key: 'sales_email', label: t('salesCommissions.salesUser') },
  { key: 'amount_cny', label: t('salesCommissions.amount') },
  { key: 'note', label: t('salesCommissions.note') },
  { key: 'created_at', label: t('salesCommissions.createdAt') }
])

function formatCNY(value: number | undefined) {
  return `¥${Number(value || 0).toFixed(2)}`
}

function formatPercent(value: number | undefined) {
  return `${Number(value || 0).toFixed(2)}%`
}

function formatDate(value: string | undefined) {
  return value ? new Date(value).toLocaleString() : '-'
}

function statusLabel(status: string | undefined) {
  return status ? t(`salesCommissions.statuses.${status}`) : '-'
}

function statusBadgeClass(status: string | undefined) {
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
</script>

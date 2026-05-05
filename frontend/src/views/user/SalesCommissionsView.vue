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
        <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.records') }}</h2>
        <DataTable :columns="recordColumns" :data="records" :loading="loadingRecords">
          <template #cell-referee_email="{ row }">{{ row.referee_email || `#${row.referee_user_id}` }}</template>
          <template #cell-commission_total_cny="{ row }">{{ formatCNY(row.commission_total_cny) }}</template>
          <template #cell-frozen_cny="{ row }">{{ formatCNY(row.frozen_cny) }}</template>
          <template #cell-unlocked_cny="{ row }">{{ formatCNY(row.unlocked_cny) }}</template>
          <template #cell-settleable_cny="{ row }">{{ formatCNY(row.settleable_cny) }}</template>
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
import type { SalesCommissionRecord, SalesCommissionSummary } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const summary = ref<Partial<SalesCommissionSummary>>({})
const records = ref<SalesCommissionRecord[]>([])
const loadingSummary = ref(false)
const loadingRecords = ref(false)
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const recordColumns = [
  { key: 'referee_email', label: t('salesCommissions.refereeUser') },
  { key: 'payment_order_id', label: t('salesCommissions.paymentOrder') },
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
    const res = await salesCommissionsAPI.listRecords({ page: pagination.page, page_size: pagination.page_size })
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

function formatCNY(value: number | undefined) {
  return `¥${Number(value || 0).toFixed(2)}`
}

function formatDate(value: string | undefined) {
  return value ? new Date(value).toLocaleString() : '-'
}

onMounted(loadAll)
</script>

<template>
  <section class="space-y-3" data-testid="sales-commission-summary-table">
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.summaries') }}</h2>
      <input
        v-model="searchInput"
        type="search"
        class="input w-full sm:w-64"
        :placeholder="t('salesCommissions.searchSales')"
        @keyup.enter="emitSearch"
      />
    </div>
    <DataTable :columns="columns" :data="items" :loading="loading">
      <template #cell-sales_email="{ row }">
        <div>
          <div class="font-medium">{{ row.sales_email || '-' }}</div>
          <div class="text-xs text-gray-500">{{ row.sales_username || `#${row.sales_user_id}` }}</div>
        </div>
      </template>
      <template #cell-total_commission_cny="{ row }">{{ formatCNY(row.total_commission_cny) }}</template>
      <template #cell-frozen_cny="{ row }">{{ formatCNY(row.frozen_cny) }}</template>
      <template #cell-settleable_cny="{ row }">{{ formatCNY(row.settleable_cny) }}</template>
      <template #cell-settled_cny="{ row }">{{ formatCNY(row.settled_cny) }}</template>
    </DataTable>
    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="(p) => emit('update:page', p)"
    />
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import type { SalesCommissionSummary } from '@/types'

const props = defineProps<{
  items: SalesCommissionSummary[]
  loading: boolean
  pagination: { page: number; page_size: number; total: number }
  search: string
}>()
const emit = defineEmits<{
  (e: 'update:page', page: number): void
  (e: 'update:search', value: string): void
}>()

const { t } = useI18n()

const searchInput = ref(props.search)
watch(() => props.search, value => { searchInput.value = value })

function emitSearch() {
  emit('update:search', searchInput.value.trim())
}

function formatCNY(value: number | undefined) {
  return `¥${Number(value || 0).toFixed(2)}`
}

const columns = computed(() => [
  { key: 'sales_email', label: t('salesCommissions.salesUser') },
  { key: 'total_commission_cny', label: t('salesCommissions.totalCommission') },
  { key: 'frozen_cny', label: t('salesCommissions.frozen') },
  { key: 'settleable_cny', label: t('salesCommissions.settleable') },
  { key: 'settled_cny', label: t('salesCommissions.settled') },
  { key: 'records_count', label: t('salesCommissions.records') }
])
</script>

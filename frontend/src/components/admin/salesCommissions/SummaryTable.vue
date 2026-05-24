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
      <template #cell-actions="{ row }">
        <button
          type="button"
          class="btn btn-sm btn-primary"
          :disabled="!row.settleable_cny || row.settleable_cny <= 0"
          @click="openSettleDialog(row)"
        >
          {{ t('salesCommissions.settle') }}
        </button>
      </template>
    </DataTable>
    <Pagination
      v-if="pagination.total > 0"
      :page="pagination.page"
      :total="pagination.total"
      :page-size="pagination.page_size"
      @update:page="(p) => emit('update:page', p)"
    />

    <!-- 结算弹窗 -->
    <div v-if="settleDialogVisible" class="fixed inset-0 z-50 flex items-center justify-center bg-black/50" >
      <div class="w-full max-w-md rounded-lg bg-white p-6 shadow-xl dark:bg-dark-900">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-gray-100">{{ t('salesCommissions.settleTitle') }}</h3>
        <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">
          {{ t('salesCommissions.settleFor') }}: <strong>{{ settleTarget?.sales_email }}</strong>
        </p>
        <div class="mt-4 space-y-3">
          <div>
            <label class="input-label">{{ t('salesCommissions.settleAmount') }} (¥)</label>
            <input v-model.number="settleForm.amount_cny" type="number" step="0.01" min="0.01" :max="settleTarget?.settleable_cny" class="input mt-1" />
            <p class="mt-1 text-xs text-gray-500">{{ t('salesCommissions.settleMax') }}: {{ formatCNY(settleTarget?.settleable_cny) }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('salesCommissions.settleNote') }}</label>
            <input v-model="settleForm.note" type="text" class="input mt-1" :placeholder="t('salesCommissions.settleNotePlaceholder')" />
          </div>
        </div>
        <div class="mt-6 flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeSettleDialog">{{ t('common.cancel') }}</button>
          <button type="button" class="btn btn-primary" :disabled="settling || !settleForm.amount_cny || settleForm.amount_cny <= 0" @click="confirmSettle">
            {{ settling ? t('common.loading') : t('salesCommissions.settleConfirm') }}
          </button>
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
  (e: 'settle', payload: { sales_user_id: number; amount_cny: number; note: string }): void
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
  { key: 'records_count', label: t('salesCommissions.records') },
  { key: 'actions', label: t('salesCommissions.actions') }
])

// 结算弹窗逻辑
const settleDialogVisible = ref(false)
const settleTarget = ref<SalesCommissionSummary | null>(null)
const settleForm = reactive({ amount_cny: 0, note: '' })
const settling = ref(false)

function openSettleDialog(row: SalesCommissionSummary) {
  settleTarget.value = row
  settleForm.amount_cny = row.settleable_cny || 0
  settleForm.note = ''
  settleDialogVisible.value = true
}

function closeSettleDialog() {
  settleDialogVisible.value = false
  settleTarget.value = null
}

function confirmSettle() {
  if (!settleTarget.value || settleForm.amount_cny <= 0) return
  settling.value = true
  emit('settle', {
    sales_user_id: settleTarget.value.sales_user_id,
    amount_cny: settleForm.amount_cny,
    note: settleForm.note
  })
}

defineExpose({ closeSettleDialog, settling })
</script>

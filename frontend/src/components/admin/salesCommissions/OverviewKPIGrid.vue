<template>
  <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
    <div
      v-for="card in cards"
      :key="card.key"
      class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900"
      :data-testid="`sales-commission-kpi-${card.key}`"
    >
      <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ card.label }}</div>
      <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ card.value }}</div>
      <div v-if="card.hint" class="mt-1 text-xs text-gray-500 dark:text-dark-500">{{ card.hint }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SalesCommissionOverviewKPI } from '@/types'

const props = defineProps<{ kpi: SalesCommissionOverviewKPI }>()
const { t } = useI18n()

function formatCNY(value: number | undefined) {
  return `¥${Number(value || 0).toFixed(2)}`
}

function formatPercent(value: number | undefined) {
  return `${Number(value || 0).toFixed(2)}%`
}

function formatInt(value: number | undefined) {
  return String(Math.trunc(Number(value || 0)))
}

const cards = computed(() => [
  {
    key: 'related_order_amount',
    label: t('salesCommissions.kpi.relatedOrderAmount'),
    value: formatCNY(props.kpi.related_order_amount_cny),
    hint: t('salesCommissions.kpiHints.relatedOrderAmount')
  },
  {
    key: 'commission_total',
    label: t('salesCommissions.kpi.commissionTotal'),
    value: formatCNY(props.kpi.commission_total_cny),
    hint: t('salesCommissions.kpiHints.commissionTotal')
  },
  {
    key: 'frozen',
    label: t('salesCommissions.kpi.frozen'),
    value: formatCNY(props.kpi.frozen_cny),
    hint: t('salesCommissions.kpiHints.frozen')
  },
  {
    key: 'settleable',
    label: t('salesCommissions.kpi.settleable'),
    value: formatCNY(props.kpi.settleable_cny),
    hint: t('salesCommissions.kpiHints.settleable')
  },
  {
    key: 'settled',
    label: t('salesCommissions.kpi.settled'),
    value: formatCNY(props.kpi.settled_cny),
    hint: t('salesCommissions.kpiHints.settled')
  },
  {
    key: 'active_sales_users',
    label: t('salesCommissions.kpi.activeSalesUsers'),
    value: formatInt(props.kpi.active_sales_users),
    hint: t('salesCommissions.kpiHints.activeSalesUsers')
  },
  {
    key: 'threshold_met_users',
    label: t('salesCommissions.kpi.thresholdMetUsers'),
    value: formatInt(props.kpi.threshold_met_users),
    hint: t('salesCommissions.kpiHints.thresholdMetUsers')
  },
  {
    key: 'avg_commission_rate',
    label: t('salesCommissions.kpi.avgCommissionRate'),
    value: formatPercent(props.kpi.avg_commission_rate),
    hint: t('salesCommissions.kpiHints.avgCommissionRate')
  }
])
</script>

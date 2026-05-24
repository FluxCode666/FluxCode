<template>
  <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900" data-testid="sales-commission-top-sales">
    <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">
      {{ t('salesCommissions.charts.topSales') }}
    </h3>
    <div v-if="!hasData" class="flex h-56 items-center justify-center text-sm text-gray-500 dark:text-dark-400">
      {{ t('salesCommissions.charts.noData') }}
    </div>
    <div v-else class="h-56">
      <Bar :data="(chartData as any)" :options="(chartOptions as any)" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  BarElement,
  Tooltip,
  Legend
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import type { SalesCommissionTopSalesItem } from '@/types'

ChartJS.register(CategoryScale, LinearScale, BarElement, Tooltip, Legend)

const props = defineProps<{ items: SalesCommissionTopSalesItem[] }>()
const { t } = useI18n()

const hasData = computed(() => (props.items ?? []).length > 0)

function labelFor(item: SalesCommissionTopSalesItem) {
  return item.sales_email || item.sales_username || `#${item.sales_user_id}`
}

const chartData = computed(() => ({
  labels: props.items.map(labelFor),
  datasets: [
    {
      label: t('salesCommissions.kpi.commissionTotal'),
      backgroundColor: 'rgba(59, 130, 246, 0.7)',
      borderColor: 'rgba(59, 130, 246, 1)',
      data: props.items.map(it => Number(it.commission_total_cny || 0))
    }
  ]
}))

const chartOptions = computed(() => ({
  indexAxis: 'y' as const,
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (ctx: any) => `¥${Number(ctx.parsed.x || 0).toFixed(2)}`
      }
    }
  },
  scales: {
    x: { beginAtZero: true, ticks: { callback: (v: any) => `¥${Number(v).toLocaleString()}` } }
  }
}))
</script>

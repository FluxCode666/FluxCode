<template>
  <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900" data-testid="sales-commission-monthly-trend">
    <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">
      {{ t('salesCommissions.charts.monthlyTrend') }}
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
  PointElement,
  LineElement,
  LineController,
  BarController,
  Title,
  Tooltip,
  Legend
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import type { SalesCommissionMonthlyTrendItem } from '@/types'

ChartJS.register(CategoryScale, LinearScale, BarElement, PointElement, LineElement, LineController, BarController, Title, Tooltip, Legend)

const props = defineProps<{ trend: SalesCommissionMonthlyTrendItem[] }>()
const { t } = useI18n()

function formatMonthLabel(value: string) {
  if (!value) return ''
  const d = new Date(value)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const hasData = computed(() => (props.trend ?? []).some(it => it.related_order_amount_cny > 0 || it.commission_total_cny > 0))

const chartData = computed(() => ({
  labels: props.trend.map(it => formatMonthLabel(it.month)),
  datasets: [
    {
      type: 'bar' as const,
      label: t('salesCommissions.charts.monthlyTrendOrder'),
      backgroundColor: 'rgba(59, 130, 246, 0.55)',
      borderColor: 'rgba(59, 130, 246, 0.9)',
      yAxisID: 'y',
      data: props.trend.map(it => Number(it.related_order_amount_cny || 0))
    },
    {
      type: 'line' as const,
      label: t('salesCommissions.charts.monthlyTrendCommission'),
      borderColor: 'rgba(16, 185, 129, 1)',
      backgroundColor: 'rgba(16, 185, 129, 0.2)',
      tension: 0.25,
      yAxisID: 'y1',
      data: props.trend.map(it => Number(it.commission_total_cny || 0))
    }
  ]
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: {
    legend: { position: 'bottom' as const, labels: { boxWidth: 10 } },
    tooltip: {
      callbacks: {
        label: (ctx: any) => `${ctx.dataset.label}: ¥${Number(ctx.parsed.y || 0).toFixed(2)}`
      }
    }
  },
  scales: {
    y: {
      type: 'linear' as const,
      position: 'left' as const,
      beginAtZero: true,
      ticks: { callback: (v: any) => `¥${Number(v).toLocaleString()}` }
    },
    y1: {
      type: 'linear' as const,
      position: 'right' as const,
      beginAtZero: true,
      grid: { drawOnChartArea: false },
      ticks: { callback: (v: any) => `¥${Number(v).toLocaleString()}` }
    }
  }
}))
</script>

<template>
  <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900" data-testid="sales-commission-status-breakdown">
    <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-gray-100">
      {{ t('salesCommissions.charts.statusBreakdown') }}
    </h3>
    <div v-if="!hasData" class="flex h-56 items-center justify-center text-sm text-gray-500 dark:text-dark-400">
      {{ t('salesCommissions.charts.noData') }}
    </div>
    <div v-else class="h-56">
      <Doughnut :data="(chartData as any)" :options="(chartOptions as any)" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import type { SalesCommissionStatusBreakdown } from '@/types'

ChartJS.register(ArcElement, Tooltip, Legend)

const props = defineProps<{ data: SalesCommissionStatusBreakdown }>()
const { t } = useI18n()

const hasData = computed(() => {
  const d = props.data
  return Number(d?.frozen_cny || 0) + Number(d?.settleable_cny || 0) + Number(d?.settled_cny || 0) > 0
})

const chartData = computed(() => ({
  labels: [
    t('salesCommissions.kpi.frozen'),
    t('salesCommissions.kpi.settleable'),
    t('salesCommissions.kpi.settled')
  ],
  datasets: [
    {
      data: [
        Number(props.data?.frozen_cny || 0),
        Number(props.data?.settleable_cny || 0),
        Number(props.data?.settled_cny || 0)
      ],
      backgroundColor: ['#f59e0b', '#3b82f6', '#10b981'],
      borderWidth: 0
    }
  ]
}))

const chartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  cutout: '60%',
  plugins: {
    legend: { position: 'bottom' as const, labels: { boxWidth: 10 } },
    tooltip: {
      callbacks: {
        label: (ctx: any) => `${ctx.label}: ¥${Number(ctx.parsed || 0).toFixed(2)}`
      }
    }
  }
}))
</script>

<template>
  <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900" data-testid="sales-commission-mode-breakdown">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
        {{ t('salesCommissions.charts.modeBreakdown') }}
      </h3>
      <div class="inline-flex rounded-md border border-gray-200 p-0.5 text-xs dark:border-dark-700">
        <button
          type="button"
          class="rounded px-2 py-1"
          :class="metric === 'commission' ? 'bg-primary-600 text-white' : 'text-gray-600 dark:text-dark-300'"
          @click="metric = 'commission'"
        >{{ t('salesCommissions.modeBreakdown.byCommission') }}</button>
        <button
          type="button"
          class="rounded px-2 py-1"
          :class="metric === 'count' ? 'bg-primary-600 text-white' : 'text-gray-600 dark:text-dark-300'"
          @click="metric = 'count'"
        >{{ t('salesCommissions.modeBreakdown.byCount') }}</button>
      </div>
    </div>
    <div v-if="!hasData" class="flex h-56 items-center justify-center text-sm text-gray-500 dark:text-dark-400">
      {{ t('salesCommissions.charts.noData') }}
    </div>
    <div v-else class="h-56">
      <Doughnut :data="(chartData as any)" :options="(chartOptions as any)" />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import type { SalesCommissionModeBreakdown } from '@/types'

ChartJS.register(ArcElement, Tooltip, Legend)

const props = defineProps<{ data: SalesCommissionModeBreakdown }>()
const { t } = useI18n()

const metric = ref<'commission' | 'count'>('commission')

const values = computed(() => {
  if (metric.value === 'count') {
    return [Number(props.data?.fixed_records || 0), Number(props.data?.tiered_records || 0)]
  }
  return [Number(props.data?.fixed_commission_cny || 0), Number(props.data?.tiered_commission_cny || 0)]
})

const hasData = computed(() => values.value.some(v => v > 0))

const chartData = computed(() => ({
  labels: [
    t('salesCommissions.modeBreakdown.fixed'),
    t('salesCommissions.modeBreakdown.tiered')
  ],
  datasets: [
    {
      data: values.value,
      backgroundColor: ['#6366f1', '#f97316'],
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
        label: (ctx: any) => {
          if (metric.value === 'count') return `${ctx.label}: ${Number(ctx.parsed || 0)}`
          return `${ctx.label}: ¥${Number(ctx.parsed || 0).toFixed(2)}`
        }
      }
    }
  }
}))
</script>

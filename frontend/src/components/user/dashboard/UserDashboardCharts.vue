<template>
  <div class="space-y-6">
    <!-- Time Range Tabs -->
    <div class="card p-4">
      <div class="flex flex-wrap items-center gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('dashboard.timeRange') }}:</span>
          <div class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-gray-700 dark:bg-dark-700">
            <button
              v-for="opt in timeRangeOptions"
              :key="opt.value"
              type="button"
              @click="$emit('update:timeRange', opt.value)"
              :class="[
                'rounded-md px-3 py-1 text-sm font-medium transition-colors',
                timeRange === opt.value
                  ? 'bg-white text-primary-600 shadow-sm dark:bg-dark-800 dark:text-primary-400'
                  : 'text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'
              ]"
            >
              {{ t(opt.labelKey) }}
            </button>
          </div>
        </div>
        <button @click="$emit('refresh')" :disabled="loading" class="btn btn-secondary">
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <!-- Charts Grid -->
    <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <!-- Model Distribution Chart -->
      <div class="card relative overflow-hidden p-4">
        <div v-if="loading" class="absolute inset-0 z-10 flex items-center justify-center bg-white/50 backdrop-blur-sm dark:bg-dark-800/50">
          <LoadingSpinner size="md" />
        </div>
        <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('dashboard.modelDistribution') }}</h3>
        <div class="flex items-center gap-6">
          <div class="h-48 w-48">
            <Doughnut v-if="modelData" :data="modelData" :options="doughnutOptions" />
            <div v-else class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400">{{ t('dashboard.noDataAvailable') }}</div>
          </div>
          <div class="max-h-48 flex-1 overflow-y-auto">
            <table class="w-full text-xs">
              <thead>
                <tr class="text-gray-500 dark:text-gray-400">
                  <th class="pb-2 text-left">{{ t('dashboard.model') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.requests') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.tokens') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.actual') }}</th>
                  <th class="pb-2 text-right">{{ t('dashboard.standard') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="model in models" :key="model.model" class="border-t border-gray-100 dark:border-gray-700">
                  <td class="max-w-[100px] truncate py-1.5 font-medium text-gray-900 dark:text-white" :title="model.model">{{ model.model }}</td>
                  <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatNumber(model.requests) }}</td>
                  <td class="py-1.5 text-right text-gray-600 dark:text-gray-400">{{ formatTokens(model.total_tokens) }}</td>
                  <td class="py-1.5 text-right text-green-600 dark:text-green-400">${{ formatCost(model.actual_cost) }}</td>
                  <td class="py-1.5 text-right text-gray-400 dark:text-gray-500">${{ formatCost(model.cost) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Token Usage Trend Chart -->
      <TokenUsageTrend :trend-data="trend" :loading="loading" />
    </div>

    <!-- Full-width Charts -->
    <CostUsageTrend
      :trend-data="trend"
      :loading="loading"
      :granularity="granularity as 'day' | 'hour'"
      :title="t('dashboard.costUsageTrend')"
    />
    <RequestCountTrend
      :trend-data="trend"
      :loading="loading"
      :granularity="granularity as 'day' | 'hour'"
      :title="t('dashboard.requestCountTrend')"
      :series-label="t('dashboard.requests')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { Doughnut } from 'vue-chartjs'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import CostUsageTrend from '@/components/charts/CostUsageTrend.vue'
import RequestCountTrend from '@/components/charts/RequestCountTrend.vue'
import type { TrendDataPoint, ModelStat } from '@/types'
import { formatCostFixed as formatCost, formatNumberLocaleString as formatNumber, formatTokensK as formatTokens } from '@/utils/format'
import { Chart as ChartJS, CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler } from 'chart.js'
ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, ArcElement, Title, Tooltip, Legend, Filler)

type TimeRangeTab = '24h' | '7d' | '14d' | '30d'

const props = defineProps<{ loading: boolean, timeRange: TimeRangeTab, granularity: string, trend: TrendDataPoint[], models: ModelStat[] }>()
defineEmits<{
  (e: 'update:timeRange', value: TimeRangeTab): void
  (e: 'refresh'): void
}>()
const { t } = useI18n()

const timeRangeOptions: { value: TimeRangeTab; labelKey: string }[] = [
  { value: '24h', labelKey: 'dashboard.range24Hours' },
  { value: '7d', labelKey: 'dashboard.range7Days' },
  { value: '14d', labelKey: 'dashboard.range14Days' },
  { value: '30d', labelKey: 'dashboard.range30Days' }
]

const modelData = computed(() => !props.models?.length ? null : {
  labels: props.models.map((m: ModelStat) => m.model),
  datasets: [{
    data: props.models.map((m: ModelStat) => m.total_tokens),
    backgroundColor: ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16']
  }]
})

const doughnutOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.label}: ${formatTokens(context.parsed)} tokens`
      }
    }
  }
}
</script>

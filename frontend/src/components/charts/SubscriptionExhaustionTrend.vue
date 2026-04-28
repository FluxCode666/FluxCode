<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.dashboard.subscriptionExhaustionTrend') }}
    </h3>
    <div v-if="loading" class="flex h-48 items-center justify-center">
      <LoadingSpinner />
    </div>
    <div v-else-if="trendData.length > 0 && chartData" class="h-48">
      <Line :data="chartData" :options="lineOptions" />
    </div>
    <div
      v-else
      class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.dashboard.noDataAvailable') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { SubscriptionExhaustionTrendPoint } from '@/types'

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler
)

const { t } = useI18n()

const props = defineProps<{
  trendData: SubscriptionExhaustionTrendPoint[]
  loading?: boolean
}>()

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  total: '#2563eb',
  exhausted: '#ef4444',
  rate: '#10b981'
}))

const chartData = computed(() => {
  if (!props.trendData?.length) return null

  return {
    labels: props.trendData.map((d) => d.date),
    datasets: [
      {
        label: t('admin.dashboard.totalSubscriptions'),
        data: props.trendData.map((d) => normalizeCount(d.total_subscriptions)),
        borderColor: chartColors.value.total,
        backgroundColor: `${chartColors.value.total}20`,
        fill: true,
        tension: 0.3
      },
      {
        label: t('admin.dashboard.exhaustedSubscriptions'),
        data: props.trendData.map((d) => normalizeCount(d.exhausted_subscriptions)),
        borderColor: chartColors.value.exhausted,
        backgroundColor: `${chartColors.value.exhausted}20`,
        fill: true,
        tension: 0.3
      },
      {
        label: t('admin.dashboard.exhaustionRate'),
        data: props.trendData.map((d) => normalizeRate(d.exhaustion_rate)),
        borderColor: chartColors.value.rate,
        backgroundColor: `${chartColors.value.rate}20`,
        borderDash: [5, 5],
        fill: false,
        tension: 0.3,
        yAxisID: 'yPercent'
      }
    ]
  }
})

const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  elements: {
    point: {
      radius: 0,
      hoverRadius: 4,
      hitRadius: 6
    }
  },
  interaction: {
    intersect: false,
    mode: 'index' as const
  },
  plugins: {
    legend: {
      position: 'top' as const,
      labels: {
        color: chartColors.value.text,
        usePointStyle: true,
        pointStyle: 'circle',
        padding: 15,
        font: {
          size: 11
        }
      }
    },
    tooltip: {
      callbacks: {
        label: (context: any) => {
          if (context.dataset.yAxisID === 'yPercent') {
            return `${context.dataset.label}: ${formatPercent(Number(context.raw))}`
          }
          return `${context.dataset.label}: ${formatCount(Number(context.raw))}`
        }
      }
    }
  },
  scales: {
    x: {
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        }
      }
    },
    y: {
      beginAtZero: true,
      grid: {
        color: chartColors.value.grid
      },
      ticks: {
        color: chartColors.value.text,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatCount(Number(value))
      }
    },
    yPercent: {
      position: 'right' as const,
      min: 0,
      max: 100,
      grid: {
        drawOnChartArea: false
      },
      ticks: {
        color: chartColors.value.rate,
        font: {
          size: 10
        },
        callback: (value: string | number) => formatPercent(Number(value))
      }
    }
  }
}))

const normalizeCount = (value: number): number => {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.round(value))
}

const normalizeRate = (value: number): number => {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, value))
}

const formatCount = (value: number): string => {
  const safeValue = normalizeCount(value)
  if (safeValue >= 1_000_000_000) return `${(safeValue / 1_000_000_000).toFixed(2)}B`
  if (safeValue >= 1_000_000) return `${(safeValue / 1_000_000).toFixed(2)}M`
  if (safeValue >= 1_000) return `${(safeValue / 1_000).toFixed(2)}K`
  return safeValue.toLocaleString()
}

const formatPercent = (value: number): string => `${normalizeRate(value).toFixed(1)}%`
</script>

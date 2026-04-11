<template>
  <div class="card p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ title }}
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
      {{ t('dashboard.noDataAvailable') }}
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
import type { TrendDataPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, Filler)

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    trendData: TrendDataPoint[]
    title: string
    loading?: boolean
    granularity?: 'day' | 'hour'
  }>(),
  {
    loading: false,
    granularity: 'day'
  }
)

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  actualCost: '#8b5cf6',
  standardCost: '#94a3b8'
}))

const toNonNegativeCost = (value: number): number => {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, value)
}

const formatHourMinuteLabel = (label: string): string => {
  const trimmed = label.trim()
  if (!trimmed) return ''
  const splitter = trimmed.includes(' ') ? ' ' : trimmed.includes('T') ? 'T' : ''
  const timePart = splitter ? trimmed.split(splitter).pop() || '' : trimmed
  const cleaned = timePart.replace(/Z|[+-]\d{2}:?\d{2}$/, '')
  return cleaned.slice(0, 5)
}

const formatXAxisLabel = (label: string): string => {
  if (props.granularity !== 'hour') return label
  return formatHourMinuteLabel(label)
}

const chartData = computed(() => {
  if (!props.trendData?.length) return null

  return {
    labels: props.trendData.map((d) => d.date),
    datasets: [
      {
        label: t('dashboard.actual'),
        data: props.trendData.map((d) => toNonNegativeCost(d.actual_cost)),
        borderColor: chartColors.value.actualCost,
        backgroundColor: `${chartColors.value.actualCost}20`,
        fill: true,
        tension: 0.3,
        borderWidth: 2
      },
      {
        label: t('dashboard.standard'),
        data: props.trendData.map((d) => toNonNegativeCost(d.cost)),
        borderColor: chartColors.value.standardCost,
        backgroundColor: `${chartColors.value.standardCost}10`,
        fill: false,
        tension: 0.3,
        borderWidth: 2,
        borderDash: [5, 5]
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
        label: (context: any) => `${context.dataset.label}: $${formatCost(Number(context.raw))}`
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
        },
        callback: function (value: string | number) {
          const label =
            typeof value === 'string'
              ? value
              : (this as { getLabelForValue?: (v: string | number) => string }).getLabelForValue?.(
                  value
                ) ?? String(value)
          return formatXAxisLabel(label)
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
        callback: (value: string | number) => `$${formatCost(Number(value))}`
      }
    }
  }
}))

const formatCost = (value: number): string => {
  if (!Number.isFinite(value)) return '0'
  const safeValue = Math.max(0, value)
  if (safeValue >= 1000) {
    return (safeValue / 1000).toFixed(2) + 'K'
  } else if (safeValue >= 1) {
    return safeValue.toFixed(2)
  } else if (safeValue >= 0.01) {
    return safeValue.toFixed(3)
  }
  return safeValue.toFixed(4)
}
</script>

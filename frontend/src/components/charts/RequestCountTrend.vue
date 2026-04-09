<template>
  <div class="card overflow-hidden p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ title }}
    </h3>
    <div class="h-64 lg:h-72">
      <div v-if="loading" class="flex h-full items-center justify-center">
        <LoadingSpinner />
      </div>
      <Line v-else-if="trendData.length > 0 && chartData" :data="chartData" :options="lineOptions" />
      <div
        v-else
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('dashboard.noDataAvailable') }}
      </div>
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
  Legend
} from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { TrendDataPoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend)

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    trendData: TrendDataPoint[]
    title: string
    seriesLabel: string
    loading?: boolean
    granularity?: 'day' | 'hour'
  }>(),
  {
    granularity: 'day',
    loading: false
  }
)

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  line: '#06b6d4'
}))

const toNonNegativeRequest = (value: number): number => {
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
        label: props.seriesLabel,
        data: props.trendData.map((d) => toNonNegativeRequest(d.requests)),
        borderColor: chartColors.value.line,
        backgroundColor: `${chartColors.value.line}1f`,
        borderWidth: 3,
        fill: false,
        tension: 0.3
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
      display: false
    },
    tooltip: {
      callbacks: {
        label: (context: any) => `${context.dataset.label}: ${formatRequests(Number(context.raw))}`
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
        callback: (value: string | number) => formatRequests(Number(value))
      }
    }
  }
}))

const formatRequests = (value: number): string => {
  if (!Number.isFinite(value)) return '0'
  const safeValue = Math.max(0, value)
  if (safeValue >= 1_000_000_000) return `${(safeValue / 1_000_000_000).toFixed(2)}B`
  if (safeValue >= 1_000_000) return `${(safeValue / 1_000_000).toFixed(2)}M`
  if (safeValue >= 1_000) return `${(safeValue / 1_000).toFixed(2)}K`
  return Math.round(safeValue).toLocaleString()
}
</script>

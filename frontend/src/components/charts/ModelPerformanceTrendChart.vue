<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  Filler,
  Legend,
  LineElement,
  LinearScale,
  PointElement,
  Title,
  Tooltip
} from 'chart.js'
import { Line } from 'vue-chartjs'
import type { ModelPerformanceRange, ModelPerformanceTrendPoint } from '@/api/modelPricing'
import EmptyState from '@/components/common/EmptyState.vue'
import { formatHistoryLabel } from '@/views/admin/ops/utils/opsFormatters'

ChartJS.register(Title, Tooltip, Legend, LineElement, LinearScale, PointElement, CategoryScale, Filler)

type ModelPerformanceTrendMetric = 'average_first_token_ms' | 'availability'

interface Props {
  points: ModelPerformanceTrendPoint[]
  metric: ModelPerformanceTrendMetric
  range: ModelPerformanceRange
  loading?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  loading: false
})
const { t } = useI18n()

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))
const metricConfig = computed(() => {
  if (props.metric === 'availability') {
    return {
      label: t('modelPricing.performance.availability', '可用率'),
      color: '#10b981',
      colorAlpha: '#10b98120',
      unit: '%'
    }
  }
  return {
    label: t('modelPricing.performance.averageFirstToken', '首字时长'),
    color: '#3b82f6',
    colorAlpha: '#3b82f620',
    unit: ' ms'
  }
})

const colors = computed(() => ({
  grid: isDarkMode.value ? '#374151' : '#f3f4f6',
  text: isDarkMode.value ? '#9ca3af' : '#6b7280'
}))

function formatBucketStart(bucketStart: string): string {
  // Ops 的标签工具已经按访客时区格式化。7d 使用同一小时粒度，
  // 需传入 168h 才能保留日期，避免跨天的同一小时标签发生歧义。
  return formatHistoryLabel(bucketStart, props.range === '7d' ? '168h' : '24h')
}

function formatMetricValue(value: unknown): string {
  const number = typeof value === 'number' ? value : Number(value)
  if (!Number.isFinite(number)) return '-'
  return `${number.toFixed(1)}${metricConfig.value.unit}`
}

const chartData = computed(() => {
  const values = props.points.map((point) => point[props.metric])
  if (!values.some((value) => typeof value === 'number' && Number.isFinite(value))) return null

  return {
    labels: props.points.map((point) => formatBucketStart(point.bucket_start)),
    datasets: [
      {
        label: metricConfig.value.label,
        data: values,
        borderColor: metricConfig.value.color,
        backgroundColor: metricConfig.value.colorAlpha,
        fill: true,
        tension: 0.35,
        spanGaps: false,
        pointRadius: 0,
        pointHitRadius: 10
      }
    ]
  }
})

const options = computed(() => {
  const c = colors.value
  return {
    responsive: true,
    maintainAspectRatio: false,
    interaction: { intersect: false, mode: 'index' as const },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: isDarkMode.value ? '#1f2937' : '#ffffff',
        titleColor: isDarkMode.value ? '#f3f4f6' : '#111827',
        bodyColor: isDarkMode.value ? '#d1d5db' : '#4b5563',
        borderColor: c.grid,
        borderWidth: 1,
        padding: 10,
        displayColors: false,
        callbacks: {
          label: (context: any) => `${context.dataset.label}: ${formatMetricValue(context.raw)}`
        }
      }
    },
    scales: {
      x: {
        type: 'category' as const,
        grid: { display: false },
        ticks: {
          color: c.text,
          font: { size: 10 },
          maxTicksLimit: 8,
          autoSkip: true,
          autoSkipPadding: 10
        }
      },
      y: {
        type: 'linear' as const,
        min: 0,
        max: props.metric === 'availability' ? 100 : undefined,
        grid: { color: c.grid, borderDash: [4, 4] },
        ticks: {
          color: c.text,
          font: { size: 10 },
          callback: (value: string | number) => formatMetricValue(value)
        }
      }
    }
  }
})
</script>

<template>
  <div class="flex h-full min-h-[18rem] flex-col rounded-3xl bg-white p-6 shadow-sm ring-1 ring-gray-900/5 dark:bg-dark-800 dark:ring-dark-700">
    <div class="mb-4 flex shrink-0 items-center justify-between gap-3">
      <h3 class="text-sm font-bold text-gray-900 dark:text-white">{{ metricConfig.label }}</h3>
      <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('modelPricing.performance.hourly', '按小时') }}</span>
    </div>

    <div class="min-h-0 flex-1">
      <Line v-if="chartData" :data="chartData" :options="options" />
      <div v-else class="flex h-full items-center justify-center">
        <div v-if="loading" class="animate-pulse text-sm text-gray-400">{{ t('common.loading', '加载中') }}</div>
        <EmptyState v-else :title="t('common.noData', '暂无数据')" :description="t('modelPricing.performance.noTrendData', '当前时间范围暂无性能数据')" />
      </div>
    </div>
  </div>
</template>

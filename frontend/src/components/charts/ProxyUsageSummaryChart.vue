<template>
  <div class="card overflow-visible p-4">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">
      {{ title }}
    </h3>
    <div ref="chartContainerRef" class="h-[24rem] sm:h-[26rem]" :style="chartContainerStyle">
      <div v-if="loading" class="flex h-full items-center justify-center">
        <LoadingSpinner />
      </div>
      <div v-else-if="chartData" class="flex h-full min-h-0 flex-col gap-3">
        <div ref="legendAreaRef" class="space-y-2 text-sm lg:text-xs">
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-gray-700 dark:text-gray-300">{{
              t('admin.dashboard.metricLegend')
            }}</span>
            <button
              v-for="metric in metricLegendItems"
              :key="metric.key"
              type="button"
              class="inline-flex items-center gap-2 rounded-md px-2.5 py-1.5 text-gray-700 transition-colors dark:text-gray-300"
              :class="
                isMetricHidden(metric.key)
                  ? 'bg-gray-50 opacity-60 line-through dark:bg-dark-800'
                  : 'bg-gray-100 dark:bg-dark-700'
              "
              @click="toggleMetric(metric.key)"
            >
              <span
                class="inline-block w-7 border-t-[3px] lg:w-5 lg:border-t-2"
                :style="getMetricLineStyle(metric.borderStyle)"
              />
              <span>{{ metric.label }}</span>
            </button>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <span class="font-medium text-gray-700 dark:text-gray-300">{{
              t('admin.dashboard.proxyLegend')
            }}</span>
            <button
              v-for="proxy in proxyLegendItems"
              :key="proxy.proxyId"
              type="button"
              class="inline-flex items-center gap-2 rounded-md px-2.5 py-1.5 text-gray-700 transition-colors dark:text-gray-300"
              :class="
                isProxyHidden(proxy.proxyId)
                  ? 'bg-gray-50 opacity-60 line-through dark:bg-dark-800'
                  : 'bg-gray-100 dark:bg-dark-700'
              "
              :title="proxy.label"
              @click="toggleProxy(proxy.proxyId)"
            >
              <span
                class="inline-block h-3.5 w-3.5 rounded-full lg:h-2.5 lg:w-2.5"
                :style="{ backgroundColor: proxy.color }"
              />
              <span class="max-w-56 truncate">{{ proxy.label }}</span>
            </button>
          </div>
        </div>
        <div ref="chartAreaRef" class="relative min-h-0 flex-1">
          <Line :data="chartData" :options="lineOptions" />
          <div
            v-if="tooltipState.visible && tooltipState.rows.length > 0"
            class="absolute z-20 w-[22rem] max-w-[calc(100%-1rem)] rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
            :style="{ left: `${tooltipState.left}px`, top: `${tooltipState.top}px` }"
            @mouseenter="onTooltipMouseEnter"
            @mouseleave="onTooltipMouseLeave"
          >
            <div
              class="border-b border-gray-100 px-3 py-2 text-xs font-semibold text-gray-700 dark:border-dark-600 dark:text-gray-200"
            >
              {{ tooltipState.title }}
            </div>
            <div class="max-h-56 space-y-1 overflow-y-auto px-3 py-2">
              <div
                v-for="(row, idx) in tooltipState.rows"
                :key="`${row.label}-${idx}`"
                class="flex items-start gap-2 text-xs"
              >
                <span
                  class="mt-1 inline-block h-2 w-2 shrink-0 rounded-full"
                  :style="{ backgroundColor: row.color }"
                />
                <span class="min-w-0 flex-1 break-all text-gray-700 dark:text-gray-300">
                  {{ row.label }}
                </span>
                <span class="shrink-0 font-medium text-gray-900 dark:text-gray-100">
                  {{ row.value }}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
      <div
        v-else
        class="flex h-full items-center justify-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.dashboard.noDataAvailable') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
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
import type { ChartOptions, TooltipItem } from 'chart.js'
import { Line } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { ProxyUsageSummaryItem, ProxyUsageTimelinePoint } from '@/types'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend)

type MetricKey = 'total_count' | 'success_count' | 'failure_count'
type MetricLegendItem = {
  key: MetricKey
  label: string
  borderStyle: 'solid' | 'dashed' | 'dotted'
}
type ProxyLegendItem = {
  proxyId: number
  color: string
  label: string
  item: ProxyUsageSummaryItem
}
type TooltipRow = {
  label: string
  value: string
  color: string
}
type TooltipState = {
  visible: boolean
  left: number
  top: number
  title: string
  rows: TooltipRow[]
}

const { t } = useI18n()

const props = withDefaults(
  defineProps<{
    items: ProxyUsageSummaryItem[]
    title: string
    loading?: boolean
    granularity?: 'day' | 'hour'
    timelineLabels?: string[]
  }>(),
  {
    loading: false,
    granularity: 'hour',
    timelineLabels: () => []
  }
)

const proxyPalette = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#f97316',
  '#84cc16',
  '#ec4899',
  '#6366f1'
]

const metricLegendItems = computed<MetricLegendItem[]>(() => [
  {
    key: 'total_count' as MetricKey,
    label: t('admin.dashboard.totalCount'),
    borderStyle: 'solid'
  },
  {
    key: 'success_count' as MetricKey,
    label: t('admin.dashboard.successCount'),
    borderStyle: 'dashed'
  },
  {
    key: 'failure_count' as MetricKey,
    label: t('admin.dashboard.failureCount'),
    borderStyle: 'dotted'
  }
])

const hiddenMetricKeys = ref<Set<MetricKey>>(new Set())
const hiddenProxyIDs = ref<Set<number>>(new Set())
const chartContainerRef = ref<HTMLElement | null>(null)
const legendAreaRef = ref<HTMLElement | null>(null)
const chartAreaRef = ref<HTMLElement | null>(null)
const dynamicContainerHeight = ref<number | null>(null)
const tooltipPinned = ref(false)
const tooltipState = ref<TooltipState>({
  visible: false,
  left: 8,
  top: 8,
  title: '',
  rows: []
})
let tooltipHideTimer: ReturnType<typeof setTimeout> | null = null
let heightObserver: ResizeObserver | null = null

const MOBILE_BREAKPOINT = 640
const MOBILE_BASE_HEIGHT = 384
const DESKTOP_BASE_HEIGHT = 416
const MOBILE_MIN_CHART_HEIGHT = 220
const DESKTOP_MIN_CHART_HEIGHT = 250
const LEGEND_CHART_GAP = 12

watch(
  () => props.items,
  (items) => {
    const defaultHidden = new Set<number>()
    ;(items || []).forEach((item) => {
      if ((item.proxy_status || '').trim().toLowerCase() === 'inactive') {
        defaultHidden.add(item.proxy_id)
      }
    })
    hiddenProxyIDs.value = defaultHidden
  },
  { immediate: true }
)

const isDarkMode = computed(() => document.documentElement.classList.contains('dark'))

const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e7eb' : '#374151',
  grid: isDarkMode.value ? '#374151' : '#e5e7eb',
  metricLegend: isDarkMode.value ? '#9ca3af' : '#6b7280'
}))

const isDesktopViewport = (): boolean => {
  if (typeof window === 'undefined') return true
  return window.innerWidth >= MOBILE_BREAKPOINT
}

const updateChartContainerHeight = (): void => {
  const baseHeight = isDesktopViewport() ? DESKTOP_BASE_HEIGHT : MOBILE_BASE_HEIGHT
  const minChartHeight = isDesktopViewport() ? DESKTOP_MIN_CHART_HEIGHT : MOBILE_MIN_CHART_HEIGHT
  const legendHeight = Math.ceil(legendAreaRef.value?.getBoundingClientRect().height || 0)
  const computedHeight =
    legendHeight > 0 ? Math.max(baseHeight, legendHeight + LEGEND_CHART_GAP + minChartHeight) : baseHeight
  dynamicContainerHeight.value = computedHeight
}

const chartContainerStyle = computed((): Record<string, string> => {
  if (!dynamicContainerHeight.value) {
    return {}
  }
  return {
    minHeight: `${Math.round(dynamicContainerHeight.value)}px`
  }
})

const setupHeightObserver = (): void => {
  if (heightObserver) {
    heightObserver.disconnect()
    heightObserver = null
  }
  if (typeof ResizeObserver === 'undefined') return

  heightObserver = new ResizeObserver(() => {
    updateChartContainerHeight()
  })

  if (chartContainerRef.value) {
    heightObserver.observe(chartContainerRef.value)
  }
  if (legendAreaRef.value) {
    heightObserver.observe(legendAreaRef.value)
  }
}

const handleViewportResize = (): void => {
  updateChartContainerHeight()
}

const getMetricLineStyle = (borderStyle: 'solid' | 'dashed' | 'dotted'): Record<string, string> => ({
  borderTopStyle: borderStyle,
  borderColor: chartColors.value.metricLegend
})

const isMetricHidden = (key: MetricKey): boolean => hiddenMetricKeys.value.has(key)

const toggleMetric = (key: MetricKey): void => {
  const next = new Set(hiddenMetricKeys.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  hiddenMetricKeys.value = next
}

const getMetricValue = (point: ProxyUsageTimelinePoint, metric: MetricKey): number => {
  if (metric === 'total_count') return Math.max(0, point.total_count || 0)
  if (metric === 'success_count') return Math.max(0, point.success_count || 0)
  return Math.max(0, point.failure_count || 0)
}

const getProxyLegendLabel = (item: ProxyUsageSummaryItem): string => {
  const addr = (item.proxy_addr || '').trim()
  const name = (item.proxy_name || '').trim()
  if (addr && name && name !== addr) {
    return `${addr} (${name})`
  }
  return addr || name || `#${item.proxy_id}`
}

const proxyLegendItems = computed<ProxyLegendItem[]>(() =>
  (props.items || []).map((item, index) => ({
    proxyId: item.proxy_id,
    color: proxyPalette[index % proxyPalette.length],
    label: getProxyLegendLabel(item),
    item
  }))
)

const isProxyHidden = (proxyID: number): boolean => hiddenProxyIDs.value.has(proxyID)

const toggleProxy = (proxyID: number): void => {
  const next = new Set(hiddenProxyIDs.value)
  if (next.has(proxyID)) {
    next.delete(proxyID)
  } else {
    next.add(proxyID)
  }
  hiddenProxyIDs.value = next
}

const timelineLabels = computed(() => {
  const base = (props.timelineLabels || []).filter((label) => !!label)
  if (base.length > 0) {
    return base
  }
  const labels = new Set<string>()
  ;(props.items || []).forEach((item) => {
    ;(item.points || []).forEach((point) => {
      if (point?.bucket) {
        labels.add(point.bucket)
      }
    })
  })
  return Array.from(labels).sort()
})

const chartData = computed(() => {
  if (!proxyLegendItems.value.length || !timelineLabels.value.length) return null

  const visibleProxies = proxyLegendItems.value.filter((proxy) => !isProxyHidden(proxy.proxyId))
  const visibleMetrics = metricLegendItems.value.filter((metric) => !isMetricHidden(metric.key))
  const datasets = visibleProxies.flatMap((proxy) => {
    const pointsMap = new Map((proxy.item.points || []).map((point) => [point.bucket, point]))
    return visibleMetrics.map((metric) => ({
      label: `${proxy.label} · ${metric.label}`,
      data: timelineLabels.value.map((bucket) => {
        const point = pointsMap.get(bucket)
        return point ? getMetricValue(point, metric.key) : 0
      }),
      borderColor: proxy.color,
      backgroundColor: `${proxy.color}20`,
      pointBackgroundColor: proxy.color,
      pointBorderColor: proxy.color,
      borderDash: metric.borderStyle === 'dashed' ? [8, 5] : metric.borderStyle === 'dotted' ? [2, 5] : [],
      tension: 0.3,
      fill: false,
      borderWidth: 3
    }))
  })

  return {
    labels: timelineLabels.value,
    datasets
  }
})

watch(
  () => chartData.value,
  () => {
    tooltipPinned.value = false
    hideTooltip()
  }
)

const formatHourMinuteLabel = (label: string): string => {
  const trimmed = label.trim()
  if (!trimmed) return ''
  const splitter = trimmed.includes(' ') ? ' ' : trimmed.includes('T') ? 'T' : ''
  const timePart = splitter ? trimmed.split(splitter).pop() || '' : trimmed
  const cleaned = timePart.replace(/Z|[+-]\d{2}:?\d{2}$/, '')
  return cleaned.slice(0, 5)
}

const formatBucketLabel = (value: string): string => {
  const label = value.trim()
  if (!label) return ''
  if (props.granularity === 'hour') {
    return formatHourMinuteLabel(label)
  }
  return label.length >= 10 ? label.slice(5) : label
}

const getLabelByTick = (value: string | number): string => {
  if (typeof value === 'string') return value
  const index = Number(value)
  if (!Number.isFinite(index)) return String(value)
  return timelineLabels.value[index] || String(value)
}

const clearTooltipHideTimer = (): void => {
  if (tooltipHideTimer) {
    clearTimeout(tooltipHideTimer)
    tooltipHideTimer = null
  }
}

const hideTooltip = (): void => {
  clearTooltipHideTimer()
  tooltipState.value = {
    visible: false,
    left: tooltipState.value.left,
    top: tooltipState.value.top,
    title: tooltipState.value.title,
    rows: tooltipState.value.rows
  }
}

const scheduleTooltipHide = (): void => {
  clearTooltipHideTimer()
  tooltipHideTimer = setTimeout(() => {
    if (!tooltipPinned.value) {
      hideTooltip()
    }
  }, 120)
}

const onTooltipMouseEnter = (): void => {
  tooltipPinned.value = true
  clearTooltipHideTimer()
}

const onTooltipMouseLeave = (): void => {
  tooltipPinned.value = false
  hideTooltip()
}

const clamp = (value: number, min: number, max: number): number => {
  if (value < min) return min
  if (value > max) return max
  return value
}

const pointOverlapsPanel = (
  pointX: number,
  pointY: number,
  left: number,
  top: number,
  width: number,
  height: number,
  guard: number
): boolean => {
  return (
    pointX >= left-guard &&
    pointX <= left+width+guard &&
    pointY >= top-guard &&
    pointY <= top+height+guard
  )
}

const chooseTooltipPosition = (
  caretX: number,
  caretY: number,
  chartWidth: number,
  chartHeight: number,
  panelWidth: number,
  panelHeight: number
): { left: number; top: number } => {
  const minLeft = 8
  const minTop = 8
  const maxLeft = Math.max(minLeft, chartWidth-panelWidth-8)
  const maxTop = Math.max(minTop, chartHeight-panelHeight-8)
  const preferLeft = caretX >= chartWidth * 0.55
  const preferTop = caretY >= chartHeight * 0.55
  const sidePrimary = preferLeft ? -1 : 1
  const sideSecondary = -sidePrimary
  const verticalPrimary = preferTop ? -1 : 1
  const verticalSecondary = -verticalPrimary

  const sideOffset = 16
  const verticalOffset = 16
  const centeredOffset = 18

  const place = (side: 1 | -1, vertical: 1 | -1): { left: number; top: number } => ({
    left: side === 1 ? caretX + sideOffset : caretX - panelWidth - sideOffset,
    top: vertical === 1 ? caretY + verticalOffset : caretY - panelHeight - verticalOffset
  })

  const centered = (vertical: 1 | -1): { left: number; top: number } => ({
    left: caretX - panelWidth/2,
    top: vertical === 1 ? caretY + centeredOffset : caretY - panelHeight - centeredOffset
  })

  const candidates = [
    place(sidePrimary as 1 | -1, verticalPrimary as 1 | -1),
    place(sidePrimary as 1 | -1, verticalSecondary as 1 | -1),
    place(sideSecondary as 1 | -1, verticalPrimary as 1 | -1),
    place(sideSecondary as 1 | -1, verticalSecondary as 1 | -1),
    centered(verticalPrimary as 1 | -1),
    centered(verticalSecondary as 1 | -1)
  ]

  let fallback = { left: minLeft, top: minTop }
  let fallbackDistance = -1

  for (const candidate of candidates) {
    const left = clamp(candidate.left, minLeft, maxLeft)
    const top = clamp(candidate.top, minTop, maxTop)
    const overlap = pointOverlapsPanel(caretX, caretY, left, top, panelWidth, panelHeight, 14)
    if (!overlap) {
      return { left, top }
    }

    const centerX = left + panelWidth/2
    const centerY = top + panelHeight/2
    const distance = (centerX-caretX)*(centerX-caretX) + (centerY-caretY)*(centerY-caretY)
    if (distance > fallbackDistance) {
      fallbackDistance = distance
      fallback = { left, top }
    }
  }

  return fallback
}

const updateExternalTooltip = (context: { chart: { canvas: HTMLCanvasElement }; tooltip: any }): void => {
  const area = chartAreaRef.value
  if (!area) return

  const tooltip = context.tooltip
  if (!tooltip || tooltip.opacity === 0) {
    if (!tooltipPinned.value) {
      scheduleTooltipHide()
    }
    return
  }

  clearTooltipHideTimer()

  const rows: TooltipRow[] = (tooltip.dataPoints || []).map((point: any) => {
    const rawValue = Number(point.raw)
    const borderColor = point.dataset?.borderColor
    const datasetColor = Array.isArray(borderColor)
      ? String(borderColor[0] || chartColors.value.text)
      : String(borderColor || chartColors.value.text)
    return {
      label: String(point.dataset?.label || ''),
      value: Number.isFinite(rawValue) ? `${Math.round(rawValue)}` : String(point.raw ?? ''),
      color: datasetColor
    }
  })

  if (rows.length === 0) {
    hideTooltip()
    return
  }

  const rawTitle = Array.isArray(tooltip.title) ? String(tooltip.title[0] || '') : ''
  const title = formatBucketLabel(rawTitle)

  const width = area.clientWidth
  const height = area.clientHeight
  const panelWidth = Math.min(352, Math.max(220, width - 16))
  const panelHeight = Math.min(260, 48 + rows.length * 22)
  const { left: nextLeft, top: nextTop } = chooseTooltipPosition(
    Number(tooltip.caretX) || 0,
    Number(tooltip.caretY) || 0,
    width,
    height,
    panelWidth,
    panelHeight
  )

  tooltipState.value = {
    visible: true,
    left: nextLeft,
    top: nextTop,
    title,
    rows
  }
}

const lineOptions = computed<ChartOptions<'line'>>(() => ({
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
      enabled: false,
      external: updateExternalTooltip,
      callbacks: {
        label: (context: TooltipItem<'line'>) => {
          const value = Number(context.raw)
          return `${context.dataset.label || ''}: ${Number.isFinite(value) ? Math.round(value) : context.raw}`
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
        },
        autoSkip: true,
        maxTicksLimit: 12,
        callback: (value: string | number) => {
          const label = getLabelByTick(value)
          return formatBucketLabel(label)
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
        precision: 0,
        stepSize: 1,
        callback: (value: string | number) => {
          const numeric = Number(value)
          return Number.isFinite(numeric) ? `${Math.round(numeric)}` : value
        }
      }
    }
  }
}))

watch(
  [() => props.items, () => props.timelineLabels, () => props.loading],
  async () => {
    await nextTick()
    setupHeightObserver()
    updateChartContainerHeight()
  },
  { deep: true, immediate: true }
)

onMounted(async () => {
  window.addEventListener('resize', handleViewportResize)
  await nextTick()
  setupHeightObserver()
  updateChartContainerHeight()
})

onBeforeUnmount(() => {
  clearTooltipHideTimer()
  window.removeEventListener('resize', handleViewportResize)
  if (heightObserver) {
    heightObserver.disconnect()
    heightObserver = null
  }
})
</script>

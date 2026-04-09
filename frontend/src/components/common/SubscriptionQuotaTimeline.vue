<template>
  <div
    class="space-y-3 rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800/70"
  >
    <div class="flex items-center justify-between gap-3">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ title }}</h4>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('userSubscriptions.timeline.segmentCount', { count: segments.length }) }}
      </span>
    </div>

    <div class="relative">
      <div class="h-3 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
        <div class="flex h-full w-full">
          <button
            v-for="item in segmentPositions"
            :key="item.segment.key"
            type="button"
            :class="[
              item.segment.colorClass,
              'h-full border-r border-white/90 p-0 last:border-r-0 dark:border-dark-900/90'
            ]"
            :style="{ width: `${item.segment.widthPct}%` }"
            :title="buildSegmentTitle(item.segment)"
            @mouseenter="setHovered(item.segment.key)"
            @mouseleave="clearHovered"
            @focus="setHovered(item.segment.key)"
            @blur="clearHovered"
          >
            <span class="sr-only">{{ buildSegmentTitle(item.segment) }}</span>
          </button>
        </div>
      </div>

      <div
        v-if="hoveredSegment"
        class="pointer-events-none absolute top-0 z-10 min-w-[240px] max-w-[420px] -translate-x-1/2 -translate-y-[calc(100%+0.5rem)] rounded-md bg-gray-900 px-2 py-1 text-[11px] leading-tight text-white shadow-lg dark:bg-black"
        :style="{ left: `${hoveredSegment.centerPct}%` }"
      >
        {{ buildSegmentTitle(hoveredSegment.segment) }}
      </div>
    </div>

    <div class="space-y-1.5">
      <div v-for="segment in segments" :key="`line-${segment.key}`" class="flex items-start gap-2">
        <span :class="[segment.colorClass, 'mt-1 h-2.5 w-2.5 rounded-full']"></span>
        <span class="text-xs text-gray-600 dark:text-gray-300">
          {{
            t('userSubscriptions.timeline.segmentLine', {
              start: segment.startLabel,
              end: segment.endLabel,
              quota: segment.quotaText
            })
          }}
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

interface TimelineSegmentView {
  key: string
  startLabel: string
  endLabel: string
  quotaText: string
  widthPct: number
  colorClass: string
}

interface Props {
  title: string
  segments: TimelineSegmentView[]
}

const props = defineProps<Props>()
const { t } = useI18n()
const hoveredSegmentKey = ref<string | null>(null)

interface TimelineSegmentPosition {
  segment: TimelineSegmentView
  centerPct: number
}

const clampPct = (value: number): number => Math.min(98, Math.max(2, value))

const segmentPositions = computed<TimelineSegmentPosition[]>(() => {
  let offset = 0
  return props.segments.map((segment) => {
    const centerPct = clampPct(offset + segment.widthPct / 2)
    offset += segment.widthPct
    return { segment, centerPct }
  })
})

const hoveredSegment = computed<TimelineSegmentPosition | null>(() => {
  if (!hoveredSegmentKey.value) return null
  return segmentPositions.value.find((item) => item.segment.key === hoveredSegmentKey.value) || null
})

const setHovered = (segmentKey: string): void => {
  hoveredSegmentKey.value = segmentKey
}

const clearHovered = (): void => {
  hoveredSegmentKey.value = null
}

const buildSegmentTitle = (segment: TimelineSegmentView): string => {
  return t('userSubscriptions.timeline.segmentLine', {
    start: segment.startLabel,
    end: segment.endLabel,
    quota: segment.quotaText
  })
}
</script>


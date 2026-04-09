<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Detail View -->
      <div v-else-if="selectedSubscription" class="space-y-4">
        <div ref="detailCardRef" class="card flex flex-col items-stretch justify-start">
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <button
                type="button"
                class="inline-flex h-9 w-9 items-center justify-center rounded-lg border border-gray-200 text-gray-600 transition-colors hover:bg-gray-100 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-700"
                :aria-label="t('userSubscriptions.closeDetail')"
                @click="closeSubscriptionDetail"
              >
                <svg
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="1.5"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
              <div>
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('userSubscriptions.detailTitle') }}
                </h2>
                <p class="text-sm text-gray-500 dark:text-dark-400">
                  {{ selectedSubscription.group?.name || `Group #${selectedSubscription.group_id}` }}
                </p>
              </div>
            </div>
            <span
              :class="[
                'badge',
                selectedSubscription.status === 'active'
                  ? 'badge-success'
                  : selectedSubscription.status === 'expired'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ t(`userSubscriptions.status.${selectedSubscription.status}`) }}
            </span>
          </div>

          <div class="space-y-4 p-4">
            <div
              v-if="selectedSubscription.expires_at"
              class="flex items-center justify-between rounded-xl bg-gray-50 px-3 py-2 text-sm dark:bg-dark-800"
            >
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(selectedSubscription.expires_at)">
                {{ formatExpirationDate(selectedSubscription.expires_at) }}
              </span>
            </div>

            <div
              v-else
              class="flex items-center justify-between rounded-xl bg-gray-50 px-3 py-2 text-sm dark:bg-dark-800"
            >
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <div v-if="detailLoading" class="flex justify-center py-6">
              <div
                class="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
              ></div>
            </div>

            <p v-else-if="detailError" class="text-sm text-red-600 dark:text-red-400">
              {{ detailError }}
            </p>

            <template v-else>
              <div v-if="hasAnyTokenLimit && timelineWindows.length > 0" class="space-y-3">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('userSubscriptions.timeline.title') }}
                </h3>
                <SubscriptionQuotaTimeline
                  v-for="timeline in timelineWindows"
                  :key="timeline.key"
                  :title="timeline.label"
                  :segments="timeline.segments"
                />
              </div>
            </template>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card p-12 text-center">
        <div
          class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
        >
          <Icon name="creditCard" size="xl" class="text-gray-400" />
        </div>
        <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
          {{ t('userSubscriptions.noActiveSubscriptions') }}
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') }}
        </p>
      </div>

      <!-- Subscriptions Grid -->
      <div v-else class="grid items-start gap-6 lg:grid-cols-2">
        <button
          v-for="subscription in subscriptions"
          :key="subscription.id"
          type="button"
          class="card card-hover flex h-full flex-col items-stretch justify-start overflow-hidden text-left"
          @click="openSubscriptionDetail(subscription, $event)"
        >
          <!-- Header -->
          <div
            class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700"
          >
            <div class="flex items-center gap-3">
              <div
                class="flex h-10 w-10 items-center justify-center rounded-xl bg-purple-100 dark:bg-purple-900/30"
              >
                <Icon name="creditCard" size="md" class="text-purple-600 dark:text-purple-400" />
              </div>
              <div>
                <h3 class="font-semibold text-gray-900 dark:text-white">
                  {{ subscription.group?.name || `Group #${subscription.group_id}` }}
                  <span
                    v-if="(subscription.quota_multiplier ?? 1) > 1"
                    class="ml-2 inline-flex items-center rounded-md bg-purple-100 px-1.5 py-0.5 text-xs font-semibold text-purple-700 dark:bg-purple-900/30 dark:text-purple-300"
                  >
                    ×{{ subscription.quota_multiplier }}
                  </span>
                </h3>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group?.description || '' }}
                </p>
              </div>
            </div>
            <span
              :class="[
                'badge',
                subscription.status === 'active'
                  ? 'badge-success'
                  : subscription.status === 'expired'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ t(`userSubscriptions.status.${subscription.status}`) }}
            </span>
          </div>

          <!-- Usage Progress -->
          <div class="flex-1 space-y-4 p-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) }}
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{
                t('userSubscriptions.expires')
              }}</span>
              <span class="text-gray-700 dark:text-gray-300">{{
                t('userSubscriptions.noExpiration')
              }}</span>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.daily') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                    getEffectiveLimit(subscription.group.daily_limit_usd, subscription).toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.daily_usage_usd,
                      getEffectiveLimit(subscription.group.daily_limit_usd, subscription)
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.daily_usage_usd,
                      getEffectiveLimit(subscription.group.daily_limit_usd, subscription)
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.daily_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.daily_window_start, 24)
                  })
                }}
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscription.group?.weekly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.weekly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }} / ${{
                    getEffectiveLimit(subscription.group.weekly_limit_usd, subscription).toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.weekly_usage_usd,
                      getEffectiveLimit(subscription.group.weekly_limit_usd, subscription)
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.weekly_usage_usd,
                      getEffectiveLimit(subscription.group.weekly_limit_usd, subscription)
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.weekly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.weekly_window_start, 168)
                  })
                }}
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscription.group?.monthly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.monthly') }}
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                    getEffectiveLimit(
                      subscription.group.monthly_limit_usd,
                      subscription
                    ).toFixed(2)
                  }}
                </span>
              </div>
              <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="
                    getProgressBarClass(
                      subscription.monthly_usage_usd,
                      getEffectiveLimit(subscription.group.monthly_limit_usd, subscription)
                    )
                  "
                  :style="{
                    width: getProgressWidth(
                      subscription.monthly_usage_usd,
                      getEffectiveLimit(subscription.group.monthly_limit_usd, subscription)
                    )
                  }"
                ></div>
              </div>
              <p
                v-if="subscription.monthly_window_start"
                class="text-xs text-gray-500 dark:text-dark-400"
              >
                {{
                  t('userSubscriptions.resetIn', {
                    time: formatResetTime(subscription.monthly_window_start, 720)
                  })
                }}
              </p>
            </div>

            <!-- No limits configured - Unlimited badge -->
            <div
              v-if="
                !subscription.group?.daily_limit_usd &&
                !subscription.group?.weekly_limit_usd &&
                !subscription.group?.monthly_limit_usd
              "
              class="flex items-center justify-center rounded-xl bg-gradient-to-r from-emerald-50 to-teal-50 py-6 dark:from-emerald-900/20 dark:to-teal-900/20"
            >
              <div class="flex items-center gap-3">
                <span class="text-4xl text-emerald-600 dark:text-emerald-400">∞</span>
                <div>
                  <p class="text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    {{ t('userSubscriptions.unlimited') }}
                  </p>
                  <p class="text-xs text-emerald-600/70 dark:text-emerald-400/70">
                    {{ t('userSubscriptions.unlimitedDesc') }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </button>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import subscriptionsAPI from '@/api/subscriptions'
import type {
  AdminSubscriptionGrantUsage,
  UserSubscription,
  UserSubscriptionGrantUsageResponse
} from '@/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import SubscriptionQuotaTimeline from '@/components/common/SubscriptionQuotaTimeline.vue'
import { formatCurrency, formatDateTime } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()

const subscriptions = ref<UserSubscription[]>([])
const loading = ref(true)

const selectedSubscription = ref<UserSubscription | null>(null)
const detailCardRef = ref<HTMLElement | null>(null)
const isClosingDetail = ref(false)
const detailLoading = ref(false)
const detailError = ref('')
const grantsDetailCache = ref<Record<number, UserSubscriptionGrantUsageResponse>>({})
const DETAIL_ZOOM_IN_MS = 460
const DETAIL_ZOOM_OUT_MS = 340
const DETAIL_EASING_IN = 'cubic-bezier(0.18, 0.88, 0.2, 1)'
const DETAIL_EASING_OUT = 'cubic-bezier(0.4, 0, 0.2, 1)'

interface RectSnapshot {
  left: number
  top: number
  width: number
  height: number
}

const detailOriginRect = ref<RectSnapshot | null>(null)
const prefersReducedMotion = ref(false)
let detailAnimation: Animation | null = null
let mediaQueryList: MediaQueryList | null = null
let motionChangeHandler: ((event: MediaQueryListEvent) => void) | null = null

const TIMELINE_COLORS = [
  'bg-blue-600',
  'bg-orange-500',
  'bg-violet-600',
  'bg-lime-500',
  'bg-rose-600',
  'bg-cyan-500',
  'bg-amber-600',
  'bg-emerald-600',
  'bg-fuchsia-600',
  'bg-sky-500'
]

type TimelineWindow = 'daily' | 'weekly' | 'monthly'

interface RawTimelineSegment {
  startMs: number
  endMs: number
  quota: number
}

interface TimelineSegmentView {
  key: string
  startLabel: string
  endLabel: string
  quotaText: string
  widthPct: number
  colorClass: string
}

interface TimelineWindowView {
  key: TimelineWindow
  label: string
  segments: TimelineSegmentView[]
}

const selectedGrantsDetail = computed<UserSubscriptionGrantUsageResponse | null>(() => {
  if (!selectedSubscription.value) return null
  return grantsDetailCache.value[selectedSubscription.value.id] || null
})

const hasAnyTokenLimit = computed(() => {
  const group = selectedSubscription.value?.group
  if (!group) return false
  return (
    (group.daily_limit_usd ?? 0) > 0 ||
    (group.weekly_limit_usd ?? 0) > 0 ||
    (group.monthly_limit_usd ?? 0) > 0
  )
})

const timelineWindows = computed<TimelineWindowView[]>(() => {
  const sub = selectedSubscription.value
  const detail = selectedGrantsDetail.value
  if (!sub || !detail || !hasAnyTokenLimit.value) return []

  const nowMs = Date.now()
  const rawWindows: Array<{
    key: TimelineWindow
    label: string
    segments: RawTimelineSegment[]
  }> = []

  const allWindows: Array<{ key: TimelineWindow; label: string; limit: number | null }> = [
    { key: 'daily', label: t('userSubscriptions.daily'), limit: sub.group?.daily_limit_usd ?? null },
    { key: 'weekly', label: t('userSubscriptions.weekly'), limit: sub.group?.weekly_limit_usd ?? null },
    {
      key: 'monthly',
      label: t('userSubscriptions.monthly'),
      limit: sub.group?.monthly_limit_usd ?? null
    }
  ]

  for (const item of allWindows) {
    if (!item.limit || item.limit <= 0) continue

    let segments = buildTimelineSegments(detail.grants, item.limit, nowMs)
    if (segments.length === 0) {
      segments = buildFallbackTimelineSegments(sub, item.limit, nowMs)
    }
    if (segments.length === 0) continue
    rawWindows.push({ key: item.key, label: item.label, segments })
  }

  if (rawWindows.length === 0) return []

  const quotaColorMap = buildQuotaColorMap(rawWindows.flatMap((item) => item.segments))

  return rawWindows.map((item) => {
    const timelineStart = item.segments[0].startMs
    const timelineEnd = item.segments[item.segments.length - 1].endMs
    const totalMs = Math.max(1, timelineEnd - timelineStart)

    const segments: TimelineSegmentView[] = item.segments.map((segment, idx) => {
      const widthPct = Math.max(2, ((segment.endMs - segment.startMs) / totalMs) * 100)
      const startLabel =
        segment.startMs === nowMs
          ? t('userSubscriptions.timeline.now')
          : formatTimelineDateTime(segment.startMs)
      const endLabel = formatTimelineDateTime(segment.endMs)
      const quotaText = formatCurrency(segment.quota)

      return {
        key: `${item.key}-${segment.startMs}-${segment.endMs}-${idx}`,
        startLabel,
        endLabel,
        quotaText,
        widthPct,
        colorClass: quotaColorMap.get(normalizeQuota(segment.quota)) || TIMELINE_COLORS[0]
      }
    })

    return {
      key: item.key,
      label: item.label,
      segments
    }
  })
})

const normalizeQuota = (quota: number): string => quota.toFixed(6)

function buildQuotaColorMap(segments: RawTimelineSegment[]): Map<string, string> {
  const unique = Array.from(new Set(segments.map((segment) => normalizeQuota(segment.quota))))
  unique.sort((a, b) => Number(a) - Number(b))

  const colorMap = new Map<string, string>()
  unique.forEach((quota, index) => {
    colorMap.set(quota, TIMELINE_COLORS[index % TIMELINE_COLORS.length])
  })
  return colorMap
}

function buildTimelineSegments(
  grants: AdminSubscriptionGrantUsage[],
  baseLimit: number,
  nowMs: number
): RawTimelineSegment[] {
  if (!Number.isFinite(baseLimit) || baseLimit <= 0) return []

  const intervals: Array<{ startMs: number; endMs: number }> = []
  const boundaries = new Set<number>([nowMs])

  for (const grant of grants) {
    const startMs = new Date(grant.starts_at).getTime()
    const endMs = new Date(grant.expires_at).getTime()

    if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs <= nowMs) {
      continue
    }

    intervals.push({ startMs, endMs })
    if (startMs > nowMs) boundaries.add(startMs)
    boundaries.add(endMs)
  }

  const points = Array.from(boundaries).sort((a, b) => a - b)
  if (points.length < 2) return []

  const raw: RawTimelineSegment[] = []
  for (let i = 0; i < points.length - 1; i++) {
    const segmentStart = points[i]
    const segmentEnd = points[i + 1]
    if (segmentEnd <= segmentStart) continue

    let activeCount = 0
    for (const interval of intervals) {
      if (interval.startMs <= segmentStart && interval.endMs > segmentStart) {
        activeCount += 1
      }
    }

    if (activeCount <= 0) continue
    raw.push({
      startMs: segmentStart,
      endMs: segmentEnd,
      quota: activeCount * baseLimit
    })
  }

  if (raw.length === 0) return []

  const merged: RawTimelineSegment[] = []
  for (const segment of raw) {
    const last = merged[merged.length - 1]
    if (
      last &&
      normalizeQuota(last.quota) === normalizeQuota(segment.quota) &&
      last.endMs === segment.startMs
    ) {
      last.endMs = segment.endMs
    } else {
      merged.push({ ...segment })
    }
  }

  return merged
}

function buildFallbackTimelineSegments(
  subscription: UserSubscription,
  baseLimit: number,
  nowMs: number
): RawTimelineSegment[] {
  if (!subscription.expires_at) return []
  const expiresAtMs = new Date(subscription.expires_at).getTime()
  if (!Number.isFinite(expiresAtMs) || expiresAtMs <= nowMs) return []

  const multiplier = Math.max(1, subscription.quota_multiplier ?? 1)
  return [
    {
      startMs: nowMs,
      endMs: expiresAtMs,
      quota: multiplier * baseLimit
    }
  ]
}

async function loadSubscriptions() {
  try {
    loading.value = true
    const now = Date.now()
    const all = await subscriptionsAPI.getMySubscriptions()
    subscriptions.value = all.filter((sub) => {
      if (sub.status === 'expired') return false
      if (!sub.expires_at) return true
      const expiresAt = new Date(sub.expires_at).getTime()
      if (Number.isNaN(expiresAt)) return true
      return expiresAt > now
    })
  } catch (error) {
    console.error('Failed to load subscriptions:', error)
    appStore.showError(t('userSubscriptions.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%'
  const percentage = Math.min(((used || 0) / limit) * 100, 100)
  return `${percentage}%`
}

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400'
  const percentage = ((used || 0) / limit) * 100
  if (percentage >= 100) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

function snapshotRect(el: HTMLElement): RectSnapshot {
  const rect = el.getBoundingClientRect()
  return {
    left: rect.left,
    top: rect.top,
    width: rect.width,
    height: rect.height
  }
}

function stopDetailAnimation() {
  if (detailAnimation) {
    detailAnimation.cancel()
    detailAnimation = null
  }
}

function buildOriginTransform(originRect: RectSnapshot, targetRect: DOMRect): string {
  const translateX = originRect.left - targetRect.left
  const translateY = originRect.top - targetRect.top
  return `translate(${translateX}px, ${translateY}px)`
}

async function playDetailFromOriginAnimation(mode: 'in' | 'out'): Promise<void> {
  const originRect = detailOriginRect.value
  const detailEl = detailCardRef.value

  if (!originRect || !detailEl || prefersReducedMotion.value) return

  const targetRect = detailEl.getBoundingClientRect()
  const originTransform = buildOriginTransform(originRect, targetRect)
  detailEl.style.transformOrigin = '0 0'
  detailEl.style.overflow = 'hidden'

  stopDetailAnimation()

  const keyframes: Keyframe[] =
    mode === 'in'
      ? [
          {
            transform: originTransform,
            width: `${originRect.width}px`,
            height: `${originRect.height}px`,
            opacity: 0.24
          },
          {
            transform: 'translate(0px, 0px)',
            width: `${targetRect.width}px`,
            height: `${targetRect.height}px`,
            opacity: 1
          }
        ]
      : [
          {
            transform: 'translate(0px, 0px)',
            width: `${targetRect.width}px`,
            height: `${targetRect.height}px`,
            opacity: 1
          },
          {
            transform: originTransform,
            width: `${originRect.width}px`,
            height: `${originRect.height}px`,
            opacity: 0.22
          }
        ]

  const animation = detailEl.animate(keyframes, {
    duration: mode === 'in' ? DETAIL_ZOOM_IN_MS : DETAIL_ZOOM_OUT_MS,
    easing: mode === 'in' ? DETAIL_EASING_IN : DETAIL_EASING_OUT,
    fill: mode === 'in' ? 'none' : 'forwards'
  })
  detailAnimation = animation

  try {
    await animation.finished
  } catch {
    // 动画可能被快速交互取消
  } finally {
    if (detailAnimation === animation) {
      detailAnimation = null
    }
    detailEl.style.removeProperty('width')
    detailEl.style.removeProperty('height')
    detailEl.style.removeProperty('overflow')
    detailEl.style.removeProperty('transform')
    detailEl.style.removeProperty('opacity')
    detailEl.style.removeProperty('transform-origin')
  }
}

async function openSubscriptionDetail(subscription: UserSubscription, event?: MouseEvent) {
  if (isClosingDetail.value) return

  const sourceEl = event?.currentTarget instanceof HTMLElement ? event.currentTarget : null
  if (sourceEl) {
    detailOriginRect.value = snapshotRect(sourceEl)
  }

  stopDetailAnimation()
  isClosingDetail.value = false
  selectedSubscription.value = subscription
  detailError.value = ''

  let fetchPromise: Promise<void> | null = null
  if (grantsDetailCache.value[subscription.id]) {
    detailLoading.value = false
  } else {
    detailLoading.value = true
    fetchPromise = (async () => {
      try {
        const detail = await subscriptionsAPI.getSubscriptionGrants(subscription.id)
        grantsDetailCache.value = {
          ...grantsDetailCache.value,
          [subscription.id]: detail
        }
      } catch (error) {
        console.error('Failed to load subscription detail:', error)
        detailError.value = t('userSubscriptions.detailLoadFailed')
        appStore.showError(detailError.value)
      } finally {
        detailLoading.value = false
      }
    })()
  }

  await nextTick()
  await playDetailFromOriginAnimation('in')
  if (fetchPromise) {
    await fetchPromise
  }
}

async function closeSubscriptionDetail() {
  if (!selectedSubscription.value || isClosingDetail.value) return

  isClosingDetail.value = true
  await playDetailFromOriginAnimation('out')
  stopDetailAnimation()

  selectedSubscription.value = null
  detailError.value = ''
  detailLoading.value = false
  isClosingDetail.value = false
}

function getQuotaMultiplier(sub: UserSubscription): number {
  return Math.max(1, sub.quota_multiplier ?? 1)
}

function getEffectiveLimit(limit: number | null | undefined, sub: UserSubscription): number {
  return (limit ?? 0) * getQuotaMultiplier(sub)
}

function formatExpirationDate(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (days < 0) {
    return t('userSubscriptions.status.expired')
  }

  const dateStr = formatDateTime(expires)

  if (days === 0) {
    return `${dateStr} (Today)`
  }
  if (days === 1) {
    return `${dateStr} (Tomorrow)`
  }

  return t('userSubscriptions.daysRemaining', { days }) + ` (${dateStr})`
}

function getExpirationClass(expiresAt: string): string {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24))

  if (days <= 0) return 'text-red-600 dark:text-red-400 font-medium'
  if (days <= 3) return 'text-red-600 dark:text-red-400'
  if (days <= 7) return 'text-orange-600 dark:text-orange-400'
  return 'text-gray-700 dark:text-gray-300'
}

function formatResetTime(windowStart: string | null, windowHours: number): string {
  if (!windowStart) return t('userSubscriptions.windowNotActive')

  const start = new Date(windowStart)
  const end = new Date(start.getTime() + windowHours * 60 * 60 * 1000)
  const now = new Date()
  const diff = end.getTime() - now.getTime()

  if (diff <= 0) return t('userSubscriptions.windowNotActive')

  const hours = Math.floor(diff / (1000 * 60 * 60))
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))

  if (hours > 24) {
    const days = Math.floor(hours / 24)
    const remainingHours = hours % 24
    return `${days}d ${remainingHours}h`
  }

  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }

  return `${minutes}m`
}

onMounted(() => {
  loadSubscriptions()

  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    prefersReducedMotion.value = false
    return
  }

  mediaQueryList = window.matchMedia('(prefers-reduced-motion: reduce)')
  prefersReducedMotion.value = mediaQueryList.matches
  motionChangeHandler = (event: MediaQueryListEvent) => {
    prefersReducedMotion.value = event.matches
  }

  if (typeof mediaQueryList.addEventListener === 'function') {
    mediaQueryList.addEventListener('change', motionChangeHandler)
  } else {
    mediaQueryList.addListener(motionChangeHandler)
  }
})

onBeforeUnmount(() => {
  stopDetailAnimation()
  if (mediaQueryList && motionChangeHandler) {
    if (typeof mediaQueryList.removeEventListener === 'function') {
      mediaQueryList.removeEventListener('change', motionChangeHandler)
    } else {
      mediaQueryList.removeListener(motionChangeHandler)
    }
  }
})

function formatTimelineDateTime(timestampMs: number): string {
  const date = new Date(timestampMs)
  if (Number.isNaN(date.getTime())) return ''

  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  return `${year}-${month}-${day} ${hours}:${minutes}`
}
</script>

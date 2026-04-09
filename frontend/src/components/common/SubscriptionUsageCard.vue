<template>
  <div class="card overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between border-b border-gray-100 p-4 dark:border-dark-700">
      <div class="flex min-w-0 items-center gap-3">
        <div
          class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-purple-100 dark:bg-purple-900/30"
        >
          <svg
            class="h-5 w-5 text-purple-600 dark:text-purple-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z"
            />
          </svg>
        </div>

        <div class="min-w-0">
          <div class="flex items-center gap-2">
            <p class="text-xs font-semibold text-gray-500 dark:text-dark-400">
              {{ title }}
            </p>
            <span
              v-if="badgeText"
              class="inline-flex items-center rounded-full bg-primary-50 px-2 py-0.5 text-[10px] font-semibold text-primary-700 dark:bg-primary-500/15 dark:text-primary-200"
            >
              {{ badgeText }}
            </span>
          </div>

          <div class="mt-0.5 flex items-center gap-2">
            <h3 class="truncate font-semibold text-gray-900 dark:text-white">
              {{ groupName }}
            </h3>
            <span
              v-if="effectiveQuotaMultiplier > 1"
              :class="[
                'inline-flex items-center rounded-md px-1.5 py-0.5 text-xs font-semibold',
                quotaMultiplierChanged
                  ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
                  : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
              ]"
            >
              ×{{ effectiveQuotaMultiplier }}
            </span>
          </div>

          <p
            v-if="groupDescription"
            class="mt-0.5 truncate text-xs text-gray-500 dark:text-dark-400"
          >
            {{ groupDescription }}
          </p>
        </div>
      </div>

      <span
        v-if="subscription"
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

    <!-- Body -->
    <div class="space-y-4 p-4">
      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <!-- Placeholder -->
      <div
        v-else-if="placeholderText"
        class="rounded-xl bg-gray-50 p-4 text-sm text-gray-600 dark:bg-dark-800 dark:text-dark-300"
      >
        {{ placeholderText }}
      </div>

      <!-- Empty -->
      <div
        v-else-if="!subscription"
        class="rounded-xl bg-gray-50 p-4 text-sm text-gray-600 dark:bg-dark-800 dark:text-dark-300"
      >
        {{ emptyHint || t('common.noData') }}
      </div>

      <!-- Content -->
      <template v-else>
        <!-- Expiration Info -->
        <div v-if="effectiveExpiresAt" class="flex items-center justify-between text-sm">
          <span class="text-gray-500 dark:text-dark-400">{{ expiresLabelComputed }}</span>
          <span :class="[getExpirationClass(effectiveExpiresAt), expiresChanged ? 'font-semibold' : '']">
            {{ formatExpirationDate(effectiveExpiresAt) }}
          </span>
        </div>
        <div v-else class="flex items-center justify-between text-sm">
          <span class="text-gray-500 dark:text-dark-400">{{ expiresLabelComputed }}</span>
          <span class="text-gray-700 dark:text-gray-300">{{ t('userSubscriptions.noExpiration') }}</span>
        </div>

        <!-- Extra slot (e.g. stack until) -->
        <div v-if="$slots.extra">
          <slot name="extra"></slot>
        </div>

        <!-- Daily Usage -->
        <div v-if="props.showUsageSections && subscription.group?.daily_limit_usd" class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('userSubscriptions.daily') }}
            </span>
            <span class="text-sm text-gray-500 dark:text-dark-400">
              ${{ (subscription.daily_usage_usd || 0).toFixed(2) }} / ${{
                getEffectiveLimit(subscription.group.daily_limit_usd).toFixed(2)
              }}
            </span>
          </div>
          <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
              :class="getProgressBarClass(subscription.daily_usage_usd, getEffectiveLimit(subscription.group.daily_limit_usd))"
              :style="{ width: getProgressWidth(subscription.daily_usage_usd, getEffectiveLimit(subscription.group.daily_limit_usd)) }"
            ></div>
          </div>
          <p v-if="subscription.daily_window_start" class="text-xs text-gray-500 dark:text-dark-400">
            {{
              t('userSubscriptions.resetIn', {
                time: formatResetTime(subscription.daily_window_start, 24)
              })
            }}
          </p>
        </div>

        <!-- Weekly Usage -->
        <div v-if="props.showUsageSections && subscription.group?.weekly_limit_usd" class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('userSubscriptions.weekly') }}
            </span>
            <span class="text-sm text-gray-500 dark:text-dark-400">
              ${{ (subscription.weekly_usage_usd || 0).toFixed(2) }} / ${{
                getEffectiveLimit(subscription.group.weekly_limit_usd).toFixed(2)
              }}
            </span>
          </div>
          <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
              :class="getProgressBarClass(subscription.weekly_usage_usd, getEffectiveLimit(subscription.group.weekly_limit_usd))"
              :style="{ width: getProgressWidth(subscription.weekly_usage_usd, getEffectiveLimit(subscription.group.weekly_limit_usd)) }"
            ></div>
          </div>
          <p v-if="subscription.weekly_window_start" class="text-xs text-gray-500 dark:text-dark-400">
            {{
              t('userSubscriptions.resetIn', {
                time: formatResetTime(subscription.weekly_window_start, 168)
              })
            }}
          </p>
        </div>

        <!-- Monthly Usage -->
        <div v-if="props.showUsageSections && subscription.group?.monthly_limit_usd" class="space-y-2">
          <div class="flex items-center justify-between">
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('userSubscriptions.monthly') }}
            </span>
            <span class="text-sm text-gray-500 dark:text-dark-400">
              ${{ (subscription.monthly_usage_usd || 0).toFixed(2) }} / ${{
                getEffectiveLimit(subscription.group.monthly_limit_usd).toFixed(2)
              }}
            </span>
          </div>
          <div class="relative h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
            <div
              class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
              :class="getProgressBarClass(subscription.monthly_usage_usd, getEffectiveLimit(subscription.group.monthly_limit_usd))"
              :style="{ width: getProgressWidth(subscription.monthly_usage_usd, getEffectiveLimit(subscription.group.monthly_limit_usd)) }"
            ></div>
          </div>
          <p v-if="subscription.monthly_window_start" class="text-xs text-gray-500 dark:text-dark-400">
            {{
              t('userSubscriptions.resetIn', {
                time: formatResetTime(subscription.monthly_window_start, 720)
              })
            }}
          </p>
        </div>

        <!-- Unlimited badge -->
        <div
          v-if="
            props.showUsageSections &&
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
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserSubscription } from '@/types'
import { formatDateTime } from '@/utils/format'

interface Props {
  title: string
  subscription: UserSubscription | null
  loading?: boolean
  badgeText?: string
  emptyHint?: string
  placeholderText?: string
  quotaMultiplierOverride?: number | null
  expiresAtOverride?: string | null
  expiresLabel?: string
  showUsageSections?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  badgeText: '',
  emptyHint: '',
  placeholderText: '',
  quotaMultiplierOverride: null,
  expiresAtOverride: null,
  expiresLabel: '',
  showUsageSections: true
})

const { t } = useI18n()

const groupName = computed(() => {
  if (props.subscription?.group?.name) return props.subscription.group.name
  if (props.subscription) return `Group #${props.subscription.group_id}`
  return '-'
})

const groupDescription = computed(() => {
  return props.subscription?.group?.description || ''
})

function getQuotaMultiplier(sub: UserSubscription): number {
  return Math.max(1, sub.quota_multiplier ?? 1)
}

const baseQuotaMultiplier = computed(() => {
  if (!props.subscription) return 1
  return getQuotaMultiplier(props.subscription)
})

const effectiveQuotaMultiplier = computed(() => {
  if (!props.subscription) return 1
  const override = props.quotaMultiplierOverride
  if (override && Number.isFinite(override) && override > 0) {
    return Math.max(1, Math.round(override))
  }
  return baseQuotaMultiplier.value
})

const quotaMultiplierChanged = computed(() => {
  return !!props.subscription && effectiveQuotaMultiplier.value !== baseQuotaMultiplier.value
})

const effectiveExpiresAt = computed(() => {
  if (!props.subscription) return props.expiresAtOverride || null
  return props.expiresAtOverride || props.subscription.expires_at || null
})

const expiresChanged = computed(() => {
  return (
    !!props.subscription &&
    !!props.expiresAtOverride &&
    props.expiresAtOverride !== props.subscription.expires_at
  )
})

const expiresLabelComputed = computed(() => {
  return props.expiresLabel || t('userSubscriptions.expires')
})

function getEffectiveLimit(limit: number | null | undefined): number {
  return (limit ?? 0) * effectiveQuotaMultiplier.value
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
</script>


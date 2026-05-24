<template>
  <section
    class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-900"
    data-testid="sales-commission-monthly-progress"
  >
    <div class="flex flex-col gap-5 lg:flex-row lg:items-stretch">
      <div class="flex-1 min-w-0 space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="text-base font-semibold text-gray-900 dark:text-gray-100">
            {{ t('salesCommissions.monthlyProgress.title') }}
          </h2>
          <span :class="['badge', progress.snapshot_frozen ? 'badge-primary' : 'badge-gray']">
            {{
              progress.snapshot_frozen
                ? t('salesCommissions.monthlyProgress.frozenBadge')
                : t('salesCommissions.monthlyProgress.previewBadge')
            }}
          </span>
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('salesCommissions.monthlyProgress.currentMonth') }}: {{ formatMonth(progress.commission_month) }}
          </span>
        </div>

        <div class="flex items-baseline gap-3">
          <div class="text-3xl font-semibold text-gray-900 dark:text-gray-100">
            {{ formatCNY(progress.monthly_sales_cny) }}
          </div>
          <div class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('salesCommissions.monthlyProgress.monthlySales') }}
          </div>
        </div>

        <div class="text-sm text-gray-600 dark:text-dark-300">
          {{ t('salesCommissions.monthlyProgress.monthlyCommission') }}:
          <span class="font-semibold text-gray-900 dark:text-gray-100">
            {{ formatCNY(progress.monthly_commission_cny) }}
          </span>
        </div>

        <div>
          <div class="mb-1 flex items-center justify-between text-xs text-gray-500 dark:text-dark-400">
            <span>{{ progressBarLabels.left }}</span>
            <span>{{ progressBarLabels.right }}</span>
          </div>
          <div class="h-2 w-full overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800">
            <div
              :class="['h-full rounded-full transition-all duration-300', progress.threshold_met ? 'bg-primary-500' : 'bg-amber-500']"
              :style="{ width: progressBarPercent + '%' }"
            />
          </div>
          <div
            class="mt-2 text-sm"
            :class="progress.threshold_met ? 'text-gray-700 dark:text-dark-200' : 'text-amber-600 dark:text-amber-400'"
          >
            {{ progressHintText }}
          </div>
        </div>

        <p v-if="!progress.snapshot_frozen" class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('salesCommissions.monthlyProgress.ruleEditedHint') }}
        </p>
      </div>

      <div class="lg:w-72 lg:border-l lg:border-gray-200 lg:pl-5 dark:lg:border-dark-700">
        <div class="text-xs font-medium text-gray-500 dark:text-dark-400">
          {{ progress.commission_mode === 'tiered'
            ? t('salesCommissions.monthlyProgress.currentTier')
            : t('salesCommissions.monthlyProgress.fixedRateMode') }}
        </div>
        <div class="mt-2 text-2xl font-semibold text-gray-900 dark:text-gray-100">
          {{ currentRateLabel }}
        </div>
        <div class="mt-1 text-sm text-gray-600 dark:text-dark-300">
          {{ currentTierRangeLabel }}
        </div>
        <div v-if="progress.commission_mode === 'tiered' && progress.next_tier_index >= 0" class="mt-3 rounded border border-dashed border-gray-200 p-2 text-xs text-gray-600 dark:border-dark-700 dark:text-dark-300">
          <div>{{ t('salesCommissions.monthlyProgress.nextTier') }}: {{ formatPercentValue(progress.next_tier_rate) }}%</div>
          <div class="mt-1">{{ t('salesCommissions.monthlyProgress.toNextTier', { amount: formatNumber(progress.to_next_tier_cny), rate: formatPercentValue(progress.next_tier_rate) }) }}</div>
        </div>
      </div>
    </div>

    <div v-if="tierItems.length > 0" class="mt-5 border-t border-gray-100 pt-4 dark:border-dark-800">
      <div class="text-xs font-medium text-gray-500 dark:text-dark-400">
        {{ t('salesCommissions.monthlyProgress.tiersTitle') }}
      </div>
      <ol class="mt-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        <li
          v-for="item in tierItems"
          :key="item.index"
          :class="[
            'rounded border px-3 py-2 text-sm',
            item.status === 'current'
              ? 'border-primary-300 bg-primary-50 text-primary-900 dark:border-primary-600 dark:bg-primary-900/30 dark:text-primary-100'
              : item.status === 'achieved'
                ? 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200'
                : 'border-dashed border-gray-200 text-gray-500 dark:border-dark-700 dark:text-dark-400'
          ]"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="font-medium">{{ item.rangeText }}</span>
            <span class="text-xs">{{ item.statusLabel }}</span>
          </div>
          <div class="mt-1 text-xs">{{ item.rateText }}</div>
        </li>
      </ol>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SalesCommissionMonthlyProgress, SalesCommissionTier } from '@/types'

const props = defineProps<{ progress: SalesCommissionMonthlyProgress }>()
const { t } = useI18n()

function clampPercent(v: number) {
  if (!Number.isFinite(v) || v < 0) return 0
  if (v > 100) return 100
  return v
}

function formatNumber(value: number | undefined) {
  return Number(value || 0).toFixed(2)
}

function formatPercentValue(value: number | undefined) {
  return Number(value || 0).toFixed(2)
}

function formatCNY(value: number | undefined) {
  return `¥${formatNumber(value)}`
}

function formatMonth(value: string | undefined) {
  if (!value) return '-'
  return new Date(value).toLocaleDateString(undefined, { year: 'numeric', month: 'short' })
}

function formatTierRange(tier: SalesCommissionTier) {
  const from = formatNumber(tier.month_sales_from_cny)
  if (tier.month_sales_to_cny == null) {
    return t('salesCommissions.monthlyProgress.tierRangeOpen', { from })
  }
  return t('salesCommissions.monthlyProgress.tierRange', { from, to: formatNumber(tier.month_sales_to_cny) })
}

const progressBarPercent = computed(() => {
  const p = props.progress
  if (!p.threshold_met) {
    if (p.min_monthly_sales_cny <= 0) return 100
    return clampPercent((p.monthly_sales_cny / p.min_monthly_sales_cny) * 100)
  }
  if (p.commission_mode !== 'tiered') return 100
  if (p.next_tier_index < 0 || p.current_tier_index < 0) return 100
  const current = p.tiers[p.current_tier_index]
  const next = p.tiers[p.next_tier_index]
  if (!current || !next) return 100
  const span = next.month_sales_from_cny - current.month_sales_from_cny
  if (span <= 0) return 100
  const filled = p.monthly_sales_cny - current.month_sales_from_cny
  return clampPercent((filled / span) * 100)
})

const progressBarLabels = computed(() => {
  const p = props.progress
  if (!p.threshold_met) {
    return {
      left: t('salesCommissions.monthlyProgress.thresholdNotMet'),
      right: `${t('salesCommissions.monthlyProgress.threshold')} ${formatCNY(p.min_monthly_sales_cny)}`
    }
  }
  if (p.commission_mode !== 'tiered' || p.next_tier_index < 0) {
    return {
      left: t('salesCommissions.monthlyProgress.thresholdMet'),
      right: t('salesCommissions.monthlyProgress.atTopTier')
    }
  }
  const next = p.tiers[p.next_tier_index]
  return {
    left: t('salesCommissions.monthlyProgress.thresholdMet'),
    right: next ? formatCNY(next.month_sales_from_cny) : ''
  }
})

const progressHintText = computed(() => {
  const p = props.progress
  if (p.commission_mode !== 'tiered') {
    if (p.min_monthly_sales_cny > 0 && !p.threshold_met) {
      return t('salesCommissions.monthlyProgress.toThreshold', { amount: formatNumber(p.to_threshold_cny) })
    }
    return t('salesCommissions.monthlyProgress.fixedNoThreshold', { rate: formatPercentValue(p.fixed_commission_rate) })
  }
  if (!p.threshold_met) {
    return t('salesCommissions.monthlyProgress.toThreshold', { amount: formatNumber(p.to_threshold_cny) })
  }
  if (p.next_tier_index < 0) {
    return t('salesCommissions.monthlyProgress.atTopTier')
  }
  return t('salesCommissions.monthlyProgress.toNextTier', {
    amount: formatNumber(p.to_next_tier_cny),
    rate: formatPercentValue(p.next_tier_rate)
  })
})

const currentRateLabel = computed(() => {
  const p = props.progress
  if (p.commission_mode !== 'tiered') {
    return `${formatPercentValue(p.fixed_commission_rate)}%`
  }
  if (p.current_tier_index < 0) return '-'
  const current = p.tiers[p.current_tier_index]
  return current ? `${formatPercentValue(current.commission_rate)}%` : '-'
})

const currentTierRangeLabel = computed(() => {
  const p = props.progress
  if (p.commission_mode !== 'tiered') {
    return t('salesCommissions.monthlyProgress.fixedRateText', { rate: formatPercentValue(p.fixed_commission_rate) })
  }
  if (p.current_tier_index < 0) {
    return t('salesCommissions.monthlyProgress.thresholdNotMet')
  }
  const current = p.tiers[p.current_tier_index]
  if (!current) return ''
  return formatTierRange(current)
})

interface TierItem {
  index: number
  rangeText: string
  rateText: string
  status: 'current' | 'achieved' | 'upcoming'
  statusLabel: string
}

const tierItems = computed<TierItem[]>(() => {
  const p = props.progress
  if (p.commission_mode !== 'tiered') return []
  return p.tiers.map((tier, index) => {
    let status: TierItem['status'] = 'upcoming'
    if (index === p.current_tier_index) status = 'current'
    else if (p.current_tier_index >= 0 && index < p.current_tier_index) status = 'achieved'
    return {
      index,
      rangeText: formatTierRange(tier),
      rateText: `${formatPercentValue(tier.commission_rate)}%`,
      status,
      statusLabel: status === 'current'
        ? t('salesCommissions.monthlyProgress.tierCurrent')
        : status === 'achieved'
          ? t('salesCommissions.monthlyProgress.tierAchieved')
          : t('salesCommissions.monthlyProgress.tierUpcoming')
    }
  })
})
</script>

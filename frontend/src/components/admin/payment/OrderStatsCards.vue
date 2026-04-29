<template>
  <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
    <!-- Range Revenue -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
          <Icon name="dollar" size="md" class="text-green-600 dark:text-green-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.rangeRevenue') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">${{ formatMoney(stats.total_amount) }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ stats.total_count }} {{ t('payment.admin.orders') }}
          </p>
        </div>
      </div>
    </div>

    <!-- Range Orders -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
          <Icon name="creditCard" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.rangeOrders') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.total_count }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('payment.admin.paidOrders') }}
          </p>
        </div>
      </div>
    </div>

    <!-- Average Amount -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
          <Icon name="chart" size="md" class="text-purple-600 dark:text-purple-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.avgAmount') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">${{ formatMoney(stats.avg_amount) }}</p>
        </div>
      </div>
    </div>

    <!-- Daily Average Revenue -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
          <Icon name="chart" size="md" class="text-amber-600 dark:text-amber-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.dailyAvgRevenue') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">${{ formatMoney(dailyAvgRevenue) }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ rangeDayCount }} {{ t('payment.admin.days') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { DashboardStats } from '@/types/payment'

const { t } = useI18n()

const props = defineProps<{
  stats: DashboardStats
}>()

const rangeDayCount = computed(() => props.stats.daily_series?.length ?? 0)

const dailyAvgRevenue = computed(() => {
  const days = rangeDayCount.value
  if (days === 0) return 0
  return props.stats.total_amount / days
})

function formatMoney(value: number): string {
  return value.toFixed(2)
}
</script>

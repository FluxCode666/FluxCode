<template>
  <AppLayout>
    <div class="space-y-6 p-6">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-end">
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="loading"
          @click="loadSummary"
        >
          <svg
            :class="['mr-2 h-5 w-5', loading ? 'animate-spin' : '']"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="1.5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99"
            />
          </svg>
          {{ t('common.refresh') }}
        </button>
        <RouterLink to="/admin/pool-monitor/config" class="btn btn-primary">
          {{ t('admin.poolMonitorDashboard.openConfig') }}
        </RouterLink>
      </div>

      <div class="card overflow-hidden">
        <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.poolMonitorDashboard.platformSummaryTitle') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.poolMonitorDashboard.platformSummaryDescription') }}
          </p>
        </div>

        <div class="space-y-5 p-6">
          <div class="flex flex-wrap gap-2">
            <button
              v-for="tab in platformTabs"
              :key="tab.value"
              type="button"
              class="rounded-full px-3 py-1.5 text-sm font-medium transition-colors"
              :class="selectedPlatform === tab.value
                ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300'
                : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-300 dark:hover:bg-dark-600'"
              @click="selectedPlatform = tab.value"
            >
              {{ tab.label }}
            </button>
          </div>

          <div v-if="loadError" class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300">
            {{ loadError }}
          </div>

          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            <StatCard
              v-for="metric in metrics"
              :key="metric.key"
              :title="metric.label"
              :value="metric.value"
              :icon="metric.icon"
              :icon-variant="metric.variant"
            />
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Squares2X2Icon,
  CheckCircleIcon,
  PauseCircleIcon,
  ClockIcon,
  NoSymbolIcon,
  ExclamationTriangleIcon,
  BoltSlashIcon,
  ShieldExclamationIcon,
  SignalSlashIcon,
  FireIcon
} from '@heroicons/vue/24/outline'
import AppLayout from '@/components/layout/AppLayout.vue'
import StatCard from '@/components/common/StatCard.vue'
import { adminAPI } from '@/api/admin'
import type { AccountSummaryCounts, AccountSummaryResponse } from '@/types'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)
const loadError = ref('')
const summary = ref<AccountSummaryResponse | null>(null)
const selectedPlatform = ref('__all__')

const emptyCounts: AccountSummaryCounts = {
  all: 0,
  active: 0,
  inactive: 0,
  expired: 0,
  error: 0,
  banned: 0,
  available: 0,
  manual_unschedulable: 0,
  temp_unschedulable: 0,
  rate_limited: 0,
  overloaded: 0
}

const knownPlatforms = ['anthropic', 'kiro', 'openai', 'gemini', 'antigravity'] as const

const platformLabel = (platform: string) => {
  if (platform === '__all__') return t('admin.poolMonitorDashboard.allPlatforms')
  const key = `admin.accounts.platforms.${platform}`
  const translated = t(key)
  return translated === key ? platform : translated
}

const platformTabs = computed(() => {
  const items = summary.value?.platforms ?? []
  const byPlatform = new Map(items.map((item) => [item.platform, item]))
  const tabs = [{ value: '__all__', label: platformLabel('__all__') }]
  for (const platform of knownPlatforms) {
    tabs.push({ value: platform, label: platformLabel(platform) })
    byPlatform.delete(platform)
  }
  const extraPlatforms = Array.from(byPlatform.keys()).sort((a, b) => a.localeCompare(b))
  for (const platform of extraPlatforms) {
    tabs.push({ value: platform, label: platformLabel(platform) })
  }
  return tabs
})

const currentCounts = computed<AccountSummaryCounts>(() => {
  if (!summary.value) return emptyCounts
  if (selectedPlatform.value === '__all__') return summary.value.overall
  return summary.value.platforms.find((item) => item.platform === selectedPlatform.value)?.counts ?? emptyCounts
})

const metrics = computed(() => [
  { key: 'all', label: t('common.all'), value: currentCounts.value.all, icon: Squares2X2Icon, variant: 'primary' as const },
  { key: 'active', label: t('common.active'), value: currentCounts.value.active, icon: CheckCircleIcon, variant: 'success' as const },
  { key: 'inactive', label: t('common.inactive'), value: currentCounts.value.inactive, icon: PauseCircleIcon, variant: 'warning' as const },
  { key: 'expired', label: t('admin.accounts.expiration.expired'), value: currentCounts.value.expired, icon: ClockIcon, variant: 'warning' as const },
  { key: 'banned', label: t('common.banned'), value: currentCounts.value.banned, icon: NoSymbolIcon, variant: 'danger' as const },
  { key: 'error', label: t('common.error'), value: currentCounts.value.error, icon: ExclamationTriangleIcon, variant: 'danger' as const },
  { key: 'available', label: t('admin.proxies.countStates.available'), value: currentCounts.value.available, icon: CheckCircleIcon, variant: 'success' as const },
  { key: 'manual_unschedulable', label: t('admin.proxies.countStates.manualUnschedulable'), value: currentCounts.value.manual_unschedulable, icon: BoltSlashIcon, variant: 'warning' as const },
  { key: 'temp_unschedulable', label: t('admin.proxies.countStates.tempUnschedulable'), value: currentCounts.value.temp_unschedulable, icon: ShieldExclamationIcon, variant: 'warning' as const },
  { key: 'rate_limited', label: t('admin.proxies.countStates.rateLimited'), value: currentCounts.value.rate_limited, icon: SignalSlashIcon, variant: 'warning' as const },
  { key: 'overloaded', label: t('admin.proxies.countStates.overloaded'), value: currentCounts.value.overloaded, icon: FireIcon, variant: 'danger' as const }
])

const loadSummary = async () => {
  loading.value = true
  loadError.value = ''
  try {
    summary.value = await adminAPI.accounts.getSummary()
    if (!platformTabs.value.some((tab) => tab.value === selectedPlatform.value)) {
      selectedPlatform.value = '__all__'
    }
  } catch (error: any) {
    const message = t('admin.poolMonitorDashboard.failedToLoadSummary')
    loadError.value = `${message}: ${error?.message || t('common.unknownError')}`
    appStore.showError(loadError.value)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadSummary()
})
</script>

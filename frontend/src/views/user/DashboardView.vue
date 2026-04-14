<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'; import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { UsageLog, TrendDataPoint, ModelStat } from '@/types'
import { fillTrendDataGaps } from '@/utils/trendFill'

const authStore = useAuthStore(); const user = computed(() => authStore.user)
const stats = ref<UserStatsType | null>(null); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])

type TimeRangeTab = '24h' | '7d' | '14d' | '30d'
const formatLD = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
const timeRange = ref<TimeRangeTab>('24h')
const startDate = ref(''); const endDate = ref(''); const granularity = ref<'day' | 'hour'>('hour'); const startHour = ref(0)

const applyTimeRange = (value: TimeRangeTab) => {
  const now = new Date()
  if (value === '24h') {
    const start = new Date(now); start.setHours(start.getHours() - 24)
    startDate.value = formatLD(start); endDate.value = formatLD(now); granularity.value = 'hour'; startHour.value = start.getHours()
  } else {
    const days = value === '7d' ? 7 : value === '14d' ? 14 : 30
    const start = new Date(now); start.setDate(start.getDate() - (days - 1))
    startDate.value = formatLD(start); endDate.value = formatLD(now); granularity.value = 'day'; startHour.value = 0
  }
}
applyTimeRange(timeRange.value)
watch(timeRange, (val) => { applyTimeRange(val); loadCharts() })

const loadStats = async () => { loading.value = true; try { await authStore.refreshUser(); stats.value = await usageAPI.getDashboardStats() } catch (error) { console.error('Failed to load dashboard stats:', error) } finally { loading.value = false } }
const loadCharts = async () => { loadingCharts.value = true; try { const res = await Promise.all([usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }), usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })]); let filled = fillTrendDataGaps(res[0].trend || [], startDate.value, endDate.value, granularity.value as 'day' | 'hour', timeRange.value === '24h' ? { startHour: startHour.value } : undefined); if (timeRange.value === '24h') { filled = filled.map(d => ({ ...d, date: d.date.split(' ')[1] || d.date })) }; trendData.value = filled; modelStats.value = res[1].models || [] } catch (error) { console.error('Failed to load charts:', error) } finally { loadingCharts.value = false } }
const loadRecent = async () => { loadingUsage.value = true; try { const res = await usageAPI.getByDateRange(startDate.value, endDate.value); recentUsage.value = res.items.slice(0, 5) } catch (error) { console.error('Failed to load recent usage:', error) } finally { loadingUsage.value = false } }
const refreshAll = () => { loadStats(); loadCharts(); loadRecent() }

onMounted(() => { refreshAll() })
</script>

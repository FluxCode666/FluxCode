<template>
  <AppLayout>
    <div class="space-y-5" data-test="growth-dashboard">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
            {{ t('admin.growth.timezoneLabel') }}
          </p>
          <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ t('admin.growth.timezone') }}
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-dark-300">
            {{ t('admin.growth.description') }}
          </p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <DateRangePicker
            v-model:start-date="startDate"
            v-model:end-date="endDate"
            :custom-presets="growthDatePresets"
            dropdown-align="right"
            @change="onDateRangeChange"
          />
          <div class="w-28">
            <Select
              v-model="granularity"
              :options="granularityOptions"
              @change="onGranularityChange"
            />
          </div>
          <button
            type="button"
            class="btn btn-secondary"
            :disabled="anyLoading"
            :title="t('admin.growth.refreshAll')"
            @click="loadAll"
          >
            <Icon name="refresh" size="md" :class="anyLoading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>

      <section class="space-y-3" data-test="growth-overview">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.growth.overview') }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-dark-400">
            {{ startDate }} - {{ endDate }}
          </span>
        </div>

        <div v-if="overview.loading && !overview.data" class="flex items-center justify-center py-10">
          <LoadingSpinner />
        </div>
        <div
          v-else-if="overview.error"
          class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300"
          data-test="growth-overview-error"
        >
          {{ overview.error }}
        </div>
        <div v-else class="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <div
            v-for="card in overviewCards"
            :key="card.key"
            class="card p-4"
            :data-test="`growth-kpi-${card.key}`"
          >
            <div class="text-xs font-medium text-gray-500 dark:text-dark-400">{{ card.label }}</div>
            <div class="mt-2 truncate text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {{ card.value }}
            </div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ card.hint }}</div>
          </div>
        </div>
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.growth.userGrowth') }}
        </h2>
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
          <ChartCard
            class="xl:col-span-2"
            :title="t('admin.growth.userTrend')"
            :loading="userTrend.loading"
            :error="userTrend.error"
            error-test="growth-user-trend-error"
          >
            <Line v-if="userTrendChartData" :data="userTrendChartData" :options="lineOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.sources')"
            :loading="userSources.loading"
            :error="userSources.error"
            error-test="growth-user-sources-error"
          >
            <Bar v-if="sourceChartData" :data="sourceChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            class="xl:col-span-3"
            :title="t('admin.growth.sourcePaymentRates')"
            :loading="sourcePaymentRates.loading"
            :error="sourcePaymentRates.error"
            error-test="growth-source-payment-rates-error"
          >
            <div v-if="sourcePaymentRateItems.length" class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">
                    <th class="px-3 py-2">{{ t('admin.growth.source') }}</th>
                    <th class="px-3 py-2 text-right">{{ t('admin.growth.registeredUsers') }}</th>
                    <th class="px-3 py-2 text-right">{{ t('admin.growth.paidUsers') }}</th>
                    <th class="px-3 py-2 text-right">{{ t('admin.growth.conversionRate') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr v-for="item in sourcePaymentRateItems" :key="item.source">
                    <td class="px-3 py-2 font-medium text-gray-900 dark:text-gray-100">{{ item.source }}</td>
                    <td class="px-3 py-2 text-right text-gray-600 dark:text-dark-300">{{ formatInt(item.registered_users) }}</td>
                    <td class="px-3 py-2 text-right text-gray-600 dark:text-dark-300">{{ formatInt(item.paid_users) }}</td>
                    <td class="px-3 py-2 text-right font-medium text-gray-900 dark:text-gray-100">{{ formatPercent(item.conversion_rate) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>
        </div>
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.growth.audienceProfile') }}
        </h2>
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-4">
          <ChartCard
            :title="t('admin.growth.deviceProfile')"
            :loading="audienceDevices.loading"
            :error="audienceDevices.error"
            error-test="growth-audience-devices-error"
          >
            <Bar v-if="audienceDevicesChartData" :data="audienceDevicesChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.osProfile')"
            :loading="audienceOS.loading"
            :error="audienceOS.error"
            error-test="growth-audience-os-error"
          >
            <Bar v-if="audienceOSChartData" :data="audienceOSChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.browserProfile')"
            :loading="audienceBrowsers.loading"
            :error="audienceBrowsers.error"
            error-test="growth-audience-browsers-error"
          >
            <Bar v-if="audienceBrowsersChartData" :data="audienceBrowsersChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.clientProfile')"
            :loading="audienceClients.loading"
            :error="audienceClients.error"
            error-test="growth-audience-clients-error"
          >
            <Bar v-if="audienceClientsChartData" :data="audienceClientsChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>
        </div>
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.growth.retention') }}
        </h2>
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
          <ChartCard
            :title="t('admin.growth.retentionMatrix')"
            :loading="retentionMatrix.loading"
            :error="retentionMatrix.error"
            error-test="growth-retention-matrix-error"
          >
            <div v-if="retentionMatrixRows.length" class="overflow-x-auto">
              <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
                <thead>
                  <tr class="text-left text-xs font-medium uppercase text-gray-500 dark:text-dark-400">
                    <th class="px-3 py-2">{{ t('admin.growth.newRegistered') }}</th>
                    <th class="px-3 py-2 text-right">{{ t('admin.growth.users') }}</th>
                    <th
                      v-for="column in retentionMatrixColumns"
                      :key="column"
                      class="px-3 py-2 text-right"
                    >
                      {{ column }}
                    </th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
                  <tr v-for="cohort in retentionMatrixRows" :key="cohort.date">
                    <td class="px-3 py-2 font-medium text-gray-900 dark:text-gray-100">{{ cohort.date }}</td>
                    <td class="px-3 py-2 text-right text-gray-600 dark:text-dark-300">{{ formatInt(cohort.new_users) }}</td>
                    <td
                      v-for="column in retentionMatrixColumns"
                      :key="`${cohort.date}-${column}`"
                      class="px-3 py-2 text-right text-gray-700 dark:text-dark-200"
                    >
                      {{ formatPercent(cohort.retention[column] ?? 0) }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.retentionTrend')"
            :loading="retentionTrend.loading"
            :error="retentionTrend.error"
            error-test="growth-retention-trend-error"
          >
            <Line v-if="retentionTrendChartData" :data="retentionTrendChartData" :options="percentLineOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>
        </div>
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.growth.paymentConversion') }}
        </h2>
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
          <ChartCard
            :title="t('admin.growth.paymentFunnel')"
            :loading="paymentFunnel.loading"
            :error="paymentFunnel.error"
            error-test="growth-payment-funnel-error"
          >
            <div v-if="paymentFunnelSteps.length" class="space-y-3">
              <div
                v-if="paymentFunnel.data && !paymentFunnel.data.tracking_ready"
                class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
              >
                {{ t('admin.growth.trackingNotReady') }}
              </div>
              <div v-for="step in paymentFunnelSteps" :key="step.key" class="space-y-1">
                <div class="flex items-center justify-between gap-3">
                  <span class="truncate text-sm font-medium text-gray-800 dark:text-gray-100">{{ step.label }}</span>
                  <span class="shrink-0 text-xs text-gray-500 dark:text-dark-400">
                    {{ formatInt(step.users) }} / {{ formatPercent(step.conversion_rate) }}
                  </span>
                </div>
                <div class="h-2 rounded-full bg-gray-100 dark:bg-dark-800">
                  <div
                    class="h-2 rounded-full bg-blue-500"
                    :style="{ width: funnelWidth(step.conversion_rate) }"
                  ></div>
                </div>
              </div>
            </div>
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.paymentPlans')"
            :loading="paymentPlans.loading"
            :error="paymentPlans.error"
            error-test="growth-payment-plans-error"
          >
            <Bar v-if="paymentPlanChartData" :data="paymentPlanChartData" :options="horizontalCurrencyBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.firstPayment')"
            :loading="firstPayment.loading"
            :error="firstPayment.error"
            error-test="growth-first-payment-error"
          >
            <Bar v-if="firstPaymentChartData" :data="firstPaymentChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>
        </div>
      </section>

      <section class="space-y-3">
        <h2 class="text-sm font-semibold text-gray-900 dark:text-gray-100">
          {{ t('admin.growth.featureUsage') }}
        </h2>
        <div class="grid grid-cols-1 gap-4 xl:grid-cols-3">
          <ChartCard
            class="xl:col-span-2"
            :title="t('admin.growth.featureRanking')"
            :loading="featureRanking.loading"
            :error="featureRanking.error"
            error-test="growth-feature-ranking-error"
          >
            <Bar v-if="featureRankingChartData" :data="featureRankingChartData" :options="horizontalBarOptions" />
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>

          <ChartCard
            :title="t('admin.growth.sessionMetrics')"
            :loading="sessionMetrics.loading"
            :error="sessionMetrics.error"
            error-test="growth-session-metrics-error"
          >
            <div v-if="sessionMetrics.data" class="grid gap-3">
              <div
                v-for="metric in sessionMetricCards"
                :key="metric.key"
                class="rounded-lg border border-gray-100 px-3 py-2 dark:border-dark-800"
              >
                <div class="text-xs text-gray-500 dark:text-dark-400">{{ metric.label }}</div>
                <div class="mt-1 text-lg font-semibold text-gray-900 dark:text-gray-100">{{ metric.value }}</div>
              </div>
            </div>
            <EmptyState v-else :label="t('admin.growth.noData')" />
          </ChartCard>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  Tooltip,
  Legend,
  Filler
} from 'chart.js'
import { Bar, Line } from 'vue-chartjs'
import { adminAPI } from '@/api/admin'
import type {
  GrowthFirstPaymentResponse,
  GrowthGranularity,
  GrowthOverview,
  GrowthPaymentFunnel,
  GrowthPaymentPlansResponse,
  GrowthQueryParams,
  GrowthRetentionMatrix,
  GrowthRetentionTrendResponse,
  GrowthSessionMetrics,
  GrowthSourcePaymentRatesResponse,
  GrowthSourcesResponse,
  GrowthUserTrendResponse,
  GrowthFeatureRankingResponse,
  GrowthAudienceResponse
} from '@/api/admin/growth'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import type { DatePreset } from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, BarElement, Tooltip, Legend, Filler)

const { t } = useI18n()
const appStore = useAppStore()

interface SectionState<T> {
  loading: boolean
  data: T | null
  error: string
}

const ChartCard = defineComponent({
  props: {
    title: { type: String, required: true },
    loading: { type: Boolean, default: false },
    error: { type: String, default: '' },
    errorTest: { type: String, default: '' }
  },
  setup(props, { slots }) {
    return () =>
      h('div', { class: 'card p-4' }, [
        h('div', { class: 'mb-3 flex items-center justify-between gap-3' }, [
          h('h3', { class: 'truncate text-sm font-semibold text-gray-900 dark:text-gray-100' }, props.title),
          props.loading
            ? h(LoadingSpinner, { size: 'sm' })
            : null
        ]),
        props.error
          ? h(
              'div',
              {
                class:
                  'flex h-56 items-center justify-center rounded-lg border border-red-200 bg-red-50 px-3 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-900/20 dark:text-red-300',
                'data-test': props.errorTest || undefined
              },
              props.error
            )
          : props.loading
            ? h('div', { class: 'flex h-56 items-center justify-center' }, [h(LoadingSpinner, { size: 'md' })])
          : h('div', { class: 'h-56' }, slots.default?.())
      ])
  }
})

const EmptyState = defineComponent({
  props: {
    label: { type: String, required: true }
  },
  setup(props) {
    return () =>
      h(
        'div',
        { class: 'flex h-full items-center justify-center text-sm text-gray-500 dark:text-dark-400' },
        props.label
      )
  }
})

const defaultEndDate = formatDateInShanghai(new Date())
const startDate = ref(addDays(defaultEndDate, -29))
const endDate = ref(defaultEndDate)
const granularity = ref<GrowthGranularity>('day')

const growthDatePresets: DatePreset[] = [
  {
    labelKey: 'dates.last7Days',
    value: 'last7Days',
    getRange: () => {
      const end = formatDateInShanghai(new Date())
      return { start: addDays(end, -6), end }
    }
  },
  {
    labelKey: 'dates.last30Days',
    value: 'last30Days',
    getRange: () => {
      const end = formatDateInShanghai(new Date())
      return { start: addDays(end, -29), end }
    }
  },
  {
    labelKey: 'dates.thisMonth',
    value: 'thisMonth',
    getRange: () => {
      const end = formatDateInShanghai(new Date())
      return { start: `${end.slice(0, 8)}01`, end }
    }
  },
  {
    labelKey: 'dates.thisYear',
    value: 'thisYear',
    getRange: () => {
      const end = formatDateInShanghai(new Date())
      return { start: `${end.slice(0, 4)}-01-01`, end }
    }
  }
]

const overview = reactive<SectionState<GrowthOverview>>({ loading: false, data: null, error: '' })
const userTrend = reactive<SectionState<GrowthUserTrendResponse>>({ loading: false, data: null, error: '' })
const userSources = reactive<SectionState<GrowthSourcesResponse>>({ loading: false, data: null, error: '' })
const sourcePaymentRates = reactive<SectionState<GrowthSourcePaymentRatesResponse>>({ loading: false, data: null, error: '' })
const retentionMatrix = reactive<SectionState<GrowthRetentionMatrix>>({ loading: false, data: null, error: '' })
const retentionTrend = reactive<SectionState<GrowthRetentionTrendResponse>>({ loading: false, data: null, error: '' })
const paymentFunnel = reactive<SectionState<GrowthPaymentFunnel>>({ loading: false, data: null, error: '' })
const paymentPlans = reactive<SectionState<GrowthPaymentPlansResponse>>({ loading: false, data: null, error: '' })
const firstPayment = reactive<SectionState<GrowthFirstPaymentResponse>>({ loading: false, data: null, error: '' })
const featureRanking = reactive<SectionState<GrowthFeatureRankingResponse>>({ loading: false, data: null, error: '' })
const sessionMetrics = reactive<SectionState<GrowthSessionMetrics>>({ loading: false, data: null, error: '' })
const audienceDevices = reactive<SectionState<GrowthAudienceResponse>>({ loading: false, data: null, error: '' })
const audienceOS = reactive<SectionState<GrowthAudienceResponse>>({ loading: false, data: null, error: '' })
const audienceBrowsers = reactive<SectionState<GrowthAudienceResponse>>({ loading: false, data: null, error: '' })
const audienceClients = reactive<SectionState<GrowthAudienceResponse>>({ loading: false, data: null, error: '' })

const allStates = [
  overview,
  userTrend,
  userSources,
  sourcePaymentRates,
  retentionMatrix,
  retentionTrend,
  paymentFunnel,
  paymentPlans,
  firstPayment,
  featureRanking,
  sessionMetrics,
  audienceDevices,
  audienceOS,
  audienceBrowsers,
  audienceClients
]

const anyLoading = computed(() => allStates.some((state) => state.loading))

const queryParams = computed<GrowthQueryParams>(() => ({
  start_date: startDate.value,
  end_date: endDate.value,
  granularity: granularity.value
}))

const granularityOptions = computed(() => [
  { value: 'day', label: t('admin.growth.day') },
  { value: 'week', label: t('admin.growth.week') },
  { value: 'month', label: t('admin.growth.month') }
])

const overviewCards = computed(() => {
  const data = overview.data
  return [
    { key: 'total-users', label: t('admin.growth.totalUsers'), value: formatInt(data?.total_users), hint: t('admin.growth.users') },
    { key: 'dau', label: t('admin.growth.dau'), value: formatInt(data?.dau), hint: t('admin.growth.users') },
    { key: 'mau', label: t('admin.growth.mau'), value: formatInt(data?.mau), hint: t('admin.growth.users') },
    { key: 'today-new-users', label: t('admin.growth.todayNewUsers'), value: formatInt(data?.today_new_users), hint: t('admin.growth.newRegistered') },
    { key: 'today-paid-users', label: t('admin.growth.todayPaidUsers'), value: formatInt(data?.today_paid_users), hint: t('admin.growth.paidUsers') },
    { key: 'month-revenue', label: t('admin.growth.monthRevenue'), value: formatCurrency(data?.month_revenue), hint: t('admin.growth.revenue') },
    { key: 'arpu', label: t('admin.growth.arpu'), value: formatCurrency(data?.arpu), hint: t('admin.growth.users') },
    { key: 'payment-rate', label: t('admin.growth.paymentConversionRate'), value: formatPercent(data?.payment_conversion_rate), hint: t('admin.growth.repurchaseRate') + ': ' + formatPercent(data?.repurchase_rate) }
  ]
})

const userTrendChartData = computed(() => {
  const series = userTrend.data?.series ?? []
  if (!series.length) return null
  return {
    labels: series.map((item) => item.date),
    datasets: [
      makeLineDataset(t('admin.growth.newRegistered'), series.map((item) => item.new_registered), '#2563eb'),
      makeLineDataset(t('admin.growth.newActivated'), series.map((item) => item.new_activated), '#059669'),
      makeLineDataset(t('admin.growth.newPaid'), series.map((item) => item.new_paid), '#d97706')
    ]
  }
})

const sourceChartData = computed(() => {
  const items = userSources.data?.items ?? []
  if (!items.length) return null
  return makeBarData(
    items.map((item) => item.source),
    t('admin.growth.users'),
    items.map((item) => item.users),
    '#2563eb'
  )
})

const sourcePaymentRateItems = computed(() => sourcePaymentRates.data?.items ?? [])
const retentionMatrixColumns = computed(() => retentionMatrix.data?.columns ?? [])
const retentionMatrixRows = computed(() => retentionMatrix.data?.cohorts ?? [])

const retentionTrendChartData = computed(() => {
  const series = retentionTrend.data?.series ?? []
  if (!series.length) return null
  return {
    labels: series.map((item) => item.date),
    datasets: [
      makeLineDataset('D1', series.map((item) => ratioToPercent(item.d1)), '#2563eb'),
      makeLineDataset('D7', series.map((item) => ratioToPercent(item.d7)), '#059669'),
      makeLineDataset('D30', series.map((item) => ratioToPercent(item.d30)), '#d97706')
    ]
  }
})

const paymentFunnelSteps = computed(() => paymentFunnel.data?.steps ?? [])

const paymentPlanChartData = computed(() => {
  const items = paymentPlans.data?.items ?? []
  if (!items.length) return null
  return makeBarData(
    items.map((item) => item.plan_name || item.category || t('common.unknown')),
    t('admin.growth.revenue'),
    items.map((item) => item.revenue),
    '#059669'
  )
})

const firstPaymentChartData = computed(() => {
  const items = firstPayment.data?.items ?? []
  if (!items.length) return null
  return makeBarData(
    items.map((item) => item.label || item.bucket),
    t('admin.growth.users'),
    items.map((item) => item.users),
    '#7c3aed'
  )
})

const featureRankingChartData = computed(() => {
  const items = featureRanking.data?.items ?? []
  if (!items.length) return null
  return makeBarData(
    items.map((item) => item.label || item.feature),
    t('admin.growth.uses'),
    items.map((item) => item.uses),
    '#0f766e'
  )
})

const audienceDevicesChartData = computed(() => makeAudienceChartData(audienceDevices.data?.items ?? [], '#2563eb'))
const audienceOSChartData = computed(() => makeAudienceChartData(audienceOS.data?.items ?? [], '#059669'))
const audienceBrowsersChartData = computed(() => makeAudienceChartData(audienceBrowsers.data?.items ?? [], '#d97706'))
const audienceClientsChartData = computed(() => makeAudienceChartData(audienceClients.data?.items ?? [], '#0f766e'))

const sessionMetricCards = computed(() => {
  const data = sessionMetrics.data
  return [
    {
      key: 'average-turns',
      label: t('admin.growth.averageTurns'),
      value: formatMetric(data?.average_turns, (value) => value.toFixed(1))
    },
    {
      key: 'average-duration',
      label: t('admin.growth.averageSessionDuration'),
      value: formatMetric(data?.average_session_duration_seconds, formatDuration)
    },
    {
      key: 'average-input',
      label: t('admin.growth.averageInputTokens'),
      value: formatMetric(data?.average_input_tokens, formatInt)
    },
    {
      key: 'average-output',
      label: t('admin.growth.averageOutputTokens'),
      value: formatMetric(data?.average_output_tokens, formatInt)
    }
  ]
})

const chartTextColor = '#6b7280'
const chartGridColor = 'rgba(156, 163, 175, 0.22)'

const lineOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index' as const, intersect: false },
  plugins: {
    legend: { position: 'bottom' as const, labels: { boxWidth: 10, color: chartTextColor } }
  },
  scales: {
    x: { grid: { color: chartGridColor }, ticks: { color: chartTextColor } },
    y: { beginAtZero: true, grid: { color: chartGridColor }, ticks: { color: chartTextColor, callback: (value: string | number) => formatCompact(Number(value)) } }
  }
}))

const percentLineOptions = computed(() => ({
  ...lineOptions.value,
  scales: {
    ...lineOptions.value.scales,
    y: {
      beginAtZero: true,
      max: 100,
      grid: { color: chartGridColor },
      ticks: { color: chartTextColor, callback: (value: string | number) => `${Number(value)}%` }
    }
  }
}))

const horizontalBarOptions = computed(() => ({
  indexAxis: 'y' as const,
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: { callbacks: { label: (ctx: any) => `${ctx.dataset.label}: ${formatCompact(Number(ctx.parsed.x || 0))}` } }
  },
  scales: {
    x: { beginAtZero: true, grid: { color: chartGridColor }, ticks: { color: chartTextColor, callback: (value: string | number) => formatCompact(Number(value)) } },
    y: { grid: { display: false }, ticks: { color: chartTextColor } }
  }
}))

const horizontalCurrencyBarOptions = computed(() => ({
  ...horizontalBarOptions.value,
  plugins: {
    legend: { display: false },
    tooltip: { callbacks: { label: (ctx: any) => `${ctx.dataset.label}: ${formatCurrency(Number(ctx.parsed.x || 0))}` } }
  },
  scales: {
    ...horizontalBarOptions.value.scales,
    x: {
      beginAtZero: true,
      grid: { color: chartGridColor },
      ticks: { color: chartTextColor, callback: (value: string | number) => formatCurrency(Number(value)) }
    }
  }
}))

async function loadSection<T>(state: SectionState<T>, loader: () => Promise<T>) {
  state.loading = true
  state.error = ''
  try {
    state.data = await loader()
  } catch {
    state.error = t('admin.growth.failedToLoad')
    appStore.showError(t('admin.growth.failedToLoad'))
  } finally {
    state.loading = false
  }
}

function loadAll() {
  const params = { ...queryParams.value }
  return Promise.allSettled([
    loadSection(overview, () => adminAPI.growth.getOverview(params)),
    loadSection(userTrend, () => adminAPI.growth.getUserTrend(params)),
    loadSection(userSources, () => adminAPI.growth.getUserSources(params)),
    loadSection(sourcePaymentRates, () => adminAPI.growth.getSourcePaymentRates(params)),
    loadSection(retentionMatrix, () => adminAPI.growth.getRetentionMatrix(params)),
    loadSection(retentionTrend, () => adminAPI.growth.getRetentionTrend(params)),
    loadSection(paymentFunnel, () => adminAPI.growth.getPaymentFunnel(params)),
    loadSection(paymentPlans, () => adminAPI.growth.getPaymentPlans(params)),
    loadSection(firstPayment, () => adminAPI.growth.getFirstPayment(params)),
    loadSection(featureRanking, () => adminAPI.growth.getFeatureRanking(params)),
    loadSection(sessionMetrics, () => adminAPI.growth.getSessionMetrics(params)),
    loadSection(audienceDevices, () => adminAPI.growth.getAudienceDevices(params)),
    loadSection(audienceOS, () => adminAPI.growth.getAudienceOS(params)),
    loadSection(audienceBrowsers, () => adminAPI.growth.getAudienceBrowsers(params)),
    loadSection(audienceClients, () => adminAPI.growth.getAudienceClients(params))
  ])
}

function onDateRangeChange(range: { startDate: string; endDate: string }) {
  startDate.value = range.startDate
  endDate.value = range.endDate
  void loadAll()
}

function onGranularityChange(value: string | number | boolean | null) {
  if (value === 'day' || value === 'week' || value === 'month') {
    granularity.value = value
    void loadAll()
  }
}

function makeLineDataset(label: string, data: number[], color: string) {
  return {
    label,
    data,
    borderColor: color,
    backgroundColor: withAlpha(color, 0.12),
    tension: 0.28,
    fill: true,
    pointRadius: 2,
    pointHoverRadius: 4
  }
}

function makeBarData(labels: string[], label: string, data: number[], color: string) {
  return {
    labels,
    datasets: [
      {
        label,
        data,
        backgroundColor: withAlpha(color, 0.74),
        borderColor: color,
        borderWidth: 1,
        borderRadius: 5,
        barThickness: 18
      }
    ]
  }
}

function makeAudienceChartData(items: { label: string; key: string; users: number }[], color: string) {
  if (!items.length) return null
  return makeBarData(
    items.map((item) => item.label || item.key),
    t('admin.growth.users'),
    items.map((item) => item.users),
    color
  )
}

function withAlpha(hex: string, alpha: number): string {
  const normalized = hex.replace('#', '')
  const bigint = Number.parseInt(normalized, 16)
  const r = (bigint >> 16) & 255
  const g = (bigint >> 8) & 255
  const b = bigint & 255
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function formatInt(value: number | undefined | null): string {
  return Math.trunc(Number(value || 0)).toLocaleString()
}

function formatCompact(value: number): string {
  return Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(Number(value || 0))
}

function formatCurrency(value: number | undefined | null): string {
  return `¥${Number(value || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`
}

function formatPercent(value: number | undefined | null): string {
  return `${ratioToPercent(Number(value || 0)).toFixed(1)}%`
}

function ratioToPercent(value: number): number {
  return Number(value || 0) * 100
}

function formatDuration(value: number): string {
  const seconds = Math.round(Number(value || 0))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const remaining = seconds % 60
  return `${minutes}m ${remaining}s`
}

function formatMetric(metric: { available: boolean; value: number } | undefined, formatter: (value: number) => string): string {
  if (!metric?.available) return t('admin.growth.notAvailable')
  return formatter(metric.value)
}

function funnelWidth(rate: number): string {
  return `${Math.min(Math.max(ratioToPercent(rate), 2), 100)}%`
}

function formatDateInShanghai(date: Date): string {
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  })
  const parts = formatter.formatToParts(date).reduce<Record<string, string>>((acc, part) => {
    acc[part.type] = part.value
    return acc
  }, {})
  return `${parts.year}-${parts.month}-${parts.day}`
}

function addDays(dateText: string, delta: number): string {
  const [year, month, day] = dateText.split('-').map(Number)
  const shifted = new Date(Date.UTC(year, month - 1, day + delta))
  return [
    shifted.getUTCFullYear(),
    String(shifted.getUTCMonth() + 1).padStart(2, '0'),
    String(shifted.getUTCDate()).padStart(2, '0')
  ].join('-')
}

onMounted(() => {
  void loadAll()
})
</script>

<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('referral.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('referral.description') }}</p>
        </div>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z">
          </path>
        </svg>
      </div>

      <template v-else>
        <!-- Disabled State -->
        <div v-if="referralInfo && !referralInfo.enabled"
          class="card p-6 border-2 border-dashed border-amber-200 bg-amber-50 dark:bg-amber-900/10 dark:border-amber-700/50">
          <div class="flex items-start gap-3">
            <svg class="h-6 w-6 flex-shrink-0 text-amber-600 dark:text-amber-400" fill="none" viewBox="0 0 24 24"
              stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round"
                d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
            </svg>
            <div>
              <h3 class="font-semibold text-amber-900 dark:text-amber-200">{{ t('referral.disabledTitle') }}</h3>
              <p class="mt-1 text-sm text-amber-800 dark:text-amber-300">{{ t('referral.disabledDescription') }}</p>
            </div>
          </div>
        </div>

        <!-- Active State -->
        <template v-else-if="referralInfo">
          <!-- Referral Code Card -->
          <div class="card">
            <div class="p-6">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.myReferralCode') }}</h2>
              <div v-if="referralInfo.referral_code" class="mt-4 space-y-3">
                <div class="flex items-center gap-3">
                  <code
                    class="flex-1 rounded-lg bg-gray-100 px-4 py-3 text-lg font-mono font-semibold text-primary-600 dark:bg-dark-700 dark:text-primary-400">
                    {{ referralInfo.referral_code }}
                  </code>
                  <button @click="copyCode" class="btn btn-outline px-4 py-3" :title="t('referral.codeCopied')">
                    <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                      <path stroke-linecap="round" stroke-linejoin="round"
                        d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9.75a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                    </svg>
                  </button>
                </div>
                <button @click="copyLink" class="btn btn-primary w-full">
                  <svg class="mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round"
                      d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.556a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.5 8.688" />
                  </svg>
                  {{ t('referral.copyLink') }}
                </button>
              </div>
              <div v-else class="mt-4">
                <p class="mb-3 text-sm text-gray-500 dark:text-dark-400">{{ t('referral.noReferralCode') }}</p>
                <button @click="handleGenerateCode" :disabled="generating" class="btn btn-primary">
                  {{ generating ? t('referral.generating') : t('referral.generateCode') }}
                </button>
              </div>
            </div>
          </div>

          <!-- Overview Stats -->
          <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <div class="card p-4 text-center">
              <div class="text-2xl font-bold text-gray-900 dark:text-white">{{ referralInfo.total_invites }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('referral.totalInvites') }}</div>
            </div>
            <div class="card p-4 text-center">
              <div class="text-2xl font-bold text-green-600 dark:text-green-400">
                {{ referralInfo.completed_invites }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('referral.completedInvites') }}</div>
            </div>
            <div class="card p-4 text-center">
              <div class="text-2xl font-bold text-primary-600 dark:text-primary-400">${{
                referralInfo.total_earned.toFixed(2) }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('referral.totalEarned') }}</div>
            </div>
            <div class="card p-4 text-center">
              <div class="text-2xl font-bold text-amber-600 dark:text-amber-400">${{
                referralInfo.gift_balance_remaining.toFixed(2) }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('referral.giftBalanceRemaining') }}</div>
            </div>
          </div>

          <!-- Trend Chart -->
          <div class="card">
            <div class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-dark-600">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.trendTitle') }}</h2>
              <Select v-model="trendDays" :options="trendDaysOptions" class="w-36" @change="loadTrend" />
            </div>
            <div class="p-4">
              <div v-if="trendLoading" class="flex h-64 items-center justify-center text-sm text-gray-400">
                {{ t('common.loading') }}
              </div>
              <div v-else-if="trendChartData" class="h-64">
                <Line :data="trendChartData" :options="trendChartOptions" />
              </div>
              <div v-else class="flex h-64 items-center justify-center text-sm text-gray-400">
                {{ t('referral.noTrendData') }}
              </div>
            </div>
          </div>

          <!-- How It Works -->
          <div class="card">
            <div class="p-6">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('referral.howItWorks') }}</h2>
              <div class="mt-4 grid gap-4 sm:grid-cols-3">
                <div class="flex items-start gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-sm font-bold text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
                    1</div>
                  <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('referral.step1') }}</p>
                </div>
                <div class="flex items-start gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-sm font-bold text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
                    2</div>
                  <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('referral.step2') }}</p>
                </div>
                <div class="flex items-start gap-3 rounded-lg bg-gray-50 p-4 dark:bg-dark-700/50">
                  <div
                    class="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 text-sm font-bold text-primary-600 dark:bg-primary-900/30 dark:text-primary-400">
                    3</div>
                  <p class="text-sm text-gray-600 dark:text-dark-300">{{ t('referral.step3') }}</p>
                </div>
              </div>
            </div>
          </div>

          <!-- Tabs -->
          <div class="card">
            <div class="border-b border-gray-200 dark:border-dark-600">
              <nav class="flex -mb-px">
                <button @click="activeTab = 'invites'" :class="[
                  'flex-1 py-3 px-4 text-center text-sm font-medium border-b-2 transition-colors',
                  activeTab === 'invites'
                    ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200'
                ]">
                  {{ t('referral.inviteList') }}
                </button>
                <button @click="activeTab = 'giftBalance'" :class="[
                  'flex-1 py-3 px-4 text-center text-sm font-medium border-b-2 transition-colors',
                  activeTab === 'giftBalance'
                    ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                    : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200'
                ]">
                  {{ t('referral.giftBalanceRecords') }}
                </button>
              </nav>
            </div>

            <!-- Invite List -->
            <div v-if="activeTab === 'invites'" class="p-6">
              <div v-if="invites.length === 0" class="py-8 text-center text-gray-500 dark:text-dark-400">
                {{ t('referral.noInvites') }}
              </div>
              <div v-else class="overflow-x-auto">
                <table class="w-full text-sm">
                  <thead>
                    <tr class="border-b border-gray-200 dark:border-dark-600">
                      <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('referral.date') }}
                      </th>
                      <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.status') }}</th>
                      <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.inviteeReward') }}</th>
                      <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.inviterReward') }}</th>
                      <th class="pb-3 text-right font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.ongoingRewards') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr v-for="invite in invites" :key="invite.id">
                      <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ formatDate(invite.created_at) }}</td>
                      <td class="py-3 pr-4">
                        <span :class="[
                          'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                          invite.status === 'completed'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
                        ]">
                          {{ invite.status === 'completed' ? t('referral.completed') : t('referral.pending') }}
                        </span>
                      </td>
                      <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">${{
                        invite.invitee_reward_amount.toFixed(2) }}</td>
                      <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">${{
                        invite.inviter_reward_amount.toFixed(2) }}</td>
                      <td class="py-3 text-right text-gray-700 dark:text-dark-200">
                        {{ invite.ongoing_reward_count > 0
                          ? `${invite.ongoing_reward_count} ($${invite.ongoing_reward_total.toFixed(2)})`
                          : '-' }}
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Gift Balance Records -->
            <div v-if="activeTab === 'giftBalance'" class="p-6">
              <div v-if="giftRecords.length === 0" class="py-8 text-center text-gray-500 dark:text-dark-400">
                {{ t('referral.noGiftRecords') }}
              </div>
              <div v-else class="overflow-x-auto">
                <table class="w-full text-sm">
                  <thead>
                    <tr class="border-b border-gray-200 dark:border-dark-600">
                      <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('referral.date') }}
                      </th>
                      <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.source') }}</th>
                      <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.giftNote') }}</th>
                      <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.amount') }}</th>
                      <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.remaining') }}</th>
                      <th class="pb-3 text-right font-medium text-gray-500 dark:text-dark-400">{{
                        t('referral.expiresAt') }}</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                    <tr v-for="record in giftRecords" :key="record.id">
                      <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ formatDate(record.created_at) }}</td>
                      <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ getSourceLabel(record.source) }}</td>
                      <td class="py-3 pr-4 text-gray-500 dark:text-dark-400 text-xs max-w-[200px] truncate" :title="record.note">{{ record.note || '-' }}</td>
                      <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">${{ record.amount.toFixed(2) }}
                      </td>
                      <td class="py-3 pr-4 text-right font-medium"
                        :class="record.remaining > 0 ? 'text-green-600 dark:text-green-400' : 'text-gray-400 dark:text-dark-500'">
                        ${{ record.remaining.toFixed(2) }}</td>
                      <td class="py-3 text-right text-gray-700 dark:text-dark-200">
                        <template v-if="!record.expires_at">{{ t('referral.neverExpires') }}</template>
                        <template v-else-if="new Date(record.expires_at) < new Date()">
                          <span class="text-red-500">{{ t('referral.expired') }}</span>
                        </template>
                        <template v-else>{{ formatDate(record.expires_at) }}</template>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </template>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { AppLayout } from '@/components/layout'
import Select from '@/components/common/Select.vue'
import { useAppStore } from '@/stores'
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from 'chart.js'
import { Line } from 'vue-chartjs'
import {
  getReferralInfo,
  generateReferralCode,
  getMyReferrals,
  getMyGiftBalanceRecords,
  getMyReferralStats,
  type ReferralInfo,
  type ReferralInvite,
  type GiftBalanceRecord,
  type ReferralTrendPoint,
} from '@/api/referral'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, Filler)

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const trendLoading = ref(false)
const generating = ref(false)
const activeTab = ref<'invites' | 'giftBalance'>('invites')
const trendDays = ref<number | string | boolean | null>(30)

const trendDaysOptions = computed(() => [
  { value: 7, label: t('referral.last7Days') },
  { value: 30, label: t('referral.last30Days') },
  { value: 90, label: t('referral.last90Days') },
])

const referralInfo = ref<ReferralInfo | null>(null)
const invites = ref<ReferralInvite[]>([])
const giftRecords = ref<GiftBalanceRecord[]>([])
const trendData = ref<ReferralTrendPoint[]>([])

const trendChartData = computed(() => {
  if (!trendData.value.length) return null
  return {
    labels: trendData.value.map((p) => p.date),
    datasets: [
      {
        label: t('referral.invitations'),
        data: trendData.value.map((p) => p.invitations),
        borderColor: 'rgb(59, 130, 246)',
        backgroundColor: 'rgba(59, 130, 246, 0.1)',
        tension: 0.3,
        fill: true,
      },
      {
        label: t('referral.completions'),
        data: trendData.value.map((p) => p.completions),
        borderColor: 'rgb(16, 185, 129)',
        backgroundColor: 'rgba(16, 185, 129, 0.1)',
        tension: 0.3,
        fill: true,
      },
    ],
  }
})

const trendChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  interaction: {
    mode: 'index' as const,
    intersect: false,
  },
  plugins: {
    legend: { display: true, position: 'top' as const },
  },
  scales: {
    y: {
      beginAtZero: true,
      ticks: {
        precision: 0,
      },
    },
  },
}

onMounted(async () => {
  await loadData()
})

async function loadData() {
  loading.value = true
  try {
    const [info, invitePage, recordPage] = await Promise.all([
      getReferralInfo(),
      getMyReferrals({ page: 1, page_size: 50 }),
      getMyGiftBalanceRecords({ page: 1, page_size: 50 }),
    ])
    referralInfo.value = info
    invites.value = invitePage.items || []
    giftRecords.value = recordPage.items || []
    if (info.enabled) {
      await loadTrend()
    }
  } catch (error) {
    console.error('Failed to load referral data:', error)
    appStore.showError(t('referral.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadTrend() {
  trendLoading.value = true
  try {
    const result = await getMyReferralStats(Number(trendDays.value))
    trendData.value = result.data || []
  } catch (error) {
    console.error('Failed to load trend:', error)
    trendData.value = []
  } finally {
    trendLoading.value = false
  }
}

async function handleGenerateCode() {
  generating.value = true
  try {
    const result = await generateReferralCode()
    if (referralInfo.value) {
      referralInfo.value.referral_code = result.referral_code
    }
    appStore.showSuccess(t('referral.generateCodeSuccess'))
  } catch (error) {
    console.error('Failed to generate referral code:', error)
    appStore.showError(t('referral.generateCodeFailed'))
  } finally {
    generating.value = false
  }
}

function copyCode() {
  if (referralInfo.value?.referral_code) {
    navigator.clipboard.writeText(referralInfo.value.referral_code)
    appStore.showSuccess(t('referral.codeCopied'))
  }
}

function copyLink() {
  if (referralInfo.value?.referral_code) {
    const url = `${window.location.origin}/register?ref=${referralInfo.value.referral_code}`
    navigator.clipboard.writeText(url)
    appStore.showSuccess(t('referral.linkCopied'))
  }
}

function getSourceLabel(source: string): string {
  switch (source) {
    case 'referral_invitee':
      return t('referral.sourceReferralInvitee')
    case 'referral_inviter':
      return t('referral.sourceReferralInviter')
    case 'referral_ongoing':
      return t('referral.sourceReferralOngoing')
    case 'admin_grant':
      return t('referral.sourceAdminGrant')
    default:
      return source
  }
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
</script>

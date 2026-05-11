<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-6">
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('nav.referralManagement') }}</h1>
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
        <!-- Tabs -->
        <div class="card">
          <div class="border-b border-gray-200 dark:border-dark-600">
            <nav class="flex -mb-px overflow-x-auto">
              <button @click="activeTab = 'dashboard'" :class="tabClass('dashboard')">
                {{ t('adminReferral.tabDashboard') }}
              </button>
              <button @click="activeTab = 'config'" :class="tabClass('config')">
                {{ t('adminReferral.tabConfig') }}
              </button>
              <button @click="activeTab = 'list'" :class="tabClass('list')">
                {{ t('adminReferral.tabList') }}
              </button>
              <button @click="activeTab = 'leaderboard'" :class="tabClass('leaderboard')">
                {{ t('adminReferral.tabLeaderboard') }}
              </button>
              <button @click="activeTab = 'grant'" :class="tabClass('grant')">
                {{ t('adminReferral.tabGrant') }}
              </button>
              <button @click="activeTab = 'userConfig'" :class="tabClass('userConfig')">
                {{ t('adminReferral.tabUserConfig') }}
              </button>
            </nav>
          </div>

          <!-- Dashboard Tab -->
          <div v-if="activeTab === 'dashboard'" class="p-6 space-y-6">
            <!-- Stats Cards -->
            <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <div class="card p-4 text-center">
                <div class="text-2xl font-bold text-gray-900 dark:text-white">{{ dashboard?.summary.total_referrals ?? 0 }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalReferrals') }}</div>
              </div>
              <div class="card p-4 text-center">
                <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ dashboard?.summary.completed_referrals ?? 0 }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.completed') }}</div>
              </div>
              <div class="card p-4 text-center">
                <div class="text-2xl font-bold text-primary-600 dark:text-primary-400">${{ (dashboard?.summary.total_gift_granted ?? 0).toFixed(2) }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalIssued') }}</div>
              </div>
              <div class="card p-4 text-center">
                <div class="text-2xl font-bold text-amber-600 dark:text-amber-400">${{ (dashboard?.summary.total_gift_remaining ?? 0).toFixed(2) }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.remaining') }}</div>
              </div>
            </div>

            <!-- Conversion Funnel -->
            <div class="card p-6">
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('adminReferral.conversionFunnel') }}</h3>
              <div class="mt-4 space-y-3">
                <div class="space-y-1">
                  <div class="flex justify-between text-sm">
                    <span class="text-gray-700 dark:text-dark-200">{{ t('adminReferral.funnelRegistrations') }}</span>
                    <span class="font-semibold">{{ dashboard?.funnel.registrations ?? 0 }}</span>
                  </div>
                  <div class="h-2 rounded-full bg-blue-500" style="width: 100%"></div>
                </div>
                <div class="space-y-1">
                  <div class="flex justify-between text-sm">
                    <span class="text-gray-700 dark:text-dark-200">{{ t('adminReferral.funnelFirstRecharges') }}</span>
                    <span class="font-semibold">{{ dashboard?.funnel.first_recharges ?? 0 }}</span>
                  </div>
                  <div class="h-2 rounded-full bg-green-500" :style="{ width: funnelFirstRechargeWidth }"></div>
                </div>
                <div class="mt-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-700/50">
                  <div class="text-sm text-gray-600 dark:text-dark-300">{{ t('adminReferral.conversionRate') }}</div>
                  <div class="text-2xl font-bold text-primary-600 dark:text-primary-400">
                    {{ (dashboard?.funnel.conversion_rate ?? 0).toFixed(1) }}%
                  </div>
                </div>
              </div>
            </div>

            <!-- Trend Chart -->
            <div class="card">
              <div class="flex items-center justify-between border-b border-gray-200 p-4 dark:border-dark-600">
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('adminReferral.trendTitle') }}</h3>
                <Select v-model="trendDays" :options="trendDaysOptions" class="w-36" @change="loadDashboard" />
              </div>
              <div class="p-4">
                <div v-if="trendChartData" class="h-72">
                  <Line :data="trendChartData" :options="trendChartOptions" />
                </div>
                <div v-else class="flex h-72 items-center justify-center text-sm text-gray-400">
                  {{ t('adminReferral.noTrendData') }}
                </div>
              </div>
            </div>
          </div>

          <!-- Config Tab -->
          <div v-if="activeTab === 'config'" class="p-6">
            <div class="space-y-4 max-w-lg">
              <div class="flex items-center justify-between">
                <label class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('adminReferral.referralEnabled') }}</label>
                <button @click="config.referral_enabled = !config.referral_enabled" :class="[
                  'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                  config.referral_enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'
                ]">
                  <span :class="[
                    'inline-block h-4 w-4 transform rounded-full bg-white transition-transform',
                    config.referral_enabled ? 'translate-x-6' : 'translate-x-1'
                  ]" />
                </button>
              </div>
              <div>
                <label class="input-label">{{ t('adminReferral.inviteeReward') }}</label>
                <input v-model.number="config.referral_invitee_reward" type="number" step="0.01" min="0" class="input mt-1" />
              </div>
              <div>
                <label class="input-label">{{ t('adminReferral.inviterReward') }}</label>
                <input v-model.number="config.referral_inviter_reward" type="number" step="0.01" min="0" class="input mt-1" />
              </div>
              <div>
                <label class="input-label">{{ t('adminReferral.giftBalanceExpiry') }}</label>
                <input v-model.number="config.referral_gift_balance_expiry_days" type="number" min="0" class="input mt-1" />
              </div>
              <div>
                <label class="input-label">{{ t('adminReferral.maxInvites') }}</label>
                <input v-model.number="config.referral_max_invites" type="number" min="0" class="input mt-1" />
              </div>
              <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
                <div class="flex items-center justify-between">
                  <label class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('adminReferral.ongoingRewardEnabled') }}</label>
                  <button @click="config.referral_ongoing_reward_enabled = !config.referral_ongoing_reward_enabled" :class="[
                    'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                    config.referral_ongoing_reward_enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'
                  ]">
                    <span :class="[
                      'inline-block h-4 w-4 transform rounded-full bg-white transition-transform',
                      config.referral_ongoing_reward_enabled ? 'translate-x-6' : 'translate-x-1'
                    ]" />
                  </button>
                </div>
              </div>
              <template v-if="config.referral_ongoing_reward_enabled">
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardType') }}</label>
                  <Select v-model="config.referral_ongoing_reward_type" :options="ongoingRewardTypeOptions" class="mt-1" />
                </div>
                <div>
                  <label class="input-label">
                    {{ t('adminReferral.ongoingRewardValue') }}
                    <span v-if="config.referral_ongoing_reward_type === 'percentage'" class="text-gray-400">(%)</span>
                  </label>
                  <input v-model.number="config.referral_ongoing_reward_value" type="number" step="0.01" min="0" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardMaxCount') }}</label>
                  <input v-model.number="config.referral_ongoing_reward_max_count" type="number" min="0" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardDurationDays') }}</label>
                  <input v-model.number="config.referral_ongoing_reward_duration_days" type="number" min="0" class="input mt-1" />
                </div>
              </template>
              <button @click="saveConfig" :disabled="savingConfig" class="btn btn-primary">
                {{ savingConfig ? t('adminReferral.saving') : t('adminReferral.saveConfig') }}
              </button>
            </div>
          </div>

          <!-- Referral List Tab -->
          <div v-if="activeTab === 'list'" class="p-6">
            <div class="mb-4 flex items-center gap-2">
              <Select v-model="listFilter.status" :options="listStatusOptions" class="w-40" @change="loadList" />
            </div>
            <div v-if="referralList.length === 0" class="py-8 text-center text-gray-500 dark:text-dark-400">
              {{ t('adminReferral.noReferrals') }}
            </div>
            <div v-else class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-gray-200 dark:border-dark-600">
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.date') }}</th>
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.referrer') }}</th>
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.referee') }}</th>
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.status') }}</th>
                    <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.inviteeAmount') }}</th>
                    <th class="pb-3 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.inviterAmount') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="item in referralList" :key="item.id">
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ formatDate(item.created_at) }}</td>
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">#{{ item.referrer_id }}</td>
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ item.referee_email || `#${item.referee_id}` }}</td>
                    <td class="py-3 pr-4">
                      <span :class="[
                        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                        item.status === 'completed'
                          ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                          : 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
                      ]">
                        {{ item.status }}
                      </span>
                    </td>
                    <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">${{ item.invitee_reward_amount.toFixed(2) }}</td>
                    <td class="py-3 text-right text-gray-700 dark:text-dark-200">${{ item.inviter_reward_amount.toFixed(2) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Leaderboard Tab -->
          <div v-if="activeTab === 'leaderboard'" class="p-6">
            <div class="mb-4 flex items-center gap-2">
              <Select v-model="leaderboardPeriod" :options="leaderboardPeriodOptions" class="w-40" @change="loadLeaderboard" />
            </div>
            <div v-if="leaderboard.length === 0" class="py-8 text-center text-gray-500 dark:text-dark-400">
              {{ t('adminReferral.noLeaderboard') }}
            </div>
            <div v-else class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-gray-200 dark:border-dark-600">
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.rank') }}</th>
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.user') }}</th>
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.referralCode') }}</th>
                    <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalInvites') }}</th>
                    <th class="pb-3 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalEarned') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="(item, index) in leaderboard" :key="item.user_id">
                    <td class="py-3 pr-4 font-medium text-gray-700 dark:text-dark-200">{{ index + 1 }}</td>
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ item.email || item.username || `#${item.user_id}` }}</td>
                    <td class="py-3 pr-4 font-mono text-xs text-gray-500 dark:text-dark-400">{{ item.referral_code || '-' }}</td>
                    <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">{{ item.invite_count }}</td>
                    <td class="py-3 text-right font-medium text-primary-600 dark:text-primary-400">${{ item.total_reward.toFixed(2) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Grant Tab -->
          <div v-if="activeTab === 'grant'" class="p-6 space-y-6">
            <!-- Single grant -->
            <div class="card p-6">
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('adminReferral.grantSingle') }}</h3>
              <div class="mt-4 max-w-lg space-y-4">
                <div>
                  <label class="input-label">{{ t('adminReferral.userId') }}</label>
                  <input v-model.number="grantForm.user_id" type="number" min="1" class="input mt-1" :placeholder="t('adminReferral.userIdPlaceholder')" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.amount') }}</label>
                  <input v-model.number="grantForm.amount" type="number" step="0.01" min="0.01" class="input mt-1" :placeholder="t('adminReferral.amountPlaceholder')" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.expiryDays') }}</label>
                  <input v-model.number="grantForm.expiry_days" type="number" min="0" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.notes') }}</label>
                  <input v-model="grantForm.notes" type="text" class="input mt-1" :placeholder="t('adminReferral.notesPlaceholder')" />
                </div>
                <button @click="handleGrant" :disabled="granting || !grantForm.user_id || !grantForm.amount" class="btn btn-primary">
                  {{ granting ? t('adminReferral.granting') : t('adminReferral.grantGiftBalance') }}
                </button>
              </div>
            </div>

            <!-- Batch grant -->
            <div class="card p-6">
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('adminReferral.grantBatch') }}</h3>
              <div class="mt-4 max-w-lg space-y-4">
                <div>
                  <label class="input-label">{{ t('adminReferral.batchTarget') }}</label>
                  <Select v-model="batchForm.target" :options="batchTargetOptions" class="mt-1" />
                </div>
                <div v-if="batchForm.target === 'selected'">
                  <label class="input-label">{{ t('adminReferral.batchUserIds') }}</label>
                  <textarea v-model="batchUserIdsRaw" rows="3" class="input mt-1" :placeholder="t('adminReferral.batchUserIdsPlaceholder')" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.amount') }}</label>
                  <input v-model.number="batchForm.amount" type="number" step="0.01" min="0.01" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.expiryDays') }}</label>
                  <input v-model.number="batchForm.expiry_days" type="number" min="0" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.notes') }}</label>
                  <input v-model="batchForm.notes" type="text" class="input mt-1" />
                </div>
                <button @click="handleBatchGrant" :disabled="batchGranting || !batchForm.amount" class="btn btn-primary">
                  {{ batchGranting ? t('adminReferral.granting') : t('adminReferral.grantBatchAction') }}
                </button>
              </div>
            </div>
          </div>

          <!-- User Config Tab -->
          <div v-if="activeTab === 'userConfig'" class="p-6 space-y-4">
            <div class="card p-6">
              <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('adminReferral.userConfigTitle') }}</h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('adminReferral.userConfigDescription') }}</p>
              <div class="mt-4 flex gap-2">
                <input v-model.number="userConfigUserId" type="number" min="1" class="input" :placeholder="t('adminReferral.userIdPlaceholder')" />
                <button @click="loadUserConfig" :disabled="!userConfigUserId" class="btn btn-outline">{{ t('adminReferral.load') }}</button>
              </div>
            </div>

            <div v-if="userConfigData" class="card p-6">
              <div class="flex items-center justify-between">
                <h4 class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ userConfigData.has_custom_config ? t('adminReferral.userHasCustom') : t('adminReferral.userNoCustom') }}
                </h4>
                <button v-if="userConfigData.has_custom_config" @click="clearUserConfig" class="btn btn-outline btn-sm text-red-600">
                  {{ t('adminReferral.clearOverride') }}
                </button>
              </div>

              <div class="mt-4 grid gap-4 sm:grid-cols-2 max-w-2xl">
                <div>
                  <label class="input-label">{{ t('adminReferral.inviteeReward') }}</label>
                  <input v-model.number="userConfigForm.invitee_reward" type="number" step="0.01" min="0" class="input mt-1" :placeholder="String(userConfigData.effective.invitee_reward_amount)" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.inviterReward') }}</label>
                  <input v-model.number="userConfigForm.inviter_reward" type="number" step="0.01" min="0" class="input mt-1" :placeholder="String(userConfigData.effective.inviter_reward_amount)" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.maxInvites') }}</label>
                  <input v-model.number="userConfigForm.max_invites" type="number" min="0" class="input mt-1" :placeholder="String(userConfigData.effective.max_invites)" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.giftBalanceExpiry') }}</label>
                  <input v-model.number="userConfigForm.reward_expiry_days" type="number" min="0" class="input mt-1" :placeholder="String(userConfigData.effective.reward_expiry_days)" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardType') }}</label>
                  <Select v-model="userConfigForm.ongoing_reward_type" :options="userConfigRewardTypeOptions" class="mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardValue') }}</label>
                  <input v-model.number="userConfigForm.ongoing_reward_value" type="number" step="0.01" min="0" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardMaxCount') }}</label>
                  <input v-model.number="userConfigForm.ongoing_reward_max_count" type="number" min="0" class="input mt-1" />
                </div>
                <div>
                  <label class="input-label">{{ t('adminReferral.ongoingRewardDurationDays') }}</label>
                  <input v-model.number="userConfigForm.ongoing_reward_duration_days" type="number" min="0" class="input mt-1" />
                </div>
                <div class="sm:col-span-2">
                  <label class="input-label">{{ t('adminReferral.notes') }}</label>
                  <textarea v-model="userConfigForm.notes" rows="2" class="input mt-1" />
                </div>
              </div>

              <div class="mt-4">
                <button @click="saveUserConfig" :disabled="userConfigSaving" class="btn btn-primary">
                  {{ userConfigSaving ? t('adminReferral.saving') : t('adminReferral.saveUserConfig') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
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
import adminReferralAPI from '@/api/admin/referral'
import type {
  ReferralConfig,
  ReferralListItem,
  LeaderboardItem,
  AdminReferralDashboard,
  UserConfigResponse,
  UserConfigPayload,
} from '@/api/admin/referral'

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Title, Tooltip, Legend, Filler)

const { t } = useI18n()
const appStore = useAppStore()

type Tab = 'dashboard' | 'config' | 'list' | 'leaderboard' | 'grant' | 'userConfig'

const loading = ref(true)
const activeTab = ref<Tab>('dashboard')
const savingConfig = ref(false)
const granting = ref(false)
const batchGranting = ref(false)

const dashboard = ref<AdminReferralDashboard | null>(null)
const trendDays = ref<number | string | boolean | null>(30)

const trendDaysOptions = computed(() => [
  { value: 7, label: t('referral.last7Days') },
  { value: 30, label: t('referral.last30Days') },
  { value: 90, label: t('referral.last90Days') },
])

const ongoingRewardTypeOptions = computed(() => [
  { value: 'fixed', label: t('adminReferral.fixedAmount') },
  { value: 'percentage', label: t('adminReferral.percentage') },
])

const listStatusOptions = computed(() => [
  { value: '', label: t('adminReferral.allStatus') },
  { value: 'pending', label: t('referral.pending') },
  { value: 'completed', label: t('referral.completed') },
])

const leaderboardPeriodOptions = computed(() => [
  { value: 'all_time', label: t('adminReferral.allTime') },
  { value: 'this_month', label: t('adminReferral.thisMonth') },
  { value: 'this_week', label: t('adminReferral.thisWeek') },
])

const batchTargetOptions = computed(() => [
  { value: 'all', label: t('adminReferral.batchAll') },
  { value: 'selected', label: t('adminReferral.batchSelected') },
])

const userConfigRewardTypeOptions = computed(() => [
  { value: '', label: t('adminReferral.useGlobal') },
  { value: 'fixed', label: t('adminReferral.fixedAmount') },
  { value: 'percentage', label: t('adminReferral.percentage') },
])

const config = reactive<ReferralConfig>({
  referral_enabled: false,
  referral_invitee_reward: 0,
  referral_inviter_reward: 0,
  referral_max_invites: 0,
  referral_reward_expiry_days: 0,
  referral_gift_balance_expiry_days: 0,
  referral_ongoing_reward_enabled: false,
  referral_ongoing_reward_type: 'fixed',
  referral_ongoing_reward_value: 0,
  referral_ongoing_reward_max_count: 0,
  referral_ongoing_reward_duration_days: 0,
})

const referralList = ref<ReferralListItem[]>([])
const listFilter = reactive({ status: '' })

const leaderboard = ref<LeaderboardItem[]>([])
const leaderboardPeriod = ref<'all_time' | 'this_month' | 'this_week'>('all_time')

const grantForm = reactive({
  user_id: null as number | null,
  amount: null as number | null,
  expiry_days: 0,
  notes: '',
})

const batchForm = reactive({
  target: 'all' as 'all' | 'selected',
  amount: null as number | null,
  expiry_days: 0,
  notes: '',
})
const batchUserIdsRaw = ref('')

const userConfigUserId = ref<number | null>(null)
const userConfigData = ref<UserConfigResponse | null>(null)
const userConfigSaving = ref(false)
const userConfigForm = reactive<UserConfigPayload>({
  invitee_reward: undefined,
  inviter_reward: undefined,
  max_invites: undefined,
  reward_expiry_days: undefined,
  ongoing_reward_enabled: undefined,
  ongoing_reward_type: '',
  ongoing_reward_value: undefined,
  ongoing_reward_max_count: undefined,
  ongoing_reward_duration_days: undefined,
  notes: '',
})

// Computed
const funnelFirstRechargeWidth = computed(() => {
  const f = dashboard.value?.funnel
  if (!f || f.registrations === 0) return '0%'
  return `${Math.min(100, (f.first_recharges / f.registrations) * 100)}%`
})

const trendChartData = computed(() => {
  const trend = dashboard.value?.trend
  if (!trend || trend.length === 0) return null
  return {
    labels: trend.map((p) => p.date),
    datasets: [
      {
        label: t('adminReferral.trendInvitations'),
        data: trend.map((p) => p.invitations),
        borderColor: 'rgb(59, 130, 246)',
        backgroundColor: 'rgba(59, 130, 246, 0.1)',
        tension: 0.3,
        fill: true,
      },
      {
        label: t('adminReferral.trendCompletions'),
        data: trend.map((p) => p.completions),
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
  interaction: { mode: 'index' as const, intersect: false },
  plugins: { legend: { display: true, position: 'top' as const } },
  scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
}

onMounted(async () => {
  await loadAll()
})

async function loadAll() {
  loading.value = true
  try {
    await Promise.all([loadDashboard(), loadConfig(), loadList(), loadLeaderboard()])
  } catch (error) {
    console.error('Failed to load referral admin data:', error)
    appStore.showError(t('adminReferral.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadDashboard() {
  try {
    dashboard.value = await adminReferralAPI.getDashboard(Number(trendDays.value))
  } catch (error) {
    console.error('Failed to load dashboard:', error)
  }
}

async function loadConfig() {
  try {
    const cfg = await adminReferralAPI.getConfig()
    Object.assign(config, cfg)
  } catch (error) {
    console.error('Failed to load config:', error)
  }
}

async function loadList() {
  try {
    const result = await adminReferralAPI.listReferrals({
      page: 1,
      page_size: 50,
      status: listFilter.status || undefined,
    })
    referralList.value = result.items || []
  } catch (error) {
    console.error('Failed to load list:', error)
  }
}

async function loadLeaderboard() {
  try {
    leaderboard.value = (await adminReferralAPI.getLeaderboard({
      limit: 20,
      period: leaderboardPeriod.value,
    })) || []
  } catch (error) {
    console.error('Failed to load leaderboard:', error)
  }
}

async function saveConfig() {
  savingConfig.value = true
  try {
    await adminReferralAPI.updateConfig(config)
    appStore.showSuccess(t('adminReferral.configSaved'))
  } catch (error) {
    console.error('Failed to save config:', error)
    appStore.showError(t('adminReferral.configSaveFailed'))
  } finally {
    savingConfig.value = false
  }
}

async function handleGrant() {
  if (!grantForm.user_id || !grantForm.amount) return
  granting.value = true
  try {
    await adminReferralAPI.grantGiftBalance({
      user_id: grantForm.user_id,
      amount: grantForm.amount,
      expiry_days: grantForm.expiry_days || undefined,
      notes: grantForm.notes || undefined,
    })
    appStore.showSuccess(t('adminReferral.grantSuccess'))
    grantForm.user_id = null
    grantForm.amount = null
    grantForm.expiry_days = 0
    grantForm.notes = ''
    await loadDashboard()
  } catch (error) {
    console.error('Failed to grant gift balance:', error)
    appStore.showError(t('adminReferral.grantFailed'))
  } finally {
    granting.value = false
  }
}

async function handleBatchGrant() {
  if (!batchForm.amount) return

  let userIDs: number[] = []
  if (batchForm.target === 'selected') {
    userIDs = batchUserIdsRaw.value
      .split(/[\s,;\n]+/)
      .map((s) => parseInt(s.trim(), 10))
      .filter((n) => Number.isFinite(n) && n > 0)
    if (userIDs.length === 0) {
      appStore.showError(t('adminReferral.batchUserIdsEmpty'))
      return
    }
  }

  if (!confirm(t('adminReferral.batchConfirm'))) return

  batchGranting.value = true
  try {
    const result = await adminReferralAPI.batchGrantGiftBalance({
      target: batchForm.target,
      user_ids: userIDs,
      amount: batchForm.amount,
      expiry_days: batchForm.expiry_days || undefined,
      notes: batchForm.notes || undefined,
    })
    appStore.showSuccess(t('adminReferral.batchSuccess', { count: result.granted_count }))
    batchForm.amount = null
    batchForm.expiry_days = 0
    batchForm.notes = ''
    batchUserIdsRaw.value = ''
    await loadDashboard()
  } catch (error) {
    console.error('Failed to batch grant:', error)
    appStore.showError(t('adminReferral.grantFailed'))
  } finally {
    batchGranting.value = false
  }
}

async function loadUserConfig() {
  if (!userConfigUserId.value) return
  try {
    const data = await adminReferralAPI.getUserConfig(userConfigUserId.value)
    userConfigData.value = data
    // Populate form with existing custom values
    const c = data.config
    userConfigForm.invitee_reward = c?.invitee_reward_amount ?? undefined
    userConfigForm.inviter_reward = c?.inviter_reward_amount ?? undefined
    userConfigForm.max_invites = c?.max_invites ?? undefined
    userConfigForm.reward_expiry_days = c?.reward_expiry_days ?? undefined
    userConfigForm.ongoing_reward_max_count = c?.ongoing_reward_max_count ?? undefined
    userConfigForm.ongoing_reward_duration_days = c?.ongoing_reward_duration_days ?? undefined
    userConfigForm.notes = c?.notes ?? ''

    userConfigForm.ongoing_reward_type = c?.ongoing_reward_type ?? ''
    userConfigForm.ongoing_reward_value = c?.ongoing_reward_value ?? undefined
  } catch (error) {
    console.error('Failed to load user config:', error)
    appStore.showError(t('adminReferral.userConfigLoadFailed'))
  }
}

async function saveUserConfig() {
  if (!userConfigUserId.value) return
  userConfigSaving.value = true
  try {
    await adminReferralAPI.upsertUserConfig(userConfigUserId.value, {
      invitee_reward: userConfigForm.invitee_reward,
      inviter_reward: userConfigForm.inviter_reward,
      max_invites: userConfigForm.max_invites,
      reward_expiry_days: userConfigForm.reward_expiry_days,
      ongoing_reward_type: userConfigForm.ongoing_reward_type || undefined,
      ongoing_reward_value: userConfigForm.ongoing_reward_value,
      ongoing_reward_max_count: userConfigForm.ongoing_reward_max_count,
      ongoing_reward_duration_days: userConfigForm.ongoing_reward_duration_days,
      notes: userConfigForm.notes,
    })
    appStore.showSuccess(t('adminReferral.userConfigSaved'))
    await loadUserConfig()
  } catch (error) {
    console.error('Failed to save user config:', error)
    appStore.showError(t('adminReferral.userConfigSaveFailed'))
  } finally {
    userConfigSaving.value = false
  }
}

async function clearUserConfig() {
  if (!userConfigUserId.value) return
  if (!confirm(t('adminReferral.clearOverrideConfirm'))) return
  try {
    await adminReferralAPI.deleteUserConfig(userConfigUserId.value)
    appStore.showSuccess(t('adminReferral.clearOverrideSuccess'))
    await loadUserConfig()
  } catch (error) {
    console.error('Failed to clear user config:', error)
    appStore.showError(t('adminReferral.clearOverrideFailed'))
  }
}

function tabClass(tab: Tab): string {
  return [
    'flex-1 min-w-fit py-3 px-4 text-center text-sm font-medium border-b-2 transition-colors whitespace-nowrap',
    activeTab.value === tab
      ? 'border-primary-500 text-primary-600 dark:text-primary-400'
      : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200',
  ].join(' ')
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  })
}
</script>

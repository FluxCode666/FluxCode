<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <!-- Header -->
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('nav.referralManagement') }}</h1>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-12">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
        </svg>
      </div>

      <template v-else>
        <!-- Stats Cards -->
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-5">
          <div class="card p-4 text-center">
            <div class="text-2xl font-bold text-gray-900 dark:text-white">{{ stats?.total_referrals ?? 0 }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalReferrals') }}</div>
          </div>
          <div class="card p-4 text-center">
            <div class="text-2xl font-bold text-green-600 dark:text-green-400">{{ stats?.completed_referrals ?? 0 }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.completed') }}</div>
          </div>
          <div class="card p-4 text-center">
            <div class="text-2xl font-bold text-primary-600 dark:text-primary-400">${{ (stats?.total_gift_balance_issued ?? 0).toFixed(2) }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalIssued') }}</div>
          </div>
          <div class="card p-4 text-center">
            <div class="text-2xl font-bold text-amber-600 dark:text-amber-400">${{ (stats?.total_gift_balance_remaining ?? 0).toFixed(2) }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.remaining') }}</div>
          </div>
          <div class="card p-4 text-center">
            <div class="text-2xl font-bold text-red-600 dark:text-red-400">${{ (stats?.total_gift_balance_consumed ?? 0).toFixed(2) }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('adminReferral.consumed') }}</div>
          </div>
        </div>

        <!-- Tabs -->
        <div class="card">
          <div class="border-b border-gray-200 dark:border-dark-600">
            <nav class="flex -mb-px">
              <button
                @click="activeTab = 'config'"
                :class="tabClass('config')"
              >
                {{ t('adminReferral.tabConfig') }}
              </button>
              <button
                @click="activeTab = 'list'"
                :class="tabClass('list')"
              >
                {{ t('adminReferral.tabList') }}
              </button>
              <button
                @click="activeTab = 'leaderboard'"
                :class="tabClass('leaderboard')"
              >
                {{ t('adminReferral.tabLeaderboard') }}
              </button>
              <button
                @click="activeTab = 'grant'"
                :class="tabClass('grant')"
              >
                {{ t('adminReferral.tabGrant') }}
              </button>
            </nav>
          </div>

          <!-- Config Tab -->
          <div v-if="activeTab === 'config'" class="p-6">
            <div class="space-y-4 max-w-lg">
              <div class="flex items-center justify-between">
                <label class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('adminReferral.referralEnabled') }}</label>
                <button
                  @click="config.referral_enabled = !config.referral_enabled"
                  :class="[
                    'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                    config.referral_enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'
                  ]"
                >
                  <span
                    :class="[
                      'inline-block h-4 w-4 transform rounded-full bg-white transition-transform',
                      config.referral_enabled ? 'translate-x-6' : 'translate-x-1'
                    ]"
                  />
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
                <label class="input-label">{{ t('adminReferral.inviterRewardType') }}</label>
                <select v-model="config.referral_inviter_reward_type" class="input mt-1">
                  <option value="fixed">{{ t('adminReferral.fixedAmount') }}</option>
                  <option value="percentage">{{ t('adminReferral.percentageOfFirstRecharge') }}</option>
                </select>
              </div>
              <div>
                <label class="input-label">{{ t('adminReferral.giftBalanceExpiry') }}</label>
                <input v-model.number="config.referral_gift_balance_expiry_days" type="number" min="0" class="input mt-1" />
              </div>
              <div>
                <label class="input-label">{{ t('adminReferral.maxInvites') }}</label>
                <input v-model.number="config.referral_max_invites" type="number" min="0" class="input mt-1" />
              </div>
              <div class="flex items-center justify-between">
                <label class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('adminReferral.ongoingRewardEnabled') }}</label>
                <button
                  @click="config.referral_ongoing_reward_enabled = !config.referral_ongoing_reward_enabled"
                  :class="[
                    'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                    config.referral_ongoing_reward_enabled ? 'bg-primary-600' : 'bg-gray-300 dark:bg-dark-600'
                  ]"
                >
                  <span
                    :class="[
                      'inline-block h-4 w-4 transform rounded-full bg-white transition-transform',
                      config.referral_ongoing_reward_enabled ? 'translate-x-6' : 'translate-x-1'
                    ]"
                  />
                </button>
              </div>
              <div v-if="config.referral_ongoing_reward_enabled">
                <label class="input-label">{{ t('adminReferral.ongoingRewardValue') }}</label>
                <input v-model.number="config.referral_ongoing_reward_value" type="number" step="0.01" min="0" class="input mt-1" />
              </div>
              <div v-if="config.referral_ongoing_reward_enabled">
                <label class="input-label">{{ t('adminReferral.ongoingRewardType') }}</label>
                <select v-model="config.referral_ongoing_reward_type" class="input mt-1">
                  <option value="fixed">{{ t('adminReferral.fixedAmount') }}</option>
                  <option value="percentage">{{ t('adminReferral.percentage') }}</option>
                </select>
              </div>
              <div v-if="config.referral_ongoing_reward_enabled">
                <label class="input-label">{{ t('adminReferral.ongoingRewardMaxCount') }}</label>
                <input v-model.number="config.referral_ongoing_reward_max_count" type="number" min="0" class="input mt-1" />
              </div>
              <button @click="saveConfig" :disabled="savingConfig" class="btn btn-primary">
                {{ savingConfig ? t('adminReferral.saving') : t('adminReferral.saveConfig') }}
              </button>
            </div>
          </div>

          <!-- Referral List Tab -->
          <div v-if="activeTab === 'list'" class="p-6">
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
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ item.referrer_email }}</td>
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ item.referee_email }}</td>
                    <td class="py-3 pr-4">
                      <span
                        :class="[
                          'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                          item.status === 'completed'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
                        ]"
                      >
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
            <div v-if="leaderboard.length === 0" class="py-8 text-center text-gray-500 dark:text-dark-400">
              {{ t('adminReferral.noLeaderboard') }}
            </div>
            <div v-else class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-gray-200 dark:border-dark-600">
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.rank') }}</th>
                    <th class="pb-3 pr-4 text-left font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.user') }}</th>
                    <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalInvites') }}</th>
                    <th class="pb-3 pr-4 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.completed') }}</th>
                    <th class="pb-3 text-right font-medium text-gray-500 dark:text-dark-400">{{ t('adminReferral.totalEarned') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                  <tr v-for="(item, index) in leaderboard" :key="item.user_id">
                    <td class="py-3 pr-4 font-medium text-gray-700 dark:text-dark-200">{{ index + 1 }}</td>
                    <td class="py-3 pr-4 text-gray-700 dark:text-dark-200">{{ item.email }}</td>
                    <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">{{ item.total_invites }}</td>
                    <td class="py-3 pr-4 text-right text-gray-700 dark:text-dark-200">{{ item.completed_invites }}</td>
                    <td class="py-3 text-right font-medium text-primary-600 dark:text-primary-400">${{ item.total_earned.toFixed(2) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- Grant Balance Tab -->
          <div v-if="activeTab === 'grant'" class="p-6">
            <div class="max-w-lg space-y-4">
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
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { AppLayout } from '@/components/layout'
import { useAppStore } from '@/stores'
import adminReferralAPI from '@/api/admin/referral'
import type {
  ReferralStats,
  ReferralConfig,
  ReferralListItem,
  LeaderboardItem
} from '@/api/admin/referral'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const activeTab = ref<'config' | 'list' | 'leaderboard' | 'grant'>('config')
const savingConfig = ref(false)
const granting = ref(false)

const stats = ref<ReferralStats | null>(null)
const config = reactive<ReferralConfig>({
  referral_enabled: false,
  referral_invitee_reward: 0,
  referral_inviter_reward: 0,
  referral_inviter_reward_type: 'fixed',
  referral_gift_balance_expiry_days: 0,
  referral_max_invites: 0,
  referral_ongoing_reward_enabled: false,
  referral_ongoing_reward_value: 0,
  referral_ongoing_reward_type: 'fixed',
  referral_ongoing_reward_max_count: 0
})
const referralList = ref<ReferralListItem[]>([])
const leaderboard = ref<LeaderboardItem[]>([])

const grantForm = reactive({
  user_id: null as number | null,
  amount: null as number | null,
  expiry_days: 0,
  notes: ''
})

onMounted(async () => {
  await loadData()
})

async function loadData() {
  loading.value = true
  try {
    const [statsData, configData, listData, leaderboardData] = await Promise.all([
      adminReferralAPI.getStats(),
      adminReferralAPI.getConfig(),
      adminReferralAPI.listReferrals({ page: 1, page_size: 50 }),
      adminReferralAPI.getLeaderboard(10)
    ])
    stats.value = statsData
    Object.assign(config, configData)
    referralList.value = listData.items || []
    leaderboard.value = leaderboardData || []
  } catch (error) {
    console.error('Failed to load referral management data:', error)
    appStore.showError(t('adminReferral.loadFailed'))
  } finally {
    loading.value = false
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
      notes: grantForm.notes || undefined
    })
    appStore.showSuccess(t('adminReferral.grantSuccess'))
    grantForm.user_id = null
    grantForm.amount = null
    grantForm.expiry_days = 0
    grantForm.notes = ''
    // Refresh stats
    stats.value = await adminReferralAPI.getStats()
  } catch (error) {
    console.error('Failed to grant gift balance:', error)
    appStore.showError(t('adminReferral.grantFailed'))
  } finally {
    granting.value = false
  }
}

function tabClass(tab: string): string {
  return [
    'flex-1 py-3 px-4 text-center text-sm font-medium border-b-2 transition-colors',
    activeTab.value === tab
      ? 'border-primary-500 text-primary-600 dark:text-primary-400'
      : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200'
  ].join(' ')
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  })
}
</script>

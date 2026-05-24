<template>
  <BaseDialog :show="show" :title="t('admin.users.auditLogsTitle')" width="wide" :close-on-click-outside="true" :z-index="40" @close="$emit('close')">
    <div v-if="user" class="space-y-4">
      <!-- User header -->
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
            <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
              {{ user.email.charAt(0).toUpperCase() }}
            </span>
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <p class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
              <span
                v-if="user.username"
                class="flex-shrink-0 rounded bg-primary-50 px-1.5 py-0.5 text-xs text-primary-600 dark:bg-primary-900/20 dark:text-primary-400"
              >
                {{ user.username }}
              </span>
            </div>
            <p class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.users.createdAt') }}: {{ formatDateTime(user.created_at) }}
            </p>
          </div>
          <div class="flex-shrink-0 text-right">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.users.currentBalance') }}</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">
              ${{ user.balance?.toFixed(2) || '0.00' }}
            </p>
          </div>
        </div>
      </div>

      <!-- Loading -->
      <div v-if="loading" class="flex justify-center py-8">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
      </div>

      <!-- Empty state -->
      <div v-else-if="entries.length === 0" class="py-8 text-center">
        <p class="text-sm text-gray-500">{{ t('admin.users.noAuditLogs') }}</p>
      </div>

      <!-- Audit log entries -->
      <div v-else class="max-h-[32rem] space-y-3 overflow-y-auto">
        <div
          v-for="entry in entries"
          :key="entry.order_id"
          class="rounded-xl border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-800"
        >
          <!-- Order header (clickable to toggle) -->
          <button
            class="flex w-full items-center justify-between px-4 py-3 text-left transition-colors hover:bg-gray-50 dark:hover:bg-dark-750"
            :class="{ 'border-b border-gray-100 dark:border-dark-700': expandedOrders.has(entry.order_id) }"
            @click="toggleOrder(entry.order_id)"
          >
            <div class="flex items-center gap-3">
              <Icon
                name="chevronRight"
                size="sm"
                class="h-4 w-4 flex-shrink-0 text-gray-400 transition-transform duration-150"
                :class="{ 'rotate-90': expandedOrders.has(entry.order_id) }"
              />
              <div
                :class="[
                  'flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg',
                  getPaymentTypeBg(entry.payment_type)
                ]"
              >
                <Icon name="dollar" size="sm" :class="getPaymentTypeColor(entry.payment_type)" />
              </div>
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-gray-900 dark:text-white">#{{ entry.order_id }}</span>
                  <span :class="['inline-flex rounded-full px-2 py-0.5 text-xs font-medium', getStatusClass(entry.status)]">
                    {{ t('payment.status.' + entry.status.toLowerCase(), entry.status) }}
                  </span>
                  <span class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-600 dark:bg-dark-600 dark:text-gray-400">
                    {{ t('payment.methods.' + entry.payment_type, entry.payment_type) }}
                  </span>
                </div>
                <p class="text-xs text-gray-400 dark:text-dark-500">
                  {{ formatDateTime(entry.created_at) }}
                </p>
              </div>
            </div>
            <div class="flex items-center gap-3">
              <span v-if="entry.audit_logs.length > 0" class="rounded-full bg-blue-50 px-2 py-0.5 text-xs text-blue-600 dark:bg-blue-900/20 dark:text-blue-400">
                {{ entry.audit_logs.length }}
              </span>
              <div class="text-right">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">${{ entry.amount.toFixed(2) }}</p>
                <p v-if="entry.pay_amount !== entry.amount" class="text-xs text-gray-400 dark:text-dark-500">
                  {{ t('admin.users.auditPayAmount') }}: ¥{{ entry.pay_amount.toFixed(2) }}
                </p>
              </div>
            </div>
          </button>

          <!-- Audit logs for this order (collapsible) -->
          <div v-if="expandedOrders.has(entry.order_id) && entry.audit_logs.length > 0" class="divide-y divide-gray-50 dark:divide-dark-700">
            <div
              v-for="log in entry.audit_logs"
              :key="log.id"
              class="px-4 py-2.5"
            >
              <div class="flex items-start gap-3">
                <button
                  v-if="formatDetail(log.detail)"
                  class="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-blue-50 transition-colors hover:bg-blue-100 dark:bg-blue-900/20 dark:hover:bg-blue-900/40"
                  @click="toggleDetail(log.id)"
                >
                  <Icon
                    name="chevronRight"
                    size="xs"
                    class="h-3 w-3 text-blue-500 transition-transform duration-150"
                    :class="{ 'rotate-90': expandedLogs.has(log.id) }"
                  />
                </button>
                <div v-else class="mt-0.5 flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full bg-blue-50 dark:bg-blue-900/20">
                  <div class="h-1.5 w-1.5 rounded-full bg-blue-500"></div>
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center gap-2">
                    <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ log.action }}</span>
                    <span class="text-xs text-gray-400 dark:text-dark-500">{{ log.operator }}</span>
                  </div>
                  <!-- Collapsed: single-line summary -->
                  <p
                    v-if="formatDetail(log.detail) && !expandedLogs.has(log.id)"
                    class="mt-0.5 max-w-full cursor-pointer truncate text-xs text-gray-500 dark:text-dark-400"
                    @click="toggleDetail(log.id)"
                  >
                    {{ formatDetailSummary(log.detail) }}
                  </p>
                  <p class="mt-0.5 text-xs text-gray-400 dark:text-dark-500">{{ formatDateTime(log.created_at) }}</p>
                </div>
              </div>
              <!-- Expanded: full detail -->
              <div
                v-if="expandedLogs.has(log.id)"
                class="ml-8 mt-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-700"
              >
                <dl class="space-y-1">
                  <div v-for="(value, key) in parseDetail(log.detail)" :key="String(key)" class="flex gap-2 text-xs">
                    <dt class="flex-shrink-0 font-medium text-gray-600 dark:text-gray-400">{{ key }}:</dt>
                    <dd class="break-all text-gray-500 dark:text-dark-400">{{ value }}</dd>
                  </div>
                </dl>
                <p v-if="!parseDetail(log.detail)" class="whitespace-pre-wrap break-all text-xs text-gray-500 dark:text-dark-400">{{ log.detail }}</p>
              </div>
            </div>
          </div>
          <div v-else-if="expandedOrders.has(entry.order_id)" class="px-4 py-2.5">
            <p class="text-xs text-gray-400 dark:text-dark-500 italic">{{ t('admin.users.noAuditLogs') }}</p>
          </div>
        </div>
      </div>

      <!-- Pagination -->
      <div v-if="totalPages > 1" class="flex items-center justify-center gap-2 pt-2">
        <button
          :disabled="currentPage <= 1"
          class="btn btn-secondary px-3 py-1 text-sm"
          @click="loadLogs(currentPage - 1)"
        >
          {{ t('pagination.previous') }}
        </button>
        <span class="text-sm text-gray-500 dark:text-dark-400">
          {{ currentPage }} / {{ totalPages }}
        </span>
        <button
          :disabled="currentPage >= totalPages"
          class="btn btn-secondary px-3 py-1 text-sm"
          @click="loadLogs(currentPage + 1)"
        >
          {{ t('pagination.next') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI, type UserAuditLogEntry } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type { AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null }>()
defineEmits(['close'])
const { t } = useI18n()

const entries = ref<UserAuditLogEntry[]>([])
const loading = ref(false)
const expandedLogs = ref<Set<number>>(new Set())
const expandedOrders = ref<Set<number>>(new Set())
const currentPage = ref(1)
const total = ref(0)
const pageSize = 10

const totalPages = computed(() => Math.ceil(total.value / pageSize) || 1)

watch(() => props.show, (v) => {
  if (v && props.user) {
    expandedLogs.value = new Set()
    expandedOrders.value = new Set()
    loadLogs(1)
  }
})

const toggleOrder = (orderId: number) => {
  const s = new Set(expandedOrders.value)
  if (s.has(orderId)) {
    s.delete(orderId)
  } else {
    s.add(orderId)
  }
  expandedOrders.value = s
}

const toggleDetail = (logId: number) => {
  const s = new Set(expandedLogs.value)
  if (s.has(logId)) {
    s.delete(logId)
  } else {
    s.add(logId)
  }
  expandedLogs.value = s
}

const loadLogs = async (page: number) => {
  if (!props.user) return
  loading.value = true
  currentPage.value = page
  try {
    const res = await adminAPI.users.getUserAuditLogs(props.user.id, page, pageSize)
    entries.value = res.items || []
    total.value = res.total || 0
  } catch (error) {
    console.error('Failed to load audit logs:', error)
  } finally {
    loading.value = false
  }
}

const getPaymentTypeBg = (type: string) => {
  if (type === 'offline') return 'bg-amber-100 dark:bg-amber-900/30'
  return 'bg-emerald-100 dark:bg-emerald-900/30'
}

const getPaymentTypeColor = (type: string) => {
  if (type === 'offline') return 'text-amber-600 dark:text-amber-400'
  return 'text-emerald-600 dark:text-emerald-400'
}

const getStatusClass = (status: string) => {
  const s = status.toLowerCase()
  if (s === 'completed') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
  if (s === 'paid' || s === 'recharging') return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
  if (s === 'pending') return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400'
  if (s === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400'
  if (s === 'expired' || s === 'cancelled') return 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'
  if (s.includes('refund')) return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'
}

const formatDetail = (detail: string): string => {
  if (!detail) return ''
  try {
    const obj = JSON.parse(detail)
    return Object.entries(obj)
      .map(([k, v]) => `${k}: ${v}`)
      .join(', ')
  } catch {
    return detail
  }
}

const formatDetailSummary = (detail: string): string => {
  const full = formatDetail(detail)
  return full.length > 80 ? full.substring(0, 75) + '...' : full
}

const parseDetail = (detail: string): Record<string, unknown> | null => {
  if (!detail) return null
  try {
    const obj = JSON.parse(detail)
    return typeof obj === 'object' && obj !== null ? obj : null
  } catch {
    return null
  }
}
</script>

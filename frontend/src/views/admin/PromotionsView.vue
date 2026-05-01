<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <!-- Left: Search + Filters -->
          <div class="flex-1 sm:max-w-64">
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('admin.promotions.searchPlaceholder')"
              class="input"
              @input="handleSearch"
            />
          </div>
          <Select
            v-model="filters.status"
            :options="filterStatusOptions"
            class="w-36"
            @change="loadPromotions"
          />
          <Select
            v-model="filters.promotion_type"
            :options="filterTypeOptions"
            class="w-36"
            @change="loadPromotions"
          />

          <!-- Right: Action buttons -->
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button
              @click="loadPromotions"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-1" />
              {{ t('admin.promotions.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="promotions"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="priority"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ value, row }">
            <div>
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ value }}</span>
              <p v-if="row.description" class="mt-0.5 text-xs text-gray-500 dark:text-gray-400 line-clamp-1">
                {{ row.description }}
              </p>
            </div>
          </template>

          <template #cell-promotion_type="{ value }">
            <span
              :class="[
                'badge',
                value === 'recharge' ? 'badge-primary' : 'badge-purple'
              ]"
            >
              {{ value === 'recharge' ? t('admin.promotions.typeRecharge') : t('admin.promotions.typeSubscription') }}
            </span>
          </template>

          <template #cell-discount_info="{ row }">
            <div class="text-sm text-gray-700 dark:text-gray-300">
              <template v-if="row.promotion_type === 'recharge'">
                <template v-if="row.discount_mode === 'reduce_pay'">
                  <span class="font-medium text-green-600 dark:text-green-400">
                    {{ t('admin.promotions.reducePay') }}
                  </span>
                  <span v-if="row.recharge_rate != null" class="ml-1">
                    {{ (row.recharge_rate * 100).toFixed(0) }}%
                  </span>
                </template>
                <template v-else-if="row.discount_mode === 'bonus_credit'">
                  <span class="font-medium text-blue-600 dark:text-blue-400">
                    {{ t('admin.promotions.bonusCredit') }}
                  </span>
                  <span v-if="row.recharge_bonus_rate != null" class="ml-1">
                    +{{ ((row.recharge_bonus_rate - 1) * 100).toFixed(0) }}%
                  </span>
                </template>
              </template>
              <template v-else>
                <span class="text-xs text-gray-500 dark:text-gray-400">
                  {{ (row.plan_rules?.length || 0) }} {{ t('admin.promotions.planRulesCount') }}
                </span>
              </template>
            </div>
          </template>

          <template #cell-max_uses_per_user="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">
              {{ value === 0 ? '∞' : value }}
            </span>
          </template>

          <template #cell-time_range="{ row }">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              <div v-if="row.starts_at">
                {{ t('admin.promotions.from') }}: {{ formatDateTime(row.starts_at) }}
              </div>
              <div v-if="row.ends_at">
                {{ t('admin.promotions.to') }}: {{ formatDateTime(row.ends_at) }}
              </div>
              <span v-if="!row.starts_at && !row.ends_at">{{ t('admin.promotions.always') }}</span>
            </div>
          </template>

          <template #cell-status="{ value, row }">
            <span :class="['badge', getStatusClass(value, row)]">
              {{ getStatusLabel(value, row) }}
            </span>
          </template>

          <template #cell-priority="{ value }">
            <span class="text-sm font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center space-x-1">
              <button
                @click="handleViewUsages(row)"
                class="flex items-center rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
                :title="t('admin.promotions.viewUsages')"
              >
                <Icon name="eye" size="sm" />
              </button>
              <button
                @click="handleToggleStatus(row)"
                :class="[
                  'flex items-center rounded-lg p-1.5 transition-colors',
                  row.status === 'active'
                    ? 'text-yellow-500 hover:bg-yellow-50 hover:text-yellow-600 dark:hover:bg-yellow-900/20'
                    : 'text-green-500 hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20'
                ]"
                :title="row.status === 'active' ? t('admin.promotions.disable') : t('admin.promotions.enable')"
              >
                <Icon :name="row.status === 'active' ? 'ban' : 'check'" size="sm" />
              </button>
              <button
                @click="handleEdit(row)"
                class="flex items-center rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-600 dark:hover:text-gray-300"
                :title="t('common.edit')"
              >
                <Icon name="edit" size="sm" />
              </button>
              <button
                @click="handleDelete(row)"
                class="flex items-center rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create / Edit Dialog -->
    <BaseDialog
      :show="showFormDialog"
      :title="editingPromotion ? t('admin.promotions.edit') : t('admin.promotions.create')"
      width="wide"
      @close="closeFormDialog"
    >
      <form id="promotion-form" @submit.prevent="handleSubmit" class="space-y-4">
        <!-- Basic Info -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.promotions.name') }}</label>
            <input
              v-model="formData.name"
              type="text"
              required
              class="input"
              :placeholder="t('admin.promotions.namePlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.promotions.promotionType') }}</label>
            <Select
              v-model="formData.promotion_type"
              :options="promotionTypeOptions"
              :disabled="!!editingPromotion"
            />
          </div>
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.promotions.descriptionLabel') }}
            <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
          </label>
          <textarea
            v-model="formData.description"
            rows="2"
            class="input"
            :placeholder="t('admin.promotions.descriptionPlaceholder')"
          ></textarea>
        </div>

        <!-- Recharge Settings -->
        <template v-if="formData.promotion_type === 'recharge'">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <h4 class="mb-3 text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.promotions.rechargeSettings') }}
            </h4>
            <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.promotions.discountMode') }}</label>
                <Select
                  v-model="formData.discount_mode"
                  :options="rechargeDiscountModeOptions"
                />
              </div>
              <div v-if="formData.discount_mode === 'reduce_pay'">
                <label class="input-label">
                  {{ t('admin.promotions.rechargeRate') }}
                  <span class="ml-1 text-xs font-normal text-gray-400">({{ t('admin.promotions.rechargeRateHint') }})</span>
                </label>
                <input
                  v-model.number="formData.recharge_rate"
                  type="number"
                  step="0.01"
                  min="0.01"
                  max="1"
                  class="input"
                  placeholder="0.80"
                />
              </div>
              <div v-if="formData.discount_mode === 'bonus_credit'">
                <label class="input-label">
                  {{ t('admin.promotions.rechargeBonusRate') }}
                  <span class="ml-1 text-xs font-normal text-gray-400">({{ t('admin.promotions.rechargeBonusRateHint') }})</span>
                </label>
                <input
                  v-model.number="formData.recharge_bonus_rate"
                  type="number"
                  step="0.01"
                  min="0.01"
                  class="input"
                  placeholder="0.20"
                />
              </div>
            </div>
          </div>
        </template>

        <!-- Subscription Plan Rules -->
        <template v-if="formData.promotion_type === 'subscription'">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
            <div class="mb-3 flex items-center justify-between">
              <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.promotions.planRules') }}
              </h4>
              <button type="button" @click="addPlanRule" class="btn btn-secondary btn-sm">
                <Icon name="plus" size="sm" class="mr-1" />
                {{ t('admin.promotions.addPlanRule') }}
              </button>
            </div>
            <div v-if="formData.plan_rules.length === 0" class="py-4 text-center text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.promotions.noPlanRules') }}
            </div>
            <div v-else class="space-y-3">
              <div
                v-for="(rule, index) in formData.plan_rules"
                :key="index"
                class="relative rounded-lg border border-gray-100 bg-gray-50 p-3 dark:border-dark-500 dark:bg-dark-700"
              >
                <button
                  type="button"
                  @click="removePlanRule(index)"
                  class="absolute right-2 top-2 text-gray-400 hover:text-red-500"
                >
                  <Icon name="x" size="sm" />
                </button>
                <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
                  <div>
                    <label class="input-label text-xs">{{ t('admin.promotions.plan') }}</label>
                    <select v-model.number="rule.plan_id" class="input" required>
                      <option :value="0" disabled>{{ t('admin.promotions.selectPlan') }}</option>
                      <option
                        v-for="plan in availablePlans"
                        :key="plan.id"
                        :value="plan.id"
                      >
                        {{ plan.name }} (${{ plan.price.toFixed(2) }})
                      </option>
                    </select>
                  </div>
                  <div>
                    <label class="input-label text-xs">{{ t('admin.promotions.discountMode') }}</label>
                    <Select v-model="rule.discount_mode" :options="planDiscountModeOptions" />
                  </div>
                  <div v-if="rule.discount_mode === 'rate'">
                    <label class="input-label text-xs">
                      {{ t('admin.promotions.discountRate') }}
                      <span class="text-xs text-gray-400">({{ t('admin.promotions.discountRateHint') }})</span>
                    </label>
                    <input
                      v-model.number="rule.discount_rate"
                      type="number"
                      step="0.01"
                      min="0.01"
                      max="1"
                      class="input"
                      placeholder="0.80"
                    />
                  </div>
                  <div v-if="rule.discount_mode === 'amount'">
                    <label class="input-label text-xs">
                      {{ t('admin.promotions.discountAmount') }} ($)
                    </label>
                    <input
                      v-model.number="rule.discount_amount"
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      placeholder="5.00"
                    />
                  </div>
                  <div>
                    <label class="input-label text-xs">
                      {{ t('admin.promotions.minPriceFloor') }} ($)
                    </label>
                    <input
                      v-model.number="rule.min_price_floor"
                      type="number"
                      step="0.01"
                      min="0"
                      class="input"
                      placeholder="0.00"
                    />
                  </div>
                  <div>
                    <label class="input-label text-xs">
                      {{ t('admin.promotions.maxUsesPerUser') }}
                      <span class="text-xs text-gray-400">({{ t('admin.promotions.zeroUnlimited') }})</span>
                    </label>
                    <input
                      v-model.number="rule.max_uses_per_user"
                      type="number"
                      min="0"
                      class="input"
                      placeholder="0"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>

        <!-- Common Settings -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label class="input-label">
              {{ t('admin.promotions.maxUsesPerUser') }}
              <span class="ml-1 text-xs font-normal text-gray-400">({{ t('admin.promotions.zeroUnlimited') }})</span>
            </label>
            <input
              v-model.number="formData.max_uses_per_user"
              type="number"
              min="0"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.promotions.priority') }}</label>
            <input
              v-model.number="formData.priority"
              type="number"
              min="0"
              class="input"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.promotions.statusLabel') }}</label>
            <Select v-model="formData.status" :options="statusOptions" />
          </div>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">
              {{ t('admin.promotions.startsAt') }}
              <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
            </label>
            <input v-model="formData.starts_at_str" type="datetime-local" class="input" />
          </div>
          <div>
            <label class="input-label">
              {{ t('admin.promotions.endsAt') }}
              <span class="ml-1 text-xs font-normal text-gray-400">({{ t('common.optional') }})</span>
            </label>
            <input v-model="formData.ends_at_str" type="datetime-local" class="input" />
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" @click="closeFormDialog" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="promotion-form" :disabled="submitting" class="btn btn-primary">
            {{ submitting ? t('common.saving') : (editingPromotion ? t('common.save') : t('common.create')) }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Usages Dialog -->
    <BaseDialog
      :show="showUsagesDialog"
      :title="t('admin.promotions.usageRecords')"
      width="wide"
      @close="showUsagesDialog = false"
    >
      <div v-if="usagesLoading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>
      <div v-else-if="usages.length === 0" class="py-8 text-center text-gray-500 dark:text-gray-400">
        {{ t('admin.promotions.noUsages') }}
      </div>
      <div v-else class="space-y-3">
        <div
          v-for="usage in usages"
          :key="usage.id"
          class="flex items-center justify-between rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="flex items-center gap-3">
            <div class="flex h-8 w-8 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
              <Icon name="user" size="sm" class="text-green-600 dark:text-green-400" />
            </div>
            <div>
              <p class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.promotions.userPrefix', { id: usage.user_id }) }}
              </p>
              <p class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.promotions.orderPrefix', { id: usage.order_id }) }} · {{ formatDateTime(usage.used_at) }}
              </p>
            </div>
          </div>
          <div class="text-right text-sm">
            <div v-if="usage.discount_amount > 0" class="font-medium text-green-600 dark:text-green-400">
              -${{ usage.discount_amount.toFixed(2) }}
            </div>
            <div v-if="usage.bonus_amount > 0" class="font-medium text-blue-600 dark:text-blue-400">
              +${{ usage.bonus_amount.toFixed(2) }}
            </div>
          </div>
        </div>
        <div v-if="usagesTotal > usagesPageSize" class="mt-4">
          <Pagination
            :page="usagesPage"
            :total="usagesTotal"
            :page-size="usagesPageSize"
            @update:page="handleUsagesPageChange"
            @update:page-size="(size: number) => { usagesPageSize = size; usagesPage = 1; loadUsages() }"
          />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button type="button" @click="showUsagesDialog = false" class="btn btn-secondary">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.promotions.deleteTitle')"
      :message="t('admin.promotions.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type {
  Promotion,
  PromotionUsage,
  CreatePromotionRequest,
  UpdatePromotionRequest,
  CreatePromotionPlanRuleRequest
} from '@/types'
import type { SubscriptionPlan } from '@/types/payment'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

// State
const promotions = ref<Promotion[]>([])
const loading = ref(false)
const submitting = ref(false)
const searchQuery = ref('')
const availablePlans = ref<SubscriptionPlan[]>([])

const filters = reactive({
  status: '',
  promotion_type: ''
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
})
const sortState = reactive({
  sort_by: 'priority',
  sort_order: 'desc' as 'asc' | 'desc'
})

// Dialogs
const showFormDialog = ref(false)
const showDeleteDialog = ref(false)
const showUsagesDialog = ref(false)

const editingPromotion = ref<Promotion | null>(null)
const deletingPromotion = ref<Promotion | null>(null)

// Usages
const usages = ref<PromotionUsage[]>([])
const usagesLoading = ref(false)
const currentViewingPromotion = ref<Promotion | null>(null)
const usagesPage = ref(1)
const usagesPageSize = ref(20)
const usagesTotal = ref(0)

// Form Data
interface PlanRuleForm {
  plan_id: number
  discount_mode: string
  discount_rate: number | null
  discount_amount: number | null
  min_price_floor: number
  max_uses_per_user: number
}

const defaultFormData = () => ({
  name: '',
  description: '',
  promotion_type: 'recharge' as 'recharge' | 'subscription',
  discount_mode: 'reduce_pay',
  recharge_rate: 0.8 as number | null,
  recharge_bonus_rate: 0.2 as number | null,
  max_uses_per_user: 0,
  starts_at_str: '',
  ends_at_str: '',
  status: 'active' as 'active' | 'disabled',
  priority: 0,
  plan_rules: [] as PlanRuleForm[]
})

const formData = reactive(defaultFormData())

// Options
const filterStatusOptions = computed(() => [
  { value: '', label: t('admin.promotions.allStatus') },
  { value: 'active', label: t('admin.promotions.statusActive') },
  { value: 'disabled', label: t('admin.promotions.statusDisabled') }
])

const filterTypeOptions = computed(() => [
  { value: '', label: t('admin.promotions.allTypes') },
  { value: 'recharge', label: t('admin.promotions.typeRecharge') },
  { value: 'subscription', label: t('admin.promotions.typeSubscription') }
])

const promotionTypeOptions = computed(() => [
  { value: 'recharge', label: t('admin.promotions.typeRecharge') },
  { value: 'subscription', label: t('admin.promotions.typeSubscription') }
])

const rechargeDiscountModeOptions = computed(() => [
  { value: 'reduce_pay', label: t('admin.promotions.reducePay') },
  { value: 'bonus_credit', label: t('admin.promotions.bonusCredit') }
])

const planDiscountModeOptions = computed(() => [
  { value: 'rate', label: t('admin.promotions.discountByRate') },
  { value: 'amount', label: t('admin.promotions.discountByAmount') }
])

const statusOptions = computed(() => [
  { value: 'active', label: t('admin.promotions.statusActive') },
  { value: 'disabled', label: t('admin.promotions.statusDisabled') }
])

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.promotions.columns.name') },
  { key: 'promotion_type', label: t('admin.promotions.columns.type') },
  { key: 'discount_info', label: t('admin.promotions.columns.discount') },
  { key: 'max_uses_per_user', label: t('admin.promotions.columns.maxUses'), sortable: true },
  { key: 'time_range', label: t('admin.promotions.columns.timeRange') },
  { key: 'status', label: t('admin.promotions.columns.status'), sortable: true },
  { key: 'priority', label: t('admin.promotions.columns.priority'), sortable: true },
  { key: 'actions', label: t('admin.promotions.columns.actions') }
])

// Helpers
const getStatusClass = (status: string, row: Promotion) => {
  if (row.ends_at && new Date(row.ends_at) < new Date()) {
    return 'badge-danger'
  }
  return status === 'active' ? 'badge-success' : 'badge-gray'
}

const getStatusLabel = (status: string, row: Promotion) => {
  if (row.ends_at && new Date(row.ends_at) < new Date()) {
    return t('admin.promotions.statusExpired')
  }
  return status === 'active' ? t('admin.promotions.statusActive') : t('admin.promotions.statusDisabled')
}

// API calls
let abortController: AbortController | null = null

const loadPromotions = async () => {
  if (abortController) {
    abortController.abort()
  }
  const currentController = new AbortController()
  abortController = currentController
  loading.value = true

  try {
    const response = await adminAPI.promotions.list(
      pagination.page,
      pagination.page_size,
      {
        status: filters.status || undefined,
        promotion_type: filters.promotion_type || undefined,
        search: searchQuery.value || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      { signal: currentController.signal }
    )
    if (currentController.signal.aborted || abortController !== currentController) return

    promotions.value = response.items
    pagination.total = response.total
  } catch (error: any) {
    if (
      currentController.signal.aborted ||
      abortController !== currentController ||
      error?.name === 'AbortError' ||
      error?.code === 'ERR_CANCELED'
    ) {
      return
    }
    appStore.showError(t('admin.promotions.failedToLoad'))
    console.error('Error loading promotions:', error)
  } finally {
    if (abortController === currentController) {
      loading.value = false
      abortController = null
    }
  }
}

const loadPlans = async () => {
  try {
    const resp = await adminAPI.payment.getPlans()
    availablePlans.value = resp.data
  } catch (error) {
    console.error('Error loading plans:', error)
  }
}

let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadPromotions()
  }, 300)
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadPromotions()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadPromotions()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadPromotions()
}

// Form Management
const resetForm = () => {
  const defaults = defaultFormData()
  Object.assign(formData, defaults)
}

const openCreateDialog = () => {
  editingPromotion.value = null
  resetForm()
  showFormDialog.value = true
  loadPlans()
}

const handleEdit = (promotion: Promotion) => {
  editingPromotion.value = promotion
  formData.name = promotion.name
  formData.description = promotion.description
  formData.promotion_type = promotion.promotion_type
  formData.discount_mode = promotion.discount_mode || 'reduce_pay'
  formData.recharge_rate = promotion.recharge_rate
  formData.recharge_bonus_rate = promotion.recharge_bonus_rate != null ? promotion.recharge_bonus_rate - 1 : 0.2
  formData.max_uses_per_user = promotion.max_uses_per_user
  formData.starts_at_str = promotion.starts_at ? new Date(promotion.starts_at).toISOString().slice(0, 16) : ''
  formData.ends_at_str = promotion.ends_at ? new Date(promotion.ends_at).toISOString().slice(0, 16) : ''
  formData.status = promotion.status
  formData.priority = promotion.priority
  formData.plan_rules = (promotion.plan_rules || []).map(r => ({
    plan_id: r.plan_id,
    discount_mode: r.discount_mode,
    discount_rate: r.discount_rate,
    discount_amount: r.discount_amount,
    min_price_floor: r.min_price_floor,
    max_uses_per_user: r.max_uses_per_user
  }))
  showFormDialog.value = true
  loadPlans()
}

const closeFormDialog = () => {
  showFormDialog.value = false
  editingPromotion.value = null
}

const addPlanRule = () => {
  formData.plan_rules.push({
    plan_id: 0,
    discount_mode: 'rate',
    discount_rate: 0.8,
    discount_amount: null,
    min_price_floor: 0,
    max_uses_per_user: 0
  })
}

const removePlanRule = (index: number) => {
  formData.plan_rules.splice(index, 1)
}

const buildPlanRules = (): CreatePromotionPlanRuleRequest[] => {
  return formData.plan_rules
    .filter(r => r.plan_id > 0)
    .map(r => ({
      plan_id: r.plan_id,
      discount_mode: r.discount_mode as 'rate' | 'amount',
      discount_rate: r.discount_mode === 'rate' ? r.discount_rate : undefined,
      discount_amount: r.discount_mode === 'amount' ? r.discount_amount : undefined,
      min_price_floor: r.min_price_floor,
      max_uses_per_user: r.max_uses_per_user
    }))
}

const handleSubmit = async () => {
  submitting.value = true
  try {
    if (editingPromotion.value) {
      const payload: UpdatePromotionRequest = {
        name: formData.name,
        description: formData.description,
        discount_mode: formData.discount_mode,
        recharge_rate: formData.discount_mode === 'reduce_pay' ? formData.recharge_rate : undefined,
        recharge_bonus_rate: formData.discount_mode === 'bonus_credit' && formData.recharge_bonus_rate != null ? formData.recharge_bonus_rate + 1 : undefined,
        max_uses_per_user: formData.max_uses_per_user,
        starts_at: formData.starts_at_str ? Math.floor(new Date(formData.starts_at_str).getTime() / 1000) : undefined,
        clear_starts_at: !formData.starts_at_str && !!editingPromotion.value.starts_at,
        ends_at: formData.ends_at_str ? Math.floor(new Date(formData.ends_at_str).getTime() / 1000) : undefined,
        clear_ends_at: !formData.ends_at_str && !!editingPromotion.value.ends_at,
        status: formData.status,
        priority: formData.priority,
        plan_rules: formData.promotion_type === 'subscription' ? buildPlanRules() : undefined
      }
      await adminAPI.promotions.update(editingPromotion.value.id, payload)
      appStore.showSuccess(t('admin.promotions.updated'))
    } else {
      const payload: CreatePromotionRequest = {
        name: formData.name,
        description: formData.description || undefined,
        promotion_type: formData.promotion_type,
        discount_mode: formData.promotion_type === 'recharge' ? formData.discount_mode : undefined,
        recharge_rate: formData.discount_mode === 'reduce_pay' ? formData.recharge_rate : undefined,
        recharge_bonus_rate: formData.discount_mode === 'bonus_credit' && formData.recharge_bonus_rate != null ? formData.recharge_bonus_rate + 1 : undefined,
        max_uses_per_user: formData.max_uses_per_user,
        starts_at: formData.starts_at_str ? Math.floor(new Date(formData.starts_at_str).getTime() / 1000) : undefined,
        ends_at: formData.ends_at_str ? Math.floor(new Date(formData.ends_at_str).getTime() / 1000) : undefined,
        status: formData.status,
        priority: formData.priority,
        plan_rules: formData.promotion_type === 'subscription' ? buildPlanRules() : undefined
      }
      await adminAPI.promotions.create(payload)
      appStore.showSuccess(t('admin.promotions.created'))
    }
    closeFormDialog()
    loadPromotions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promotions.failedToSave'))
  } finally {
    submitting.value = false
  }
}

// Toggle Status
const handleToggleStatus = async (promotion: Promotion) => {
  const newStatus = promotion.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.promotions.setStatus(promotion.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active'
        ? t('admin.promotions.enabled')
        : t('admin.promotions.disabled')
    )
    loadPromotions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promotions.failedToToggle'))
  }
}

// Delete
const handleDelete = (promotion: Promotion) => {
  deletingPromotion.value = promotion
  showDeleteDialog.value = true
}

const confirmDelete = async () => {
  if (!deletingPromotion.value) return
  try {
    await adminAPI.promotions.delete(deletingPromotion.value.id)
    appStore.showSuccess(t('admin.promotions.deleted'))
    showDeleteDialog.value = false
    deletingPromotion.value = null
    loadPromotions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promotions.failedToDelete'))
  }
}

// Usages
const handleViewUsages = async (promotion: Promotion) => {
  currentViewingPromotion.value = promotion
  showUsagesDialog.value = true
  usagesPage.value = 1
  await loadUsages()
}

const loadUsages = async () => {
  if (!currentViewingPromotion.value) return
  usagesLoading.value = true
  usages.value = []
  try {
    const response = await adminAPI.promotions.getUsages(
      currentViewingPromotion.value.id,
      usagesPage.value,
      usagesPageSize.value
    )
    usages.value = response.items
    usagesTotal.value = response.total
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.promotions.failedToLoadUsages'))
  } finally {
    usagesLoading.value = false
  }
}

const handleUsagesPageChange = (page: number) => {
  usagesPage.value = page
  loadUsages()
}

onMounted(() => {
  loadPromotions()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
})
</script>

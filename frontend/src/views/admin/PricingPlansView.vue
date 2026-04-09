<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex justify-end gap-3">
          <button @click="loadGroups" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
            <svg
              :class="['h-5 w-5', loading ? 'animate-spin' : '']"
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
          </button>
          <button @click="openCreateGroup" class="btn btn-primary">
            <svg class="mr-2 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
            </svg>
            {{ t('admin.pricingPlans.createGroup') }}
          </button>
        </div>
      </template>

      <template #table>
        <div class="p-6">
          <div v-if="loading" class="space-y-4">
            <div v-for="i in 3" :key="i" class="h-24 animate-pulse rounded-2xl bg-gray-100 dark:bg-dark-700"></div>
          </div>

          <EmptyState
            v-else-if="!groups.length"
            :title="t('admin.pricingPlans.emptyTitle')"
            :description="t('admin.pricingPlans.emptyDesc')"
            :action-text="t('admin.pricingPlans.createGroup')"
            @action="openCreateGroup"
          />

          <div v-else class="space-y-6">
            <div
              v-for="group in groups"
              :key="group.id"
              class="rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900"
            >
              <div class="flex flex-wrap items-start justify-between gap-4 p-6">
                <div class="min-w-0 space-y-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
                      {{ group.name }}
                    </h3>
                    <span :class="['badge', group.status === 'active' ? 'badge-success' : 'badge-danger']">
                      {{ group.status }}
                    </span>
                    <span class="text-xs text-gray-400 dark:text-dark-500">#{{ group.id }}</span>
                    <span class="text-xs text-gray-400 dark:text-dark-500"
                      >{{ t('admin.pricingPlans.sortOrder') }}: {{ group.sort_order }}</span
                    >
                  </div>
                  <p v-if="group.description" class="text-sm text-gray-600 dark:text-dark-300">
                    {{ group.description }}
                  </p>
                </div>

                <div class="flex flex-wrap gap-2">
                  <button class="btn btn-secondary" @click="openCreatePlan(group)">
                    {{ t('admin.pricingPlans.createPlan') }}
                  </button>
                  <button class="btn btn-secondary" @click="openEditGroup(group)">{{ t('common.edit') }}</button>
                  <button class="btn btn-danger" @click="askDeleteGroup(group)">{{ t('common.delete') }}</button>
                </div>
              </div>

              <div class="px-6 pb-6">
                <DataTable :columns="planColumns" :data="group.plans || []" :loading="false">
                  <template #cell-name="{ row }">
                    <div class="space-y-1">
                      <div class="flex items-center gap-2">
                        <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
                        <span v-if="row.is_featured" class="badge badge-primary">{{ t('common.recommended') }}</span>
                      </div>
                      <p v-if="row.description" class="max-w-xl truncate text-xs text-gray-500 dark:text-dark-400">
                        {{ row.description }}
                      </p>
                    </div>
                  </template>

                  <template #cell-price="{ row }">
                    <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatPlanPrice(row) }}</span>
                  </template>

                  <template #cell-status="{ value }">
                    <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">
                      {{ value }}
                    </span>
                  </template>

                  <template #cell-actions="{ row }">
                    <div class="flex items-center gap-1">
                      <button
                        @click="openEditPlan(group, row)"
                        class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                      >
                        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10"
                          />
                        </svg>
                        <span class="text-xs">{{ t('common.edit') }}</span>
                      </button>

                      <button
                        @click="askDeletePlan(group, row)"
                        class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      >
                        <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                          <path
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0"
                          />
                        </svg>
                        <span class="text-xs">{{ t('common.delete') }}</span>
                      </button>
                    </div>
                  </template>

                  <template #empty>
                    <div class="py-10 text-center text-sm text-gray-500 dark:text-dark-400">
                      {{ t('admin.pricingPlans.noPlans') }}
                      <button class="ml-2 text-primary-600 hover:underline dark:text-primary-400" @click="openCreatePlan(group)">
                        {{ t('admin.pricingPlans.createPlan') }}
                      </button>
                    </div>
                  </template>
                </DataTable>
              </div>
            </div>
          </div>
        </div>
      </template>
    </TablePageLayout>

    <!-- Group Dialog -->
    <BaseDialog
      :show="showGroupDialog"
      :title="groupDialogMode === 'create' ? t('admin.pricingPlans.createGroup') : t('admin.pricingPlans.editGroup')"
      width="normal"
      @close="closeGroupDialog"
    >
      <form id="pricing-group-form" class="space-y-5" @submit.prevent="submitGroup">
        <div>
          <label class="input-label">{{ t('admin.pricingPlans.groupForm.name') }}</label>
          <input v-model="groupForm.name" type="text" class="input" required />
        </div>

        <div>
          <label class="input-label">{{ t('admin.pricingPlans.groupForm.description') }}</label>
          <textarea v-model="groupForm.description" rows="3" class="textarea"></textarea>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.pricingPlans.groupForm.sortOrder') }}</label>
            <input v-model.number="groupForm.sort_order" type="number" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.pricingPlans.groupForm.status') }}</label>
            <Select v-model="groupForm.status" :options="statusOptions" class="w-full" />
          </div>
        </div>

        <div class="flex justify-end gap-3 pt-2">
          <button type="button" class="btn btn-secondary" @click="closeGroupDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="submittingGroup">
            {{ submittingGroup ? t('common.saving') : groupDialogMode === 'create' ? t('common.create') : t('common.update') }}
          </button>
        </div>
      </form>
    </BaseDialog>

    <!-- Plan Dialog -->
    <BaseDialog
      :show="showPlanDialog"
      :title="planDialogMode === 'create' ? t('admin.pricingPlans.createPlan') : t('admin.pricingPlans.editPlan')"
      width="normal"
      @close="closePlanDialog"
    >
      <form id="pricing-plan-form" class="space-y-5" @submit.prevent="submitPlan">
        <div>
          <label class="input-label">{{ t('admin.pricingPlans.planForm.group') }}</label>
          <Select v-model="planForm.group_id" :options="groupOptions" class="w-full" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.pricingPlans.planForm.name') }}</label>
          <input v-model="planForm.name" type="text" class="input" required />
        </div>

        <div>
          <label class="input-label">{{ t('admin.pricingPlans.planForm.description') }}</label>
          <textarea v-model="planForm.description" rows="3" class="textarea"></textarea>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div class="sm:col-span-2">
            <label class="input-label">{{ t('admin.pricingPlans.planForm.iconUrl') }}</label>
            <input v-model="planForm.icon_url" type="text" class="input" :placeholder="t('admin.pricingPlans.planForm.iconUrlHint')" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.pricingPlans.planForm.badgeText') }}</label>
            <input v-model="planForm.badge_text" type="text" class="input" :placeholder="t('admin.pricingPlans.planForm.badgeTextHint')" />
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.pricingPlans.planForm.priceText') }}</label>
          <input v-model="planForm.price_text" type="text" class="input" :placeholder="t('admin.pricingPlans.planForm.priceTextHint')" />
        </div>

        <div>
          <label class="input-label">{{ t('admin.pricingPlans.planForm.tagline') }}</label>
          <input v-model="planForm.tagline" type="text" class="input" :placeholder="t('admin.pricingPlans.planForm.taglineHint')" />
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div>
            <label class="input-label">{{ t('admin.pricingPlans.planForm.priceAmount') }}</label>
            <input v-model="planForm.price_amount" type="text" inputmode="decimal" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.pricingPlans.planForm.priceCurrency') }}</label>
            <Select v-model="planForm.price_currency" :options="currencyOptions" class="w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.pricingPlans.planForm.pricePeriod') }}</label>
            <Select v-model="planForm.price_period" :options="periodOptions" class="w-full" />
          </div>
        </div>

        <div>
          <label class="input-label">{{ t('admin.pricingPlans.planForm.features') }}</label>
          <textarea v-model="planForm.featuresText" rows="5" class="textarea" :placeholder="t('admin.pricingPlans.planForm.featuresHint')"></textarea>
        </div>

        <div>
          <div class="flex items-end justify-between gap-3">
            <label class="input-label mb-0">{{ t('admin.pricingPlans.planForm.purchaseEntries') }}</label>
            <button
              type="button"
              class="flex h-9 w-9 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-600 shadow-sm transition-colors hover:bg-gray-50 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('admin.pricingPlans.planForm.addPurchaseEntry')"
              @click="addPurchaseEntry"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
              </svg>
            </button>
          </div>

          <div class="mt-2 space-y-3">
            <div
              v-for="(entry, idx) in planForm.purchaseEntries"
              :key="entry.id"
              :class="[
                'flex items-start gap-3 rounded-2xl border bg-white/60 p-3 shadow-sm transition-colors dark:bg-dark-900/40',
                purchaseEntryDragOverIndex === idx
                  ? 'border-primary-300 ring-2 ring-primary-500/20 dark:border-primary-500/40'
                  : 'border-gray-200 dark:border-dark-700'
              ]"
              @dragenter.prevent="onPurchaseEntryDragEnter(idx)"
              @dragover.prevent="onPurchaseEntryDragOver(idx)"
              @drop.prevent="onPurchaseEntryDrop(idx)"
            >
              <div class="pt-1">
                <button
                  type="button"
                  draggable="true"
                  class="flex h-9 w-9 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-400 shadow-sm transition-colors hover:bg-gray-50 hover:text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-dark-100"
                  :title="t('admin.pricingPlans.planForm.dragToSort')"
                  @dragstart="onPurchaseEntryDragStart(idx)"
                  @dragend="onPurchaseEntryDragEnd"
                >
                  <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                    <path
                      d="M7 4a1 1 0 11-2 0 1 1 0 012 0zm8 0a1 1 0 11-2 0 1 1 0 012 0zM7 10a1 1 0 11-2 0 1 1 0 012 0zm8 0a1 1 0 11-2 0 1 1 0 012 0zM7 16a1 1 0 11-2 0 1 1 0 012 0zm8 0a1 1 0 11-2 0 1 1 0 012 0z"
                    />
                  </svg>
                </button>
              </div>

              <div class="grid flex-1 grid-cols-1 gap-3 sm:grid-cols-5">
                <input
                  v-model="entry.label"
                  type="text"
                  class="input sm:col-span-2"
                  :placeholder="t('admin.pricingPlans.planForm.purchaseEntryLabelPlaceholder')"
                />
                <input
                  v-model="entry.value"
                  type="text"
                  class="input sm:col-span-3"
                  :placeholder="t('admin.pricingPlans.planForm.purchaseEntryValuePlaceholder')"
                />
              </div>

              <div class="pt-1">
                <button
                  type="button"
                  class="flex h-9 w-9 items-center justify-center rounded-xl border border-gray-200 bg-white text-gray-400 shadow-sm transition-colors hover:border-red-200 hover:bg-red-50 hover:text-red-600 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-red-900/40 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                  :title="t('admin.pricingPlans.planForm.removePurchaseEntry')"
                  @click="removePurchaseEntry(idx)"
                >
                  <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14" />
                  </svg>
                </button>
              </div>
            </div>
          </div>

          <p class="input-hint">{{ t('admin.pricingPlans.planForm.purchaseEntriesHint') }}</p>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-4">
          <div class="flex items-center gap-3">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.pricingPlans.planForm.featured') }}</span>
            <Toggle v-model="planForm.is_featured" />
          </div>

          <div class="flex flex-wrap gap-4">
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.pricingPlans.planForm.sortOrder') }}</span>
              <input v-model.number="planForm.sort_order" type="number" class="input w-24" />
            </div>
            <div class="flex items-center gap-2">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.pricingPlans.planForm.status') }}</span>
              <Select v-model="planForm.status" :options="statusOptions" class="w-32" />
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-3 pt-2">
          <button type="button" class="btn btn-secondary" @click="closePlanDialog">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="submittingPlan || !planForm.group_id">
            {{ submittingPlan ? t('common.saving') : planDialogMode === 'create' ? t('common.create') : t('common.update') }}
          </button>
        </div>
      </form>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="deleteDialogTitle"
      :message="deleteDialogMessage"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { PricingPlan, PricingPlanContactMethod, PricingPlanGroup } from '@/types'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'

const { t } = useI18n()
const appStore = useAppStore()

type PurchaseEntryDraft = {
  id: string
  label: string
  value: string
}

const loading = ref(false)
const groups = ref<PricingPlanGroup[]>([])

const statusOptions = computed(() => [
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') }
])

const currencyOptions = computed(() => [
  { value: 'CNY', label: t('common.currency.cny') },
  { value: 'USD', label: t('common.currency.usd') },
  { value: 'EUR', label: t('common.currency.eur') },
  { value: 'GBP', label: t('common.currency.gbp') }
])

const periodOptions = computed(() => [
  { value: 'day', label: t('common.period.day') },
  { value: 'week', label: t('common.period.week') },
  { value: 'month', label: t('common.period.month') },
  { value: 'year', label: t('common.period.year') },
  { value: 'once', label: t('common.period.once') }
])

const normalizePricePeriod = (value?: string): string => {
  const period = (value || '').trim().toLowerCase()
  if (!period) return 'month'
  if (period === 'one_time') return 'once'
  return period
}

const groupOptions = computed(() => groups.value.map((g) => ({ value: g.id, label: g.name })))

const planColumns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.pricingPlans.columns.name'), sortable: true },
  { key: 'price', label: t('admin.pricingPlans.columns.price'), sortable: false },
  { key: 'sort_order', label: t('admin.pricingPlans.columns.sortOrder'), sortable: true },
  { key: 'status', label: t('admin.pricingPlans.columns.status'), sortable: true },
  { key: 'actions', label: t('admin.pricingPlans.columns.actions'), sortable: false }
])

function currencySymbol(currency: string): string {
  const c = (currency || '').toUpperCase()
  if (c === 'USD') return '$'
  if (c === 'CNY' || c === 'RMB') return '¥'
  if (c === 'EUR') return '€'
  if (c === 'GBP') return '£'
  return c ? `${c} ` : ''
}

function formatAmount(amount: number): string {
  if (Number.isInteger(amount)) return String(amount)
  const fixed = amount.toFixed(2)
  return fixed.replace(/\\.00$/, '').replace(/(\\.\\d)0$/, '$1')
}

function formatPlanPrice(plan: PricingPlan): string {
  if (plan.price_text) return plan.price_text
  if (plan.price_amount === null || plan.price_amount === undefined) return '—'
  const suffix = plan.price_period ? ` / ${plan.price_period}` : ''
  return `${currencySymbol(plan.price_currency)}${formatAmount(plan.price_amount)}${suffix}`
}

async function loadGroups() {
  loading.value = true
  try {
    groups.value = await adminAPI.pricingPlans.listPlanGroups()
  } catch (error) {
    appStore.showError(t('admin.pricingPlans.failedToLoad'))
    console.error('Failed to load pricing plans:', error)
  } finally {
    loading.value = false
  }
}

onMounted(loadGroups)

// Group dialog
const showGroupDialog = ref(false)
const groupDialogMode = ref<'create' | 'edit'>('create')
const submittingGroup = ref(false)
const groupForm = reactive({
  id: null as number | null,
  name: '',
  description: '',
  sort_order: 0,
  status: 'active' as 'active' | 'inactive'
})

function openCreateGroup() {
  groupDialogMode.value = 'create'
  groupForm.id = null
  groupForm.name = ''
  groupForm.description = ''
  groupForm.sort_order = 0
  groupForm.status = 'active'
  showGroupDialog.value = true
}

function openEditGroup(group: PricingPlanGroup) {
  groupDialogMode.value = 'edit'
  groupForm.id = group.id
  groupForm.name = group.name
  groupForm.description = group.description || ''
  groupForm.sort_order = group.sort_order
  groupForm.status = group.status
  showGroupDialog.value = true
}

function closeGroupDialog() {
  showGroupDialog.value = false
}

async function submitGroup() {
  submittingGroup.value = true
  try {
    if (groupDialogMode.value === 'create') {
      await adminAPI.pricingPlans.createPlanGroup({
        name: groupForm.name,
        description: groupForm.description.trim() ? groupForm.description.trim() : null,
        sort_order: groupForm.sort_order,
        status: groupForm.status
      })
      appStore.showSuccess(t('common.success'))
    } else if (groupForm.id) {
      await adminAPI.pricingPlans.updatePlanGroup(groupForm.id, {
        name: groupForm.name,
        description: groupForm.description,
        sort_order: groupForm.sort_order,
        status: groupForm.status
      })
      appStore.showSuccess(t('common.success'))
    }
    closeGroupDialog()
    await loadGroups()
  } catch (error) {
    appStore.showError(t('common.error'))
    console.error('Failed to save group:', error)
  } finally {
    submittingGroup.value = false
  }
}

// Plan dialog
const showPlanDialog = ref(false)
const planDialogMode = ref<'create' | 'edit'>('create')
const submittingPlan = ref(false)
const planForm = reactive({
  id: null as number | null,
  group_id: null as number | null,
  name: '',
  description: '',
  icon_url: '',
  badge_text: '',
  tagline: '',
  price_text: '',
  price_amount: '',
  price_currency: 'CNY',
  price_period: 'month',
  featuresText: '',
  purchaseEntries: [] as PurchaseEntryDraft[],
  is_featured: false,
  sort_order: 0,
  status: 'active' as 'active' | 'inactive'
})

const purchaseEntryDraggingIndex = ref<number | null>(null)
const purchaseEntryDragOverIndex = ref<number | null>(null)

function createUID(): string {
  return `${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`
}

function createEmptyPurchaseEntry(): PurchaseEntryDraft {
  return { id: createUID(), label: '', value: '' }
}

function ensurePurchaseEntries() {
  if (!planForm.purchaseEntries.length) {
    planForm.purchaseEntries = [createEmptyPurchaseEntry()]
  }
}

function addPurchaseEntry() {
  planForm.purchaseEntries.push(createEmptyPurchaseEntry())
}

function removePurchaseEntry(index: number) {
  planForm.purchaseEntries.splice(index, 1)
}

function onPurchaseEntryDragStart(index: number) {
  purchaseEntryDraggingIndex.value = index
}

function onPurchaseEntryDragEnd() {
  purchaseEntryDraggingIndex.value = null
  purchaseEntryDragOverIndex.value = null
}

function onPurchaseEntryDragEnter(index: number) {
  if (purchaseEntryDraggingIndex.value === null) return
  purchaseEntryDragOverIndex.value = index
}

function onPurchaseEntryDragOver(index: number) {
  if (purchaseEntryDraggingIndex.value === null) return
  purchaseEntryDragOverIndex.value = index
}

function onPurchaseEntryDrop(targetIndex: number) {
  const fromIndex = purchaseEntryDraggingIndex.value
  if (fromIndex === null) return
  if (fromIndex === targetIndex) {
    onPurchaseEntryDragEnd()
    return
  }

  const list = planForm.purchaseEntries
  if (fromIndex < 0 || fromIndex >= list.length) {
    onPurchaseEntryDragEnd()
    return
  }
  if (targetIndex < 0 || targetIndex >= list.length) {
    onPurchaseEntryDragEnd()
    return
  }

  const originalLen = list.length
  const isTargetLast = targetIndex === originalLen - 1

  const [moved] = list.splice(fromIndex, 1)
  if (!moved) {
    onPurchaseEntryDragEnd()
    return
  }

  if (isTargetLast) {
    list.splice(list.length, 0, moved)
    onPurchaseEntryDragEnd()
    return
  }

  const insertIndex = fromIndex < targetIndex ? targetIndex - 1 : targetIndex
  list.splice(insertIndex, 0, moved)
  onPurchaseEntryDragEnd()
}

function openCreatePlan(group: PricingPlanGroup) {
  planDialogMode.value = 'create'
  planForm.id = null
  planForm.group_id = group.id
  planForm.name = ''
  planForm.description = ''
  planForm.icon_url = ''
  planForm.badge_text = ''
  planForm.tagline = ''
  planForm.price_text = ''
  planForm.price_amount = ''
  planForm.price_currency = 'CNY'
  planForm.price_period = 'month'
  planForm.featuresText = ''
  planForm.purchaseEntries = [createEmptyPurchaseEntry()]
  planForm.is_featured = false
  planForm.sort_order = 0
  planForm.status = 'active'
  showPlanDialog.value = true
}

function openEditPlan(group: PricingPlanGroup, plan: PricingPlan) {
  planDialogMode.value = 'edit'
  planForm.id = plan.id
  planForm.group_id = plan.group_id || group.id
  planForm.name = plan.name
  planForm.description = plan.description || ''
  planForm.icon_url = plan.icon_url || ''
  planForm.badge_text = plan.badge_text || ''
  planForm.tagline = plan.tagline || ''
  planForm.price_text = plan.price_text || ''
  planForm.price_amount = plan.price_amount === null || plan.price_amount === undefined ? '' : String(plan.price_amount)
  planForm.price_currency = plan.price_currency || 'CNY'
  planForm.price_period = normalizePricePeriod(plan.price_period)
  planForm.featuresText = (plan.features || []).join('\n')
  planForm.purchaseEntries = (plan.contact_methods || []).map((m) => ({
    id: createUID(),
    label: (m?.type || '').trim(),
    value: (m?.value || '').trim()
  }))
  ensurePurchaseEntries()
  planForm.is_featured = !!plan.is_featured
  planForm.sort_order = plan.sort_order || 0
  planForm.status = plan.status
  showPlanDialog.value = true
}

function closePlanDialog() {
  showPlanDialog.value = false
}

function parseFeatures(text: string): string[] {
  return (text || '')
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
}

function buildPurchaseEntries(entries: PurchaseEntryDraft[]): PricingPlanContactMethod[] {
  return (entries || [])
    .map((e) => {
      const label = (e?.label || '').trim()
      const value = (e?.value || '').trim()
      if (!label || !value) return null
      return { type: label, value }
    })
    .filter((v): v is PricingPlanContactMethod => !!v)
}

function parsePriceAmount(text: string): number | undefined {
  const trimmed = (text || '').trim()
  if (!trimmed) return undefined
  const v = Number(trimmed)
  if (!Number.isFinite(v)) return undefined
  return v
}

async function submitPlan() {
  if (!planForm.group_id) return
  submittingPlan.value = true
  try {
    const features = parseFeatures(planForm.featuresText)
    const purchaseEntries = buildPurchaseEntries(planForm.purchaseEntries)
    const priceAmount = parsePriceAmount(planForm.price_amount)

    if (planDialogMode.value === 'create') {
      await adminAPI.pricingPlans.createPlan({
        group_id: planForm.group_id,
        name: planForm.name,
        description: planForm.description.trim() ? planForm.description.trim() : null,
        icon_url: planForm.icon_url.trim() ? planForm.icon_url.trim() : null,
        badge_text: planForm.badge_text.trim() ? planForm.badge_text.trim() : null,
        tagline: planForm.tagline.trim() ? planForm.tagline.trim() : null,
        price_amount: priceAmount,
        price_currency: planForm.price_currency,
        price_period: planForm.price_period,
        price_text: planForm.price_text.trim() ? planForm.price_text.trim() : null,
        features,
        contact_methods: purchaseEntries,
        is_featured: planForm.is_featured,
        sort_order: planForm.sort_order,
        status: planForm.status
      })
      appStore.showSuccess(t('common.success'))
    } else if (planForm.id) {
      await adminAPI.pricingPlans.updatePlan(planForm.id, {
        group_id: planForm.group_id,
        name: planForm.name,
        description: planForm.description,
        icon_url: planForm.icon_url,
        badge_text: planForm.badge_text,
        tagline: planForm.tagline,
        price_amount: priceAmount,
        price_currency: planForm.price_currency,
        price_period: planForm.price_period,
        price_text: planForm.price_text,
        features,
        contact_methods: purchaseEntries,
        is_featured: planForm.is_featured,
        sort_order: planForm.sort_order,
        status: planForm.status
      })
      appStore.showSuccess(t('common.success'))
    }

    closePlanDialog()
    await loadGroups()
  } catch (error) {
    appStore.showError(t('common.error'))
    console.error('Failed to save plan:', error)
  } finally {
    submittingPlan.value = false
  }
}

// Delete confirmation
const showDeleteDialog = ref(false)
const deleteTarget = ref<
  | { type: 'group'; id: number; name: string }
  | { type: 'plan'; id: number; name: string; groupName: string }
  | null
>(null)

const deleteDialogTitle = computed(() => {
  if (!deleteTarget.value) return ''
  return deleteTarget.value.type === 'group'
    ? t('admin.pricingPlans.deleteGroup')
    : t('admin.pricingPlans.deletePlan')
})

const deleteDialogMessage = computed(() => {
  if (!deleteTarget.value) return ''
  if (deleteTarget.value.type === 'group') {
    return t('admin.pricingPlans.deleteGroupConfirm', { name: deleteTarget.value.name })
  }
  return t('admin.pricingPlans.deletePlanConfirm', {
    name: deleteTarget.value.name,
    group: deleteTarget.value.groupName
  })
})

function askDeleteGroup(group: PricingPlanGroup) {
  deleteTarget.value = { type: 'group', id: group.id, name: group.name }
  showDeleteDialog.value = true
}

function askDeletePlan(group: PricingPlanGroup, plan: PricingPlan) {
  deleteTarget.value = { type: 'plan', id: plan.id, name: plan.name, groupName: group.name }
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  try {
    if (deleteTarget.value.type === 'group') {
      await adminAPI.pricingPlans.deletePlanGroup(deleteTarget.value.id)
    } else {
      await adminAPI.pricingPlans.deletePlan(deleteTarget.value.id)
    }
    appStore.showSuccess(t('common.success'))
    showDeleteDialog.value = false
    deleteTarget.value = null
    await loadGroups()
  } catch (error) {
    appStore.showError(t('common.error'))
    console.error('Failed to delete:', error)
  }
}
</script>

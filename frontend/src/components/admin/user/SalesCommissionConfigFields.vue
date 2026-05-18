<template>
  <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
    <div class="flex items-start justify-between gap-4">
      <div>
        <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
          {{ t('admin.users.sales.title') }}
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.users.sales.hint') }}
        </p>
      </div>
      <input
        v-model="form.isSales"
        type="checkbox"
        class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
      />
    </div>

    <div v-if="form.isSales" class="mt-4 space-y-4">
      <div>
        <label class="input-label">{{ t('admin.users.sales.mode') }}</label>
        <div class="grid grid-cols-2 gap-2">
          <button
            type="button"
            data-testid="sales-mode-fixed"
            :class="modeButtonClass(form.salesCommissionMode === 'fixed')"
            @click="setMode('fixed')"
          >
            {{ t('admin.users.sales.modeFixed') }}
          </button>
          <button
            type="button"
            data-testid="sales-mode-tiered"
            :class="modeButtonClass(form.salesCommissionMode === 'tiered')"
            @click="setMode('tiered')"
          >
            {{ t('admin.users.sales.modeTiered') }}
          </button>
        </div>
      </div>

      <div v-if="form.salesCommissionMode === 'fixed'">
        <label class="input-label">{{ t('admin.users.sales.commissionRate') }}</label>
        <input
          v-model.number="form.salesCommissionRate"
          name="sales_commission_rate"
          type="number"
          min="0.01"
          max="100"
          step="0.01"
          class="input"
        />
      </div>

      <div v-else class="space-y-4">
        <div>
          <label class="input-label">
            {{ t('admin.users.sales.minMonthlySales') }}
          </label>
          <input
            v-model.number="form.salesCommissionMinMonthlySales"
            name="sales_commission_min_monthly_sales"
            type="number"
            min="0"
            step="0.01"
            class="input"
          />
        </div>

        <div class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <div>
              <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
                {{ t('admin.users.sales.tiers') }}
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ t('admin.users.sales.tierHint') }}
              </p>
            </div>
            <button
              type="button"
              data-testid="sales-add-tier"
              class="btn btn-secondary btn-sm"
              @click="addTier"
            >
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.users.sales.addTier') }}
            </button>
          </div>

          <div
            v-if="!form.salesCommissionTiers.length"
            class="rounded-lg border border-dashed border-gray-300 px-4 py-5 text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
          >
            {{ t('admin.users.sales.noTiers') }}
          </div>

          <div
            v-for="(tier, index) in form.salesCommissionTiers"
            :key="index"
            class="rounded-lg border border-gray-200 p-3 dark:border-dark-700"
          >
            <div class="flex items-center justify-between gap-3">
              <div class="text-sm font-medium text-gray-900 dark:text-gray-100">
                {{ t('admin.users.sales.tierLabel', { index: index + 1 }) }}
              </div>
              <button
                type="button"
                class="rounded-lg p-1.5 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20"
                :title="t('common.delete')"
                @click="removeTier(index)"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>

            <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-3">
              <div>
                <label class="input-label">
                  {{ t('admin.users.sales.tierFrom') }}
                </label>
                <input
                  v-model.number="tier.month_sales_from_cny"
                  :name="`sales_commission_tier_from_${index}`"
                  type="number"
                  min="0"
                  step="0.01"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">
                  {{ t('admin.users.sales.tierTo') }}
                </label>
                <input
                  :name="`sales_commission_tier_to_${index}`"
                  :value="tier.month_sales_to_cny ?? ''"
                  type="number"
                  min="0"
                  step="0.01"
                  class="input"
                  :placeholder="t('admin.users.sales.tierToPlaceholder')"
                  @input="updateTierUpper(index, ($event.target as HTMLInputElement).value)"
                />
              </div>
              <div>
                <label class="input-label">
                  {{ t('admin.users.sales.tierRate') }}
                </label>
                <input
                  v-model.number="tier.commission_rate"
                  :name="`sales_commission_tier_rate_${index}`"
                  type="number"
                  min="0"
                  max="100"
                  step="0.01"
                  class="input"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import {
  createSalesCommissionTierDraft,
  type SalesCommissionFormState
} from './salesCommissionForm'

const props = defineProps<{
  form: SalesCommissionFormState
}>()

const { t } = useI18n()

function setMode(mode: 'fixed' | 'tiered') {
  props.form.salesCommissionMode = mode
  if (mode === 'tiered' && props.form.salesCommissionTiers.length === 0) {
    addTier()
  }
}

function addTier() {
  const lastTier = props.form.salesCommissionTiers.at(-1)
  const nextFrom =
    lastTier?.month_sales_to_cny ?? lastTier?.month_sales_from_cny ?? 0

  props.form.salesCommissionTiers.push(
    createSalesCommissionTierDraft({
      month_sales_from_cny: nextFrom,
      sort_order: props.form.salesCommissionTiers.length + 1
    })
  )
}

function removeTier(index: number) {
  props.form.salesCommissionTiers.splice(index, 1)
}

function updateTierUpper(index: number, value: string) {
  const trimmed = value.trim()
  props.form.salesCommissionTiers[index].month_sales_to_cny =
    trimmed === '' ? null : Number(trimmed)
}

function modeButtonClass(active: boolean) {
  return [
    'rounded-lg border px-3 py-2 text-sm font-medium transition-colors',
    active
      ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-400 dark:bg-primary-900/20 dark:text-primary-300'
      : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:text-gray-900 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300 dark:hover:border-dark-500 dark:hover:text-gray-100'
  ]
}
</script>

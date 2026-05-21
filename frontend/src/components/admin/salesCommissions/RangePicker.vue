<template>
  <div class="flex flex-wrap items-center gap-2" data-testid="sales-commission-range-picker">
    <div class="inline-flex flex-wrap items-center rounded-md border border-gray-200 p-0.5 text-xs dark:border-dark-700">
      <button
        v-for="opt in presetOptions"
        :key="opt.key"
        type="button"
        :class="[
          'rounded px-2 py-1 transition-colors',
          modelValue.key === opt.key
            ? 'bg-primary-600 text-white'
            : 'text-gray-600 hover:text-gray-900 dark:text-dark-300 dark:hover:text-gray-100'
        ]"
        :data-testid="`range-${opt.key}`"
        @click="selectPreset(opt.key)"
      >{{ opt.label }}</button>
    </div>

    <div v-if="modelValue.key === 'custom'" class="flex flex-wrap items-center gap-2">
      <input v-model="customStart" type="date" class="input w-40" :aria-label="t('salesCommissions.range.pickStart')" />
      <span class="text-xs text-gray-500 dark:text-dark-400">→</span>
      <input v-model="customEnd" type="date" class="input w-40" :aria-label="t('salesCommissions.range.pickEnd')" />
      <button type="button" class="btn btn-primary btn-sm" :disabled="!canApplyCustom" @click="applyCustom">
        {{ t('common.apply') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SalesCommissionOverviewRangeKey } from '@/types'

export interface RangeSelection {
  key: SalesCommissionOverviewRangeKey
  start?: string
  end?: string
}

const props = defineProps<{ modelValue: RangeSelection }>()
const emit = defineEmits<{ (e: 'update:modelValue', value: RangeSelection): void }>()
const { t } = useI18n()

const presetOptions = computed<{ key: SalesCommissionOverviewRangeKey; label: string }[]>(() => [
  { key: 'today', label: t('salesCommissions.range.today') },
  { key: 'this_week', label: t('salesCommissions.range.thisWeek') },
  { key: 'this_month', label: t('salesCommissions.range.thisMonth') },
  { key: 'this_quarter', label: t('salesCommissions.range.thisQuarter') },
  { key: 'this_year', label: t('salesCommissions.range.thisYear') },
  { key: 'last_30d', label: t('salesCommissions.range.last30d') },
  { key: 'last_90d', label: t('salesCommissions.range.last90d') },
  { key: 'custom', label: t('salesCommissions.range.custom') }
])

const customStart = ref(props.modelValue.start || '')
const customEnd = ref(props.modelValue.end || '')

watch(() => props.modelValue, value => {
  customStart.value = value.start || ''
  customEnd.value = value.end || ''
})

const canApplyCustom = computed(() => {
  if (!customStart.value || !customEnd.value) return false
  return customStart.value <= customEnd.value
})

function selectPreset(key: SalesCommissionOverviewRangeKey) {
  if (key === 'custom') {
    emit('update:modelValue', { key: 'custom', start: customStart.value, end: customEnd.value })
    return
  }
  emit('update:modelValue', { key })
}

function applyCustom() {
  if (!canApplyCustom.value) return
  emit('update:modelValue', { key: 'custom', start: customStart.value, end: customEnd.value })
}
</script>

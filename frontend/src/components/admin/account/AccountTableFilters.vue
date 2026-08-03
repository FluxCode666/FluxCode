<template>
  <div class="flex flex-wrap items-center gap-3">
    <div class="flex w-full flex-nowrap items-center gap-2 sm:w-auto">
      <SearchInput
        :model-value="searchQuery"
        :placeholder="t('admin.accounts.searchAccounts')"
        class="min-w-0 flex-1 sm:w-64"
        @update:model-value="$emit('update:searchQuery', $event)"
        @search="$emit('change')"
      />
      <div class="relative shrink-0" ref="filterSettingsRef">
        <button
          type="button"
          class="flex items-center gap-2 whitespace-nowrap rounded-xl border border-gray-200 bg-white px-3 py-2.5 text-sm text-gray-700 transition-all duration-200 hover:border-gray-300 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200 dark:hover:border-dark-500"
          :title="t('admin.accounts.filterSettings')"
          @click="showFilterSettings = !showFilterSettings"
        >
          <Icon name="filter" size="sm" />
          <span class="hidden sm:inline">{{ t('admin.accounts.filterSettings') }}</span>
        </button>
        <div
          v-if="showFilterSettings"
          class="absolute right-0 z-50 mt-2 w-56 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
        >
          <div class="max-h-80 overflow-y-auto p-2">
            <button
              v-for="filter in configurableFilters"
              :key="filter.key"
              type="button"
              class="flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
              @click="toggleFilterVisibility(filter.key)"
            >
              <span>{{ filter.label }}</span>
              <Icon v-if="isFilterVisible(filter.key)" name="check" size="sm" class="text-primary-500" />
            </button>
          </div>
        </div>
      </div>
    </div>
    <Select
      :model-value="filters.platform"
      v-if="isFilterVisible('platform')"
      class="w-40"
      :options="platformOptions"
      @update:model-value="updatePlatform"
      @change="$emit('change')"
    />
    <Select
      v-if="isFilterVisible('type')"
      :model-value="filters.type"
      class="w-40"
      :options="typeOptions"
      @update:model-value="updateType"
      @change="$emit('change')"
    />
    <Select
      v-if="isFilterVisible('status')"
      :model-value="filters.status"
      class="w-40"
      :options="statusOptions"
      @update:model-value="updateStatus"
      @change="$emit('change')"
    />
    <Select
      v-if="isFilterVisible('privacy_mode')"
      :model-value="filters.privacy_mode"
      class="w-40"
      :options="privacyOptions"
      @update:model-value="updatePrivacyMode"
      @change="$emit('change')"
    />
    <Select
      v-if="isFilterVisible('schedulable_status')"
      :model-value="filters.schedulable_status"
      class="w-48"
      :options="schedulingStatusOptions"
      @update:model-value="updateSchedulingStatus"
      @change="$emit('change')"
    />
    <Select
      v-if="isFilterVisible('group')"
      :model-value="filters.group"
      data-test="account-group-filter"
      class="w-44"
      :options="groupOptions"
      searchable
      :search-placeholder="t('admin.accounts.searchGroups')"
      @update:model-value="updateGroup"
      @change="$emit('change')"
    />
    <Select
      v-if="isFilterVisible('model')"
      :model-value="filters.model"
      data-test="account-model-filter"
      class="w-52"
      :options="modelOptions"
      searchable
      creatable
      :search-placeholder="t('admin.accounts.searchModels')"
      :creatable-prefix="t('admin.accounts.queryModel')"
      @update:model-value="updateModel"
      @change="$emit('change')"
    />
    <div v-if="isFilterVisible('proxy_ids')" class="w-full min-w-[14rem] sm:w-56">
      <ProxyMultiSelectFilter
        :model-value="filters.proxy_ids || []"
        :options="proxies || []"
        :placeholder="t('admin.accounts.allProxies')"
        :show-no-proxy-option="true"
        :no-proxy-label="t('admin.accounts.noProxy')"
        @update:model-value="updateProxyIDs"
        @change="$emit('change')"
      />
    </div>
    <div v-if="isFilterVisible('created_at')" class="min-w-[16rem]">
      <DateRangePicker
        :start-date="filters.created_start_date || ''"
        :end-date="filters.created_end_date || ''"
        :placeholder="t('admin.accounts.createdTime')"
        @update:start-date="updateCreatedStartDate"
        @update:end-date="updateCreatedEndDate"
        @change="$emit('change')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Icon from '@/components/icons/Icon.vue'
import ProxyMultiSelectFilter from '@/components/common/ProxyMultiSelectFilter.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import { allModels } from '@/composables/useModelWhitelist'
import type { AccountSchedulingState, AdminGroup, Proxy } from '@/types'

const props = defineProps<{
  searchQuery: string
  filters: Record<string, any>
  groups?: AdminGroup[]
  proxies?: Proxy[]
}>()

const emit = defineEmits<{
  (e: 'update:searchQuery', value: string): void
  (e: 'update:filters', value: Record<string, any>): void
  (e: 'change'): void
}>()

const { t } = useI18n()

type FilterKey =
  | 'platform'
  | 'type'
  | 'status'
  | 'privacy_mode'
  | 'schedulable_status'
  | 'group'
  | 'model'
  | 'proxy_ids'
  | 'created_at'

const FILTER_SETTINGS_STORAGE_KEY = 'account-hidden-filters'
const DEFAULT_HIDDEN_FILTERS: FilterKey[] = ['privacy_mode', 'proxy_ids', 'type']
const allFilterKeys: FilterKey[] = [
  'platform',
  'type',
  'status',
  'privacy_mode',
  'schedulable_status',
  'group',
  'model',
  'proxy_ids',
  'created_at'
]
const hiddenFilters = ref<Set<FilterKey>>(new Set(DEFAULT_HIDDEN_FILTERS))
const showFilterSettings = ref(false)
const filterSettingsRef = ref<HTMLElement | null>(null)

const isFilterKey = (value: unknown): value is FilterKey => {
  return typeof value === 'string' && allFilterKeys.includes(value as FilterKey)
}

const loadFilterSettings = () => {
  if (typeof window === 'undefined') return
  try {
    const saved = localStorage.getItem(FILTER_SETTINGS_STORAGE_KEY)
    if (!saved) return
    const parsed = JSON.parse(saved) as unknown
    if (!Array.isArray(parsed)) return
    hiddenFilters.value = new Set(parsed.filter(isFilterKey))
  } catch (error) {
    console.error('Failed to load account filter settings:', error)
  }
}

const saveFilterSettings = () => {
  if (typeof window === 'undefined') return
  try {
    localStorage.setItem(FILTER_SETTINGS_STORAGE_KEY, JSON.stringify([...hiddenFilters.value]))
  } catch (error) {
    console.error('Failed to save account filter settings:', error)
  }
}

loadFilterSettings()

const isFilterVisible = (key: FilterKey) => !hiddenFilters.value.has(key)

const toggleFilterVisibility = (key: FilterKey) => {
  const next = new Set(hiddenFilters.value)
  if (next.has(key)) {
    next.delete(key)
  } else {
    next.add(key)
  }
  hiddenFilters.value = next
  saveFilterSettings()
}

const configurableFilters = computed<Array<{ key: FilterKey; label: string }>>(() => [
  { key: 'platform', label: t('admin.accounts.filterLabels.platform') },
  { key: 'type', label: t('admin.accounts.filterLabels.type') },
  { key: 'status', label: t('admin.accounts.filterLabels.status') },
  { key: 'privacy_mode', label: t('admin.accounts.filterLabels.privacyMode') },
  { key: 'schedulable_status', label: t('admin.accounts.filterLabels.schedulingStatus') },
  { key: 'group', label: t('admin.accounts.filterLabels.group') },
  { key: 'model', label: t('admin.accounts.filterLabels.model') },
  { key: 'proxy_ids', label: t('admin.accounts.filterLabels.proxy') },
  { key: 'created_at', label: t('admin.accounts.filterLabels.createdAt') }
])

const handleClickOutside = (event: MouseEvent) => {
  if (filterSettingsRef.value && !filterSettingsRef.value.contains(event.target as Node)) {
    showFilterSettings.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})

const emitFilters = (patch: Record<string, any>) => {
  emit('update:filters', { ...props.filters, ...patch })
}

const updatePlatform = (value: string | number | boolean | null) => {
  emitFilters({ platform: value ?? '' })
}

const updateType = (value: string | number | boolean | null) => {
  emitFilters({ type: value ?? '' })
}

const updateStatus = (value: string | number | boolean | null) => {
  emitFilters({ status: value ?? '' })
}

const updatePrivacyMode = (value: string | number | boolean | null) => {
  emitFilters({ privacy_mode: value ?? '' })
}

const updateSchedulingStatus = (value: string | number | boolean | null) => {
  emitFilters({ schedulable_status: (value ?? '') as '' | AccountSchedulingState })
}

const updateGroup = (value: string | number | boolean | null) => {
  emitFilters({ group: value ?? '' })
}

const updateModel = (value: string | number | boolean | null) => {
  emitFilters({ model: value ?? '' })
}

const updateProxyIDs = (value: number[]) => {
  emitFilters({ proxy_ids: value })
}

const updateCreatedStartDate = (value: string) => {
  emitFilters({ created_start_date: value })
}

const updateCreatedEndDate = (value: string) => {
  emitFilters({ created_end_date: value })
}

const platformOptions = computed(() => [
  { value: '', label: t('admin.accounts.allPlatforms') },
  { value: 'anthropic', label: t('admin.accounts.platforms.anthropic') },
  { value: 'openai', label: t('admin.accounts.platforms.openai') },
  { value: 'codex2api', label: t('admin.accounts.platforms.codex2api') },
  { value: 'gemini', label: t('admin.accounts.platforms.gemini') },
  { value: 'antigravity', label: t('admin.accounts.platforms.antigravity') },
  { value: 'embedding', label: t('admin.accounts.platforms.embedding', 'Embedding') },
  { value: 'sora', label: t('admin.accounts.platforms.sora') }
])

const typeOptions = computed(() => [
  { value: '', label: t('admin.accounts.allTypes') },
  { value: 'oauth', label: t('admin.accounts.oauthType') },
  { value: 'setup-token', label: t('admin.accounts.setupToken') },
  { value: 'apikey', label: t('admin.accounts.apiKey') },
  { value: 'upstream', label: t('admin.accounts.types.upstream') },
  { value: 'bedrock', label: t('admin.accounts.bedrockLabel') }
])

const statusOptions = computed(() => [
  { value: '', label: t('admin.accounts.allStatus') },
  { value: 'active', label: t('common.active') },
  { value: 'inactive', label: t('common.inactive') },
  { value: 'error', label: t('common.error') },
  { value: 'banned', label: t('common.banned') },
  { value: 'rate_limited', label: t('admin.accounts.status.rateLimited') },
  { value: 'temp_unschedulable', label: t('admin.accounts.status.tempUnschedulable') },
  { value: 'expired', label: t('admin.accounts.expiration.expired') }
])

const privacyOptions = computed(() => [
  { value: '', label: t('admin.accounts.allPrivacyModes') },
  { value: '__unset__', label: t('admin.accounts.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])

const schedulingStatusOptions = computed(() => [
  { value: '', label: t('admin.accounts.allSchedulingStatus') },
  { value: 'available', label: t('admin.proxies.countStates.available') },
  { value: 'manual_unschedulable', label: t('admin.proxies.countStates.manualUnschedulable') },
  { value: 'temp_unschedulable', label: t('admin.proxies.countStates.tempUnschedulable') },
  { value: 'rate_limited', label: t('admin.proxies.countStates.rateLimited') },
  { value: 'overloaded', label: t('admin.proxies.countStates.overloaded') },
  { value: 'expired', label: t('admin.proxies.countStates.expired') },
  { value: 'inactive', label: t('admin.proxies.countStates.inactive') },
  { value: 'error', label: t('admin.proxies.countStates.error') },
  { value: 'banned', label: t('admin.proxies.countStates.banned') }
])

const groupOptions = computed(() => [
  { value: '', label: t('admin.accounts.allGroups') },
  { value: 'ungrouped', label: t('admin.accounts.ungroupedGroup') },
  ...(props.groups || []).map((group) => ({ value: String(group.id), label: group.name }))
])

const modelOptions = computed(() => {
  const options = [{ value: '', label: t('admin.accounts.allModels') }, ...allModels]
  const selectedModel = typeof props.filters.model === 'string' ? props.filters.model.trim() : ''
  if (selectedModel && !options.some((option) => option.value === selectedModel)) {
    options.push({ value: selectedModel, label: selectedModel })
  }
  return options
})
</script>

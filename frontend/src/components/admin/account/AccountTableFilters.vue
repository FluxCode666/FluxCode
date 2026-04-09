<template>
  <div class="flex flex-wrap items-center gap-3">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.accounts.searchAccounts')"
      class="w-full sm:w-64"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />
    <Select
      :model-value="filters.platform"
      class="w-40"
      :options="platformOptions"
      @update:model-value="updatePlatform"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.type"
      class="w-40"
      :options="typeOptions"
      @update:model-value="updateType"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.status"
      class="w-40"
      :options="statusOptions"
      @update:model-value="updateStatus"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.schedulable_status"
      class="w-48"
      :options="schedulingStatusOptions"
      @update:model-value="updateSchedulingStatus"
      @change="$emit('change')"
    />
    <Select
      :model-value="filters.group"
      class="w-44"
      :options="groupOptions"
      @update:model-value="updateGroup"
      @change="$emit('change')"
    />
    <div class="w-full min-w-[14rem] sm:w-56">
      <ProxyMultiSelectFilter
        :model-value="filters.proxy_ids || []"
        :options="proxies || []"
        :placeholder="t('admin.accounts.allProxies')"
        @update:model-value="updateProxyIDs"
        @change="$emit('change')"
      />
    </div>
    <div class="min-w-[16rem]">
      <DateRangePicker
        :start-date="filters.created_start_date || ''"
        :end-date="filters.created_end_date || ''"
        @update:start-date="updateCreatedStartDate"
        @update:end-date="updateCreatedEndDate"
        @change="$emit('change')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import DateRangePicker from '@/components/common/DateRangePicker.vue'
import ProxyMultiSelectFilter from '@/components/common/ProxyMultiSelectFilter.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
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

const updateSchedulingStatus = (value: string | number | boolean | null) => {
  emitFilters({ schedulable_status: (value ?? '') as '' | AccountSchedulingState })
}

const updateGroup = (value: string | number | boolean | null) => {
  emitFilters({ group: value ?? '' })
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
  { value: 'gemini', label: t('admin.accounts.platforms.gemini') },
  { value: 'antigravity', label: t('admin.accounts.platforms.antigravity') },
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
</script>

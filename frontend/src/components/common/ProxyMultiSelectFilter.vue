<template>
  <div class="relative" ref="containerRef">
    <button
      type="button"
      class="select-trigger"
      @click="toggle"
    >
      <span class="select-value">{{ selectedLabel }}</span>
      <span class="select-icon">
        <svg
          :class="['h-5 w-5 transition-transform duration-200', isOpen && 'rotate-180']"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          stroke-width="1.5"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
        </svg>
      </span>
    </button>

    <Transition name="select-dropdown">
      <div v-if="isOpen" class="select-dropdown">
        <div class="border-b border-gray-200 p-2 dark:border-dark-600">
          <input
            v-model="searchQuery"
            type="text"
            class="select-search-input w-full"
            :placeholder="t('admin.proxies.searchProxies')"
          />
        </div>
        <div class="max-h-64 overflow-auto">
          <label
            v-for="proxy in filteredOptions"
            :key="proxy.id"
            class="flex cursor-pointer items-start gap-2 px-3 py-2 hover:bg-gray-50 dark:hover:bg-dark-700"
          >
            <input
              type="checkbox"
              class="mt-1 h-3.5 w-3.5 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
              :checked="modelValue.includes(proxy.id)"
              @change="toggleOption(proxy.id, ($event.target as HTMLInputElement).checked)"
            />
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="truncate text-sm text-gray-900 dark:text-gray-100">{{ proxy.name }}</span>
                <span
                  :class="[
                    'rounded px-1.5 py-0.5 text-xs',
                    proxy.status === 'active'
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                      : 'bg-gray-100 text-gray-600 dark:bg-dark-600 dark:text-gray-400'
                  ]"
                >
                  {{ proxy.status === 'active' ? t('common.active') : t('common.inactive') }}
                </span>
              </div>
              <div class="truncate text-xs text-gray-500 dark:text-gray-400">
                {{ proxy.host }}:{{ proxy.port }}
              </div>
            </div>
          </label>
          <div v-if="filteredOptions.length === 0" class="px-3 py-4 text-center text-sm text-gray-500 dark:text-gray-400">
            {{ t('common.noOptionsFound') }}
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import type { Proxy } from '@/types'

interface Props {
  modelValue: number[]
  options: Proxy[]
  placeholder: string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
  change: [value: number[]]
}>()

const { t } = useI18n()

const isOpen = ref(false)
const searchQuery = ref('')
const containerRef = ref<HTMLElement | null>(null)

const selectedLabel = computed(() => {
  if (props.modelValue.length === 0) return props.placeholder
  return t('admin.accounts.selectedProxies', { count: props.modelValue.length })
})

const filteredOptions = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return props.options
  return props.options.filter((item) => {
    const name = item.name.toLowerCase()
    const host = item.host.toLowerCase()
    return name.includes(query) || host.includes(query)
  })
})

const toggle = () => {
  isOpen.value = !isOpen.value
  if (!isOpen.value) {
    searchQuery.value = ''
  }
}

const toggleOption = (id: number, checked: boolean) => {
  const next = checked
    ? [...props.modelValue, id]
    : props.modelValue.filter((item) => item !== id)
  emit('update:modelValue', next)
  emit('change', next)
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (!containerRef.value || containerRef.value.contains(target)) return
  isOpen.value = false
  searchQuery.value = ''
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>

<style scoped>
.select-trigger {
  @apply flex w-full items-center justify-between gap-2;
  @apply rounded-xl px-4 py-2.5 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply transition-all duration-200;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
}

.select-value {
  @apply truncate text-left;
}

.select-icon {
  @apply text-gray-500 dark:text-gray-400;
}

.select-dropdown {
  @apply absolute z-50 mt-2 w-full overflow-hidden rounded-xl;
  @apply border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800;
}

.select-search-input {
  @apply rounded-lg border border-gray-200 bg-white px-3 py-1.5 text-sm;
  @apply text-gray-900 placeholder-gray-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20;
  @apply dark:border-dark-500 dark:bg-dark-700 dark:text-gray-100 dark:placeholder-dark-400;
}
</style>

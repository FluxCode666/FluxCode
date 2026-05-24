<template>
  <div class="relative" ref="containerRef">
    <div
      class="input flex cursor-text items-center gap-1.5"
      :class="{ 'ring-2 ring-primary-500': isOpen }"
      @click="openDropdown"
    >
      <template v-if="selectedUser">
        <span class="truncate text-sm text-gray-900 dark:text-white">{{ selectedUser.email }}</span>
        <button
          type="button"
          class="ml-auto flex-shrink-0 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          @click.stop="clearSelection"
        >
          <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </template>
      <template v-else>
        <input
          ref="inputRef"
          v-model="searchQuery"
          type="text"
          class="w-full border-0 bg-transparent p-0 text-sm outline-none placeholder:text-gray-400 focus:ring-0 dark:text-white dark:placeholder:text-dark-500"
          :placeholder="placeholder"
          @input="onInput"
          @focus="openDropdown"
          @keydown.escape="closeDropdown"
        />
      </template>
    </div>

    <!-- Dropdown -->
    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="dropdownRef"
        class="fixed z-[60] max-h-52 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
        :style="dropdownStyle"
      >
        <div v-if="loading" class="px-3 py-2 text-center text-xs text-gray-400">
          {{ t('common.loading') }}…
        </div>
        <div v-else-if="options.length === 0 && searchQuery.length > 0" class="px-3 py-2 text-center text-xs text-gray-400">
          {{ t('common.noResults') }}
        </div>
        <div v-else-if="options.length === 0" class="px-3 py-2 text-center text-xs text-gray-400">
          {{ t('common.typeToSearch') }}
        </div>
        <button
          v-for="user in options"
          :key="user.id"
          type="button"
          class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
          @click="selectUser(user)"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</div>
            <div v-if="user.username" class="truncate text-xs text-gray-500 dark:text-dark-400">{{ user.username }}</div>
          </div>
          <span class="flex-shrink-0 text-xs text-gray-400 dark:text-dark-500">#{{ user.id }}</span>
        </button>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, nextTick, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'

const props = defineProps<{
  modelValue: string // user ID as string, or empty
  placeholder?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const { t } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)

const isOpen = ref(false)
const searchQuery = ref('')
const loading = ref(false)
const options = ref<AdminUser[]>([])
const selectedUser = ref<{ id: number; email: string; username?: string } | null>(null)

let searchTimeout: ReturnType<typeof setTimeout> | null = null
let abortController: AbortController | null = null

const dropdownStyle = computed(() => {
  if (!containerRef.value) return {}
  const rect = containerRef.value.getBoundingClientRect()
  return {
    top: `${rect.bottom + 4}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`
  }
})

watch(() => props.modelValue, async (val) => {
  if (!val) {
    selectedUser.value = null
    return
  }
  // If we already have this user selected, skip
  if (selectedUser.value && String(selectedUser.value.id) === val) return
  // Load user info
  try {
    const res = await adminAPI.users.list(1, 1, { search: val })
    const found = res.items?.find(u => String(u.id) === val)
    if (found) {
      selectedUser.value = { id: found.id, email: found.email, username: found.username }
    }
  } catch { /* ignore */ }
}, { immediate: true })

function openDropdown() {
  isOpen.value = true
  nextTick(() => inputRef.value?.focus())
}

function closeDropdown() {
  isOpen.value = false
  searchQuery.value = ''
  options.value = []
}

function onInput() {
  if (searchTimeout) clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    doSearch()
  }, 300)
}

async function doSearch() {
  const q = searchQuery.value.trim()
  if (!q) {
    options.value = []
    return
  }
  if (abortController) abortController.abort()
  abortController = new AbortController()
  loading.value = true
  try {
    const res = await adminAPI.users.list(1, 10, { search: q }, { signal: abortController.signal })
    options.value = res.items || []
  } catch (e: any) {
    if (e?.name !== 'AbortError') {
      options.value = []
    }
  } finally {
    loading.value = false
  }
}

function selectUser(user: AdminUser) {
  selectedUser.value = { id: user.id, email: user.email, username: user.username }
  emit('update:modelValue', String(user.id))
  closeDropdown()
}

function clearSelection() {
  selectedUser.value = null
  emit('update:modelValue', '')
}

function handleClickOutside(e: MouseEvent) {
  if (!isOpen.value) return
  const target = e.target as Node
  if (containerRef.value?.contains(target)) return
  if (dropdownRef.value?.contains(target)) return
  closeDropdown()
}

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('mousedown', handleClickOutside)
  if (searchTimeout) clearTimeout(searchTimeout)
  if (abortController) abortController.abort()
})
</script>

<template>
  <div ref="containerRef" class="relative space-y-2">
    <div v-if="displayUsers.length > 0" class="flex flex-wrap gap-2">
      <span
        v-for="user in displayUsers"
        :key="user.id"
        class="inline-flex max-w-full items-center gap-1 rounded border border-gray-200 bg-gray-50 px-2 py-1 text-xs text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-dark-100"
      >
        <span class="truncate">{{ user.email }}</span>
        <span class="text-gray-400">#{{ user.id }}</span>
        <button
          type="button"
          data-test="user-multi-remove"
          class="text-gray-400 hover:text-red-500"
          @click="removeUser(user.id)"
        >
          ×
        </button>
      </span>
    </div>

    <div
      class="input flex cursor-text items-center gap-1.5"
      :class="{ 'ring-2 ring-primary-500': isOpen }"
      @click="openDropdown"
    >
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
    </div>

    <Teleport to="body">
      <div
        v-if="isOpen"
        ref="dropdownRef"
        class="fixed z-[100000020] max-h-52 overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
        :style="dropdownStyle"
      >
        <div v-if="loading" class="px-3 py-2 text-center text-xs text-gray-400">
          {{ t('common.loading') }}...
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
          data-test="user-multi-search-option"
          class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-gray-100 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700"
          :disabled="modelValue.includes(user.id)"
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
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'

type SelectedUser = { id: number; email: string; username?: string }

const props = withDefaults(defineProps<{
  modelValue: number[]
  selectedUsers?: SelectedUser[]
  placeholder?: string
}>(), {
  selectedUsers: () => [],
  placeholder: '',
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: number[]): void
  (event: 'update:selectedUsers', value: SelectedUser[]): void
}>()

const { t } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const isOpen = ref(false)
const searchQuery = ref('')
const loading = ref(false)
const options = ref<AdminUser[]>([])
const localUsers = ref<SelectedUser[]>([...props.selectedUsers])

let searchTimeout: ReturnType<typeof setTimeout> | null = null
let abortController: AbortController | null = null

const displayUsers = computed(() => localUsers.value.filter(user => props.modelValue.includes(user.id)))

const dropdownStyle = computed(() => {
  if (!containerRef.value) return {}
  const rect = containerRef.value.getBoundingClientRect()
  return {
    top: `${rect.bottom + 4}px`,
    left: `${rect.left}px`,
    width: `${rect.width}px`,
  }
})

watch(() => props.selectedUsers, users => {
  localUsers.value = [...users]
})

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
  searchTimeout = setTimeout(doSearch, 300)
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
  } catch (error: any) {
    if (error?.name !== 'AbortError') {
      options.value = []
    }
  } finally {
    loading.value = false
  }
}

function selectUser(user: AdminUser) {
  if (props.modelValue.includes(user.id)) return
  const selected = { id: user.id, email: user.email, username: user.username }
  const nextIDs = [...props.modelValue, user.id]
  const nextUsers = [...localUsers.value.filter(item => item.id !== user.id), selected]
  localUsers.value = nextUsers
  emit('update:modelValue', nextIDs)
  emit('update:selectedUsers', nextUsers)
  closeDropdown()
}

function removeUser(userID: number) {
  const nextIDs = props.modelValue.filter(id => id !== userID)
  const nextUsers = localUsers.value.filter(user => nextIDs.includes(user.id))
  localUsers.value = nextUsers
  emit('update:modelValue', nextIDs)
  emit('update:selectedUsers', nextUsers)
}

function handleClickOutside(event: MouseEvent) {
  if (!isOpen.value) return
  const target = event.target as Node
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

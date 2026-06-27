<template>
  <AppLayout>
    <div class="space-y-5">
      <div class="flex flex-col gap-3 md:flex-row md:items-end md:justify-between">
        <div class="min-w-0">
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">
            {{ t('admin.generatedImages.title') }}
          </h1>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.generatedImages.description') }}
          </p>
        </div>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="grid gap-3 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_minmax(16rem,0.9fr)_auto] lg:items-end">
          <div class="relative">
            <label class="input-label">{{ t('admin.generatedImages.userEmail') }}</label>
            <input
              v-model="userEmailSearch"
              type="text"
              class="input"
              data-test="generated-images-user-email-search"
              :placeholder="t('admin.generatedImages.userEmailPlaceholder')"
              autocomplete="off"
              @focus="openUserEmailDropdown"
              @input="handleUserEmailInput"
            />
            <div
              v-if="userEmailDropdownOpen"
              class="absolute left-0 right-0 z-20 mt-1 max-h-52 overflow-auto rounded-xl border border-gray-200 bg-white py-1 shadow-lg shadow-black/10 dark:border-dark-700 dark:bg-dark-800 dark:shadow-black/30"
            >
              <div v-if="userEmailLoading" class="px-4 py-2.5 text-sm text-gray-500 dark:text-gray-400">
                {{ t('common.loading') }}
              </div>
              <button
                v-for="user in userEmailOptions"
                :key="user.id"
                type="button"
                class="block w-full px-4 py-2.5 text-left text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700"
                :data-test="`generated-images-user-option-${user.id}`"
                @click="selectUserEmail(user.email)"
              >
                <span class="block truncate font-medium">{{ user.email }}</span>
              </button>
              <div
                v-if="!userEmailLoading && userEmailOptions.length === 0 && userEmailSearch.trim()"
                class="px-4 py-2.5 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noResults') }}
              </div>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.generatedImages.channelGroup') }}</label>
            <Select
              v-model="filters.group_id"
              :options="groupFilterOptions"
              data-test="generated-images-group-filter"
            />
          </div>

          <div class="min-w-0">
            <label class="input-label">{{ t('admin.generatedImages.dateRange') }}</label>
            <DateRangePicker
              v-model:start-date="filters.start_at"
              v-model:end-date="filters.end_at"
              class="w-full"
              data-test="generated-images-date-range"
              @change="onDateRangeChange"
            />
          </div>

          <div class="flex flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-primary"
              data-test="generated-images-apply-filters"
              @click="applyFilters"
            >
              {{ t('admin.generatedImages.query') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="!hasActiveFilters"
              @click="resetFilters"
            >
              {{ t('admin.generatedImages.resetFilters') }}
            </button>
            <button
              type="button"
              class="btn border-red-200 bg-red-50 text-red-700 hover:bg-red-100 focus:ring-red-500 dark:border-red-900/60 dark:bg-red-950/30 dark:text-red-300 dark:hover:bg-red-950/50"
              data-test="generated-images-open-cleanup"
              :disabled="cleanupSubmitting"
              @click="openCleanupDialog"
            >
              <Icon name="trash" size="sm" class="mr-2" />
              {{ t('admin.generatedImages.cleanup') }}
            </button>
          </div>
        </div>
      </div>

      <div v-if="loading && images.length === 0" class="flex min-h-[360px] items-center justify-center">
        <LoadingSpinner size="lg" />
      </div>

      <div
        v-else-if="images.length === 0"
        class="flex min-h-[360px] items-center justify-center rounded-lg border border-dashed border-gray-300 bg-white p-6 dark:border-dark-600 dark:bg-dark-800"
      >
        <EmptyState
          :title="t('admin.generatedImages.emptyTitle')"
          :description="t('admin.generatedImages.emptyDescription')"
          :action-text="t('admin.generatedImages.refresh')"
          action-icon-name="refresh"
          @action="refreshData"
        />
      </div>

      <div v-else class="space-y-4">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          <article
            v-for="image in images"
            :key="image.id"
            class="card overflow-hidden transition hover:-translate-y-0.5 hover:shadow-lg"
          >
            <button
              type="button"
              class="group block w-full text-left"
              data-test="generated-image-card"
              @click="openPreview(image)"
            >
              <div class="relative aspect-square bg-gray-100 dark:bg-dark-700">
                <img
                  v-if="imageUrls.get(image.id)"
                  data-test="generated-image-thumb"
                  :src="imageUrls.get(image.id)"
                  :alt="image.prompt || t('admin.generatedImages.preview')"
                  class="h-full w-full object-cover"
                />
                <div
                  v-else-if="imageLoadErrors.has(image.id)"
                  class="flex h-full w-full items-center justify-center p-4 text-center text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('admin.generatedImages.failedToLoadImage') }}
                </div>
                <div v-else class="flex h-full w-full items-center justify-center">
                  <LoadingSpinner size="md" color="secondary" />
                </div>
                <div
                  class="absolute inset-x-0 bottom-0 flex items-center justify-between bg-black/55 px-3 py-2 text-xs text-white opacity-0 transition group-hover:opacity-100"
                >
                  <span class="truncate">{{ t('admin.generatedImages.preview') }}</span>
                  <Icon name="eye" size="sm" />
                </div>
              </div>
            </button>

            <div class="space-y-3 p-4">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <h2 class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                    {{ image.model || '-' }}
                  </h2>
                  <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ formatDate(image.created_at) }}
                  </p>
                </div>
                <span class="rounded-md bg-primary-50 px-2 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">
                  {{ formatBytes(image.size_bytes) }}
                </span>
              </div>

              <p class="line-clamp-3 min-h-[3.75rem] text-sm text-gray-700 dark:text-gray-300">
                {{ image.prompt || '-' }}
              </p>

              <dl class="grid grid-cols-2 gap-x-3 gap-y-2 text-xs">
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.provider') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ image.provider || '-' }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.source') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ image.source || '-' }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.responseFormat') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ image.response_format || '-' }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.userEmail') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ displayText(image.user_email) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.apiKeyName') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ displayText(image.api_key_name) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.accountName') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ displayText(image.account_name) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.channelGroup') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ formatGroups(image.account_group_names) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.requestId') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ image.request_id || '-' }}</dd>
                </div>
              </dl>
            </div>
          </article>
        </div>

        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          :page-size-options="tablePageSizeOptions"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </div>
    </div>

    <div
      v-if="previewImage"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      @mousedown.self="closePreview"
    >
      <div class="flex max-h-full w-full max-w-7xl flex-col overflow-hidden rounded-lg bg-white shadow-xl dark:bg-dark-800">
        <div class="flex items-start justify-between gap-4 border-b border-gray-200 px-4 py-3 dark:border-dark-700">
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">
              {{ previewImage.model || '-' }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ formatDate(previewImage.created_at) }}
            </p>
          </div>
          <button
            type="button"
            class="btn btn-ghost btn-sm"
            :aria-label="t('admin.generatedImages.closePreview')"
            @click="closePreview"
          >
            <Icon name="x" size="md" />
          </button>
        </div>

        <div class="grid min-h-0 overflow-auto lg:grid-cols-[minmax(0,1fr)_360px]">
          <div class="flex min-h-[70vh] min-w-0 items-center justify-center bg-gray-950" data-test="generated-image-preview-pane">
            <img
              v-if="imageUrls.get(previewImage.id)"
              data-test="generated-image-preview"
              :src="imageUrls.get(previewImage.id)"
              :alt="previewImage.prompt || t('admin.generatedImages.preview')"
              class="h-full w-full object-contain"
            />
          </div>

          <aside class="min-w-0 divide-y divide-gray-200 border-t border-gray-200 dark:divide-dark-700 dark:border-dark-700 lg:border-l lg:border-t-0">
            <section class="p-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.generatedImages.prompt') }}
              </h3>
              <p class="mt-2 max-h-36 overflow-auto whitespace-pre-wrap rounded-md bg-gray-50 p-3 text-sm text-gray-800 dark:bg-dark-700 dark:text-gray-200">
                {{ previewImage.prompt || '-' }}
              </p>
              <template v-if="previewImage.revised_prompt">
                <h3 class="mt-4 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.generatedImages.revisedPrompt') }}
                </h3>
                <p class="mt-2 max-h-36 overflow-auto whitespace-pre-wrap rounded-md bg-gray-50 p-3 text-sm text-gray-800 dark:bg-dark-700 dark:text-gray-200">
                  {{ previewImage.revised_prompt }}
                </p>
              </template>
            </section>

            <section class="p-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.generatedImages.metadata') }}
              </h3>
              <dl class="mt-3 grid grid-cols-2 gap-3 text-xs">
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.provider') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ previewImage.provider || '-' }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.size') }}</dt>
                  <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatBytes(previewImage.size_bytes) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.source') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ previewImage.source || '-' }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.responseFormat') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ previewImage.response_format || '-' }}</dd>
                </div>
              </dl>
            </section>

            <section class="p-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.generatedImages.ownership') }}
              </h3>
              <dl class="mt-3 space-y-3 text-xs">
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.userEmail') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ displayText(previewImage.user_email) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.apiKeyName') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ displayText(previewImage.api_key_name) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.accountName') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ displayText(previewImage.account_name) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.channelGroup') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ formatGroups(previewImage.account_group_names) }}</dd>
                </div>
              </dl>
            </section>

            <section class="p-4">
              <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.generatedImages.request') }}
              </h3>
              <dl class="mt-3 space-y-3 text-xs">
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.requestId') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ previewImage.request_id || '-' }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.contentType') }}</dt>
                  <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ previewImage.content_type || '-' }}</dd>
                </div>
              </dl>
            </section>
          </aside>
        </div>
      </div>
    </div>

    <ConfirmDialog
      :show="cleanupDialogVisible"
      :title="t('admin.generatedImages.cleanupConfirmTitle')"
      :message="t('admin.generatedImages.cleanupConfirmMessage', { start: filters.start_at, end: filters.end_at })"
      :confirm-text="t('admin.generatedImages.cleanupConfirmSubmit')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmCleanup"
      @cancel="cleanupDialogVisible = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI, type GeneratedImage, type GeneratedImagesQuery } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { getConfiguredTablePageSizeOptions } from '@/utils/tablePreferences'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const images = ref<GeneratedImage[]>([])
const loading = ref(false)
const previewImage = ref<GeneratedImage | null>(null)
const imageUrls = ref(new Map<number, string>())
const imageLoadErrors = ref(new Set<number>())
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})
const filters = reactive({
  user_email: '',
  group_id: '',
  start_at: '',
  end_at: ''
})
const userEmailSearch = ref('')
const userEmailOptions = ref<Array<{ id: number; email: string }>>([])
const userEmailLoading = ref(false)
const userEmailDropdownOpen = ref(false)
const channelGroups = ref<Array<{ id: number; name: string }>>([])
const cleanupDialogVisible = ref(false)
const cleanupSubmitting = ref(false)
const tablePageSizeOptions = getConfiguredTablePageSizeOptions()

const hasActiveFilters = computed(() => (
  Boolean(filters.user_email.trim()) ||
  Boolean(filters.group_id) ||
  Boolean(filters.start_at) ||
  Boolean(filters.end_at)
))

const groupFilterOptions = computed(() => [
  { value: '', label: t('admin.generatedImages.allGroups') },
  ...channelGroups.value.map((group) => ({
    value: String(group.id),
    label: group.name
  }))
])

let listAbortController: AbortController | null = null
const contentAbortControllers = new Map<number, AbortController>()
let requestSeq = 0

function isCanceled(error: unknown): boolean {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as { code?: string }).code === 'ERR_CANCELED'
  )
}

function setObjectUrl(id: number, url: string): void {
  const current = imageUrls.value.get(id)
  if (current) {
    URL.revokeObjectURL(current)
  }

  const next = new Map(imageUrls.value)
  next.set(id, url)
  imageUrls.value = next
}

function revokeMissingUrls(nextIds: Set<number>): void {
  const next = new Map(imageUrls.value)

  for (const [id, url] of imageUrls.value) {
    if (!nextIds.has(id)) {
      URL.revokeObjectURL(url)
      next.delete(id)
    }
  }

  imageUrls.value = next
}

function revokeAllUrls(): void {
  for (const url of imageUrls.value.values()) {
    URL.revokeObjectURL(url)
  }

  imageUrls.value = new Map()
}

function abortContentLoads(): void {
  for (const controller of contentAbortControllers.values()) {
    controller.abort()
  }

  contentAbortControllers.clear()
}

async function loadImageContent(image: GeneratedImage): Promise<void> {
  if (imageUrls.value.has(image.id)) return

  contentAbortControllers.get(image.id)?.abort()
  const controller = new AbortController()
  contentAbortControllers.set(image.id, controller)

  try {
    const blob = await adminAPI.generatedImages.getContentBlob(image.id, {
      signal: controller.signal
    })
    if (!images.value.some((item) => item.id === image.id)) {
      return
    }
    setObjectUrl(image.id, URL.createObjectURL(blob))
  } catch (error) {
    if (!isCanceled(error)) {
      const next = new Set(imageLoadErrors.value)
      next.add(image.id)
      imageLoadErrors.value = next
    }
  } finally {
    if (contentAbortControllers.get(image.id) === controller) {
      contentAbortControllers.delete(image.id)
    }
  }
}

async function loadData(): Promise<void> {
  listAbortController?.abort()
  abortContentLoads()
  const controller = new AbortController()
  listAbortController = controller
  const seq = ++requestSeq
  loading.value = true

  try {
    const result = await adminAPI.generatedImages.list(
      buildListQuery(),
      {
        signal: controller.signal
      }
    )

    if (seq !== requestSeq) return

    images.value = result.items
    pagination.total = result.total
    pagination.page = result.page
    pagination.page_size = result.page_size
    pagination.pages = result.pages
    imageLoadErrors.value = new Set()

    const ids = new Set(result.items.map((image) => image.id))
    revokeMissingUrls(ids)

    await Promise.all(result.items.map((image) => loadImageContent(image)))
  } catch (error) {
    if (!isCanceled(error)) {
      appStore.showError(extractApiErrorMessage(error, t('admin.generatedImages.failedToLoad')))
    }
  } finally {
    if (listAbortController === controller) {
      listAbortController = null
    }
    if (seq === requestSeq) {
      loading.value = false
    }
  }
}

function buildListQuery(): GeneratedImagesQuery {
  const query: GeneratedImagesQuery = {
    page: pagination.page,
    page_size: pagination.page_size
  }
  const userEmail = filters.user_email.trim()
  if (userEmail) {
    query.user_email = userEmail
  }
  if (filters.group_id) {
    query.group_id = Number(filters.group_id)
  }
  if (filters.start_at) {
    query.start_at = filters.start_at
  }
  if (filters.end_at) {
    query.end_at = filters.end_at
  }
  return query
}

async function loadChannelGroups(): Promise<void> {
  try {
    const groups = await adminAPI.groups.getAll()
    channelGroups.value = groups
      .map((group) => ({ id: group.id, name: group.name }))
      .filter((group) => group.id > 0 && group.name)
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.generatedImages.failedToLoadGroups')))
  }
}

function openUserEmailDropdown(): void {
  if (userEmailOptions.value.length > 0 || userEmailSearch.value.trim()) {
    userEmailDropdownOpen.value = true
  }
}

function handleUserEmailInput(): void {
  filters.user_email = userEmailSearch.value.trim()
  void searchUserEmails()
}

async function searchUserEmails(): Promise<void> {
  const keyword = userEmailSearch.value.trim()
  if (!keyword) {
    userEmailOptions.value = []
    userEmailDropdownOpen.value = false
    return
  }

  userEmailLoading.value = true
  userEmailDropdownOpen.value = true
  try {
    userEmailOptions.value = await adminAPI.usage.searchUsers(keyword)
  } catch {
    userEmailOptions.value = []
  } finally {
    userEmailLoading.value = false
  }
}

function selectUserEmail(email: string): void {
  filters.user_email = email
  userEmailSearch.value = email
  userEmailDropdownOpen.value = false
}

function onDateRangeChange(range: { startDate: string; endDate: string; preset: string | null }): void {
  filters.start_at = range.startDate
  filters.end_at = range.endDate
}

function applyFilters(): void {
  pagination.page = 1
  void loadData()
}

function resetFilters(): void {
  filters.user_email = ''
  filters.group_id = ''
  filters.start_at = ''
  filters.end_at = ''
  userEmailSearch.value = ''
  userEmailOptions.value = []
  userEmailDropdownOpen.value = false
  pagination.page = 1
  void loadData()
}

function openCleanupDialog(): void {
  if (!filters.start_at || !filters.end_at) {
    appStore.showError(t('admin.generatedImages.cleanupMissingRange'))
    return
  }

  cleanupDialogVisible.value = true
}

async function confirmCleanup(): Promise<void> {
  if (cleanupSubmitting.value) return

  cleanupSubmitting.value = true
  try {
    const result = await adminAPI.generatedImages.deleteByDateRange({
      start_at: filters.start_at,
      end_at: filters.end_at
    })
    cleanupDialogVisible.value = false
    appStore.showSuccess(t('admin.generatedImages.cleanupSuccess', { count: result.deleted_count }))
    pagination.page = 1
    await loadData()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.generatedImages.cleanupFailed')))
  } finally {
    cleanupSubmitting.value = false
  }
}

function refreshData(): void {
  void loadData()
}

function handlePageChange(page: number): void {
  pagination.page = page
  void loadData()
}

function handlePageSizeChange(pageSize: number): void {
  pagination.page = 1
  pagination.page_size = pageSize
  void loadData()
}

function openPreview(image: GeneratedImage): void {
  previewImage.value = image
}

function closePreview(): void {
  previewImage.value = null
}

function formatDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B'

  const units = ['B', 'KB', 'MB', 'GB']
  let size = value
  let unitIndex = 0

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024
    unitIndex += 1
  }

  const precision = unitIndex === 0 ? 0 : 1
  return `${size.toFixed(precision)} ${units[unitIndex]}`
}

function displayText(value: string): string {
  const text = value?.trim()
  return text || '-'
}

function formatGroups(values: string[]): string {
  if (!Array.isArray(values) || values.length === 0) return '-'
  return values.filter(Boolean).join(', ') || '-'
}

onMounted(() => {
  void loadChannelGroups()
  void loadData()
})

onUnmounted(() => {
  requestSeq += 1
  listAbortController?.abort()
  abortContentLoads()
  revokeAllUrls()
})
</script>

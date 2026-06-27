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
        <button
          type="button"
          class="btn btn-secondary w-full justify-center md:w-auto"
          :disabled="loading"
          @click="refreshData"
        >
          <Icon name="refresh" size="sm" class="mr-2" />
          {{ t('admin.generatedImages.refresh') }}
        </button>
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
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.userId') }}</dt>
                  <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatId(image.user_id) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.apiKeyId') }}</dt>
                  <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatId(image.api_key_id) }}</dd>
                </div>
                <div>
                  <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.accountId') }}</dt>
                  <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatId(image.account_id) }}</dd>
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
      <div class="flex max-h-full w-full max-w-5xl flex-col overflow-hidden rounded-lg bg-white shadow-xl dark:bg-dark-800">
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

        <div class="min-h-0 overflow-auto bg-gray-950">
          <img
            v-if="imageUrls.get(previewImage.id)"
            data-test="generated-image-preview"
            :src="imageUrls.get(previewImage.id)"
            :alt="previewImage.prompt || t('admin.generatedImages.preview')"
            class="mx-auto max-h-[72vh] w-auto max-w-full object-contain"
          />
        </div>

        <div class="grid gap-4 border-t border-gray-200 p-4 dark:border-dark-700 lg:grid-cols-[1fr_320px]">
          <div class="min-w-0 space-y-2">
            <div>
              <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ t('admin.generatedImages.prompt') }}
              </p>
              <p class="mt-1 whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-200">
                {{ previewImage.prompt || '-' }}
              </p>
            </div>
            <div v-if="previewImage.revised_prompt">
              <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-gray-400">
                {{ t('admin.generatedImages.revisedPrompt') }}
              </p>
              <p class="mt-1 whitespace-pre-wrap text-sm text-gray-800 dark:text-gray-200">
                {{ previewImage.revised_prompt }}
              </p>
            </div>
          </div>

          <dl class="grid grid-cols-2 gap-3 text-xs">
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.provider') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ previewImage.provider || '-' }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.size') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatBytes(previewImage.size_bytes) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.responseFormat') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ previewImage.response_format || '-' }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.userId') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatId(previewImage.user_id) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.apiKeyId') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatId(previewImage.api_key_id) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.accountId') }}</dt>
              <dd class="mt-0.5 font-medium text-gray-800 dark:text-gray-200">{{ formatId(previewImage.account_id) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.generatedImages.requestId') }}</dt>
              <dd class="mt-0.5 truncate font-medium text-gray-800 dark:text-gray-200">{{ previewImage.request_id || '-' }}</dd>
            </div>
          </dl>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI, type GeneratedImage } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
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
  page_size: 24,
  total: 0,
  pages: 0
})

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
      {
        page: pagination.page,
        page_size: pagination.page_size
      },
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

function formatId(value: number): string {
  return value > 0 ? `#${value}` : '-'
}

onMounted(() => {
  void loadData()
})

onUnmounted(() => {
  requestSeq += 1
  listAbortController?.abort()
  abortContentLoads()
  revokeAllUrls()
})
</script>

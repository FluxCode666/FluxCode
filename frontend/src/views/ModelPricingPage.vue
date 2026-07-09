<template>
  <div class="min-h-screen overflow-x-hidden bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="mx-auto w-full max-w-7xl px-4 pb-16 pt-24 sm:px-6 lg:px-8">
      <div class="mb-8 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
        <div class="max-w-5xl">
          <h1 class="text-3xl font-semibold tracking-tight text-gray-950 dark:text-white sm:text-4xl">
            {{ t('modelPricing.title', '模型定价') }}
          </h1>
          <p class="mt-3 max-w-2xl text-sm leading-6 text-gray-600 dark:text-dark-300">
            {{ t('modelPricing.description', '按模型查看不同分组的调用价格') }}
          </p>
        </div>

        <div class="grid w-full gap-2 sm:grid-cols-[minmax(220px,1fr)_160px_160px] lg:max-w-2xl">
          <input
            v-model="searchInput"
            data-testid="model-pricing-search"
            class="input"
            :placeholder="t('modelPricing.searchPlaceholder', '搜索模型、平台或能力')"
          />
          <Select
            :model-value="platformFilter"
            :options="platformOptions"
            @update:modelValue="setPlatformFilter"
          />
          <Select
            :model-value="capabilityFilter"
            :options="capabilityOptions"
            @update:modelValue="setCapabilityFilter"
          />
        </div>
      </div>

      <div
        v-if="error"
        class="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/30 dark:text-red-300"
      >
        <div class="flex items-center justify-between gap-3">
          <span>{{ t('modelPricing.queryError', '查询异常') }}</span>
          <button
            data-testid="model-pricing-retry"
            type="button"
            class="btn btn-secondary"
            @click="loadModels"
          >
            {{ t('common.retry', '重试') }}
          </button>
        </div>
      </div>

      <section v-else>
        <div v-if="loading" class="rounded-2xl border border-black/5 bg-white/70 p-8 text-sm text-gray-500 shadow-sm dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-300">
          {{ t('common.loading', '加载中...') }}
        </div>
        <div v-else-if="models.length === 0" class="rounded-2xl border border-black/5 bg-white/70 p-8 text-sm text-gray-500 shadow-sm dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-300">
          {{ t('modelPricing.empty', '未找到匹配模型') }}
        </div>

        <div v-else class="grid grid-flow-dense gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <button
            v-for="model in models"
            :key="model.id"
            :data-testid="`model-card-${model.id}`"
            type="button"
            class="group relative flex min-h-[220px] w-full flex-col overflow-hidden rounded-2xl border border-black/5 bg-white/75 p-5 text-left shadow-sm transition duration-300 hover:-translate-y-1 hover:border-primary-300 hover:bg-white hover:shadow-xl hover:shadow-black/5 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 dark:border-white/10 dark:bg-dark-900/45 dark:hover:bg-dark-900/70"
            @click="selectModel(model.id)"
          >
            <div class="pointer-events-none absolute inset-x-0 top-0 h-24 bg-gradient-to-b from-primary-100/50 to-transparent opacity-0 transition-opacity duration-500 group-hover:opacity-100 dark:from-primary-900/20"></div>

            <div class="relative flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="truncate text-lg font-semibold text-gray-950 dark:text-white">
                  {{ model.display_name || model.id }}
                </div>
                <div class="mt-2 text-xs uppercase tracking-[0.2em] text-gray-500 dark:text-dark-400">
                  {{ model.platform }}
                </div>
              </div>
              <span class="rounded-full bg-gray-950 px-2.5 py-1 text-xs font-semibold text-white dark:bg-white dark:text-gray-950">
                {{ model.supported_group_count }}
              </span>
            </div>

            <div class="relative mt-5 flex flex-wrap gap-1.5">
              <span
                v-for="capability in model.capabilities"
                :key="capability"
                class="rounded-full bg-black/5 px-2.5 py-1 text-xs text-gray-700 dark:bg-white/10 dark:text-dark-100"
              >
                {{ capabilityLabel(capability) }}
              </span>
            </div>

            <div class="relative mt-auto grid grid-cols-2 gap-3 pt-6 text-xs">
              <div class="rounded-xl bg-black/[0.03] p-3 dark:bg-white/[0.06]">
                <div class="text-gray-500 dark:text-dark-400">{{ t('modelPricing.input', '输入') }}</div>
                <div class="mt-1 font-semibold text-gray-950 dark:text-white">
                  {{ formatTokenPrice(summaryDisplayPrice(model).input_price) }}
                </div>
              </div>
              <div class="rounded-xl bg-black/[0.03] p-3 dark:bg-white/[0.06]">
                <div class="text-gray-500 dark:text-dark-400">{{ t('modelPricing.output', '输出') }}</div>
                <div class="mt-1 font-semibold text-gray-950 dark:text-white">
                  {{ formatTokenPrice(summaryDisplayPrice(model).output_price) }}
                </div>
              </div>
            </div>
          </button>
        </div>
      </section>
    </main>

    <BaseDialog
      :show="detailModalOpen"
      :title="detailTitle"
      width="extra-wide"
      :close-on-click-outside="true"
      @close="closeDetailModal"
    >
      <div data-testid="model-pricing-detail-modal">
        <div v-if="detailLoading" class="py-12 text-center text-sm text-gray-500 dark:text-dark-300">
          {{ t('common.loading', '加载中...') }}
        </div>
        <div
          v-else-if="detailError"
          class="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/40 dark:bg-red-950/30 dark:text-red-300"
        >
          <div class="flex items-center justify-between gap-3">
            <span>{{ t('modelPricing.queryError', '查询异常') }}</span>
            <button
              data-testid="model-pricing-detail-retry"
              type="button"
              class="btn btn-secondary"
              @click="retrySelectedModel"
            >
              {{ t('common.retry', '重试') }}
            </button>
          </div>
        </div>
        <div v-else-if="!detail" class="py-12 text-center text-sm text-gray-500 dark:text-dark-300">
          {{ t('modelPricing.selectHint', '选择模型查看分组价格') }}
        </div>
        <div v-else>
          <div class="mb-5 flex flex-col gap-3 border-b border-black/5 pb-5 dark:border-white/10 md:flex-row md:items-end md:justify-between">
            <div>
              <h2 class="text-2xl font-semibold text-gray-950 dark:text-white">{{ detail.display_name || detail.id }}</h2>
              <div class="mt-2 flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-dark-400">
                <span>{{ detail.platform }}</span>
                <span
                  v-for="capability in detail.capabilities"
                  :key="capability"
                  class="rounded-full bg-black/5 px-2.5 py-1 text-xs text-gray-700 dark:bg-white/10 dark:text-dark-100"
                >
                  {{ capabilityLabel(capability) }}
                </span>
              </div>
            </div>
            <div class="grid grid-cols-2 gap-2 text-xs">
              <div class="rounded-xl bg-black/[0.03] px-3 py-2 dark:bg-white/[0.06]">
                <div class="text-gray-500 dark:text-dark-400">{{ t('modelPricing.input', '输入') }}</div>
                <div class="mt-1 font-semibold text-gray-950 dark:text-white">
                  {{ formatTokenPrice(detail.official_price.input_price) }}
                </div>
              </div>
              <div class="rounded-xl bg-black/[0.03] px-3 py-2 dark:bg-white/[0.06]">
                <div class="text-gray-500 dark:text-dark-400">{{ t('modelPricing.output', '输出') }}</div>
                <div class="mt-1 font-semibold text-gray-950 dark:text-white">
                  {{ formatTokenPrice(detail.official_price.output_price) }}
                </div>
              </div>
            </div>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full text-sm">
              <thead>
                <tr class="border-b border-black/5 text-left text-xs text-gray-500 dark:border-white/10 dark:text-dark-400">
                  <th class="py-2 pr-4">{{ t('modelPricing.group', '分组') }}</th>
                  <th class="py-2 pr-4">{{ t('modelPricing.rate', '倍率') }}</th>
                  <th class="py-2 pr-4">{{ t('modelPricing.input', '输入') }}</th>
                  <th class="py-2 pr-4">{{ t('modelPricing.output', '输出') }}</th>
                  <th class="py-2 pr-4">{{ t('modelPricing.cacheWrite', '缓存写入') }}</th>
                  <th class="py-2 pr-4">{{ t('modelPricing.cacheRead', '缓存读取') }}</th>
                  <th class="py-2 pr-4">{{ t('modelPricing.requestOrImage', '按次/图片') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="group in detail.groups"
                  :key="group.group_id"
                  class="border-b border-black/5 dark:border-white/10"
                >
                  <td class="py-3 pr-4 font-medium text-gray-950 dark:text-white">{{ group.group_name }}</td>
                  <td class="py-3 pr-4">{{ group.rate_multiplier.toFixed(2) }}x</td>
                  <td class="py-3 pr-4">{{ formatPrice(group.price.input_price) }}</td>
                  <td class="py-3 pr-4">{{ formatPrice(group.price.output_price) }}</td>
                  <td class="py-3 pr-4">
                    {{ formatPrice(group.price.cache_write_price) }}
                  </td>
                  <td class="py-3 pr-4">
                    {{ formatPrice(group.price.cache_read_price) }}
                  </td>
                  <td class="py-3 pr-4">
                    {{
                      formatPrice(group.price.per_request_price || group.price.image_output_price)
                    }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { modelPricingAPI, type ModelCapability, type ModelPricingDetail, type ModelPricingSummary } from '@/api/modelPricing'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')
const models = ref<ModelPricingSummary[]>([])
const detail = ref<ModelPricingDetail | null>(null)
const selectedModelId = ref('')
const detailModalOpen = ref(false)
const searchInput = ref('')
const debouncedSearch = ref('')
const platformFilter = ref('')
const capabilityFilter = ref<ModelCapability | ''>('')
const loading = ref(false)
const detailLoading = ref(false)
const error = ref(false)
const detailError = ref(false)

let searchTimer: number | null = null
let listAbortController: AbortController | null = null
let detailAbortController: AbortController | null = null

const platforms = computed(() =>
  Array.from(new Set(models.value.flatMap((model) => model.platforms?.length ? model.platforms : [model.platform]))).sort()
)

const platformOptions = computed(() => [
  { value: '', label: t('modelPricing.allPlatforms', '全部平台') },
  ...platforms.value.map((platform) => ({ value: platform, label: platform }))
])

const capabilityOptions = computed(() => [
  { value: '', label: t('modelPricing.allCapabilities', '全部能力') },
  { value: 'streaming', label: t('modelPricing.capabilities.streaming', '流式输出') },
  { value: 'system_prompt', label: t('modelPricing.capabilities.system_prompt', '系统提示词') },
  { value: 'function_calling', label: t('modelPricing.capabilities.function_calling', '函数调用') },
  { value: 'tools', label: t('modelPricing.capabilities.tools', '工具') },
  { value: 'json_mode', label: t('modelPricing.capabilities.json_mode', 'JSON 模式') },
  { value: 'structured_output', label: t('modelPricing.capabilities.structured_output', '结构化输出') },
  { value: 'prompt_cache', label: t('modelPricing.capabilities.prompt_cache', '提示词缓存') },
  { value: 'vision', label: t('modelPricing.capabilities.vision', '视觉理解') },
  { value: 'image_generation', label: t('modelPricing.capabilities.image_generation', '图片生成') },
  { value: 'video_generation', label: t('modelPricing.capabilities.video_generation', '视频生成') },
  { value: 'audio_input', label: t('modelPricing.capabilities.audio_input', '音频输入') },
  { value: 'audio_output', label: t('modelPricing.capabilities.audio_output', '音频输出') }
])

const detailTitle = computed(() => detail.value?.display_name || selectedModelId.value || t('modelPricing.title', '模型定价'))

watch(searchInput, (value) => {
  if (searchTimer) {
    window.clearTimeout(searchTimer)
  }
  searchTimer = window.setTimeout(() => {
    debouncedSearch.value = value.trim()
  }, 300)
})

watch([debouncedSearch, platformFilter, capabilityFilter], () => {
  loadModels()
})

function isCanceledError(error: unknown): boolean {
  const candidate = error as { code?: string; name?: string; message?: string }
  return candidate?.code === 'ERR_CANCELED' || candidate?.name === 'CanceledError' || candidate?.message === 'canceled'
}

function setPlatformFilter(value: string | number | boolean | null) {
  platformFilter.value = typeof value === 'string' ? value : ''
}

function setCapabilityFilter(value: string | number | boolean | null) {
  if (isModelCapability(value)) {
    capabilityFilter.value = value
    return
  }
  capabilityFilter.value = ''
}

function isModelCapability(value: unknown): value is ModelCapability {
  return typeof value === 'string' && value !== '' && capabilityOptions.value.some((option) => option.value === value)
}

async function loadModels() {
  listAbortController?.abort()
  const controller = new AbortController()
  listAbortController = controller

  loading.value = true
  error.value = false

  try {
    const nextModels = await modelPricingAPI.listModels(
      {
        q: debouncedSearch.value,
        platform: platformFilter.value,
        capability: capabilityFilter.value
      },
      { signal: controller.signal }
    )

    if (listAbortController !== controller) {
      return
    }

    models.value = nextModels

    if (!models.value.some((model) => model.id === selectedModelId.value)) {
      selectedModelId.value = ''
      detail.value = null
      detailError.value = false
      detailModalOpen.value = false
    }
  } catch (caughtError) {
    if (listAbortController !== controller || isCanceledError(caughtError)) {
      return
    }
    error.value = true
  } finally {
    if (listAbortController === controller) {
      loading.value = false
    }
  }
}

async function selectModel(modelId: string) {
  detailAbortController?.abort()
  const controller = new AbortController()
  detailAbortController = controller

  selectedModelId.value = modelId
  detailModalOpen.value = true
  detailLoading.value = true
  detailError.value = false
  detail.value = null

  try {
    const nextDetail = await modelPricingAPI.getModel(modelId, { signal: controller.signal })
    if (detailAbortController !== controller) {
      return
    }
    detail.value = nextDetail
  } catch (caughtError) {
    if (detailAbortController !== controller || isCanceledError(caughtError)) {
      return
    }
    detail.value = null
    detailError.value = true
  } finally {
    if (detailAbortController === controller) {
      detailLoading.value = false
    }
  }
}

function closeDetailModal() {
  detailModalOpen.value = false
}

function retrySelectedModel() {
  if (!selectedModelId.value) return
  selectModel(selectedModelId.value)
}

function capabilityLabel(capability: string): string {
  if (capability === 'streaming') return t('modelPricing.capabilities.streaming', '流式输出')
  if (capability === 'system_prompt') return t('modelPricing.capabilities.system_prompt', '系统提示词')
  if (capability === 'function_calling') return t('modelPricing.capabilities.function_calling', '函数调用')
  if (capability === 'tools') return t('modelPricing.capabilities.tools', '工具')
  if (capability === 'json_mode') return t('modelPricing.capabilities.json_mode', 'JSON 模式')
  if (capability === 'structured_output') return t('modelPricing.capabilities.structured_output', '结构化输出')
  if (capability === 'prompt_cache') return t('modelPricing.capabilities.prompt_cache', '提示词缓存')
  if (capability === 'vision') return t('modelPricing.capabilities.vision', '视觉理解')
  if (capability === 'image_generation') return t('modelPricing.capabilities.image_generation', '图片生成')
  if (capability === 'video_generation') return t('modelPricing.capabilities.video_generation', '视频生成')
  if (capability === 'audio_input') return t('modelPricing.capabilities.audio_input', '音频输入')
  if (capability === 'audio_output') return t('modelPricing.capabilities.audio_output', '音频输出')
  return capability
}

function formatTokenPrice(value: number): string {
  if (!value) return '-'
  return `$${(value * 1_000_000).toPrecision(6)}/M`
}

function summaryDisplayPrice(model: ModelPricingSummary) {
  return model.lowest_group_price || model.official_price
}

function formatPrice(price: number): string {
  if (!price) return '-'
  return price < 0.001 ? formatTokenPrice(price) : `$${price.toFixed(6)}`
}

onMounted(() => {
  appStore.fetchPublicSettings?.()
  loadModels()
})

onBeforeUnmount(() => {
  listAbortController?.abort()
  detailAbortController?.abort()
  if (searchTimer) {
    window.clearTimeout(searchTimer)
  }
})
</script>

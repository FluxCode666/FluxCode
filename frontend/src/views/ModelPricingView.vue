<template>
  <div class="min-h-screen bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="mx-auto max-w-7xl px-4 pb-16 pt-24 sm:px-6 lg:px-8">
      <div class="mb-6 flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">
            {{ t('modelPricing.title', '模型定价') }}
          </h1>
          <p class="mt-2 text-sm text-gray-600 dark:text-dark-300">
            {{ t('modelPricing.description', '按模型查看不同分组的调用价格') }}
          </p>
        </div>

        <div class="grid gap-2 sm:grid-cols-[minmax(220px,1fr)_160px_160px]">
          <input
            v-model="searchInput"
            data-testid="model-pricing-search"
            class="input"
            :placeholder="t('modelPricing.searchPlaceholder', '搜索模型、平台或能力')"
          />
          <select v-model="platformFilter" class="input">
            <option value="">{{ t('modelPricing.allPlatforms', '全部平台') }}</option>
            <option v-for="platform in platforms" :key="platform" :value="platform">{{ platform }}</option>
          </select>
          <select v-model="capabilityFilter" class="input">
            <option value="">{{ t('modelPricing.allCapabilities', '全部能力') }}</option>
            <option value="chat">{{ t('modelPricing.capabilities.chat', '对话') }}</option>
            <option value="image">{{ t('modelPricing.capabilities.image', '图片') }}</option>
            <option value="video">{{ t('modelPricing.capabilities.video', '视频') }}</option>
          </select>
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

      <div v-else class="grid gap-5 lg:grid-cols-[minmax(0,380px)_1fr]">
        <section class="rounded-3xl border border-black/5 bg-white/70 p-3 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
          <div v-if="loading" class="p-6 text-sm text-gray-500 dark:text-dark-300">
            {{ t('common.loading', '加载中...') }}
          </div>
          <div v-else-if="models.length === 0" class="p-6 text-sm text-gray-500 dark:text-dark-300">
            {{ t('modelPricing.empty', '未找到匹配模型') }}
          </div>

          <button
            v-for="model in models"
            :key="model.id"
            :data-testid="`model-card-${model.id}`"
            type="button"
            class="mb-2 block w-full rounded-2xl border p-3 text-left transition hover:border-primary-300 hover:bg-white dark:hover:bg-dark-900/60"
            :class="
              selectedModelId === model.id
                ? 'border-primary-400 bg-white dark:border-primary-600 dark:bg-dark-900/70'
                : 'border-black/5 bg-white/40 dark:border-white/10 dark:bg-dark-900/20'
            "
            @click="selectModel(model.id)"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="truncate text-sm font-semibold">{{ model.display_name || model.id }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ model.platform }}</div>
              </div>
              <span
                class="rounded-full bg-black/5 px-2 py-0.5 text-xs text-gray-700 dark:bg-white/10 dark:text-dark-200"
              >
                {{ model.supported_group_count }}
              </span>
            </div>

            <div class="mt-3 flex flex-wrap gap-1">
              <span
                v-for="capability in model.capabilities"
                :key="capability"
                class="rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10"
              >
                {{ capabilityLabel(capability) }}
              </span>
            </div>

            <div class="mt-3 text-xs text-gray-500 dark:text-dark-400">
              {{ t('modelPricing.inputOutput', '输入/输出') }}:
              {{ formatTokenPrice(model.official_price.input_price) }} /
              {{ formatTokenPrice(model.official_price.output_price) }}
            </div>
          </button>
        </section>

        <section class="rounded-3xl border border-black/5 bg-white/70 p-4 shadow-sm dark:border-white/10 dark:bg-dark-900/40">
          <div v-if="detailLoading" class="text-sm text-gray-500 dark:text-dark-300">
            {{ t('common.loading', '加载中...') }}
          </div>
          <div v-else-if="!detail" class="text-sm text-gray-500 dark:text-dark-300">
            {{ t('modelPricing.selectHint', '选择模型查看分组价格') }}
          </div>
          <div v-else>
            <div class="mb-4">
              <h2 class="text-xl font-semibold">{{ detail.display_name || detail.id }}</h2>
              <div class="mt-2 flex flex-wrap items-center gap-2 text-sm text-gray-500 dark:text-dark-400">
                <span>{{ detail.platform }}</span>
                <span
                  v-for="capability in detail.capabilities"
                  :key="capability"
                  class="rounded-full bg-black/5 px-2 py-0.5 text-xs dark:bg-white/10"
                >
                  {{ capabilityLabel(capability) }}
                </span>
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
                    <td class="py-3 pr-4 font-medium">{{ group.group_name }}</td>
                    <td class="py-3 pr-4">{{ group.rate_multiplier.toFixed(2) }}x</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.input_price, group.multipliers.input_price) }}</td>
                    <td class="py-3 pr-4">{{ formatPriceWithMultiplier(group.price.output_price, group.multipliers.output_price) }}</td>
                    <td class="py-3 pr-4">
                      {{ formatPriceWithMultiplier(group.price.cache_write_price, group.multipliers.cache_write_price) }}
                    </td>
                    <td class="py-3 pr-4">
                      {{ formatPriceWithMultiplier(group.price.cache_read_price, group.multipliers.cache_read_price) }}
                    </td>
                    <td class="py-3 pr-4">
                      {{
                        formatPriceWithMultiplier(
                          group.price.per_request_price || group.price.image_output_price,
                          group.multipliers.per_request_price || group.multipliers.image_output_price
                        )
                      }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import { modelPricingAPI, type ModelCapability, type ModelPricingDetail, type ModelPricingSummary } from '@/api/modelPricing'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')
const models = ref<ModelPricingSummary[]>([])
const detail = ref<ModelPricingDetail | null>(null)
const selectedModelId = ref('')
const searchInput = ref('')
const debouncedSearch = ref('')
const platformFilter = ref('')
const capabilityFilter = ref<ModelCapability | ''>('')
const loading = ref(false)
const detailLoading = ref(false)
const error = ref(false)

let searchTimer: ReturnType<typeof setTimeout> | null = null
let listAbortController: AbortController | null = null
let detailAbortController: AbortController | null = null

const platforms = computed(() => Array.from(new Set(models.value.map((model) => model.platform))).sort())

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

async function loadModels() {
  listAbortController?.abort()
  listAbortController = new AbortController()

  loading.value = true
  error.value = false

  try {
    models.value = await modelPricingAPI.listModels(
      {
        q: debouncedSearch.value,
        platform: platformFilter.value,
        capability: capabilityFilter.value
      },
      { signal: listAbortController.signal }
    )

    if (!models.value.some((model) => model.id === selectedModelId.value)) {
      selectedModelId.value = ''
      detail.value = null
    }
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

async function selectModel(modelId: string) {
  detailAbortController?.abort()
  detailAbortController = new AbortController()

  selectedModelId.value = modelId
  detailLoading.value = true

  try {
    detail.value = await modelPricingAPI.getModel(modelId, { signal: detailAbortController.signal })
  } finally {
    detailLoading.value = false
  }
}

function capabilityLabel(capability: string): string {
  if (capability === 'chat') return t('modelPricing.capabilities.chat', '对话')
  if (capability === 'image') return t('modelPricing.capabilities.image', '图片')
  if (capability === 'video') return t('modelPricing.capabilities.video', '视频')
  return capability
}

function formatTokenPrice(value: number): string {
  if (!value) return '-'
  return `$${(value * 1_000_000).toPrecision(6)}/MTok`
}

function formatPriceWithMultiplier(price: number, multiplier: number): string {
  if (!price) return '-'
  const priceText = price < 0.001 ? formatTokenPrice(price) : `$${price.toFixed(6)}`
  return `${priceText} · ${multiplier.toFixed(2)}x`
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

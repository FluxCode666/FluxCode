<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MediaGenerationSettings, MediaTimeoutBillingPolicy } from '@/api/admin/settings'

const props = defineProps<{ modelValue: MediaGenerationSettings }>()
const emit = defineEmits<{
  'update:modelValue': [value: MediaGenerationSettings]
}>()
const { t } = useI18n()

const penaltyPercent = computed(() =>
  Math.round(props.modelValue.media_sync_timeout_penalty_ratio * 100)
)

function update<K extends keyof MediaGenerationSettings>(
  key: K,
  value: MediaGenerationSettings[K]
) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function updateTimeout(event: Event) {
  const input = event.target as HTMLInputElement
  const value = Math.max(0, Math.floor(Number(input.value) || 0))
  update('media_sync_wait_timeout_seconds', value)
}

function updatePenalty(event: Event) {
  const input = event.target as HTMLInputElement
  const value = Math.min(100, Math.max(0, Number(input.value) || 0))
  update('media_sync_timeout_penalty_ratio', value / 100)
}

function updateBillingPolicy(event: Event) {
  const value = (event.target as HTMLSelectElement).value as MediaTimeoutBillingPolicy
  update('media_sync_timeout_billing_policy', value)
}
</script>

<template>
  <section data-test="media-generation-settings" class="card">
    <div class="border-b border-gray-100 px-4 py-4 sm:px-6 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.mediaGeneration.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.mediaGeneration.description') }}
      </p>
    </div>

    <div class="space-y-5 px-4 py-5 sm:px-6">
      <div class="space-y-2">
        <label
          for="media-sync-timeout"
          class="block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t('admin.settings.mediaGeneration.syncWaitTimeout') }}
        </label>
        <input
          id="media-sync-timeout"
          data-test="media-sync-timeout"
          class="input w-full sm:max-w-xs"
          type="number"
          min="0"
          step="1"
          :aria-describedby="modelValue.media_sync_wait_timeout_seconds === 0 ? 'media-timeout-disabled-warning' : undefined"
          :value="modelValue.media_sync_wait_timeout_seconds"
          @input="updateTimeout"
        />
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.mediaGeneration.syncWaitTimeoutHint') }}
        </p>
        <p
          v-if="modelValue.media_sync_wait_timeout_seconds === 0"
          id="media-timeout-disabled-warning"
          aria-live="polite"
          class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {{ t('admin.settings.mediaGeneration.timeoutDisabledWarning') }}
        </p>
      </div>

      <label class="flex items-start justify-between gap-4">
        <span class="min-w-0">
          <span class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.mediaGeneration.fallbackAsync') }}
          </span>
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.mediaGeneration.fallbackAsyncHint') }}
          </span>
        </span>
        <input
          data-test="media-fallback-async"
          class="mt-1 h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          type="checkbox"
          :checked="modelValue.media_sync_timeout_fallback_async_enabled"
          @change="update('media_sync_timeout_fallback_async_enabled', ($event.target as HTMLInputElement).checked)"
        />
      </label>

      <div class="space-y-2">
        <label
          for="media-timeout-billing-policy"
          class="block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t('admin.settings.mediaGeneration.timeoutBillingPolicy') }}
        </label>
        <select
          id="media-timeout-billing-policy"
          data-test="media-timeout-billing-policy"
          class="input w-full sm:max-w-xs"
          :value="modelValue.media_sync_timeout_billing_policy"
          @change="updateBillingPolicy"
        >
          <option value="penalty">{{ t('admin.settings.mediaGeneration.penalty') }}</option>
          <option value="refund">{{ t('admin.settings.mediaGeneration.refund') }}</option>
        </select>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.mediaGeneration.timeoutBillingPolicyHint') }}
        </p>
      </div>

      <div v-if="modelValue.media_sync_timeout_billing_policy === 'penalty'" class="space-y-2">
        <label
          for="media-timeout-penalty-ratio"
          class="block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t('admin.settings.mediaGeneration.penaltyRatio') }}
        </label>
        <div class="flex w-full items-center gap-2 sm:max-w-xs">
          <input
            id="media-timeout-penalty-ratio"
            data-test="media-timeout-penalty-ratio"
            class="input min-w-0 flex-1"
            type="number"
            min="0"
            max="100"
            step="1"
            :value="penaltyPercent"
            @input="updatePenalty"
          />
          <span aria-hidden="true" class="shrink-0 text-sm text-gray-500">%</span>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.mediaGeneration.penaltyRatioHint') }}
        </p>
      </div>

      <div class="space-y-2">
        <label
          for="media-video-storage-mode"
          class="block text-sm font-medium text-gray-700 dark:text-gray-300"
        >
          {{ t('admin.settings.mediaGeneration.videoStorageMode') }}
        </label>
        <select
          id="media-video-storage-mode"
          data-test="media-video-storage-mode"
          class="input w-full sm:max-w-xs"
          disabled
          :value="modelValue.media_video_storage_mode"
        >
          <option value="hybrid">{{ t('admin.settings.mediaGeneration.hybrid') }}</option>
        </select>
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.mediaGeneration.videoStorageModeHint') }}
        </p>
      </div>

      <label class="flex items-start justify-between gap-4">
        <span class="min-w-0">
          <span class="block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.mediaGeneration.proxyFallback') }}
          </span>
          <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.mediaGeneration.proxyFallbackHint') }}
          </span>
        </span>
        <input
          data-test="media-proxy-fallback"
          class="mt-1 h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          type="checkbox"
          :checked="modelValue.media_video_proxy_fallback_enabled"
          @change="update('media_video_proxy_fallback_enabled', ($event.target as HTMLInputElement).checked)"
        />
      </label>
    </div>
  </section>
</template>

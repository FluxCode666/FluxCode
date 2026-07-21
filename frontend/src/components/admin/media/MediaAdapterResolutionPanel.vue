<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { MediaAdapterResolution } from '@/types'

const props = defineProps<{
  resolution?: MediaAdapterResolution | null
}>()

const { t } = useI18n()
const ready = computed(() => props.resolution?.status === 'ready')
</script>

<template>
  <section
    data-test="media-adapter-resolution"
    class="rounded-xl border border-gray-200 bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-800/40"
  >
    <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('admin.mediaModels.resolution.title') }}
    </h4>
    <p
      v-if="!resolution"
      data-test="media-adapter-resolution-pending"
      class="mt-2 text-xs leading-5 text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.mediaModels.resolution.pending') }}
    </p>
    <template v-else>
      <p
        :data-status="resolution.status"
        class="mt-2 text-xs font-medium"
        :class="ready
          ? 'text-emerald-700 dark:text-emerald-300'
          : 'text-amber-700 dark:text-amber-300'"
      >
        {{ t(`admin.mediaModels.resolution.status.${resolution.status}`) }}
      </p>
      <code
        v-if="resolution.resolved_adapter"
        class="mt-2 block rounded-md bg-white px-2 py-1 text-xs text-gray-800 dark:bg-dark-700 dark:text-gray-200"
      >
        {{ resolution.resolved_adapter }}
      </code>
      <p v-if="resolution.matched_by" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t(`admin.mediaModels.resolution.matchedBy.${resolution.matched_by}`) }}
        <span v-if="resolution.matched_family"> · {{ resolution.matched_family }}</span>
      </p>
      <ul
        v-if="resolution.capabilities"
        class="mt-3 space-y-1 text-xs text-gray-600 dark:text-gray-300"
      >
        <li>
          {{ t('admin.mediaModels.resolution.capabilities.sync') }}:
          {{ resolution.capabilities.sync_upstream ? t('common.yes') : t('common.no') }}
        </li>
        <li>
          {{ t('admin.mediaModels.resolution.capabilities.nativeAsync') }}:
          {{ resolution.capabilities.native_async_upstream ? t('common.yes') : t('common.no') }}
        </li>
        <li>
          {{ t('admin.mediaModels.resolution.capabilities.contentFetch') }}:
          {{ resolution.capabilities.content_fetch ? t('common.yes') : t('common.no') }}
        </li>
      </ul>
      <p
        v-if="!ready"
        role="status"
        class="mt-2 text-xs leading-5 text-amber-700 dark:text-amber-300"
      >
        {{ t(`admin.mediaModels.resolution.reason.${resolution.reason_code}`) }}
      </p>
    </template>
  </section>
</template>

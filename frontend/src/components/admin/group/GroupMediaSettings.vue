<template>
  <section data-test="group-media-settings" class="space-y-3">
    <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ t('admin.groups.mediaSettingsTitle') }}
    </h4>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.groups.allowImageGeneration') }}
      </span>
      <input
        data-test="allow-image-generation"
        type="checkbox"
        :checked="local.allow_image_generation"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
        @change="update('allow_image_generation', ($event.target as HTMLInputElement).checked)"
      />
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.groups.allowVideoGeneration') }}
      </span>
      <input
        data-test="allow-video-generation"
        type="checkbox"
        :checked="local.allow_video_generation"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
        @change="update('allow_video_generation', ($event.target as HTMLInputElement).checked)"
      />
    </label>

    <label class="flex items-center justify-between gap-4">
      <span class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.groups.mediaCrossPlatformEnabled') }}
      </span>
      <input
        data-test="media-cross-platform-enabled"
        type="checkbox"
        :checked="local.media_cross_platform_enabled"
        class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
        @change="update('media_cross_platform_enabled', ($event.target as HTMLInputElement).checked)"
      />
    </label>

    <p class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.groups.mediaCrossPlatformHint') }}
    </p>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import type { GroupMediaConfig } from '@/types'

const props = defineProps<{
  modelValue: GroupMediaConfig
}>()

const emit = defineEmits<{
  'update:modelValue': [value: GroupMediaConfig]
}>()

const { t } = useI18n()
const local = ref<GroupMediaConfig>({ ...props.modelValue })

watch(
  () => props.modelValue,
  (value) => {
    local.value = { ...value }
  },
  { deep: true },
)

const update = <K extends keyof GroupMediaConfig>(
  key: K,
  value: GroupMediaConfig[K],
) => {
  local.value = { ...local.value, [key]: value }
  emit('update:modelValue', { ...local.value })
}
</script>

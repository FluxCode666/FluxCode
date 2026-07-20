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

    <label v-if="platform !== 'media'" class="flex items-center justify-between gap-4">
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

    <p v-if="platform !== 'media'" class="text-xs text-gray-500 dark:text-gray-400">
      {{ t('admin.groups.mediaCrossPlatformHint') }}
    </p>

    <div v-if="platform === 'media'" class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600">
      <div>
        <h5 class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.groups.mediaModelScopesTitle') }}
        </h5>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.groups.mediaModelScopesHint') }}
        </p>
      </div>

      <div v-if="modelsLoading" data-test="media-model-scopes-loading" class="space-y-2">
        <div v-for="index in 3" :key="index" class="h-9 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-700"></div>
      </div>
      <div
        v-else-if="availableModels.length"
        data-test="media-model-scopes"
        class="grid max-h-56 gap-2 overflow-y-auto rounded-xl border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800 sm:grid-cols-2"
      >
        <label
          v-for="model in availableModels"
          :key="model.id"
          class="flex cursor-pointer items-start gap-2 rounded-lg border border-transparent bg-white px-3 py-2 transition hover:border-primary-200 dark:bg-dark-700 dark:hover:border-primary-800"
        >
          <input
            type="checkbox"
            class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            :value="model.model_id"
            :checked="selectedModelIds.includes(model.model_id)"
            :data-test="`media-model-scope-${model.model_id}`"
            @change="toggleModel(model.model_id, ($event.target as HTMLInputElement).checked)"
          />
          <span class="min-w-0">
            <span class="block truncate font-mono text-xs font-semibold text-gray-800 dark:text-gray-100">
              {{ model.model_id }}
            </span>
            <span class="mt-0.5 block text-[11px] text-gray-500 dark:text-gray-400">
              {{ model.vendor }} · {{ t(`admin.mediaModels.types.${model.media_type}`) }}
            </span>
          </span>
        </label>
      </div>
      <div v-else class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-300">
        {{ t('admin.groups.noMediaModels') }}
        <a href="/admin/media-models" class="ml-1 font-medium underline underline-offset-2">
          {{ t('admin.groups.manageMediaModels') }}
        </a>
      </div>

      <p v-if="availableModels.length" class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.groups.mediaModelsSelected', { count: selectedModelIds.length }) }}
      </p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import type { GroupMediaConfig, GroupPlatform, MediaModelDefinition } from '@/types'

const props = withDefaults(defineProps<{
  modelValue: GroupMediaConfig
  platform?: GroupPlatform
  availableModels?: MediaModelDefinition[]
  selectedModelIds?: string[]
  modelsLoading?: boolean
}>(), {
  availableModels: () => [],
  selectedModelIds: () => [],
  modelsLoading: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: GroupMediaConfig]
  'update:selectedModelIds': [value: string[]]
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

const toggleModel = (modelID: string, checked: boolean) => {
  const current = props.selectedModelIds || []
  const next = checked
    ? [...new Set([...current, modelID])]
    : current.filter((item) => item !== modelID)
  emit('update:selectedModelIds', next)
}
</script>

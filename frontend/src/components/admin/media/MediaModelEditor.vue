<script setup lang="ts">
import { computed, ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import MediaAdapterResolutionPanel from '@/components/admin/media/MediaAdapterResolutionPanel.vue'
import type {
  MediaAdapterResolution,
  MediaModelConstraints,
  MediaModelDefinitionInput,
  MediaOperation,
  MediaType,
} from '@/types'

const props = defineProps<{
  modelValue: MediaModelDefinitionInput
  editing?: boolean
  adapterResolution?: MediaAdapterResolution | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: MediaModelDefinitionInput]
  'update:valid': [value: boolean]
}>()

const { t } = useI18n()

const imageOperations: MediaOperation[] = ['text_to_image', 'image_to_image', 'image_edit']
const videoOperations: MediaOperation[] = [
  'text_to_video',
  'image_to_video',
  'reference_to_video',
  'video_extend',
  'video_remix',
]

const local = ref(cloneInput(props.modelValue))
const aliasesText = ref(formatList(props.modelValue.aliases))
const imageSizesText = ref(formatList(props.modelValue.constraints.image_sizes))
const videoDurationsText = ref(formatList(props.modelValue.constraints.video_durations))
const videoResolutionsText = ref(formatList(props.modelValue.constraints.video_resolutions))
let syncingFromParent = false
let lastEmittedObject: MediaModelDefinitionInput | null = null
const resolutionDirty = ref(false)

const availableOperations = computed(() =>
  local.value.media_type === 'image' ? imageOperations : videoOperations,
)

const validationErrors = computed(() => {
  const errors: string[] = []
  const value = local.value
  if (!/^[a-z0-9][a-z0-9._:/-]{0,127}$/.test(value.model_id.trim())) {
    errors.push(t('admin.mediaModels.validation.modelId'))
  }
  if (!value.vendor.trim()) errors.push(t('admin.mediaModels.validation.vendor'))
  if (!value.billing_unit.trim()) errors.push(t('admin.mediaModels.validation.billingUnit'))
  if (value.operations.length === 0) errors.push(t('admin.mediaModels.validation.operations'))
  const integerConstraints = [
    value.constraints.max_image_count,
    value.constraints.min_fps,
    value.constraints.max_fps,
    value.constraints.max_reference_images,
  ]
  if (integerConstraints.some((item) =>
    item !== undefined && (!Number.isInteger(item) || item < 0))) {
    errors.push(t('admin.mediaModels.validation.nonNegativeInteger'))
  }
  if (value.constraints.min_fps && value.constraints.max_fps &&
    value.constraints.min_fps > value.constraints.max_fps) {
    errors.push(t('admin.mediaModels.validation.fps'))
  }
  const aliases = parseList(aliasesText.value)
  if (new Set(aliases).size !== aliases.length || aliases.includes(value.model_id.trim())) {
    errors.push(t('admin.mediaModels.validation.aliases'))
  }
  return errors
})

function cloneConstraints(value: MediaModelConstraints | undefined): MediaModelConstraints {
  return {
    image_sizes: [...(value?.image_sizes || [])],
    max_image_count: value?.max_image_count || 0,
    video_durations: [...(value?.video_durations || [])],
    video_resolutions: [...(value?.video_resolutions || [])],
    min_fps: value?.min_fps || 0,
    max_fps: value?.max_fps || 0,
    max_reference_images: value?.max_reference_images || 0,
  }
}

function cloneInput(value: MediaModelDefinitionInput): MediaModelDefinitionInput {
  return {
    ...value,
    operations: [...value.operations],
    constraints: cloneConstraints(value.constraints),
    aliases: [...value.aliases],
  }
}

function formatList(value: Array<string | number> | undefined): string {
  return (value || []).join(', ')
}

function parseList(value: string): string[] {
  return value
    .split(/[\n,]/)
    .map((item) => item.trim().toLowerCase())
    .filter(Boolean)
}

function parseNumberList(value: string): number[] {
  return [...new Set(parseList(value).map(Number).filter((item) => Number.isInteger(item) && item > 0))]
}

function normalizeOptionalInteger(value: unknown): number {
  if (value === '' || value === null || value === undefined) return 0
  return typeof value === 'number' ? value : Number(value)
}

function normalizeIntegerConstraints() {
  const constraints = local.value.constraints
  constraints.max_image_count = normalizeOptionalInteger(constraints.max_image_count)
  constraints.min_fps = normalizeOptionalInteger(constraints.min_fps)
  constraints.max_fps = normalizeOptionalInteger(constraints.max_fps)
  constraints.max_reference_images = normalizeOptionalInteger(constraints.max_reference_images)
}

function publish() {
  if (syncingFromParent) return
  normalizeIntegerConstraints()
  local.value.model_id = local.value.model_id.trim().toLowerCase()
  local.value.vendor = local.value.vendor.trim().toLowerCase()
  local.value.billing_unit = local.value.billing_unit.trim().toLowerCase()
  local.value.aliases = parseList(aliasesText.value)
  const value = cloneInput(local.value)
  lastEmittedObject = value
  emit('update:modelValue', value)
  emit('update:valid', validationErrors.value.length === 0)
}

function hydrateDrafts(value: MediaModelDefinitionInput) {
  aliasesText.value = formatList(value.aliases)
  imageSizesText.value = formatList(value.constraints.image_sizes)
  videoDurationsText.value = formatList(value.constraints.video_durations)
  videoResolutionsText.value = formatList(value.constraints.video_resolutions)
}

function setMediaType(mediaType: MediaType) {
  if (local.value.media_type === mediaType) return
  if (props.editing) resolutionDirty.value = true
  local.value.media_type = mediaType
  local.value.operations = mediaType === 'image' ? ['text_to_image'] : ['text_to_video']
  local.value.constraints = cloneConstraints(undefined)
  imageSizesText.value = ''
  videoDurationsText.value = ''
  videoResolutionsText.value = ''
  publish()
}

function toggleOperation(operation: MediaOperation) {
  if (props.editing) resolutionDirty.value = true
  const index = local.value.operations.indexOf(operation)
  if (index >= 0) local.value.operations.splice(index, 1)
  else local.value.operations.push(operation)
  publish()
}

function updateVendor() {
  if (props.editing) resolutionDirty.value = true
  publish()
}

function updateStringList(key: 'image_sizes' | 'video_resolutions', draft: string) {
  local.value.constraints[key] = parseList(draft)
  publish()
}

function updateNumberList() {
  local.value.constraints.video_durations = parseNumberList(videoDurationsText.value)
  publish()
}

watch(
  () => props.modelValue,
  (value) => {
    if (toRaw(value) === lastEmittedObject) {
      lastEmittedObject = null
      return
    }
    lastEmittedObject = null
    syncingFromParent = true
    local.value = cloneInput(value)
    hydrateDrafts(value)
    syncingFromParent = false
    emit('update:valid', validationErrors.value.length === 0)
  },
  { deep: true },
)
</script>

<template>
  <div class="space-y-5" data-test="media-model-editor">
    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label for="media-registry-model-id" class="input-label">
          {{ t('admin.mediaModels.form.modelId') }}
        </label>
        <input
          id="media-registry-model-id"
          v-model="local.model_id"
          data-test="media-registry-model-id"
          class="input font-mono"
          type="text"
          :disabled="editing"
          :placeholder="t('admin.mediaModels.form.modelIdPlaceholder')"
          @input="publish"
        />
      </div>
      <div>
        <label for="media-registry-vendor" class="input-label">
          {{ t('admin.mediaModels.form.vendor') }}
        </label>
        <input
          id="media-registry-vendor"
          v-model="local.vendor"
          data-test="media-registry-vendor"
          class="input"
          type="text"
          :placeholder="t('admin.mediaModels.form.vendorPlaceholder')"
          @input="updateVendor"
        />
      </div>
    </div>

    <div>
      <span class="input-label">{{ t('admin.mediaModels.form.mediaType') }}</span>
      <div class="grid grid-cols-2 gap-2 rounded-xl bg-gray-100 p-1 dark:bg-dark-700">
        <button
          v-for="mediaType in (['image', 'video'] as MediaType[])"
          :key="mediaType"
          type="button"
          class="rounded-lg px-4 py-2 text-sm font-medium transition"
          :class="local.media_type === mediaType
            ? 'bg-white text-rose-600 shadow-sm dark:bg-dark-600 dark:text-rose-400'
            : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
          :data-test="`media-registry-type-${mediaType}`"
          @click="setMediaType(mediaType)"
        >
          {{ t(`admin.mediaModels.types.${mediaType}`) }}
        </button>
      </div>
    </div>

    <div>
      <span class="input-label">{{ t('admin.mediaModels.form.operations') }}</span>
      <div class="grid gap-2 sm:grid-cols-2">
        <label
          v-for="operation in availableOperations"
          :key="operation"
          class="flex cursor-pointer items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm text-gray-700 transition hover:border-primary-300 dark:border-dark-600 dark:text-gray-300"
        >
          <input
            type="checkbox"
            :checked="local.operations.includes(operation)"
            class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
            :data-test="`media-registry-operation-${operation}`"
            @change="toggleOperation(operation)"
          />
          {{ t(`admin.mediaModels.operations.${operation}`) }}
        </label>
      </div>
    </div>

    <div>
      <MediaAdapterResolutionPanel :resolution="adapterResolution" />
      <p
        v-if="editing && resolutionDirty"
        data-test="media-adapter-resolution-dirty"
        class="mt-2 text-xs text-amber-700 dark:text-amber-300"
      >
        {{ t('admin.mediaModels.resolution.recalculateAfterSave') }}
      </p>
    </div>

    <div class="grid gap-4 sm:grid-cols-2">
      <div>
        <label for="media-registry-billing-unit" class="input-label">
          {{ t('admin.mediaModels.form.billingUnit') }}
        </label>
        <input
          id="media-registry-billing-unit"
          v-model="local.billing_unit"
          data-test="media-registry-billing-unit"
          class="input"
          type="text"
          :placeholder="local.media_type === 'image' ? 'image' : 'second'"
          @input="publish"
        />
      </div>
      <label class="flex items-end gap-3 pb-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="local.enabled"
          data-test="media-registry-enabled"
          type="checkbox"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          @change="publish"
        />
        {{ t('admin.mediaModels.form.enabled') }}
      </label>
    </div>

    <div>
      <label for="media-registry-aliases" class="input-label">
        {{ t('admin.mediaModels.form.aliases') }}
      </label>
      <textarea
        id="media-registry-aliases"
        v-model="aliasesText"
        data-test="media-registry-aliases"
        class="input min-h-20 font-mono text-xs"
        :placeholder="t('admin.mediaModels.form.aliasesPlaceholder')"
        @input="publish"
      />
      <p class="input-hint">{{ t('admin.mediaModels.form.aliasesHint') }}</p>
    </div>

    <fieldset class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
      <legend class="px-2 text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.mediaModels.form.constraints') }}
      </legend>

      <template v-if="local.media_type === 'image'">
        <div>
          <label for="media-registry-image-sizes" class="input-label">
            {{ t('admin.mediaModels.form.imageSizes') }}
          </label>
          <input
            id="media-registry-image-sizes"
            v-model="imageSizesText"
            data-test="media-registry-image-sizes"
            class="input"
            type="text"
            placeholder="1024x1024, 1536x1024"
            @input="updateStringList('image_sizes', imageSizesText)"
          />
        </div>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label for="media-registry-max-images" class="input-label">
              {{ t('admin.mediaModels.form.maxImageCount') }}
            </label>
            <input id="media-registry-max-images" v-model.number="local.constraints.max_image_count" class="input" min="0" step="1" type="number" @input="publish" />
          </div>
          <div>
            <label for="media-registry-max-reference-images" class="input-label">
              {{ t('admin.mediaModels.form.maxReferenceImages') }}
            </label>
            <input id="media-registry-max-reference-images" v-model.number="local.constraints.max_reference_images" class="input" min="0" step="1" type="number" @input="publish" />
          </div>
        </div>
      </template>

      <template v-else>
        <div class="grid gap-4 sm:grid-cols-2">
          <div>
            <label for="media-registry-video-durations" class="input-label">
              {{ t('admin.mediaModels.form.videoDurations') }}
            </label>
            <input
              id="media-registry-video-durations"
              v-model="videoDurationsText"
              data-test="media-registry-video-durations"
              class="input"
              type="text"
              placeholder="5, 10, 15"
              @input="updateNumberList"
            />
          </div>
          <div>
            <label for="media-registry-video-resolutions" class="input-label">
              {{ t('admin.mediaModels.form.videoResolutions') }}
            </label>
            <input
              id="media-registry-video-resolutions"
              v-model="videoResolutionsText"
              data-test="media-registry-video-resolutions"
              class="input"
              type="text"
              placeholder="720p, 1080p"
              @input="updateStringList('video_resolutions', videoResolutionsText)"
            />
          </div>
        </div>
        <div class="grid gap-4 sm:grid-cols-3">
          <div>
            <label for="media-registry-min-fps" class="input-label">{{ t('admin.mediaModels.form.minFps') }}</label>
            <input id="media-registry-min-fps" v-model.number="local.constraints.min_fps" class="input" min="0" step="1" type="number" @input="publish" />
          </div>
          <div>
            <label for="media-registry-max-fps" class="input-label">{{ t('admin.mediaModels.form.maxFps') }}</label>
            <input id="media-registry-max-fps" v-model.number="local.constraints.max_fps" class="input" min="0" step="1" type="number" @input="publish" />
          </div>
          <div>
            <label for="media-registry-max-video-references" class="input-label">{{ t('admin.mediaModels.form.maxReferenceImages') }}</label>
            <input id="media-registry-max-video-references" v-model.number="local.constraints.max_reference_images" class="input" min="0" step="1" type="number" @input="publish" />
          </div>
        </div>
      </template>
    </fieldset>

    <div
      v-if="validationErrors.length"
      data-test="media-registry-validation-errors"
      class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-300"
    >
      {{ validationErrors.join('；') }}
    </div>
  </div>
</template>

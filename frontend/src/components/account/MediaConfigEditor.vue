<script setup lang="ts">
import { ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type {
  MediaAccountConfig,
  MediaAccountModelOverride,
  NativeAsyncMode
} from '@/types'

interface OverrideRow {
  id: string
  model: string
  upstream_model: string
  native_async_mode: NativeAsyncMode | ''
}

const props = defineProps<{
  modelValue: MediaAccountConfig
}>()

const emit = defineEmits<{
  'update:modelValue': [value: MediaAccountConfig]
  'update:valid': [value: boolean]
}>()

const { t } = useI18n()

function cloneModelOverrides(
  source: Record<string, MediaAccountModelOverride> | undefined
): Record<string, MediaAccountModelOverride> {
  const result = Object.create(null) as Record<string, MediaAccountModelOverride>
  for (const [model, override] of Object.entries(source || {})) {
    result[model] = override
  }
  return result
}

const local = ref<MediaAccountConfig>({
  ...props.modelValue,
  model_overrides: cloneModelOverrides(props.modelValue.model_overrides)
})
const rows = ref<OverrideRow[]>([])
const duplicateModel = ref('')
let nextRowId = 0
let currentValid = true
let lastEmittedObject: MediaAccountConfig | null = null

function createRow(model = '', override: MediaAccountModelOverride = {}): OverrideRow {
  nextRowId += 1
  return {
    id: `media-override-${nextRowId}`,
    model,
    upstream_model: override.upstream_model || '',
    native_async_mode: override.native_async_mode || ''
  }
}

function setValidity(value: boolean) {
  if (currentValid === value) {
    return
  }
  currentValid = value
  emit('update:valid', value)
}

function findDuplicateModel(): string {
  const seen = new Set<string>()
  for (const row of rows.value) {
    const model = row.model.trim()
    if (!model) {
      continue
    }
    if (seen.has(model)) {
      return model
    }
    seen.add(model)
  }
  return ''
}

function refreshValidity(): boolean {
  duplicateModel.value = findDuplicateModel()
  const valid = !duplicateModel.value
  setValidity(valid)
  return valid
}

function hydrate(value: MediaAccountConfig) {
  local.value = {
    ...value,
    model_overrides: cloneModelOverrides(value.model_overrides)
  }
  rows.value = Object.entries(value.model_overrides || {}).map(([model, override]) =>
    createRow(model, override)
  )
  refreshValidity()
}

watch(
  () => props.modelValue,
  (value) => {
    if (toRaw(value) === lastEmittedObject) {
      lastEmittedObject = null
      return
    }
    lastEmittedObject = null
    hydrate(value)
  },
  { immediate: true, deep: true }
)

function buildModelOverrides(): Record<string, MediaAccountModelOverride> | null {
  if (!refreshValidity()) {
    return null
  }

  const result = cloneModelOverrides(undefined)

  for (const row of rows.value) {
    const model = row.model.trim()
    if (!model) {
      continue
    }
    const override: MediaAccountModelOverride = {}
    const upstreamModel = row.upstream_model.trim()
    if (upstreamModel) {
      override.upstream_model = upstreamModel
    }
    if (row.native_async_mode) {
      override.native_async_mode = row.native_async_mode
    }
    result[model] = override
  }

  return result
}

function publish(patch: Partial<MediaAccountConfig> = {}) {
  local.value = {
    ...local.value,
    ...patch
  }
  const modelOverrides = buildModelOverrides()
  if (!modelOverrides) {
    return
  }

  const next: MediaAccountConfig = {
    ...local.value,
    model_overrides: modelOverrides
  }
  const emittedValue: MediaAccountConfig = {
    ...next,
    model_overrides: cloneModelOverrides(next.model_overrides)
  }
  local.value = emittedValue
  lastEmittedObject = emittedValue
  emit('update:modelValue', emittedValue)
}

function updateAdapter(event: Event) {
  publish({ adapter: (event.target as HTMLInputElement).value })
}

function updateDefaultMode(event: Event) {
  publish({ native_async_mode: (event.target as HTMLSelectElement).value as NativeAsyncMode })
}

function addOverride() {
  rows.value.push(createRow())
}

function removeOverride(index: number) {
  rows.value.splice(index, 1)
  publish()
}
</script>

<template>
  <section
    data-test="media-config-editor"
    class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600"
    :aria-labelledby="'media-config-title'"
  >
    <div>
      <h3 id="media-config-title" class="text-sm font-semibold text-gray-900 dark:text-gray-100">
        {{ t('admin.accounts.mediaConfig.title') }}
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.mediaConfig.description') }}
      </p>
    </div>

    <div>
      <label for="media-adapter" class="input-label">
        {{ t('admin.accounts.mediaConfig.adapter') }}
      </label>
      <input
        id="media-adapter"
        data-test="media-adapter"
        class="input"
        type="text"
        :value="local.adapter"
        :placeholder="t('admin.accounts.mediaConfig.adapterPlaceholder')"
        aria-describedby="media-adapter-hint"
        @input="updateAdapter"
      />
      <p id="media-adapter-hint" class="input-hint">
        {{ t('admin.accounts.mediaConfig.adapterHint') }}
      </p>
    </div>

    <div>
      <label for="media-default-async-mode" class="input-label">
        {{ t('admin.accounts.mediaConfig.nativeAsyncMode') }}
      </label>
      <select
        id="media-default-async-mode"
        data-test="media-default-async-mode"
        class="input"
        :value="local.native_async_mode"
        aria-describedby="media-native-async-mode-hint"
        @change="updateDefaultMode"
      >
        <option value="unsupported">{{ t('admin.accounts.mediaConfig.modes.unsupported') }}</option>
        <option value="optional">{{ t('admin.accounts.mediaConfig.modes.optional') }}</option>
        <option value="required">{{ t('admin.accounts.mediaConfig.modes.required') }}</option>
      </select>
      <p id="media-native-async-mode-hint" class="input-hint">
        {{ t('admin.accounts.mediaConfig.nativeAsyncModeHint') }}
      </p>
    </div>

    <div v-if="rows.length" class="space-y-3">
      <div
        v-for="(row, index) in rows"
        :key="row.id"
        class="grid gap-2 rounded-lg border border-gray-200 p-3 dark:border-dark-600 md:grid-cols-[1fr_1fr_1fr_auto]"
      >
        <div>
          <label :for="`${row.id}-model`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.model') }}
          </label>
          <input
            :id="`${row.id}-model`"
            v-model="row.model"
            :data-test="`media-override-model-${index}`"
            class="input"
            type="text"
            @input="publish()"
          />
        </div>
        <div>
          <label :for="`${row.id}-upstream`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.upstreamModel') }}
          </label>
          <input
            :id="`${row.id}-upstream`"
            v-model="row.upstream_model"
            :data-test="`media-override-upstream-${index}`"
            class="input"
            type="text"
            @input="publish()"
          />
        </div>
        <div>
          <label :for="`${row.id}-mode`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.overrideAsyncMode') }}
          </label>
          <select
            :id="`${row.id}-mode`"
            v-model="row.native_async_mode"
            :data-test="`media-override-mode-${index}`"
            class="input"
            @change="publish()"
          >
            <option value="">{{ t('admin.accounts.mediaConfig.inherit') }}</option>
            <option value="unsupported">{{ t('admin.accounts.mediaConfig.modes.unsupported') }}</option>
            <option value="optional">{{ t('admin.accounts.mediaConfig.modes.optional') }}</option>
            <option value="required">{{ t('admin.accounts.mediaConfig.modes.required') }}</option>
          </select>
        </div>
        <button
          type="button"
          class="btn btn-secondary self-end"
          :data-test="`media-override-remove-${index}`"
          :aria-label="t('admin.accounts.mediaConfig.removeOverride')"
          @click="removeOverride(index)"
        >
          {{ t('admin.accounts.mediaConfig.remove') }}
        </button>
      </div>
    </div>

    <p
      v-if="duplicateModel"
      data-test="media-override-duplicate-error"
      class="text-sm text-red-600 dark:text-red-400"
      role="alert"
    >
      {{ t('admin.accounts.mediaConfig.duplicateModel', { model: duplicateModel }) }}
    </p>

    <button
      data-test="media-add-model-override"
      type="button"
      class="btn btn-secondary"
      @click="addOverride"
    >
      {{ t('admin.accounts.mediaConfig.addOverride') }}
    </button>
  </section>
</template>

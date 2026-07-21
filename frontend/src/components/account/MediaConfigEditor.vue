<script setup lang="ts">
import { computed, onMounted, ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  MediaAccountConfig,
  MediaAccountModelBinding,
  MediaBindingAsyncMode,
  MediaMappingOperation,
  MediaModelDefinition,
  MediaRequestMapping,
  MediaRequestMappingRule
} from '@/types'

interface ModelRow {
  id: string
  model: string
  enabled: boolean
  upstreamModelID: string
  asyncMode: MediaBindingAsyncMode
  requestMappingText: string
  requestMappingError: boolean
}

const props = defineProps<{
  modelValue: MediaAccountConfig
}>()

const emit = defineEmits<{
  'update:modelValue': [value: MediaAccountConfig]
  'update:valid': [value: boolean]
}>()

const { t } = useI18n()
const provider = ref('')
const rows = ref<ModelRow[]>([])
const registryModels = ref<MediaModelDefinition[]>([])
const registryLoading = ref(false)
const registryLoadFailed = ref(false)
const registryLoaded = ref(false)
const duplicateModel = ref('')
const missingModelFields = ref(false)
let nextRowID = 0
let currentValid = true
let lastEmittedObject: MediaAccountConfig | null = null

const registryModelsByID = computed(() => new Map(
  registryModels.value.map((model) => [model.model_id, model]),
))

async function loadRegistryModels() {
  const loader = adminAPI.mediaModels?.listEnabled
  if (!loader) return
  registryLoading.value = true
  registryLoadFailed.value = false
  registryLoaded.value = false
  refreshValidity()
  try {
    registryModels.value = await loader()
    registryLoaded.value = true
  } catch {
    registryModels.value = []
    registryLoadFailed.value = true
  } finally {
    registryLoading.value = false
    refreshValidity()
  }
}

function isSelectedByAnotherRow(modelID: string, current: ModelRow): boolean {
  return rows.value.some((row) => row !== current && row.model.trim().toLowerCase() === modelID)
}

function modelOptionLabel(model: MediaModelDefinition): string {
  return `${model.model_id} · ${model.vendor} · ${t(`admin.mediaModels.types.${model.media_type}`)}`
}

function selectedDefinition(row: ModelRow): MediaModelDefinition | undefined {
  return registryModelsByID.value.get(row.model.trim().toLowerCase())
}

function rowSupportsSelectedMode(row: ModelRow): boolean {
  if (!row.enabled) return true
  const capabilities = selectedDefinition(row)?.adapter_resolution.capabilities
  if (!capabilities) return false
  return row.asyncMode === 'native'
    ? capabilities.native_async_upstream
    : capabilities.sync_upstream
}

function rowModelUnavailable(row: ModelRow): boolean {
  return registryLoaded.value
    && Boolean(row.model.trim())
    && !selectedDefinition(row)
}

function mappingText(mapping: MediaRequestMapping | undefined): string {
  if (!mapping?.rules?.length) {
    return ''
  }
  return JSON.stringify(mapping, null, 2)
}

function createRow(model = '', binding?: MediaAccountModelBinding): ModelRow {
  nextRowID += 1
  return {
    id: `media-model-${nextRowID}`,
    model,
    enabled: binding?.enabled ?? true,
    upstreamModelID: binding?.upstream_model_id || '',
    asyncMode: binding?.async_mode || 'unsupported',
    requestMappingText: mappingText(binding?.request_mapping),
    requestMappingError: false
  }
}

function setValidity(value: boolean) {
  if (currentValid === value) return
  currentValid = value
  emit('update:valid', value)
}

const mappingOperations = new Set(['rename', 'copy', 'default', 'enum', 'cast'])
const mappingCasts = new Set(['string', 'number', 'integer', 'boolean'])
const mappingRuleKeys = new Set(['source', 'target', 'operation', 'value', 'values', 'cast'])
const safeMappingPath = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/

function isValidMappingRules(value: unknown): boolean {
  if (value === undefined) return true
  if (!Array.isArray(value)) return false
  const targets = new Set<string>()
  for (const item of value) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return false
    const rule = item as Record<string, unknown>
    if (Object.keys(rule).some((key) => !mappingRuleKeys.has(key))) return false
    const operation = typeof rule.operation === 'string' ? rule.operation.trim().toLowerCase() : ''
    const target = typeof rule.target === 'string' ? rule.target.trim() : ''
    if (!mappingOperations.has(operation) || !safeMappingPath.test(target) || targets.has(target)) return false
    targets.add(target)
    if (operation === 'rename' || operation === 'copy' || operation === 'enum' || operation === 'cast') {
      const source = typeof rule.source === 'string' ? rule.source.trim() : ''
      if (!safeMappingPath.test(source) || (operation === 'rename' && source === target)) return false
    }
    if (operation === 'enum') {
      if (!rule.values || typeof rule.values !== 'object' || Array.isArray(rule.values) ||
        Object.keys(rule.values as Record<string, unknown>).length === 0 ||
        Object.values(rule.values as Record<string, unknown>).some((mapped) => typeof mapped !== 'string')) return false
    }
    if (operation === 'cast' && (typeof rule.cast !== 'string' || !mappingCasts.has(rule.cast.trim().toLowerCase()))) {
      return false
    }
  }
  return true
}

function parseRequestMapping(row: ModelRow): MediaRequestMapping | null {
  const source = row.requestMappingText.trim()
  if (!source) {
    row.requestMappingError = false
    return {}
  }
  try {
    const parsed = JSON.parse(source) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      row.requestMappingError = true
      return null
    }
    const mapping = parsed as Record<string, unknown>
    if (Object.keys(mapping).some((key) => key !== 'rules')) {
      row.requestMappingError = true
      return null
    }
    if (!isValidMappingRules(mapping.rules)) {
      row.requestMappingError = true
      return null
    }
    const rules = Array.isArray(mapping.rules)
      ? mapping.rules.map((item): MediaRequestMappingRule => {
          const rule = item as Record<string, unknown>
          const normalized: MediaRequestMappingRule = {
            operation: String(rule.operation).trim().toLowerCase() as MediaMappingOperation,
            target: String(rule.target).trim()
          }
          if (typeof rule.source === 'string') normalized.source = rule.source.trim()
          if (Object.prototype.hasOwnProperty.call(rule, 'value')) normalized.value = rule.value
          if (rule.values && typeof rule.values === 'object' && !Array.isArray(rule.values)) {
            normalized.values = rule.values as Record<string, string>
          }
          if (typeof rule.cast === 'string') {
            normalized.cast = rule.cast.trim().toLowerCase() as MediaRequestMappingRule['cast']
          }
          return normalized
        })
      : undefined
    row.requestMappingError = false
    return rules ? { rules } : {}
  } catch {
    row.requestMappingError = true
    return null
  }
}

function refreshValidity(): boolean {
  const seen = new Set<string>()
  duplicateModel.value = ''
  missingModelFields.value = rows.value.length === 0
  let mappingValid = true
  let accountModesValid = true

  for (const row of rows.value) {
    const model = row.model.trim().toLowerCase()
    if (!model || !row.upstreamModelID.trim()) {
      missingModelFields.value = true
    }
    if (model && seen.has(model)) {
      duplicateModel.value = model
    }
    if (model) seen.add(model)
    if (!parseRequestMapping(row)) mappingValid = false
    if (!rowSupportsSelectedMode(row)) accountModesValid = false
  }

  const registryValidated = registryLoaded.value && !registryLoading.value && !registryLoadFailed.value
  const valid = registryValidated && Boolean(provider.value.trim()) && !missingModelFields.value &&
    !duplicateModel.value && mappingValid && accountModesValid
  setValidity(valid)
  return valid
}

function hydrate(value: MediaAccountConfig) {
  provider.value = value.provider || ''
  rows.value = Object.entries(value.models || {}).map(([model, binding]) =>
    createRow(model, binding)
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

function publish() {
  const models = Object.create(null) as Record<string, MediaAccountModelBinding>
  for (const row of rows.value) {
    const model = row.model.trim().toLowerCase()
    if (!model || models[model]) continue
    models[model] = {
      enabled: row.enabled,
      upstream_model_id: row.upstreamModelID.trim(),
      async_mode: row.asyncMode,
      request_mapping: parseRequestMapping(row) || {}
    }
  }
  const emittedValue: MediaAccountConfig = {
    version: 1,
    provider: provider.value,
    models
  }
  lastEmittedObject = emittedValue
  emit('update:modelValue', emittedValue)
  refreshValidity()
}

function addModel() {
  rows.value.push(createRow())
  publish()
}

function removeModel(index: number) {
  rows.value.splice(index, 1)
  publish()
}

onMounted(loadRegistryModels)
</script>

<template>
  <section
    data-test="media-config-editor"
    class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600"
    aria-labelledby="media-config-title"
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
      <label for="media-provider" class="input-label">
        {{ t('admin.accounts.mediaConfig.provider') }}
      </label>
      <input
        id="media-provider"
        v-model="provider"
        data-test="media-provider"
        class="input"
        type="text"
        :placeholder="t('admin.accounts.mediaConfig.providerPlaceholder')"
        aria-describedby="media-provider-hint"
        @input="publish"
      />
      <p id="media-provider-hint" class="input-hint">
        {{ t('admin.accounts.mediaConfig.providerHint') }}
      </p>
    </div>

    <div class="space-y-3">
      <div
        v-for="(row, index) in rows"
        :key="row.id"
        class="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-dark-600"
      >
        <div class="grid gap-3 md:grid-cols-2">
          <div>
            <label :for="`${row.id}-model`" class="input-label text-xs">
              {{ t('admin.accounts.mediaConfig.model') }}
            </label>
            <select
              :id="`${row.id}-model`"
              v-model="row.model"
              :data-test="`media-model-id-${index}`"
              class="input"
              :disabled="registryLoading"
              @change="publish"
            >
              <option value="" disabled>
                {{ registryLoading
                  ? t('admin.accounts.mediaConfig.loadingModels')
                  : t('admin.accounts.mediaConfig.selectModel') }}
              </option>
              <option
                v-if="row.model && !registryModelsByID.has(row.model.trim().toLowerCase())"
                :value="row.model"
              >
                {{ row.model }} · {{ t('admin.accounts.mediaConfig.legacyModel') }}
              </option>
              <option
                v-for="model in registryModels"
                :key="model.id"
                :value="model.model_id"
                :disabled="isSelectedByAnotherRow(model.model_id, row)"
              >
                {{ modelOptionLabel(model) }}
              </option>
            </select>
            <p v-if="selectedDefinition(row)" class="input-hint">
              {{ t('admin.accounts.mediaConfig.registryModelHint', {
                adapter: selectedDefinition(row)?.adapter_resolution.resolved_adapter,
                match: selectedDefinition(row)?.adapter_resolution.matched_family
                  || t(`admin.mediaModels.resolution.matchedBy.${selectedDefinition(row)?.adapter_resolution.matched_by}`),
                sync: selectedDefinition(row)?.adapter_resolution.capabilities?.sync_upstream
                  ? t('common.yes') : t('common.no'),
                native: selectedDefinition(row)?.adapter_resolution.capabilities?.native_async_upstream
                  ? t('common.yes') : t('common.no'),
                content: selectedDefinition(row)?.adapter_resolution.capabilities?.content_fetch
                  ? t('common.yes') : t('common.no')
              }) }}
            </p>
            <p
              v-if="rowModelUnavailable(row)"
              :data-test="`media-model-unavailable-${index}`"
              class="mt-1 text-xs text-amber-700 dark:text-amber-300"
            >
              {{ t('admin.accounts.mediaConfig.legacyModelUnavailable') }}
            </p>
          </div>
          <div>
            <label :for="`${row.id}-upstream`" class="input-label text-xs">
              {{ t('admin.accounts.mediaConfig.upstreamModel') }}
            </label>
            <input
              :id="`${row.id}-upstream`"
              v-model="row.upstreamModelID"
              :data-test="`media-upstream-model-${index}`"
              class="input"
              type="text"
              @input="publish"
            />
          </div>
        </div>

        <div class="grid items-end gap-3 md:grid-cols-[auto_1fr_auto]">
          <label class="flex min-h-10 items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input
              v-model="row.enabled"
              :data-test="`media-model-enabled-${index}`"
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              @change="publish"
            />
            {{ t('admin.accounts.mediaConfig.enabled') }}
          </label>
          <div>
            <label :for="`${row.id}-mode`" class="input-label text-xs">
              {{ t('admin.accounts.mediaConfig.asyncMode') }}
            </label>
            <select
              :id="`${row.id}-mode`"
              v-model="row.asyncMode"
              :data-test="`media-async-mode-${index}`"
              class="input"
              @change="publish"
            >
              <option
                value="unsupported"
                :disabled="Boolean(selectedDefinition(row)) && !selectedDefinition(row)?.adapter_resolution.capabilities?.sync_upstream"
              >
                {{ t('admin.accounts.mediaConfig.modes.unsupported') }}
              </option>
              <option
                value="native"
                :disabled="Boolean(selectedDefinition(row)) && !selectedDefinition(row)?.adapter_resolution.capabilities?.native_async_upstream"
              >
                {{ t('admin.accounts.mediaConfig.modes.native') }}
              </option>
            </select>
            <p
              v-if="selectedDefinition(row) && !rowSupportsSelectedMode(row)"
              :data-test="`media-mode-capability-error-${index}`"
              class="mt-1 text-xs text-red-600 dark:text-red-400"
            >
              {{ t('admin.accounts.mediaConfig.modeCapabilityMismatch') }}
            </p>
          </div>
          <button
            type="button"
            class="btn btn-secondary"
            :data-test="`media-model-remove-${index}`"
            :aria-label="t('admin.accounts.mediaConfig.removeModel')"
            @click="removeModel(index)"
          >
            {{ t('admin.accounts.mediaConfig.remove') }}
          </button>
        </div>

        <div>
          <label :for="`${row.id}-mapping`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.requestMapping') }}
          </label>
          <textarea
            :id="`${row.id}-mapping`"
            v-model="row.requestMappingText"
            :data-test="`media-request-mapping-${index}`"
            class="input min-h-24 font-mono text-xs"
            :class="{ 'border-red-500 focus:border-red-500 focus:ring-red-500/30': row.requestMappingError }"
            :placeholder="t('admin.accounts.mediaConfig.requestMappingPlaceholder')"
            :aria-invalid="row.requestMappingError"
            @input="publish"
          />
          <p class="input-hint">{{ t('admin.accounts.mediaConfig.requestMappingHint') }}</p>
          <p v-if="row.requestMappingError" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
            {{ t('admin.accounts.mediaConfig.invalidRequestMapping') }}
          </p>
        </div>
      </div>
    </div>

    <p
      v-if="duplicateModel"
      data-test="media-duplicate-model-error"
      class="text-sm text-red-600 dark:text-red-400"
      role="alert"
    >
      {{ t('admin.accounts.mediaConfig.duplicateModel', { model: duplicateModel }) }}
    </p>
    <p
      v-else-if="missingModelFields"
      data-test="media-model-required-error"
      class="text-sm text-amber-700 dark:text-amber-400"
      role="status"
    >
      {{ t('admin.accounts.mediaConfig.modelRequired') }}
    </p>

    <div
      v-if="registryLoadFailed || (!registryLoading && registryModels.length === 0)"
      data-test="media-registry-empty-warning"
      class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-300"
    >
      {{ registryLoadFailed
        ? t('admin.accounts.mediaConfig.registryLoadFailed')
        : t('admin.accounts.mediaConfig.registryEmpty') }}
      <a href="/admin/media-models" class="ml-1 font-medium underline underline-offset-2">
        {{ t('admin.accounts.mediaConfig.manageRegistry') }}
      </a>
    </div>

    <button
      data-test="media-add-model"
      type="button"
      class="btn btn-secondary"
      @click="addModel"
    >
      {{ t('admin.accounts.mediaConfig.addModel') }}
    </button>
  </section>
</template>

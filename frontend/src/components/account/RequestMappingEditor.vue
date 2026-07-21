<script setup lang="ts">
import { ref, toRaw, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  MediaMappingOperation,
  MediaRequestMapping,
  MediaRequestMappingRule,
} from '@/types'

type MappingCast = NonNullable<MediaRequestMappingRule['cast']>

interface EnumEntry {
  id: string
  source: string
  target: string
}

interface EditableRule {
  id: string
  operation: MediaMappingOperation
  source: string
  target: string
  valueText: string
  values: EnumEntry[]
  cast: MappingCast
}

interface RuleErrors {
  source?: string
  target?: string
  value?: string
  values?: string
}

const props = withDefaults(defineProps<{
  modelValue: MediaRequestMapping
  idPrefix?: string
}>(), {
  idPrefix: 'request-mapping',
})

const emit = defineEmits<{
  'update:modelValue': [value: MediaRequestMapping]
  'update:valid': [value: boolean]
}>()

const { t } = useI18n()
const operations: MediaMappingOperation[] = ['rename', 'copy', 'default', 'enum', 'cast']
const casts: MappingCast[] = ['string', 'number', 'integer', 'boolean']
const safePath = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/
const rules = ref<EditableRule[]>([])
const ruleErrors = ref<Record<string, RuleErrors>>(Object.create(null))
const previewSample = ref(JSON.stringify({
  model: 'example-image-model',
  prompt: 'A cinematic landscape',
  size: '1024x1024',
  n: 1,
}, null, 2))
const previewResult = ref('')
const previewError = ref('')
const previewLoading = ref(false)
let nextID = 0
let currentValid: boolean | null = null
let lastEmittedMapping: MediaRequestMapping | null = null
let previewGeneration = 0

function nextItemID(prefix: string): string {
  nextID += 1
  return `${props.idPrefix}-${prefix}-${nextID}`
}

function operationNeedsSource(operation: MediaMappingOperation): boolean {
  return operation !== 'default'
}

function createRule(rule?: MediaRequestMappingRule): EditableRule {
  const operation = operations.includes(rule?.operation as MediaMappingOperation)
    ? rule!.operation
    : 'rename'
  const valueText = Object.prototype.hasOwnProperty.call(rule || {}, 'value')
    ? JSON.stringify(rule?.value, null, 2) ?? 'null'
    : 'null'
  return {
    id: nextItemID('rule'),
    operation,
    source: rule?.source || '',
    target: rule?.target || '',
    valueText,
    values: Object.entries(rule?.values || {}).map(([source, target]) => ({
      id: nextItemID('enum'),
      source,
      target,
    })),
    cast: rule?.cast || 'string',
  }
}

function setValidity(valid: boolean) {
  if (currentValid === valid) return
  currentValid = valid
  emit('update:valid', valid)
}

function pathsOverlap(left: string, right: string): boolean {
  return left === right || left.startsWith(`${right}.`) || right.startsWith(`${left}.`)
}

function pathStyle(path: string): 'envelope' | 'body' {
  const [root] = path.split('.', 1)
  return root === 'image' || root === 'video' ? 'envelope' : 'body'
}

function validateRules(): boolean {
  const errors: Record<string, RuleErrors> = Object.create(null)
  const targets: Array<{ path: string; rule: EditableRule }> = []
  const sources: Array<{ path: string; rule: EditableRule }> = []
  let mappingPathStyle: 'envelope' | 'body' | '' = ''

  const errorFor = (rule: EditableRule): RuleErrors => {
    if (!errors[rule.id]) errors[rule.id] = {}
    return errors[rule.id]
  }

  for (const rule of rules.value) {
    const current = errorFor(rule)
    const target = rule.target.trim()
    if (!safePath.test(target)) {
      current.target = 'admin.accounts.mediaConfig.mappingEditor.errors.path'
    } else {
      if (targets.some((entry) => pathsOverlap(entry.path, target))) {
        current.target = 'admin.accounts.mediaConfig.mappingEditor.errors.pathConflict'
      }
      targets.push({ path: target, rule })
    }

    if (operationNeedsSource(rule.operation)) {
      const source = rule.source.trim()
      if (!safePath.test(source)) {
        current.source = 'admin.accounts.mediaConfig.mappingEditor.errors.path'
      } else {
        sources.push({ path: source, rule })
        if ((rule.operation === 'rename' || rule.operation === 'copy') && source === target) {
          current.source = 'admin.accounts.mediaConfig.mappingEditor.errors.samePath'
        } else if (source !== target && safePath.test(target) && pathsOverlap(source, target)) {
          current.source = 'admin.accounts.mediaConfig.mappingEditor.errors.pathConflict'
        }
      }
    }

    for (const [field, path] of [
      ['source', operationNeedsSource(rule.operation) ? rule.source.trim() : ''],
      ['target', target],
    ] as const) {
      if (!safePath.test(path)) continue
      if (path === 'image' || path === 'video') {
        current[field] = 'admin.accounts.mediaConfig.mappingEditor.errors.envelopeRoot'
        continue
      }
      const currentStyle = pathStyle(path)
      if (mappingPathStyle && currentStyle !== mappingPathStyle) {
        current[field] = 'admin.accounts.mediaConfig.mappingEditor.errors.mixedPathStyle'
      } else if (!mappingPathStyle) {
        mappingPathStyle = currentStyle
      }
    }

    if (rule.operation === 'default') {
      try {
        JSON.parse(rule.valueText)
      } catch {
        current.value = 'admin.accounts.mediaConfig.mappingEditor.errors.defaultJSON'
      }
    }

    if (rule.operation === 'enum') {
      if (rule.values.length === 0) {
        current.values = 'admin.accounts.mediaConfig.mappingEditor.errors.enumRequired'
      } else {
        const sources = new Set<string>()
        for (const entry of rule.values) {
          if (sources.has(entry.source)) {
            current.values = 'admin.accounts.mediaConfig.mappingEditor.errors.enumDuplicate'
            break
          }
          sources.add(entry.source)
        }
      }
    }
  }

  for (const source of sources) {
    for (const target of targets) {
      if (source.path !== target.path && pathsOverlap(source.path, target.path)) {
        errorFor(source.rule).source = 'admin.accounts.mediaConfig.mappingEditor.errors.pathConflict'
      }
    }
  }

  for (const rule of rules.value) {
    if (Object.keys(errors[rule.id] || {}).length === 0) delete errors[rule.id]
  }
  ruleErrors.value = errors
  return Object.keys(errors).length === 0
}

function buildMapping(): MediaRequestMapping {
  if (rules.value.length === 0) return {}
  return {
    rules: rules.value.map((rule): MediaRequestMappingRule => {
      const output: MediaRequestMappingRule = {
        operation: rule.operation,
        target: rule.target.trim(),
      }
      if (operationNeedsSource(rule.operation)) output.source = rule.source.trim()
      if (rule.operation === 'default') output.value = JSON.parse(rule.valueText)
      if (rule.operation === 'enum') {
        output.values = Object.fromEntries(
          rule.values.map((entry) => [entry.source, entry.target]),
        )
      }
      if (rule.operation === 'cast') output.cast = rule.cast
      return output
    }),
  }
}

function clearPreview() {
  previewGeneration += 1
  previewResult.value = ''
  previewError.value = ''
  previewLoading.value = false
}

function commit() {
  clearPreview()
  const valid = validateRules()
  setValidity(valid)
  if (!valid) return
  const mapping = buildMapping()
  lastEmittedMapping = mapping
  emit('update:modelValue', mapping)
}

function hydrate(mapping: MediaRequestMapping) {
  rules.value = (mapping.rules || []).map((rule) => createRule(rule))
  clearPreview()
  setValidity(validateRules())
}

watch(
  () => props.modelValue,
  (mapping) => {
    if (toRaw(mapping) === lastEmittedMapping) {
      lastEmittedMapping = null
      return
    }
    lastEmittedMapping = null
    hydrate(mapping || {})
  },
  { immediate: true, deep: true },
)

function addRule() {
  rules.value.push(createRule())
  commit()
}

function removeRule(index: number) {
  rules.value.splice(index, 1)
  commit()
}

function moveRule(index: number, offset: -1 | 1) {
  const target = index + offset
  if (target < 0 || target >= rules.value.length) return
  const [rule] = rules.value.splice(index, 1)
  rules.value.splice(target, 0, rule)
  commit()
}

function changeOperation(rule: EditableRule) {
  if (rule.operation === 'enum' && rule.values.length === 0) {
    rule.values.push({ id: nextItemID('enum'), source: '', target: '' })
  }
  if (rule.operation === 'default' && !rule.valueText.trim()) rule.valueText = 'null'
  commit()
}

function addEnumEntry(rule: EditableRule) {
  rule.values.push({ id: nextItemID('enum'), source: '', target: '' })
  commit()
}

function removeEnumEntry(rule: EditableRule, index: number) {
  rule.values.splice(index, 1)
  commit()
}

function apiErrorMessage(error: unknown): string {
  const candidate = error as {
    message?: unknown
    response?: { data?: { message?: unknown; detail?: unknown } }
  }
  const message = candidate.response?.data?.message
    || candidate.response?.data?.detail
    || candidate.message
  return typeof message === 'string' && message.trim()
    ? message
    : t('admin.accounts.mediaConfig.mappingEditor.previewFailed')
}

async function runPreview() {
  const generation = previewGeneration + 1
  previewGeneration = generation
  previewResult.value = ''
  previewError.value = ''
  const valid = validateRules()
  setValidity(valid)
  if (!valid) {
    previewError.value = t('admin.accounts.mediaConfig.mappingEditor.fixRulesBeforePreview')
    return
  }

  let request: unknown
  try {
    request = JSON.parse(previewSample.value)
  } catch {
    previewError.value = t('admin.accounts.mediaConfig.mappingEditor.invalidSampleJSON')
    return
  }
  if (!request || typeof request !== 'object' || Array.isArray(request)) {
    previewError.value = t('admin.accounts.mediaConfig.mappingEditor.sampleMustBeObject')
    return
  }

  previewLoading.value = true
  try {
    const result = await adminAPI.mediaModels.previewRequestMapping(
      request as Record<string, unknown>,
      buildMapping(),
    )
    if (previewGeneration === generation) {
      previewResult.value = JSON.stringify(result, null, 2)
    }
  } catch (error: unknown) {
    if (previewGeneration === generation) {
      previewError.value = apiErrorMessage(error)
    }
  } finally {
    if (previewGeneration === generation) {
      previewLoading.value = false
    }
  }
}
</script>

<template>
  <section class="space-y-3" :aria-label="t('admin.accounts.mediaConfig.mappingEditor.title')">
    <div class="flex flex-wrap items-start justify-between gap-2">
      <div>
        <p class="text-xs font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.mediaConfig.mappingEditor.title') }}
        </p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.mediaConfig.mappingEditor.hint') }}
        </p>
      </div>
      <button
        type="button"
        class="btn btn-secondary"
        :data-test="`${idPrefix}-add-rule`"
        @click="addRule"
      >
        {{ t('admin.accounts.mediaConfig.mappingEditor.addRule') }}
      </button>
    </div>

    <p
      v-if="rules.length === 0"
      class="rounded-lg border border-dashed border-gray-300 px-3 py-4 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400"
      :data-test="`${idPrefix}-empty`"
    >
      {{ t('admin.accounts.mediaConfig.mappingEditor.empty') }}
    </p>

    <article
      v-for="(rule, index) in rules"
      :key="rule.id"
      class="space-y-3 rounded-lg border border-gray-200 bg-gray-50/60 p-3 dark:border-dark-600 dark:bg-dark-800/30"
      :data-test="`${idPrefix}-rule-${index}`"
    >
      <div class="flex items-center justify-between gap-2">
        <p class="text-xs font-semibold text-gray-700 dark:text-gray-300">
          {{ t('admin.accounts.mediaConfig.mappingEditor.ruleNumber', { number: index + 1 }) }}
        </p>
        <div class="flex items-center gap-1">
          <button
            type="button"
            class="btn btn-secondary px-2 py-1 text-xs"
            :disabled="index === 0"
            :data-test="`${idPrefix}-move-up-${index}`"
            :aria-label="t('admin.accounts.mediaConfig.mappingEditor.moveUp')"
            @click="moveRule(index, -1)"
          >
            ↑
          </button>
          <button
            type="button"
            class="btn btn-secondary px-2 py-1 text-xs"
            :disabled="index === rules.length - 1"
            :data-test="`${idPrefix}-move-down-${index}`"
            :aria-label="t('admin.accounts.mediaConfig.mappingEditor.moveDown')"
            @click="moveRule(index, 1)"
          >
            ↓
          </button>
          <button
            type="button"
            class="btn btn-secondary px-2 py-1 text-xs"
            :data-test="`${idPrefix}-remove-rule-${index}`"
            :aria-label="t('admin.accounts.mediaConfig.mappingEditor.removeRule')"
            @click="removeRule(index)"
          >
            {{ t('admin.accounts.mediaConfig.remove') }}
          </button>
        </div>
      </div>

      <div class="grid gap-3 md:grid-cols-3">
        <div>
          <label :for="`${rule.id}-operation`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.mappingEditor.operation') }}
          </label>
          <select
            :id="`${rule.id}-operation`"
            v-model="rule.operation"
            class="input"
            :data-test="`${idPrefix}-operation-${index}`"
            @change="changeOperation(rule)"
          >
            <option v-for="operation in operations" :key="operation" :value="operation">
              {{ t(`admin.accounts.mediaConfig.mappingEditor.operations.${operation}`) }}
            </option>
          </select>
        </div>
        <div v-if="operationNeedsSource(rule.operation)">
          <label :for="`${rule.id}-source`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.mappingEditor.source') }}
          </label>
          <input
            :id="`${rule.id}-source`"
            v-model="rule.source"
            class="input"
            :class="{ 'border-red-500': ruleErrors[rule.id]?.source }"
            :data-test="`${idPrefix}-source-${index}`"
            :aria-invalid="Boolean(ruleErrors[rule.id]?.source)"
            placeholder="size"
            @input="commit"
          />
          <p v-if="ruleErrors[rule.id]?.source" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
            {{ t(ruleErrors[rule.id].source!) }}
          </p>
        </div>
        <div>
          <label :for="`${rule.id}-target`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.mappingEditor.target') }}
          </label>
          <input
            :id="`${rule.id}-target`"
            v-model="rule.target"
            class="input"
            :class="{ 'border-red-500': ruleErrors[rule.id]?.target }"
            :data-test="`${idPrefix}-target-${index}`"
            :aria-invalid="Boolean(ruleErrors[rule.id]?.target)"
            placeholder="image_size"
            @input="commit"
          />
          <p v-if="ruleErrors[rule.id]?.target" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
            {{ t(ruleErrors[rule.id].target!) }}
          </p>
        </div>
      </div>

      <div v-if="rule.operation === 'default'">
        <label :for="`${rule.id}-value`" class="input-label text-xs">
          {{ t('admin.accounts.mediaConfig.mappingEditor.defaultValue') }}
        </label>
        <textarea
          :id="`${rule.id}-value`"
          v-model="rule.valueText"
          class="input min-h-20 font-mono text-xs"
          :class="{ 'border-red-500': ruleErrors[rule.id]?.value }"
          :data-test="`${idPrefix}-default-value-${index}`"
          :aria-invalid="Boolean(ruleErrors[rule.id]?.value)"
          @input="commit"
        />
        <p class="input-hint">{{ t('admin.accounts.mediaConfig.mappingEditor.defaultValueHint') }}</p>
        <p v-if="ruleErrors[rule.id]?.value" class="mt-1 text-xs text-red-600 dark:text-red-400" role="alert">
          {{ t(ruleErrors[rule.id].value!) }}
        </p>
      </div>

      <div v-if="rule.operation === 'enum'" class="space-y-2">
        <div class="flex items-center justify-between gap-2">
          <p class="text-xs font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.accounts.mediaConfig.mappingEditor.enumValues') }}
          </p>
          <button type="button" class="btn btn-secondary" @click="addEnumEntry(rule)">
            {{ t('admin.accounts.mediaConfig.mappingEditor.addEnumValue') }}
          </button>
        </div>
        <div
          v-for="(entry, entryIndex) in rule.values"
          :key="entry.id"
          class="grid items-end gap-2 sm:grid-cols-[1fr_1fr_auto]"
        >
          <div>
            <label :for="`${entry.id}-source`" class="input-label text-xs">
              {{ t('admin.accounts.mediaConfig.mappingEditor.enumInput') }}
            </label>
            <input
              :id="`${entry.id}-source`"
              v-model="entry.source"
              class="input"
              :data-test="`${idPrefix}-enum-source-${index}-${entryIndex}`"
              @input="commit"
            />
          </div>
          <div>
            <label :for="`${entry.id}-target`" class="input-label text-xs">
              {{ t('admin.accounts.mediaConfig.mappingEditor.enumOutput') }}
            </label>
            <input
              :id="`${entry.id}-target`"
              v-model="entry.target"
              class="input"
              :data-test="`${idPrefix}-enum-target-${index}-${entryIndex}`"
              @input="commit"
            />
          </div>
          <button
            type="button"
            class="btn btn-secondary"
            :aria-label="t('admin.accounts.mediaConfig.mappingEditor.removeEnumValue')"
            @click="removeEnumEntry(rule, entryIndex)"
          >
            {{ t('admin.accounts.mediaConfig.remove') }}
          </button>
        </div>
        <p v-if="ruleErrors[rule.id]?.values" class="text-xs text-red-600 dark:text-red-400" role="alert">
          {{ t(ruleErrors[rule.id].values!) }}
        </p>
      </div>

      <div v-if="rule.operation === 'cast'" class="max-w-xs">
        <label :for="`${rule.id}-cast`" class="input-label text-xs">
          {{ t('admin.accounts.mediaConfig.mappingEditor.castType') }}
        </label>
        <select
          :id="`${rule.id}-cast`"
          v-model="rule.cast"
          class="input"
          :data-test="`${idPrefix}-cast-${index}`"
          @change="commit"
        >
          <option v-for="cast in casts" :key="cast" :value="cast">
            {{ t(`admin.accounts.mediaConfig.mappingEditor.casts.${cast}`) }}
          </option>
        </select>
      </div>
    </article>

    <details class="rounded-lg border border-gray-200 p-3 dark:border-dark-600" :data-test="`${idPrefix}-preview`">
      <summary class="cursor-pointer text-xs font-medium text-gray-700 dark:text-gray-300">
        {{ t('admin.accounts.mediaConfig.mappingEditor.previewTitle') }}
      </summary>
      <div class="mt-3 space-y-3">
        <div>
          <label :for="`${idPrefix}-sample`" class="input-label text-xs">
            {{ t('admin.accounts.mediaConfig.mappingEditor.sampleRequest') }}
          </label>
          <textarea
            :id="`${idPrefix}-sample`"
            v-model="previewSample"
            class="input min-h-36 font-mono text-xs"
            :data-test="`${idPrefix}-sample`"
            @input="clearPreview"
          />
        </div>
        <button
          type="button"
          class="btn btn-secondary"
          :disabled="previewLoading"
          :data-test="`${idPrefix}-run-preview`"
          @click="runPreview"
        >
          {{ previewLoading
            ? t('admin.accounts.mediaConfig.mappingEditor.previewing')
            : t('admin.accounts.mediaConfig.mappingEditor.runPreview') }}
        </button>
        <p v-if="previewError" class="text-xs text-red-600 dark:text-red-400" role="alert" :data-test="`${idPrefix}-preview-error`">
          {{ previewError }}
        </p>
        <div v-if="previewResult">
          <p class="input-label text-xs">{{ t('admin.accounts.mediaConfig.mappingEditor.previewResult') }}</p>
          <pre class="max-h-72 overflow-auto rounded-lg bg-gray-900 p-3 text-xs text-gray-100" :data-test="`${idPrefix}-preview-result`">{{ previewResult }}</pre>
        </div>
      </div>
    </details>
  </section>
</template>

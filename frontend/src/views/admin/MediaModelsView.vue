<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { adminAPI } from '@/api/admin'
import MediaModelEditor from '@/components/admin/media/MediaModelEditor.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import type { Column } from '@/components/common/types'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { useAppStore } from '@/stores/app'
import type {
  MediaModelDefinition,
  MediaModelDefinitionInput,
  MediaOperation,
  MediaType,
} from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

const models = ref<MediaModelDefinition[]>([])
const loading = ref(false)
const submitting = ref(false)
const search = ref('')
const mediaType = ref<'' | MediaType>('')
const showEditor = ref(false)
const showDelete = ref(false)
const editing = ref<MediaModelDefinition | null>(null)
const deleting = ref<MediaModelDefinition | null>(null)
const editorValid = ref(false)

const createDefault = (): MediaModelDefinitionInput => ({
  model_id: '',
  vendor: '',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
})

const form = ref<MediaModelDefinitionInput>(createDefault())

const columns = computed<Column[]>(() => [
  { key: 'model_id', label: t('admin.mediaModels.columns.model') },
  { key: 'vendor', label: t('admin.mediaModels.columns.vendor') },
  { key: 'media_type', label: t('admin.mediaModels.columns.type') },
  { key: 'operations', label: t('admin.mediaModels.columns.operations') },
  { key: 'adapter_resolution', label: t('admin.mediaModels.columns.adapter') },
  { key: 'aliases', label: t('admin.mediaModels.columns.aliases') },
  { key: 'enabled', label: t('admin.mediaModels.columns.status') },
  { key: 'actions', label: t('admin.mediaModels.columns.actions') },
])

const filteredModels = computed(() => {
  const query = search.value.trim().toLowerCase()
  return models.value.filter((model) => {
    if (mediaType.value && model.media_type !== mediaType.value) return false
    if (!query) return true
    return [model.model_id, model.vendor, ...model.aliases, ...resolutionSearchValues(model)]
      .some((value) => value.toLowerCase().includes(query))
  })
})

const stats = computed(() => ({
  total: models.value.length,
  image: models.value.filter((item) => item.media_type === 'image' && item.enabled).length,
  video: models.value.filter((item) => item.media_type === 'video' && item.enabled).length,
  disabled: models.value.filter((item) => !item.enabled).length,
}))

function operationLabel(operation: MediaOperation): string {
  return t(`admin.mediaModels.operations.${operation}`)
}

function resolutionSearchValues(model: MediaModelDefinition): string[] {
  return [
    model.adapter_resolution.resolved_adapter,
    model.adapter_resolution.matched_family,
    model.adapter_resolution.reason_code,
  ]
}

function toInput(model: MediaModelDefinition): MediaModelDefinitionInput {
  return {
    model_id: model.model_id,
    vendor: model.vendor,
    media_type: model.media_type,
    operations: [...model.operations],
    constraints: { ...model.constraints },
    billing_unit: model.billing_unit,
    enabled: model.enabled,
    aliases: [...model.aliases],
  }
}

const knownResolutionReasonCodes = new Set([
  'MEDIA_MODEL_DEFINITION_INVALID',
  'MEDIA_ADAPTER_UNRESOLVED',
  'MEDIA_ADAPTER_AMBIGUOUS',
  'MEDIA_ADAPTER_IMPLEMENTATION_MISSING',
  'MEDIA_ADAPTER_CAPABILITY_MISMATCH',
])

function saveErrorMessage(error: any): string {
  const reason = error?.reason
    || error?.response?.data?.reason
    || error?.response?.data?.error?.code
  if (knownResolutionReasonCodes.has(reason)) {
    return t(`admin.mediaModels.resolution.reason.${reason}`)
  }
  return error?.message || t('admin.mediaModels.messages.saveFailed')
}

async function loadModels() {
  loading.value = true
  try {
    models.value = await adminAPI.mediaModels.list()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.mediaModels.messages.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editing.value = null
  form.value = createDefault()
  editorValid.value = false
  showEditor.value = true
}

function openEdit(model: MediaModelDefinition) {
  editing.value = model
  form.value = toInput(model)
  editorValid.value = true
  showEditor.value = true
}

function closeEditor() {
  showEditor.value = false
  editing.value = null
  form.value = createDefault()
}

async function saveModel() {
  if (!editorValid.value || submitting.value) return
  submitting.value = true
  try {
    if (editing.value) {
      await adminAPI.mediaModels.update(editing.value.id, form.value)
      appStore.showSuccess(t('admin.mediaModels.messages.updated'))
    } else {
      await adminAPI.mediaModels.create(form.value)
      appStore.showSuccess(t('admin.mediaModels.messages.created'))
    }
    closeEditor()
    await loadModels()
  } catch (error: any) {
    appStore.showError(saveErrorMessage(error))
  } finally {
    submitting.value = false
  }
}

async function toggleEnabled(model: MediaModelDefinition) {
  try {
    await adminAPI.mediaModels.update(model.id, { ...toInput(model), enabled: !model.enabled })
    await loadModels()
  } catch (error: any) {
    appStore.showError(saveErrorMessage(error))
  }
}

function requestDelete(model: MediaModelDefinition) {
  deleting.value = model
  showDelete.value = true
}

async function confirmDelete() {
  if (!deleting.value) return
  const target = deleting.value
  showDelete.value = false
  try {
    await adminAPI.mediaModels.remove(target.id)
    appStore.showSuccess(t('admin.mediaModels.messages.deleted'))
    await loadModels()
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.mediaModels.messages.deleteFailed'))
  } finally {
    deleting.value = null
  }
}

onMounted(loadModels)
</script>

<template>
  <AppLayout>
    <TablePageLayout natural-height>
      <template #actions>
        <div class="registry-banner overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <div class="grid gap-5 p-5 lg:grid-cols-[1fr_auto] lg:items-center">
            <div>
              <div class="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-rose-600 dark:text-rose-400">
                <span class="h-2 w-2 rounded-full bg-rose-500"></span>
                {{ t('admin.mediaModels.registryLabel') }}
              </div>
              <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-gray-300">
                {{ t('admin.mediaModels.registryHint') }}
              </p>
            </div>
            <button class="btn btn-primary" type="button" data-test="create-media-model" @click="openCreate">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.mediaModels.create') }}
            </button>
          </div>
          <div class="grid grid-cols-2 border-t border-gray-100 bg-gray-50/70 dark:border-dark-700 dark:bg-dark-900/40 sm:grid-cols-4">
            <div v-for="(value, key) in stats" :key="key" class="border-r border-gray-100 px-5 py-3 last:border-r-0 dark:border-dark-700">
              <div class="text-xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ value }}</div>
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ t(`admin.mediaModels.stats.${key}`) }}</div>
            </div>
          </div>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex flex-1 flex-col gap-3 sm:flex-row">
            <div class="relative w-full sm:max-w-sm">
              <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
              <input v-model="search" class="input pl-10" type="search" :placeholder="t('admin.mediaModels.search')" />
            </div>
            <div class="inline-flex rounded-lg bg-gray-100 p-1 dark:bg-dark-700">
              <button
                v-for="option in (['', 'image', 'video'] as Array<'' | MediaType>)"
                :key="option || 'all'"
                type="button"
                class="rounded-md px-3 py-2 text-xs font-medium transition"
                :class="mediaType === option
                  ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white'
                  : 'text-gray-500 dark:text-gray-400'"
                @click="mediaType = option"
              >
                {{ option ? t(`admin.mediaModels.types.${option}`) : t('admin.mediaModels.types.all') }}
              </button>
            </div>
          </div>
          <button class="btn btn-secondary" type="button" :disabled="loading" @click="loadModels">
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredModels" :loading="loading" row-key="id">
          <template #cell-model_id="{ row }">
            <div>
              <div class="font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ row.model_id }}</div>
              <div class="mt-0.5 text-xs text-gray-400">#{{ row.id }}</div>
            </div>
          </template>
          <template #cell-vendor="{ value }">
            <span class="rounded-md bg-gray-100 px-2 py-1 text-xs font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ value }}</span>
          </template>
          <template #cell-media_type="{ value }">
            <span
              class="inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium"
              :class="value === 'image'
                ? 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300'
                : 'bg-rose-50 text-rose-700 dark:bg-rose-950/40 dark:text-rose-300'"
            >
              <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
              {{ t(`admin.mediaModels.types.${value}`) }}
            </span>
          </template>
          <template #cell-operations="{ row }">
            <div class="flex max-w-sm flex-wrap gap-1">
              <span v-for="operation in row.operations" :key="operation" class="rounded bg-gray-100 px-1.5 py-0.5 text-[11px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                {{ operationLabel(operation) }}
              </span>
            </div>
          </template>
          <template #cell-adapter_resolution="{ row }">
            <div class="space-y-1 text-xs" :data-resolution-status="row.adapter_resolution.status">
              <div
                class="font-medium"
                :class="row.adapter_resolution.status === 'ready'
                  ? 'text-emerald-700 dark:text-emerald-300'
                  : 'text-amber-700 dark:text-amber-300'"
              >
                {{ t(`admin.mediaModels.resolution.status.${row.adapter_resolution.status}`) }}
              </div>
              <div v-if="row.adapter_resolution.resolved_adapter" class="font-mono text-gray-700 dark:text-gray-300">
                {{ row.adapter_resolution.resolved_adapter }}
              </div>
              <div v-if="row.adapter_resolution.matched_by" class="text-[11px] text-gray-400">
                {{ t(`admin.mediaModels.resolution.matchedBy.${row.adapter_resolution.matched_by}`) }}
                <span v-if="row.adapter_resolution.matched_family"> · {{ row.adapter_resolution.matched_family }}</span>
              </div>
              <div v-if="row.adapter_resolution.capabilities" class="text-[11px] text-gray-400">
                {{ t('admin.mediaModels.resolution.capabilities.sync') }}:
                {{ row.adapter_resolution.capabilities.sync_upstream ? t('common.yes') : t('common.no') }} ·
                {{ t('admin.mediaModels.resolution.capabilities.nativeAsync') }}:
                {{ row.adapter_resolution.capabilities.native_async_upstream ? t('common.yes') : t('common.no') }} ·
                {{ t('admin.mediaModels.resolution.capabilities.contentFetch') }}:
                {{ row.adapter_resolution.capabilities.content_fetch ? t('common.yes') : t('common.no') }}
              </div>
            </div>
          </template>
          <template #cell-aliases="{ row }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ row.aliases.length ? row.aliases.join(', ') : '—' }}</span>
          </template>
          <template #cell-enabled="{ row }">
            <button
              type="button"
              :data-test="`toggle-media-model-${row.id}`"
              class="rounded-full px-2.5 py-1 text-xs font-medium transition"
              :class="row.enabled
                ? 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100 dark:bg-emerald-950/40 dark:text-emerald-300'
                : 'bg-gray-100 text-gray-500 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-400'"
              @click="toggleEnabled(row)"
            >
              {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
            </button>
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button :data-test="`edit-media-model-${row.id}`" class="rounded-lg p-2 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700" type="button" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="rounded-lg p-2 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-950/30" type="button" :title="t('common.delete')" @click="requestDelete(row)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>
          <template #empty>
            <EmptyState
              :title="t('admin.mediaModels.emptyTitle')"
              :description="t('admin.mediaModels.emptyHint')"
              :action-text="t('admin.mediaModels.create')"
              @action="openCreate"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showEditor"
      :title="editing ? t('admin.mediaModels.edit') : t('admin.mediaModels.create')"
      width="wide"
      @close="closeEditor"
    >
      <form id="media-model-form" @submit.prevent="saveModel">
        <MediaModelEditor
          v-model="form"
          :editing="Boolean(editing)"
          :adapter-resolution="editing?.adapter_resolution ?? null"
          @update:valid="editorValid = $event"
        />
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" type="button" @click="closeEditor">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="submit" form="media-model-form" :disabled="!editorValid || submitting">
            {{ submitting ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDelete"
      :title="t('admin.mediaModels.deleteTitle')"
      :message="t('admin.mediaModels.deleteHint', { model: deleting?.model_id || '' })"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDelete = false"
    />
  </AppLayout>
</template>

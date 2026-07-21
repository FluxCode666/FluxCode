<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  getMediaStorageConfig,
  testMediaStorageConfig,
  updateMediaStorageConfig,
  type MediaStorageConfig,
  type MediaStorageProvider,
} from '@/api/admin/settings'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const loading = ref(true)
const saving = ref(false)
const testing = ref(false)
const error = ref('')
const notice = ref('')

const form = reactive<MediaStorageConfig>({
  provider: 'local',
  local_path: './data/generated',
  minio: {
    endpoint: '',
    bucket: '',
    access_key_id: '',
    secret_access_key: '',
    secret_access_key_configured: false,
    region: 'us-east-1',
    use_ssl: true,
    force_path_style: true,
    prefix: 'media',
  },
})

function applyConfig(config: MediaStorageConfig) {
  form.provider = config.provider
  form.local_path = config.local_path
  Object.assign(form.minio, config.minio, { secret_access_key: '' })
}

function payload(): MediaStorageConfig {
  return {
    provider: form.provider,
    local_path: form.local_path.trim(),
    minio: {
      ...form.minio,
      endpoint: form.minio.endpoint.trim(),
      bucket: form.minio.bucket.trim(),
      access_key_id: form.minio.access_key_id.trim(),
      secret_access_key: form.minio.secret_access_key,
      region: form.minio.region.trim(),
      prefix: form.minio.prefix.trim(),
    },
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    applyConfig(await getMediaStorageConfig())
  } catch (caught) {
    error.value = extractApiErrorMessage(caught)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    applyConfig(await updateMediaStorageConfig(payload()))
    notice.value = t('admin.settings.mediaStorage.saved')
  } catch (caught) {
    error.value = extractApiErrorMessage(caught)
  } finally {
    saving.value = false
  }
}

async function testConnection() {
  testing.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await testMediaStorageConfig(payload())
    if (result.ok) notice.value = result.message || t('admin.settings.mediaStorage.testSucceeded')
    else error.value = result.message || t('admin.settings.mediaStorage.testFailed')
  } catch (caught) {
    error.value = extractApiErrorMessage(caught)
  } finally {
    testing.value = false
  }
}

function setProvider(event: Event) {
  form.provider = (event.target as HTMLSelectElement).value as MediaStorageProvider
}

onMounted(load)
</script>

<template>
  <section data-test="media-storage-settings" class="card">
    <div class="border-b border-gray-100 px-4 py-4 sm:px-6 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.mediaStorage.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.mediaStorage.description') }}
      </p>
    </div>

    <div v-if="loading" class="px-4 py-6 text-sm text-gray-500 sm:px-6">
      {{ t('common.loading') }}
    </div>
    <div v-else class="space-y-5 px-4 py-5 sm:px-6">
      <div class="space-y-2">
        <label for="media-storage-provider" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.mediaStorage.provider') }}
        </label>
        <select
          id="media-storage-provider"
          data-test="media-storage-provider"
          class="input w-full sm:max-w-xs"
          :value="form.provider"
          @change="setProvider"
        >
          <option value="local">Local</option>
          <option value="minio">MinIO / S3</option>
        </select>
      </div>

      <div class="space-y-2">
        <label for="media-local-path" class="block text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.settings.mediaStorage.localPath') }}
        </label>
        <input id="media-local-path" v-model="form.local_path" data-test="media-local-path" class="input w-full font-mono text-sm" />
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.settings.mediaStorage.localPathHint') }}
        </p>
        <p v-if="form.provider === 'local'" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-700 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
          {{ t('admin.settings.mediaStorage.multiInstanceWarning') }}
        </p>
      </div>

      <div v-if="form.provider === 'minio'" data-test="media-minio-fields" class="grid gap-4 md:grid-cols-2">
        <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
          <span>{{ t('admin.settings.mediaStorage.endpoint') }}</span>
          <input v-model="form.minio.endpoint" class="input w-full font-mono text-sm" placeholder="https://minio.example.com" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
          <span>{{ t('admin.settings.mediaStorage.bucket') }}</span>
          <input v-model="form.minio.bucket" class="input w-full" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
          <span>{{ t('admin.settings.mediaStorage.region') }}</span>
          <input v-model="form.minio.region" class="input w-full" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
          <span>{{ t('admin.settings.mediaStorage.accessKey') }}</span>
          <input v-model="form.minio.access_key_id" class="input w-full font-mono text-sm" autocomplete="off" />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300">
          <span>{{ t('admin.settings.mediaStorage.secretKey') }}</span>
          <input
            v-model="form.minio.secret_access_key"
            class="input w-full font-mono text-sm"
            type="password"
            autocomplete="new-password"
            :placeholder="form.minio.secret_access_key_configured ? t('admin.settings.mediaStorage.secretConfigured') : ''"
          />
        </label>
        <label class="space-y-1 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
          <span>{{ t('admin.settings.mediaStorage.prefix') }}</span>
          <input v-model="form.minio.prefix" class="input w-full font-mono text-sm" />
        </label>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.minio.use_ssl" type="checkbox" />
          {{ t('admin.settings.mediaStorage.useSSL') }}
        </label>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="form.minio.force_path_style" type="checkbox" />
          {{ t('admin.settings.mediaStorage.pathStyle') }}
        </label>
      </div>

      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <p v-if="notice" role="status" class="text-sm text-green-600 dark:text-green-400">{{ notice }}</p>

      <div class="flex flex-wrap gap-3">
        <button type="button" class="btn btn-secondary" :disabled="testing || saving" @click="testConnection">
          {{ testing ? t('admin.settings.mediaStorage.testing') : t('admin.settings.mediaStorage.test') }}
        </button>
        <button type="button" class="btn btn-primary" :disabled="saving || testing" @click="save">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </section>
</template>

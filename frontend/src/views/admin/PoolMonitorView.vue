<template>
  <AppLayout>
    <div class="space-y-6 p-6">
      <div class="flex justify-end">
        <RouterLink to="/admin/pool-monitor" class="btn btn-secondary">
          {{ t('admin.poolMonitorConfig.backToDashboard') }}
        </RouterLink>
      </div>
      <div v-if="loading" class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>

      <div v-else class="space-y-6">
        <div class="grid grid-cols-1 items-start gap-6 md:grid-cols-2 xl:grid-cols-3">
          <div class="card overflow-hidden">
            <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.poolMonitor.cards.alertDeliveryTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.poolMonitor.cards.alertDeliveryDescription') }}
              </p>
            </div>
            <div class="space-y-4 p-4">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.alertCooldownMinutes') }}
                </label>
                <input
                  v-model.number="form.alert_cooldown_minutes"
                  type="number"
                  min="0"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.alertCooldownMinutesHint') }}
                </p>
              </div>

              <div class="space-y-3">
                <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.alertEmails') }}
                </label>

                <div
                  v-for="(_, index) in form.alert_emails"
                  :key="index"
                  class="flex flex-col gap-3 md:flex-row md:items-center"
                >
                  <input
                    v-model="form.alert_emails[index]"
                    type="email"
                    class="input flex-1"
                    :placeholder="t('admin.poolMonitor.alertEmailPlaceholder')"
                  />
                  <button type="button" class="btn btn-ghost btn-sm self-end md:self-auto" @click="removeAlertEmail(index)">
                    {{ t('admin.poolMonitor.removeEmail') }}
                  </button>
                </div>

                <button type="button" class="btn btn-secondary btn-sm" @click="addAlertEmail">
                  {{ t('admin.poolMonitor.addEmail') }}
                </button>
              </div>

              <div class="flex justify-end">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="savingSection !== ''"
                  @click="saveSection('alert')"
                >
                  {{ savingSection === 'alert' ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </div>
          </div>

          <div class="card overflow-hidden">
            <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.poolMonitor.cards.poolThresholdTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.poolMonitor.cards.poolThresholdDescription') }}
              </p>
            </div>
            <div class="space-y-4 p-4">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t('admin.poolMonitor.poolThresholdEnabled') }}
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.poolMonitor.poolThresholdEnabledHint') }}
                  </p>
                </div>
                <Toggle v-model="form.pool_threshold_enabled" />
              </div>
              <p
                v-if="!form.pool_threshold_enabled"
                class="text-xs text-amber-700 dark:text-amber-300"
              >
                {{ t('admin.poolMonitor.poolThresholdDisabledHint') }}
              </p>

              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.availableCountThreshold') }}
                </label>
                <input
                  v-model.number="form.available_count_threshold"
                  type="number"
                  min="0"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.availableCountThresholdHint') }}
                </p>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.availableRatioThreshold') }}
                </label>
                <input
                  v-model.number="form.available_ratio_threshold"
                  type="number"
                  min="0"
                  max="100"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.availableRatioThresholdHint') }}
                </p>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.checkIntervalMinutes') }}
                </label>
                <input
                  v-model.number="form.check_interval_minutes"
                  type="number"
                  min="1"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.checkIntervalMinutesHint') }}
                </p>
              </div>

              <div class="flex justify-end">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="savingSection !== ''"
                  @click="saveSection('pool')"
                >
                  {{ savingSection === 'pool' ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </div>
          </div>

          <div class="card overflow-hidden">
            <div class="border-b border-gray-100 px-4 py-3 dark:border-dark-700">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">
                {{ t('admin.poolMonitor.cards.proxyFailureTitle') }}
              </h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.poolMonitor.cards.proxyFailureDescription') }}
              </p>
            </div>
            <div class="space-y-4 p-4">
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t('admin.poolMonitor.proxyFailureEnabled') }}
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.poolMonitor.proxyFailureEnabledHint') }}
                  </p>
                </div>
                <Toggle v-model="form.proxy_failure_enabled" />
              </div>
              <div class="flex items-center justify-between">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t('admin.poolMonitor.proxyActiveProbeEnabled') }}
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.poolMonitor.proxyActiveProbeEnabledHint') }}
                  </p>
                </div>
                <Toggle v-model="form.proxy_active_probe_enabled" />
              </div>
              <p
                v-if="!form.proxy_active_probe_enabled"
                class="text-xs text-amber-700 dark:text-amber-300"
              >
                {{ t('admin.poolMonitor.proxyActiveProbeDisabledHint') }}
              </p>
              <div class="space-y-2">
                <div>
                  <label class="font-medium text-gray-900 dark:text-white">
                    {{ t('admin.poolMonitor.disabledProxyScheduleMode') }}
                  </label>
                  <p class="text-sm text-gray-500 dark:text-gray-400">
                    {{ t('admin.poolMonitor.disabledProxyScheduleModeHint') }}
                  </p>
                </div>
                <label class="flex cursor-pointer items-start gap-2 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
                  <input
                    v-model="form.disabled_proxy_schedule_mode"
                    type="radio"
                    value="direct_without_proxy"
                    class="mt-1 h-4 w-4 border-gray-300 text-green-600 focus:ring-green-500"
                  />
                  <div>
                    <p class="font-medium text-gray-900 dark:text-white">
                      {{ t('admin.poolMonitor.disabledProxyScheduleModeDirectWithoutProxy') }}
                    </p>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.poolMonitor.disabledProxyScheduleModeDirectWithoutProxyHint') }}
                    </p>
                  </div>
                </label>
                <label class="flex cursor-pointer items-start gap-2 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
                  <input
                    v-model="form.disabled_proxy_schedule_mode"
                    type="radio"
                    value="exclude_account"
                    class="mt-1 h-4 w-4 border-gray-300 text-green-600 focus:ring-green-500"
                  />
                  <div>
                    <p class="font-medium text-gray-900 dark:text-white">
                      {{ t('admin.poolMonitor.disabledProxyScheduleModeExcludeAccount') }}
                    </p>
                    <p class="text-xs text-gray-500 dark:text-gray-400">
                      {{ t('admin.poolMonitor.disabledProxyScheduleModeExcludeAccountHint') }}
                    </p>
                  </div>
                </label>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.proxyProbeIntervalMinutes') }}
                </label>
                <input
                  v-model.number="form.proxy_probe_interval_minutes"
                  type="number"
                  min="1"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.proxyProbeIntervalMinutesHint') }}
                </p>
              </div>
              <p
                v-if="!form.proxy_failure_enabled"
                class="text-xs text-amber-700 dark:text-amber-300"
              >
                {{ t('admin.poolMonitor.proxyFailureDisabledHint') }}
              </p>

              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.proxyFailureWindowMinutes') }}
                </label>
                <input
                  v-model.number="form.proxy_failure_window_minutes"
                  type="number"
                  min="0"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.proxyFailureWindowMinutesHint') }}
                </p>
              </div>

              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.poolMonitor.proxyFailureThreshold') }}
                </label>
                <input
                  v-model.number="form.proxy_failure_threshold"
                  type="number"
                  min="0"
                  class="input"
                />
                <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.poolMonitor.proxyFailureThresholdHint') }}
                </p>
              </div>

              <div class="flex justify-end">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="savingSection !== ''"
                  @click="saveSection('proxy')"
                >
                  {{ savingSection === 'proxy' ? t('common.saving') : t('common.save') }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Toggle from '@/components/common/Toggle.vue'
import { adminAPI } from '@/api'
import type { PoolMonitorConfig, PoolMonitorConfigPatch } from '@/api/admin/poolMonitor'
import { useAppStore } from '@/stores'

type SaveSection = 'pool' | 'proxy' | 'alert'

const { t } = useI18n()
const appStore = useAppStore()
const platform = 'openai'

const loading = ref(true)
const savingSection = ref<SaveSection | ''>('')

const form = reactive<PoolMonitorConfig>({
  platform,
  pool_threshold_enabled: true,
  proxy_failure_enabled: true,
  proxy_active_probe_enabled: true,
  disabled_proxy_schedule_mode: 'direct_without_proxy',
  available_count_threshold: 0,
  available_ratio_threshold: 20,
  check_interval_minutes: 5,
  proxy_probe_interval_minutes: 5,
  proxy_failure_window_minutes: 10,
  proxy_failure_threshold: 5,
  alert_emails: [],
  alert_cooldown_minutes: 5
})

function cloneConfig(cfg: PoolMonitorConfig): PoolMonitorConfig {
  return {
    ...cfg,
    alert_emails: Array.isArray(cfg.alert_emails) ? [...cfg.alert_emails] : []
  }
}

function normalizeEmails(emails: string[]): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const raw of emails) {
    const email = raw.trim()
    if (!email) {
      continue
    }
    const key = email.toLowerCase()
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    out.push(email)
  }
  return out
}

function applyConfig(config: PoolMonitorConfig) {
  Object.assign(form, config)
  form.alert_emails = Array.isArray(config.alert_emails) ? [...config.alert_emails] : []
}

function applySectionConfig(section: SaveSection, config: PoolMonitorConfig) {
  switch (section) {
    case 'pool':
      form.pool_threshold_enabled = config.pool_threshold_enabled
      form.available_count_threshold = config.available_count_threshold
      form.available_ratio_threshold = config.available_ratio_threshold
      form.check_interval_minutes = config.check_interval_minutes
      break
    case 'proxy':
      form.proxy_failure_enabled = config.proxy_failure_enabled
      form.proxy_active_probe_enabled = config.proxy_active_probe_enabled
      form.disabled_proxy_schedule_mode = config.disabled_proxy_schedule_mode
      form.proxy_probe_interval_minutes = config.proxy_probe_interval_minutes
      form.proxy_failure_window_minutes = config.proxy_failure_window_minutes
      form.proxy_failure_threshold = config.proxy_failure_threshold
      break
    case 'alert':
      form.alert_cooldown_minutes = config.alert_cooldown_minutes
      form.alert_emails = Array.isArray(config.alert_emails) ? [...config.alert_emails] : []
      break
  }
}

function addAlertEmail() {
  form.alert_emails.push('')
}

function removeAlertEmail(index: number) {
  form.alert_emails.splice(index, 1)
}

function buildSectionPayload(section: SaveSection): PoolMonitorConfigPatch {
  const payload: PoolMonitorConfigPatch = {}
  switch (section) {
    case 'pool':
      payload.pool_threshold_enabled = form.pool_threshold_enabled
      payload.available_count_threshold = Number(form.available_count_threshold)
      payload.available_ratio_threshold = Number(form.available_ratio_threshold)
      payload.check_interval_minutes = Number(form.check_interval_minutes)
      break
    case 'proxy':
      payload.proxy_failure_enabled = form.proxy_failure_enabled
      payload.proxy_active_probe_enabled = form.proxy_active_probe_enabled
      payload.disabled_proxy_schedule_mode = form.disabled_proxy_schedule_mode
      payload.proxy_probe_interval_minutes = Number(form.proxy_probe_interval_minutes)
      payload.proxy_failure_window_minutes = Number(form.proxy_failure_window_minutes)
      payload.proxy_failure_threshold = Number(form.proxy_failure_threshold)
      break
    case 'alert':
      payload.alert_cooldown_minutes = Number(form.alert_cooldown_minutes)
      payload.alert_emails = normalizeEmails(form.alert_emails)
      break
  }

  return payload
}

async function loadConfig() {
  loading.value = true
  try {
    const config = await adminAPI.poolMonitor.getPoolMonitorConfig(platform)
    const normalized = cloneConfig({
      ...config,
      alert_emails: normalizeEmails(Array.isArray(config.alert_emails) ? config.alert_emails : [])
    })
    applyConfig(normalized)
  } catch (error: any) {
    appStore.showError(
      t('admin.poolMonitor.failedToLoad') + ': ' + (error.message || t('common.unknownError'))
    )
  } finally {
    loading.value = false
  }
}

async function saveSection(section: SaveSection) {
  if (savingSection.value !== '') {
    return
  }

  savingSection.value = section
  try {
    const payload = buildSectionPayload(section)
    const updated = await adminAPI.poolMonitor.updatePoolMonitorConfig(platform, payload)
    const normalized = cloneConfig({
      ...updated,
      alert_emails: normalizeEmails(Array.isArray(updated.alert_emails) ? updated.alert_emails : [])
    })
    applySectionConfig(section, normalized)
    appStore.showSuccess(t('admin.poolMonitor.saved'))
  } catch (error: any) {
    appStore.showError(
      t('admin.poolMonitor.failedToSave') + ': ' + (error.message || t('common.unknownError'))
    )
  } finally {
    savingSection.value = ''
  }
}

onMounted(() => {
  loadConfig()
})
</script>

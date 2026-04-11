<template>
  <BaseDialog
    :show="show"
    :title="dialogTitle"
    width="narrow"
    align-top
    @close="handleClose"
  >
    <div class="space-y-5">
      <div class="attract-popup-markdown" v-html="renderedHtml"></div>
    </div>

    <template #footer>
      <div class="flex flex-col gap-3 sm:flex-row sm:justify-end">
        <button
          v-if="dismissButtonText"
          type="button"
          class="btn btn-secondary w-full sm:w-auto"
          :disabled="savingPreference"
          @click="handleDismiss"
        >
          {{ dismissButtonText }}
        </button>

        <button
          type="button"
          class="btn btn-primary w-full sm:w-auto"
          :disabled="savingPreference"
          @click="handleClose"
        >
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { renderMarkdownToHtml } from '@/utils/markdown'
import { useAppStore, useAuthStore } from '@/stores'
import { userAPI } from '@/api'

/**
 * 引流弹窗（可配置 Markdown 文案）
 *
 * 展示规则：
 * - 官网首页（PUBLIC_PATHS）：用户点击「今日不再提醒」后，当天不再弹（localStorage 按日期）。
 * - 控制台（DASHBOARD_PATHS）：用户点击「不再提醒」后，按用户维度永久不弹（后端存储）。
 * - 当用户已登录且已设置「不再提醒」时：官网首页与控制台都不再展示弹窗。
 * - 管理员：控制台不弹。
 */
type PopupContext = 'public' | 'dashboard'

// 只在官网首页展示；使用文档页/定价页不展示。
const PUBLIC_PATHS = new Set(['/', '/home'])
const DASHBOARD_PATHS = new Set(['/dashboard', '/keys', '/usage', '/redeem', '/profile', '/subscriptions'])

// 仅用于 public 页「今日不再提醒」：避免引入登录态依赖。
const LOCAL_KEY_PUBLIC_DISMISSED_DATE = 'attract-popup:public:dismissed-date'

const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const show = ref(false)
const context = ref<PopupContext | null>(null)

const savingPreference = ref(false)
const dashboardPopupDisabled = ref<boolean | null>(null)
const dashboardPrefLoading = ref(false)

function getTodayLocalISODate(): string {
  const now = new Date()
  const y = now.getFullYear()
  const m = String(now.getMonth() + 1).padStart(2, '0')
  const d = String(now.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

function isPublicDismissedToday(): boolean {
  return localStorage.getItem(LOCAL_KEY_PUBLIC_DISMISSED_DATE) === getTodayLocalISODate()
}

function setPublicDismissedToday(): void {
  localStorage.setItem(LOCAL_KEY_PUBLIC_DISMISSED_DATE, getTodayLocalISODate())
}

async function loadDashboardPreferenceIfNeeded(): Promise<void> {
  const hasToken = !!localStorage.getItem('auth_token')
  if (!hasToken || authStore.isAdmin) {
    dashboardPopupDisabled.value = null
    return
  }
  if (dashboardPrefLoading.value) return
  if (dashboardPopupDisabled.value !== null) return

  dashboardPrefLoading.value = true
  try {
    const prefs = await userAPI.getUiPreferences()
    dashboardPopupDisabled.value = !!prefs.dashboard_attract_popup_disabled
  } catch {
    dashboardPopupDisabled.value = false
  } finally {
    dashboardPrefLoading.value = false
  }
}

const dialogTitle = computed(() => appStore.attractPopupTitle || t('attractPopup.title'))
const markdown = computed(() => (appStore.attractPopupMarkdown || '').trim())
const renderedHtml = computed(() => renderMarkdownToHtml(markdown.value))

const dismissButtonText = computed(() => {
  if (context.value === 'public') return t('attractPopup.dismissToday')
  if (context.value === 'dashboard') return t('attractPopup.dismissForever')
  return ''
})

async function maybeShow(): Promise<void> {
  if (show.value) return
  if (!markdown.value) return

  const path = route.path

  if (PUBLIC_PATHS.has(path)) {
    if (isPublicDismissedToday()) return
    if (authStore.isAdmin) return
    if (localStorage.getItem('auth_token')) {
      await loadDashboardPreferenceIfNeeded()
      if (dashboardPopupDisabled.value) return
    }
    context.value = 'public'
    show.value = true
    return
  }

  if (DASHBOARD_PATHS.has(path)) {
    if (!authStore.isAuthenticated || authStore.isAdmin) return
    await loadDashboardPreferenceIfNeeded()
    if (dashboardPopupDisabled.value) return
    context.value = 'dashboard'
    show.value = true
  }
}

function resetPopupState(): void {
  show.value = false
  context.value = null
}

async function handleClose(): Promise<void> {
  resetPopupState()
}

async function handleDismiss(): Promise<void> {
  if (!context.value) {
    resetPopupState()
    return
  }

  if (context.value === 'public') {
    setPublicDismissedToday()
    resetPopupState()
    return
  }

  if (context.value === 'dashboard') {
    if (!authStore.isAuthenticated || authStore.isAdmin) {
      resetPopupState()
      return
    }

    savingPreference.value = true
    try {
      const updated = await userAPI.updateUiPreferences({ dashboard_attract_popup_disabled: true })
      dashboardPopupDisabled.value = !!updated.dashboard_attract_popup_disabled
      resetPopupState()
    } catch (error) {
      appStore.showError((error as { message?: string }).message || t('common.error'))
    } finally {
      savingPreference.value = false
    }
  }
}

watch(
  () => authStore.user?.id,
  () => {
    dashboardPopupDisabled.value = null
  },
  { immediate: true }
)

watch(
  () => [route.path, markdown.value, authStore.isAuthenticated, authStore.isAdmin, appStore.publicSettingsLoaded],
  () => {
    void maybeShow()
  },
  { immediate: true }
)
</script>

<style scoped>
.attract-popup-markdown :deep(p) {
  margin: 0 0 0.75rem;
}

.attract-popup-markdown :deep(p:last-child) {
  margin-bottom: 0;
}

.attract-popup-markdown :deep(ul),
.attract-popup-markdown :deep(ol) {
  margin: 0.5rem 0 0.75rem;
  padding-left: 1.25rem;
}

.attract-popup-markdown :deep(li) {
  margin: 0.25rem 0;
}

.attract-popup-markdown :deep(a) {
  color: rgb(79 70 229);
  text-decoration: underline;
}

.attract-popup-markdown :deep(code) {
  padding: 0.1rem 0.35rem;
  border-radius: 0.375rem;
  background: rgba(15, 23, 42, 0.06);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  font-size: 0.875em;
}

.dark .attract-popup-markdown :deep(code) {
  background: rgba(148, 163, 184, 0.12);
}
</style>

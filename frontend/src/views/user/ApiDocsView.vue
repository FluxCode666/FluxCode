<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl">
      <header
        id="api-docs-page-title"
        class="relative overflow-hidden rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-800 sm:p-8"
      >
        <div class="pointer-events-none absolute inset-y-0 right-0 hidden w-1/3 bg-[radial-gradient(circle_at_top_right,_rgba(59,130,246,0.14),_transparent_68%)] dark:block" aria-hidden="true"></div>
        <div class="relative flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div class="max-w-3xl">
            <div class="flex items-center gap-2 text-sm font-medium text-primary-600 dark:text-primary-400">
              <Icon name="document" size="sm" aria-hidden="true" />
              <span>{{ t('apiDocs.eyebrow') }}</span>
            </div>
            <h1 class="mt-3 text-2xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-3xl">
              {{ t('apiDocs.title') }}
            </h1>
            <p class="mt-3 text-sm leading-6 text-gray-600 dark:text-dark-300 sm:text-base">
              {{ t('apiDocs.description') }}
            </p>
          </div>
          <div class="inline-flex items-center gap-2 self-start rounded-full bg-primary-50 px-3 py-1.5 text-xs font-medium text-primary-700 dark:bg-primary-900/25 dark:text-primary-300">
            <Icon name="shield" size="sm" aria-hidden="true" />
            {{ t('apiDocs.authenticatedOnly') }}
          </div>
        </div>
      </header>

      <div class="mt-6 lg:grid lg:grid-cols-[15.5rem_minmax(0,1fr)] lg:items-start lg:gap-8">
        <aside class="mb-5 lg:sticky lg:top-6 lg:mb-0 lg:self-start">
          <nav
            class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
            aria-labelledby="api-docs-navigation-title"
          >
            <div class="border-b border-gray-100 px-4 py-4 dark:border-dark-700">
              <p id="api-docs-navigation-title" class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('apiDocs.navigation.title') }}
              </p>
              <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-dark-400">
                {{ t('apiDocs.navigation.hint') }}
              </p>
            </div>

            <div class="overflow-x-auto px-2 py-2 lg:max-h-[calc(100vh-9rem)] lg:overflow-x-hidden lg:overflow-y-auto">
              <div class="flex min-w-max gap-2 lg:block lg:min-w-0 lg:space-y-2">
                <div
                  v-for="section in documentationSections"
                  :key="section.module"
                  class="w-[13.5rem] rounded-xl border border-gray-100 p-1.5 dark:border-dark-700 lg:w-auto lg:border-transparent"
                >
                  <div>
                    <button
                      :data-testid="'docs-nav-module-' + section.module"
                      type="button"
                      class="group flex w-full min-w-0 items-center gap-3 rounded-lg px-2.5 py-2 text-left text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:focus-visible:ring-offset-dark-800"
                      :class="sectionNavigationClass(section.module)"
                      :aria-expanded="expandedNavigationSections[section.module]"
                      :aria-controls="'docs-nav-' + section.module + '-endpoints'"
                      :aria-label="navigationToggleLabel(section.module)"
                      @click="toggleNavigationSection(section.module)"
                    >
                      <span
                        class="font-mono text-[11px] font-semibold tracking-wide"
                        :class="activeSection === section.module ? 'text-primary-500 dark:text-primary-300' : 'text-gray-400 dark:text-dark-500'"
                        aria-hidden="true"
                      >{{ section.number }}</span>
                      <span class="flex-1 whitespace-nowrap">{{ section.label }}</span>
                      <Icon
                        :name="expandedNavigationSections[section.module] ? 'chevronUp' : 'chevronDown'"
                        size="xs"
                        class="shrink-0"
                        aria-hidden="true"
                      />
                    </button>
                  </div>

                  <div
                    v-show="expandedNavigationSections[section.module]"
                    :id="'docs-nav-' + section.module + '-endpoints'"
                    class="mt-1 space-y-0.5 border-l border-gray-200 py-0.5 pl-3 dark:border-dark-600"
                  >
                    <a
                      v-for="endpoint in section.endpoints"
                      :key="endpoint.id"
                      :href="'#' + endpoint.id"
                      class="block rounded-md px-2 py-1.5 font-mono text-[11px] leading-4 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:focus-visible:ring-offset-dark-800"
                      :class="endpointNavigationClass(endpoint.id)"
                      :aria-current="activeTarget === endpoint.id ? 'location' : undefined"
                      :aria-label="t('apiDocs.navigation.goToEndpoint', { endpoint: endpoint.label })"
                      @click.prevent="navigateToTarget(section.module, endpoint.id)"
                    >{{ endpoint.label }}</a>
                  </div>
                </div>
              </div>
            </div>
          </nav>
        </aside>

        <main class="space-y-5" aria-labelledby="api-docs-page-title">
          <section
            id="access-key"
            class="scroll-mt-24 overflow-hidden rounded-2xl border border-primary-200 bg-white shadow-sm dark:border-primary-900/70 dark:bg-dark-800"
            aria-labelledby="access-key-title"
          >
            <div class="border-b border-primary-100 bg-primary-50/70 px-5 py-5 dark:border-primary-900/70 dark:bg-primary-900/15 sm:px-6">
              <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div class="flex min-w-0 gap-3">
                  <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-white text-primary-600 shadow-sm ring-1 ring-primary-100 dark:bg-dark-800 dark:text-primary-300 dark:ring-primary-900/70" aria-hidden="true">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <p class="font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-400">
                      01 · {{ t('apiDocs.navigation.accessKey') }}
                    </p>
                    <h2 id="access-key-title" tabindex="-1" data-docs-heading class="mt-1 text-lg font-semibold text-gray-900 outline-none dark:text-white">
                      {{ t('apiDocs.accessKey.title') }}
                    </h2>
                    <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">
                      {{ t('apiDocs.accessKey.description') }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2 self-start">
                  <span
                    class="inline-flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium"
                    :class="accessKeyStatusClass"
                    role="status"
                    aria-live="polite"
                  >
                    <span class="h-1.5 w-1.5 rounded-full" :class="accessKeyStatusDotClass" aria-hidden="true"></span>
                    {{ accessKeyStatusLabel }}
                  </span>
                  <button
                    type="button"
                    class="inline-flex shrink-0 items-center justify-center gap-2 rounded-lg border border-primary-200 bg-white px-3 py-2 text-sm font-medium text-primary-700 transition-colors hover:bg-primary-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:border-primary-800 dark:bg-dark-800 dark:text-primary-300 dark:hover:bg-primary-900/25 dark:focus-visible:ring-offset-dark-800"
                    :aria-expanded="expandedSections.accessKey"
                    aria-controls="access-key-content"
                    :aria-label="sectionToggleLabel('accessKey')"
                    @click="toggleSection('accessKey')"
                  >
                    <span>{{ expandedSections.accessKey ? t('apiDocs.sections.collapse') : t('apiDocs.sections.expand') }}</span>
                    <Icon :name="expandedSections.accessKey ? 'chevronUp' : 'chevronDown'" size="sm" aria-hidden="true" />
                  </button>
                </div>
              </div>
            </div>

            <div v-show="expandedSections.accessKey" id="access-key-content" class="space-y-5 p-5 sm:p-6">
              <div id="user-access-key" tabindex="-1" class="scroll-mt-24 rounded-xl border border-gray-200 bg-gray-50 p-4 outline-none dark:border-dark-600 dark:bg-dark-900/70" aria-labelledby="user-access-key-value-label">
                <div class="flex flex-col gap-3 lg:flex-row lg:items-center">
                  <div class="min-w-0 flex-1">
                    <div id="user-access-key-value-label" class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
                      {{ t('apiDocs.accessKey.maskedValueLabel') }}
                    </div>
                    <div v-if="accessKeyLoading" class="mt-2 flex items-center gap-2 text-sm text-gray-500 dark:text-dark-400" role="status">
                      <span class="h-4 w-4 animate-spin rounded-full border-2 border-primary-200 border-t-primary-600 dark:border-primary-900 dark:border-t-primary-400" aria-hidden="true"></span>
                      {{ t('apiDocs.accessKey.loading') }}
                    </div>
                    <code
                      v-else-if="accessKey"
                      data-testid="user-access-key-value"
                      class="mt-2 block break-all rounded-lg bg-gray-900 px-3 py-2.5 font-mono text-sm text-gray-100"
                    >{{ maskedAccessKey }}</code>
                    <p v-else class="mt-2 text-sm text-gray-500 dark:text-dark-400">
                      {{ t('apiDocs.accessKey.emptyHint') }}
                    </p>
                  </div>

                  <div class="flex flex-wrap gap-2 lg:flex-none">
                    <button
                      v-if="!accessKeyAvailable"
                      type="button"
                      data-testid="user-access-key-unavailable"
                      class="btn btn-secondary"
                      disabled
                    >
                      <Icon name="lock" size="sm" class="mr-2" aria-hidden="true" />
                      {{ t('apiDocs.accessKey.unavailable') }}
                    </button>
                    <button
                      v-else-if="!accessKeyExists"
                      type="button"
                      data-testid="user-access-key-generate"
                      class="btn btn-primary"
                      :disabled="accessKeyLoading || generatingAccessKey"
                      @click="generateAccessKey"
                    >
                      <span v-if="generatingAccessKey" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white/40 border-t-white" aria-hidden="true"></span>
                      <Icon v-else name="plus" size="sm" class="mr-2" aria-hidden="true" />
                      {{ generatingAccessKey ? t('apiDocs.accessKey.generating') : t('apiDocs.accessKey.generate') }}
                    </button>
                    <button
                      v-else
                      type="button"
                      data-testid="user-access-key-copy"
                      class="btn btn-secondary"
                      :disabled="!accessKey"
                      @click="copyAccessKey"
                    >
                      <Icon name="clipboard" size="sm" class="mr-2" aria-hidden="true" />
                      {{ t('apiDocs.accessKey.copy') }}
                    </button>
                  </div>
                </div>
              </div>

              <div
                v-if="!accessKeyAvailable"
                data-testid="user-access-key-configuration-required"
                class="flex items-start gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm leading-6 text-red-800 dark:border-red-900/70 dark:bg-red-900/15 dark:text-red-200"
                role="alert"
              >
                <Icon name="lock" size="sm" class="mt-0.5 shrink-0" aria-hidden="true" />
                <p>{{ t('apiDocs.accessKey.configurationRequired') }}</p>
              </div>
              <div v-else class="flex items-start gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-6 text-amber-800 dark:border-amber-900/70 dark:bg-amber-900/15 dark:text-amber-200">
                <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" aria-hidden="true" />
                <p>{{ t('apiDocs.accessKey.securityHint') }}</p>
              </div>

              <p v-if="accessKeyCreatedAt" class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('apiDocs.accessKey.createdAt', { date: accessKeyCreatedAt }) }}
              </p>

              <div id="authentication" tabindex="-1" class="scroll-mt-24 border-t border-gray-100 pt-5 outline-none dark:border-dark-700" aria-labelledby="authentication-title">
                <div class="flex items-center gap-2">
                  <Icon name="lock" size="md" class="text-primary-600 dark:text-primary-400" aria-hidden="true" />
                  <h3 id="authentication-title" class="text-base font-semibold text-gray-900 dark:text-white">
                    {{ t('apiDocs.authentication.title') }}
                  </h3>
                </div>
                <div class="mt-4 grid gap-4 lg:grid-cols-2">
                  <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900/70">
                    <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
                      {{ t('apiDocs.authentication.baseUrl') }}
                    </p>
                    <code class="mt-2 block break-all font-mono text-sm text-gray-900 dark:text-white">{{ openApiBase }}</code>
                  </div>
                  <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-900/70">
                    <p class="text-xs font-medium uppercase tracking-wide text-gray-500 dark:text-dark-400">
                      {{ t('apiDocs.authentication.header') }}
                    </p>
                    <code class="mt-2 block break-all font-mono text-sm text-gray-900 dark:text-white">X-User-Access-Key: YOUR_USER_ACCESS_KEY</code>
                  </div>
                </div>
                <p class="mt-4 text-sm leading-6 text-gray-600 dark:text-dark-300">
                  {{ t('apiDocs.authentication.hint') }}
                </p>
              </div>

              <div class="space-y-5 border-t border-gray-100 pt-5 dark:border-dark-700">
                <ApiEndpointCard
                  v-for="endpoint in accessKeyEndpoints"
                  :key="endpoint.id"
                  :endpoint="endpoint"
                  :base-url="userApiBase"
                />
              </div>
            </div>
          </section>

          <section
            id="account"
            class="scroll-mt-24 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
            aria-labelledby="account-title"
          >
            <div class="border-b border-gray-100 bg-gray-50/80 px-5 py-5 dark:border-dark-700 dark:bg-dark-900/50 sm:px-6">
              <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div class="flex min-w-0 gap-3">
                  <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-emerald-50 text-emerald-600 ring-1 ring-emerald-100 dark:bg-emerald-900/20 dark:text-emerald-300 dark:ring-emerald-900/50" aria-hidden="true">
                    <Icon name="creditCard" size="md" />
                  </span>
                  <div>
                    <p class="font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-emerald-700 dark:text-emerald-300">
                      02 · {{ t('apiDocs.navigation.account') }}
                    </p>
                    <h2 id="account-title" tabindex="-1" data-docs-heading class="mt-1 text-lg font-semibold text-gray-900 outline-none dark:text-white">
                      {{ t('apiDocs.account.title') }}
                    </h2>
                    <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">
                      {{ t('apiDocs.account.description') }}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  class="inline-flex shrink-0 items-center justify-center gap-2 self-start rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700 dark:focus-visible:ring-offset-dark-800"
                  :aria-expanded="expandedSections.account"
                  aria-controls="account-content"
                  :aria-label="sectionToggleLabel('account')"
                  @click="toggleSection('account')"
                >
                  <span>{{ expandedSections.account ? t('apiDocs.sections.collapse') : t('apiDocs.sections.expand') }}</span>
                  <Icon :name="expandedSections.account ? 'chevronUp' : 'chevronDown'" size="sm" aria-hidden="true" />
                </button>
              </div>
            </div>

            <div v-show="expandedSections.account" id="account-content" class="p-5 sm:p-6">
              <ApiEndpointCard :endpoint="balanceEndpoint" :base-url="openApiBase" />
            </div>
          </section>

          <section
            id="usage"
            class="scroll-mt-24 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
            aria-labelledby="usage-title"
          >
            <div class="border-b border-gray-100 bg-gray-50/80 px-5 py-5 dark:border-dark-700 dark:bg-dark-900/50 sm:px-6">
              <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div class="flex min-w-0 gap-3">
                  <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-sky-50 text-sky-600 ring-1 ring-sky-100 dark:bg-sky-900/20 dark:text-sky-300 dark:ring-sky-900/50" aria-hidden="true">
                    <Icon name="chartBar" size="md" />
                  </span>
                  <div>
                    <p class="font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-sky-700 dark:text-sky-300">
                      03 · {{ t('apiDocs.navigation.usage') }}
                    </p>
                    <h2 id="usage-title" tabindex="-1" data-docs-heading class="mt-1 text-lg font-semibold text-gray-900 outline-none dark:text-white">
                      {{ t('apiDocs.usage.title') }}
                    </h2>
                    <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">
                      {{ t('apiDocs.usage.description') }}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  class="inline-flex shrink-0 items-center justify-center gap-2 self-start rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700 dark:focus-visible:ring-offset-dark-800"
                  :aria-expanded="expandedSections.usage"
                  aria-controls="usage-content"
                  :aria-label="sectionToggleLabel('usage')"
                  @click="toggleSection('usage')"
                >
                  <span>{{ expandedSections.usage ? t('apiDocs.sections.collapse') : t('apiDocs.sections.expand') }}</span>
                  <Icon :name="expandedSections.usage ? 'chevronUp' : 'chevronDown'" size="sm" aria-hidden="true" />
                </button>
              </div>
            </div>

            <div v-show="expandedSections.usage" id="usage-content" class="space-y-5 p-5 sm:p-6">
              <ApiEndpointCard
                v-for="endpoint in usageEndpoints"
                :key="endpoint.id"
                :endpoint="endpoint"
                :base-url="openApiBase"
              />
            </div>
          </section>

          <section
            id="api-key-management"
            class="scroll-mt-24 overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800"
            aria-labelledby="api-keys-title"
          >
            <div class="border-b border-gray-100 bg-gray-50/80 px-5 py-5 dark:border-dark-700 dark:bg-dark-900/50 sm:px-6">
              <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div class="flex min-w-0 gap-3">
                  <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-violet-50 text-violet-600 ring-1 ring-violet-100 dark:bg-violet-900/20 dark:text-violet-300 dark:ring-violet-900/50" aria-hidden="true">
                    <Icon name="key" size="md" />
                  </span>
                  <div>
                    <p class="font-mono text-[11px] font-semibold uppercase tracking-[0.16em] text-violet-700 dark:text-violet-300">
                      04 · {{ t('apiDocs.navigation.apiKeys') }}
                    </p>
                    <h2 id="api-keys-title" tabindex="-1" data-docs-heading class="mt-1 text-lg font-semibold text-gray-900 outline-none dark:text-white">
                      {{ t('apiDocs.apiKeys.title') }}
                    </h2>
                    <p class="mt-2 max-w-3xl text-sm leading-6 text-gray-600 dark:text-dark-300">
                      {{ t('apiDocs.apiKeys.description') }}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  class="inline-flex shrink-0 items-center justify-center gap-2 self-start rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700 dark:focus-visible:ring-offset-dark-800"
                  :aria-expanded="expandedSections.apiKeys"
                  aria-controls="api-keys-content"
                  :aria-label="sectionToggleLabel('apiKeys')"
                  @click="toggleSection('apiKeys')"
                >
                  <span>{{ expandedSections.apiKeys ? t('apiDocs.sections.collapse') : t('apiDocs.sections.expand') }}</span>
                  <Icon :name="expandedSections.apiKeys ? 'chevronUp' : 'chevronDown'" size="sm" aria-hidden="true" />
                </button>
              </div>
            </div>

            <div v-show="expandedSections.apiKeys" id="api-keys-content" class="space-y-5 p-5 sm:p-6">
              <ApiEndpointCard
                v-for="endpoint in apiKeyEndpoints"
                :key="endpoint.id"
                :endpoint="endpoint"
                :base-url="openApiBase"
              />
            </div>
          </section>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { userAccessKeyAPI } from '@/api/userAccessKey'
import AppLayout from '@/components/layout/AppLayout.vue'
import ApiEndpointCard from '@/components/docs/ApiEndpointCard.vue'
import type {
  ApiEndpointDocumentation,
  ApiEndpointRequestParameter,
  ApiEndpointResponseParameter
} from '@/components/docs/apiEndpointDocumentation'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'

type DocumentationModule = 'accessKey' | 'account' | 'usage' | 'apiKeys'

const { t, locale } = useI18n()
const appStore = useAppStore()
const { copySensitiveToClipboard } = useClipboard()

const accessKey = ref('')
const accessKeyExists = ref(false)
const accessKeyAvailable = ref(true)
const accessKeyLoading = ref(true)
const generatingAccessKey = ref(false)
const accessKeyCreatedAtRaw = ref<string | null>(null)
const activeSection = ref<DocumentationModule>('accessKey')
const activeTarget = ref('access-key')
const expandedSections = ref<Record<DocumentationModule, boolean>>({
  accessKey: true,
  account: true,
  usage: true,
  apiKeys: true
})
const expandedNavigationSections = ref<Record<DocumentationModule, boolean>>({
  accessKey: true,
  account: true,
  usage: true,
  apiKeys: true
})
let sectionObserver: IntersectionObserver | null = null

const publicBaseUrl = computed(() => {
  const fallback = typeof window === 'undefined' ? '' : window.location.origin
  const configuredBaseUrl = (appStore.apiBaseUrl || '').trim()

  if (!configuredBaseUrl || configuredBaseUrl.startsWith('/')) {
    return fallback
  }

  return configuredBaseUrl.replace(/\/api\/v1\/?$/, '').replace(/\/+$/, '')
})

const openApiBase = computed(() => publicBaseUrl.value + '/api/v1/openapi')
const userApiBase = computed(() => publicBaseUrl.value + '/api/v1')

const documentationSections = computed(() => [
  {
    id: 'access-key',
    module: 'accessKey' as const,
    number: '01',
    label: t('apiDocs.navigation.accessKey'),
    endpoints: [
      { id: 'access-key-read', label: 'GET /user/access-key' },
      { id: 'access-key-generate', label: 'POST /user/access-key' },
      { id: 'authentication', label: t('apiDocs.authentication.title') }
    ]
  },
  {
    id: 'account',
    module: 'account' as const,
    number: '02',
    label: t('apiDocs.navigation.account'),
    endpoints: [
      { id: 'balance', label: 'GET /balance' }
    ]
  },
  {
    id: 'usage',
    module: 'usage' as const,
    number: '03',
    label: t('apiDocs.navigation.usage'),
    endpoints: [
      { id: 'usage-list', label: 'GET /usage' },
      { id: 'usage-stats', label: 'GET /usage/stats' },
      { id: 'usage-detail', label: 'GET /usage/:id' }
    ]
  },
  {
    id: 'api-key-management',
    module: 'apiKeys' as const,
    number: '04',
    label: t('apiDocs.navigation.apiKeys'),
    endpoints: [
      { id: 'api-keys-list', label: 'GET /keys' },
      { id: 'api-key-create', label: 'POST /keys' },
      { id: 'api-key-get', label: 'GET /keys/:id' },
      { id: 'api-key-update', label: 'PUT /keys/:id' },
      { id: 'api-key-delete', label: 'DELETE /keys/:id' },
      { id: 'available-groups', label: 'GET /groups/available' }
    ]
  }
])

const documentationTargets = computed(() => {
  const targets: Array<{ id: string; module: DocumentationModule }> = []

  documentationSections.value.forEach((section) => {
    targets.push({ id: section.id, module: section.module })
    section.endpoints.forEach((endpoint) => {
      targets.push({ id: endpoint.id, module: section.module })
    })
  })

  return targets
})

const accessKeyCreatedAt = computed(() => {
  if (!accessKeyCreatedAtRaw.value) return ''

  const date = new Date(accessKeyCreatedAtRaw.value)
  if (Number.isNaN(date.getTime())) return ''

  return date.toLocaleString(locale.value)
})

const maskedAccessKey = computed(() => maskAccessKey(accessKey.value))

const accessKeyStatusClass = computed(() => {
  if (!accessKeyAvailable.value) {
    return 'bg-red-100 text-red-700 dark:bg-red-900/35 dark:text-red-300'
  }
  return accessKeyExists.value
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/35 dark:text-emerald-300'
    : 'bg-amber-100 text-amber-700 dark:bg-amber-900/35 dark:text-amber-300'
})

const accessKeyStatusDotClass = computed(() => {
  if (!accessKeyAvailable.value) return 'bg-red-500'
  return accessKeyExists.value ? 'bg-emerald-500' : 'bg-amber-500'
})

const accessKeyStatusLabel = computed(() => {
  if (!accessKeyAvailable.value) return t('apiDocs.accessKey.configurationRequiredStatus')
  return accessKeyExists.value ? t('apiDocs.accessKey.ready') : t('apiDocs.accessKey.notGenerated')
})

function localized(chinese: string, english: string): string {
  return locale.value.startsWith('zh') ? chinese : english
}

function requestParameter(
  name: string,
  location: 'header' | 'query' | 'path' | 'body',
  required: boolean,
  type: string,
  description: string,
  options: Pick<ApiEndpointRequestParameter, 'defaultValue' | 'example'> = {}
): ApiEndpointRequestParameter {
  return {
    name,
    location: t('apiDocs.endpointDocumentation.locations.' + location),
    required,
    type,
    description,
    ...options
  }
}

function responseEnvelope(data: ApiEndpointResponseParameter[]): ApiEndpointResponseParameter[] {
  return [
    { name: 'code', type: 'integer', description: localized('业务状态码；成功固定为 0。', 'Business status code; 0 indicates success.'), example: '0' },
    { name: 'message', type: 'string', description: localized('业务状态说明；成功固定为 success。', 'Business status message; success returns success.'), example: 'success' },
    ...data
  ]
}

function userAccessKeyHeader(): ApiEndpointRequestParameter {
  return requestParameter(
    'X-User-Access-Key',
    'header',
    true,
    'string',
    localized('用户开发者密钥。仅接受 uk_ 开头的专用密钥，不使用 Bearer Token。', 'User developer key. Use the dedicated uk_ key, not a Bearer token.'),
    { example: 'uk_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx' }
  )
}

function loginJwtHeader(): ApiEndpointRequestParameter {
  return requestParameter(
    'Authorization',
    'header',
    true,
    'Bearer JWT',
    localized('网页登录会话的 JWT。此接口不接受 X-User-Access-Key。', 'JWT from the signed-in web session. This endpoint does not accept X-User-Access-Key.'),
    { example: 'Bearer YOUR_LOGIN_JWT' }
  )
}

function jsonContentTypeHeader(): ApiEndpointRequestParameter {
  return requestParameter(
    'Content-Type',
    'header',
    true,
    'application/json',
    localized('请求正文的编码类型。', 'Request body encoding.'),
    { example: 'application/json' }
  )
}

function apiKeyPathParameter(): ApiEndpointRequestParameter {
  return requestParameter('id', 'path', true, 'int64', localized('要操作的 API Key ID。', 'ID of the API key to operate on.'), { example: '123' })
}

function usagePathParameter(): ApiEndpointRequestParameter {
  return requestParameter('id', 'path', true, 'int64', localized('要查询的使用记录 ID。', 'ID of the usage record to retrieve.'), { example: '901' })
}

function apiKeyResponseFields(): ApiEndpointResponseParameter[] {
  return responseEnvelope([
    { name: 'data.id', type: 'int64', description: localized('API Key 的唯一标识。', 'Unique API key identifier.'), example: '123' },
    { name: 'data.key', type: 'string', description: localized('完整 API Key；请仅在安全环境保存，页面展示时应脱敏。', 'Full API key. Store it securely and mask it in user interfaces.'), example: 'sk_***' },
    { name: 'data.name', type: 'string', description: localized('用户定义的 Key 名称。', 'User-defined key name.'), example: 'automation-key' },
    { name: 'data.group_id', type: 'int64 | null', description: localized('绑定的分组 ID。', 'Bound group ID.'), example: '1' },
    { name: 'data.status', type: 'active | inactive', description: localized('当前启用状态。', 'Current activation status.'), example: 'active' },
    { name: 'data.ip_whitelist', type: 'string[] | null', description: localized('允许访问的 IP 或 CIDR 列表。', 'Allowed IP or CIDR rules.'), example: '["203.0.113.10"]' },
    { name: 'data.ip_blacklist', type: 'string[] | null', description: localized('拒绝访问的 IP 或 CIDR 列表。', 'Denied IP or CIDR rules.'), example: '[]' },
    { name: 'data.quota / quota_used', type: 'number', description: localized('USD 配额及已使用额度；quota 为 0 表示不限额。', 'USD quota and amount used; quota 0 means unlimited.'), example: '10 / 1.25' },
    { name: 'data.expires_at', type: 'RFC3339 | null', description: localized('过期时间；null 表示永不过期。', 'Expiration time; null means no expiration.'), example: '2026-08-30T00:00:00Z' },
    { name: 'data.rate_limit_*', type: 'number', description: localized('5 小时、1 天、7 天窗口的 USD 限额；0 表示不限额。', 'USD limits for 5h, 1d, and 7d windows; 0 means unlimited.'), example: '2' },
    { name: 'data.usage_*', type: 'number', description: localized('各限速窗口内已使用额度。', 'Amount used in each rate-limit window.'), example: '0.4' },
    { name: 'data.current_concurrency', type: 'integer', description: localized('当前并发请求数。', 'Current concurrent request count.'), example: '0' },
    { name: 'data.created_at / updated_at', type: 'RFC3339', description: localized('创建和最后更新时间。', 'Creation and last-update timestamps.'), example: '2026-07-30T08:00:00Z' }
  ])
}

function accessKeyResponseFields(): ApiEndpointResponseParameter[] {
  return responseEnvelope([
    { name: 'data.key', type: 'string', description: localized('完整用户密钥；服务端只返回给已登录的所有者，页面必须脱敏显示。', 'Full user key. The server returns it only to the signed-in owner; user interfaces must mask it.'), example: 'uk_***' },
    { name: 'data.exists', type: 'boolean', description: localized('是否已经生成过用户密钥。', 'Whether a user key has already been generated.'), example: 'true' },
    { name: 'data.available', type: 'boolean', description: localized('是否已配置 TOTP_ENCRYPTION_KEY 并可安全恢复密钥。', 'Whether TOTP_ENCRYPTION_KEY is configured so the key can be recovered safely.'), example: 'true' },
    { name: 'data.created_at', type: 'RFC3339 | omitted', description: localized('已生成时的创建时间。', 'Creation time when a key exists.'), example: '2026-07-30T08:00:00Z' },
    { name: 'Cache-Control', type: 'HTTP response header', description: localized('固定为 no-store, private，避免浏览器或代理缓存密钥。', 'Always no-store, private to prevent browser or proxy caching.'), example: 'no-store, private' }
  ])
}

const accessKeyEndpoints = computed<ApiEndpointDocumentation[]>(() => [
  {
    id: 'access-key-read',
    method: 'GET',
    path: '/user/access-key',
    title: localized('读取用户密钥', 'Read User Access Key'),
    description: localized('读取当前登录用户已经生成的用户密钥；仅用于已登录会话，不使用开发者密钥鉴权。', 'Read the current signed-in user’s generated key. This is a web-session endpoint, not a developer-key endpoint.'),
    requestDescription: localized('不接收 Query 参数或请求正文；必须携带当前网页登录会话的 Authorization Bearer JWT。', 'No query parameters or request body. Send the Authorization Bearer JWT for the current signed-in web session.'),
    requestParameters: [loginJwtHeader()],
    responseParameters: accessKeyResponseFields(),
    requestExample: [
      'curl --request GET "' + userApiBase.value + '/user/access-key" \\',
      '  --header "Authorization: Bearer YOUR_LOGIN_JWT"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "key": "uk_***",\n    "exists": true,\n    "available": true,\n    "created_at": "2026-07-30T08:00:00Z"\n  }\n}'
  },
  {
    id: 'access-key-generate',
    method: 'POST',
    path: '/user/access-key',
    title: localized('生成用户密钥', 'Generate User Access Key'),
    description: localized('首次调用生成密钥；已生成时返回原密钥，不会隐式轮换。未配置 TOTP_ENCRYPTION_KEY 时返回 503。', 'Generates a key on the first call and returns the existing key afterwards without rotating it. Returns 503 when TOTP_ENCRYPTION_KEY is not configured.'),
    requestDescription: localized('不接收 Query 参数或请求正文；必须携带当前网页登录会话的 Authorization Bearer JWT。该请求具备幂等性。', 'No query parameters or request body. Send the Authorization Bearer JWT for the current signed-in web session. This request is idempotent.'),
    requestParameters: [loginJwtHeader()],
    responseParameters: accessKeyResponseFields(),
    requestExample: [
      'curl --request POST "' + userApiBase.value + '/user/access-key" \\',
      '  --header "Authorization: Bearer YOUR_LOGIN_JWT"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "key": "uk_***",\n    "exists": true,\n    "available": true,\n    "created_at": "2026-07-30T08:00:00Z"\n  }\n}'
  }
])

const balanceEndpoint = computed<ApiEndpointDocumentation>(() => ({
  id: 'balance',
  method: 'GET',
  path: '/balance',
  title: t('apiDocs.balance.title'),
  description: t('apiDocs.balance.description'),
  requestDescription: localized('不接收 Query 参数或请求正文；只需在 Header 中携带用户开发者密钥。', 'No query parameters or request body. Send only the user developer key in the request header.'),
  requestParameters: [userAccessKeyHeader()],
  responseParameters: responseEnvelope([
    { name: 'data.user_id', type: 'int64', description: localized('当前用户 ID。', 'Current user ID.'), example: '42' },
    { name: 'data.balance', type: 'number', description: localized('当前可用余额。', 'Current available balance.'), example: '12.5' },
    { name: 'data.currency', type: 'string', description: localized('余额币种，固定为 USD。', 'Balance currency, always USD.'), example: 'USD' }
  ]),
  requestExample: [
    'curl --request GET "' + openApiBase.value + '/balance" \\',
    '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY"'
  ].join('\n'),
  responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "user_id": 42,\n    "balance": 12.5,\n    "currency": "USD"\n  }\n}'
}))

const usageEndpoints = computed<ApiEndpointDocumentation[]>(() => [
  {
    id: 'usage-list',
    method: 'GET',
    path: '/usage',
    title: t('apiDocs.usage.list'),
    description: localized('分页检索当前用户的使用记录，可按时间、模型、API Key 和请求类型筛选。', 'List the current user’s usage records with pagination and filters.'),
    requestDescription: localized('所有筛选与分页项均通过 Query 传递。request_type 与 stream 不能同时决定类型：提供 request_type 时优先使用它。', 'Pass all filters and pagination through the query string. request_type takes precedence over stream when both are sent.'),
    requestParameters: [
      userAccessKeyHeader(),
      requestParameter('page', 'query', false, 'integer', localized('页码，必须为正整数。', 'Page number; must be positive.'), { defaultValue: '1', example: '1' }),
      requestParameter('page_size', 'query', false, 'integer', localized('每页记录数，范围 1–1000。', 'Records per page, from 1 to 1000.'), { defaultValue: '20', example: '20' }),
      requestParameter('api_key_id', 'query', false, 'int64', localized('仅查询该用户自己的 API Key 记录。', 'Filter by one of the user’s own API keys.'), { example: '123' }),
      requestParameter('model', 'query', false, 'string', localized('按原始模型名精确筛选。', 'Exact match on the stored model name.'), { example: 'gpt-4.1' }),
      requestParameter('request_type', 'query', false, 'unknown | sync | stream | ws_v2 | embedding', localized('请求类型；提供后优先于 stream。', 'Request type; takes precedence over stream when provided.'), { example: 'stream' }),
      requestParameter('stream', 'query', false, 'boolean', localized('仅未传 request_type 时生效。', 'Only used when request_type is absent.'), { example: 'true' }),
      requestParameter('billing_type', 'query', false, 'int8', localized('计费类型，通常 0 为余额、1 为订阅。', 'Billing type; normally 0 for balance and 1 for subscription.'), { example: '0' }),
      requestParameter('start_date', 'query', false, 'YYYY-MM-DD', localized('开始日期，按 timezone 解析。', 'Start date, parsed in timezone.'), { example: '2026-07-01' }),
      requestParameter('end_date', 'query', false, 'YYYY-MM-DD', localized('结束日期，包含当天。', 'End date, inclusive of that day.'), { example: '2026-07-30' }),
      requestParameter('timezone', 'query', false, 'IANA timezone', localized('日期边界使用的时区。', 'Timezone used for date boundaries.'), { example: 'Asia/Shanghai' }),
      requestParameter('sort_by', 'query', false, 'created_at | model', localized('排序字段。', 'Sort field.'), { defaultValue: 'created_at' }),
      requestParameter('sort_order', 'query', false, 'asc | desc', localized('排序方向。', 'Sort direction.'), { defaultValue: 'desc' })
    ],
    responseParameters: responseEnvelope([
      { name: 'data.items', type: 'UsageLog[]', description: localized('使用记录列表，详情字段见下方单条记录接口。', 'Usage records; see the detail endpoint for record fields.'), example: '[{…}]' },
      { name: 'data.total', type: 'int64', description: localized('符合筛选条件的总记录数。', 'Total records matching the filters.'), example: '1' },
      { name: 'data.page / page_size / pages', type: 'integer', description: localized('当前页、页大小和总页数。', 'Current page, page size, and page count.'), example: '1 / 20 / 1' }
    ]),
    requestExample: [
      'curl --get "' + openApiBase.value + '/usage" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY" \\',
      '  --data-urlencode "page=1" \\',
      '  --data-urlencode "page_size=20" \\',
      '  --data-urlencode "start_date=2026-07-01" \\',
      '  --data-urlencode "end_date=2026-07-30" \\',
      '  --data-urlencode "sort_order=desc"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "items": [{ "id": 901, "api_key_id": 123, "model": "gpt-4.1", "total_cost": 0.003 }],\n    "total": 1,\n    "page": 1,\n    "page_size": 20,\n    "pages": 1\n  }\n}'
  },
  {
    id: 'usage-stats',
    method: 'GET',
    path: '/usage/stats',
    title: t('apiDocs.usage.stats'),
    description: localized('汇总当前用户在指定时间范围内的调用次数、Token 和费用。', 'Summarize the current user’s requests, tokens, and cost for a time range.'),
    requestDescription: localized('通过 start_date 和 end_date 同时传入来自定义统计范围；未同时传入时使用 period。所有日期按 timezone 解释。', 'Pass start_date and end_date together for a custom range; otherwise use period. All dates are interpreted in timezone.'),
    requestParameters: [
      userAccessKeyHeader(),
      requestParameter('api_key_id', 'query', false, 'int64', localized('仅统计该用户自己的 API Key。', 'Limit statistics to one of the user’s API keys.'), { example: '123' }),
      requestParameter('start_date', 'query', false, 'YYYY-MM-DD', localized('和 end_date 同时提供时使用自定义范围。', 'Used as a custom range only together with end_date.'), { example: '2026-07-01' }),
      requestParameter('end_date', 'query', false, 'YYYY-MM-DD', localized('和 start_date 同时提供时使用自定义范围。', 'Used as a custom range only together with start_date.'), { example: '2026-07-30' }),
      requestParameter('period', 'query', false, 'today | week | month', localized('未提供完整日期范围时使用。', 'Used when a complete date range is not provided.'), { defaultValue: 'today' }),
      requestParameter('timezone', 'query', false, 'IANA timezone', localized('日期边界使用的时区。', 'Timezone used for date boundaries.'), { example: 'Asia/Shanghai' })
    ],
    responseParameters: responseEnvelope([
      { name: 'data.total_requests', type: 'int64', description: localized('请求总数。', 'Total request count.'), example: '28' },
      { name: 'data.total_input_tokens / total_output_tokens', type: 'int64', description: localized('输入和输出 Token 总数。', 'Total input and output tokens.'), example: '12000 / 3200' },
      { name: 'data.total_cache_tokens / total_tokens', type: 'int64', description: localized('缓存 Token 与全部 Token 总数。', 'Total cache and all tokens.'), example: '800 / 16000' },
      { name: 'data.total_cost / total_actual_cost', type: 'number', description: localized('显示费用与实际费用总和。', 'Displayed and actual total cost.'), example: '1.28 / 1.28' },
      { name: 'data.average_duration_ms', type: 'number', description: localized('平均请求耗时（毫秒）。', 'Average request duration in milliseconds.'), example: '734.5' }
    ]),
    requestExample: [
      'curl --get "' + openApiBase.value + '/usage/stats" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY" \\',
      '  --data-urlencode "period=month" \\',
      '  --data-urlencode "timezone=Asia/Shanghai"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "total_requests": 28,\n    "total_input_tokens": 12000,\n    "total_output_tokens": 3200,\n    "total_tokens": 16000,\n    "total_cost": 1.28,\n    "average_duration_ms": 734.5\n  }\n}'
  },
  {
    id: 'usage-detail',
    method: 'GET',
    path: '/usage/:id',
    title: t('apiDocs.usage.get'),
    description: localized('读取一条属于当前用户的使用记录。', 'Retrieve one usage record belonging to the current user.'),
    requestDescription: localized('记录 ID 必须作为路径参数传入；不接收 Query 参数或请求正文。', 'Pass the record ID as a path parameter. No query parameters or request body are accepted.'),
    requestParameters: [userAccessKeyHeader(), usagePathParameter()],
    responseParameters: responseEnvelope([
      { name: 'data.id / user_id / api_key_id', type: 'int64', description: localized('使用记录、所属用户和 API Key 的标识。', 'Identifiers for the record, its user, and API key.'), example: '901 / 42 / 123' },
      { name: 'data.request_id / model', type: 'string', description: localized('上游请求标识和模型名。', 'Upstream request identifier and model name.'), example: 'req_abc / gpt-4.1' },
      { name: 'data.input_tokens / output_tokens / cache_*_tokens', type: 'integer', description: localized('各类 Token 用量。', 'Token usage by category.'), example: '120 / 50 / 0' },
      { name: 'data.total_cost / actual_cost / rate_multiplier', type: 'number', description: localized('费用与费率倍率。', 'Costs and rate multiplier.'), example: '0.003 / 0.003 / 1' },
      { name: 'data.request_type / stream / billing_type', type: 'string | boolean | int8', description: localized('请求方式、流式标记和计费类型。', 'Request mode, stream flag, and billing type.'), example: 'stream / true / 0' },
      { name: 'data.duration_ms / first_token_ms', type: 'integer | null', description: localized('总耗时和首 Token 耗时。', 'Total and first-token latency.'), example: '820 / 260' },
      { name: 'data.created_at', type: 'RFC3339', description: localized('记录创建时间。', 'Record creation time.'), example: '2026-07-30T08:00:00Z' }
    ]),
    requestExample: [
      'curl --request GET "' + openApiBase.value + '/usage/901" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "id": 901,\n    "user_id": 42,\n    "api_key_id": 123,\n    "request_id": "req_abc",\n    "model": "gpt-4.1",\n    "input_tokens": 120,\n    "output_tokens": 50,\n    "total_cost": 0.003,\n    "request_type": "stream",\n    "stream": true,\n    "created_at": "2026-07-30T08:00:00Z"\n  }\n}'
  }
])

const apiKeyEndpoints = computed<ApiEndpointDocumentation[]>(() => [
  {
    id: 'api-keys-list',
    method: 'GET',
    path: '/keys',
    title: t('apiDocs.apiKeys.list'),
    description: localized('分页列出当前用户的 API Key。', 'List the current user’s API keys with pagination.'),
    requestDescription: localized('所有筛选、排序和分页项都通过 Query 传递；不接收请求正文。', 'Pass filtering, sorting, and pagination through the query string. No request body is accepted.'),
    requestParameters: [
      userAccessKeyHeader(),
      requestParameter('page', 'query', false, 'integer', localized('页码。', 'Page number.'), { defaultValue: '1' }),
      requestParameter('page_size', 'query', false, 'integer', localized('每页数量，范围 1–1000。', 'Items per page, from 1 to 1000.'), { defaultValue: '20' }),
      requestParameter('search', 'query', false, 'string', localized('名称或 Key 的模糊搜索，最多保留 100 字节。', 'Fuzzy search by name or key; first 100 bytes are used.'), { example: 'automation' }),
      requestParameter('status', 'query', false, 'string', localized('按状态筛选。', 'Filter by status.'), { example: 'active' }),
      requestParameter('group_id', 'query', false, 'int64', localized('按分组筛选；0 表示未分组。', 'Filter by group; 0 means no group.'), { example: '1' }),
      requestParameter('sort_by', 'query', false, 'name | status | expires_at | last_used_at | created_at', localized('排序字段。', 'Sort field.'), { defaultValue: 'created_at' }),
      requestParameter('sort_order', 'query', false, 'asc | desc', localized('排序方向，不区分大小写。', 'Sort direction, case-insensitive.'), { defaultValue: 'desc' })
    ],
    responseParameters: responseEnvelope([
      { name: 'data.items', type: 'APIKey[]', description: localized('当前页的 API Key 对象。', 'API key objects for the current page.'), example: '[{…}]' },
      { name: 'data.total', type: 'int64', description: localized('符合筛选条件的总数。', 'Total matching keys.'), example: '1' },
      { name: 'data.page / page_size / pages', type: 'integer', description: localized('分页信息。', 'Pagination metadata.'), example: '1 / 20 / 1' }
    ]),
    requestExample: [
      'curl --get "' + openApiBase.value + '/keys" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY" \\',
      '  --data-urlencode "page=1" \\',
      '  --data-urlencode "status=active"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "items": [{ "id": 123, "key": "sk_***", "name": "automation-key", "status": "active" }],\n    "total": 1,\n    "page": 1,\n    "page_size": 20,\n    "pages": 1\n  }\n}'
  },
  {
    id: 'api-key-create',
    method: 'POST',
    path: '/keys',
    title: t('apiDocs.apiKeys.create'),
    description: localized('创建一个属于当前用户的 API Key。', 'Create an API key for the current user.'),
    requestDescription: localized('使用 application/json 请求正文。name 为唯一必填字段；建议附带 Idempotency-Key 以安全重试写入请求。', 'Use an application/json request body. name is the only required body field; send Idempotency-Key to safely retry the write request.'),
    requestParameters: [
      userAccessKeyHeader(),
      jsonContentTypeHeader(),
      requestParameter('Idempotency-Key', 'header', false, 'string', localized('建议传 UUID；部署关闭观察模式后为必填。相同键和相同请求会回放结果。', 'Recommended UUID; required when observe-only mode is disabled. Same key and request replays the result.'), { example: '7a0a2288-4465-4ad4-a4c9-04eaf50a6bd6' }),
      requestParameter('name', 'body', true, 'string', localized('API Key 名称，不能为空。', 'Non-empty API key name.'), { example: 'automation-key' }),
      requestParameter('group_id', 'body', false, 'int64 | null', localized('要绑定的、当前用户可用的分组。', 'An available group that the current user may use.'), { example: '1' }),
      requestParameter('custom_key', 'body', false, 'string', localized('自定义 Key；至少 16 位，仅允许字母、数字、下划线和连字符。', 'Custom key: at least 16 characters using letters, digits, underscores, and hyphens.'), { example: 'sk_custom_key_value' }),
      requestParameter('ip_whitelist', 'body', false, 'string[]', localized('允许访问的 IP 或 CIDR 规则列表。', 'Allowed IP or CIDR rule list.'), { example: '["203.0.113.10"]' }),
      requestParameter('ip_blacklist', 'body', false, 'string[]', localized('拒绝访问的 IP 或 CIDR 规则列表。', 'Denied IP or CIDR rule list.'), { example: '[]' }),
      requestParameter('system_prompt', 'body', false, 'string', localized('系统提示词。', 'System prompt.'), { example: 'You are a helpful assistant.' }),
      requestParameter('system_prompt_mode', 'body', false, 'inherit | passthrough | override | append', localized('系统提示词处理模式。', 'System prompt handling mode.'), { example: 'inherit' }),
      requestParameter('quota', 'body', false, 'number', localized('USD 配额，0 表示不限额。', 'USD quota; 0 means unlimited.'), { defaultValue: '0', example: '10' }),
      requestParameter('expires_in_days', 'body', false, 'integer', localized('大于 0 时设置过期天数。', 'Sets an expiration only when greater than 0.'), { example: '30' }),
      requestParameter('rate_limit_5h', 'body', false, 'number', localized('5 小时窗口的 USD 限额，0 表示不限额。', 'USD limit for the 5-hour window; 0 means unlimited.'), { defaultValue: '0', example: '2' }),
      requestParameter('rate_limit_1d', 'body', false, 'number', localized('1 天窗口的 USD 限额，0 表示不限额。', 'USD limit for the 1-day window; 0 means unlimited.'), { defaultValue: '0', example: '2' }),
      requestParameter('rate_limit_7d', 'body', false, 'number', localized('7 天窗口的 USD 限额，0 表示不限额。', 'USD limit for the 7-day window; 0 means unlimited.'), { defaultValue: '0', example: '2' })
    ],
    responseParameters: apiKeyResponseFields(),
    requestExample: [
      'curl --request POST "' + openApiBase.value + '/keys" \\',
      '  --header "Content-Type: application/json" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY" \\',
      '  --header "Idempotency-Key: 7a0a2288-4465-4ad4-a4c9-04eaf50a6bd6" \\',
      "  --data '{",
      '    "name": "automation-key",',
      '    "group_id": 1,',
      '    "ip_whitelist": ["203.0.113.10"],',
      '    "quota": 10,',
      '    "expires_in_days": 30,',
      '    "rate_limit_1d": 2',
      "  }'"
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "id": 123,\n    "key": "sk_***",\n    "name": "automation-key",\n    "group_id": 1,\n    "status": "active",\n    "quota": 10,\n    "quota_used": 0\n  }\n}'
  },
  {
    id: 'api-key-get',
    method: 'GET',
    path: '/keys/:id',
    title: t('apiDocs.apiKeys.get'),
    description: localized('获取当前用户的一把 API Key。', 'Retrieve one API key owned by the current user.'),
    requestDescription: localized('API Key ID 必须作为路径参数传入；不接收 Query 参数或请求正文。', 'Pass the API key ID as a path parameter. No query parameters or request body are accepted.'),
    requestParameters: [userAccessKeyHeader(), apiKeyPathParameter()],
    responseParameters: apiKeyResponseFields(),
    requestExample: [
      'curl --request GET "' + openApiBase.value + '/keys/123" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "id": 123,\n    "key": "sk_***",\n    "name": "automation-key",\n    "status": "active",\n    "created_at": "2026-07-30T08:00:00Z"\n  }\n}'
  },
  {
    id: 'api-key-update',
    method: 'PUT',
    path: '/keys/:id',
    title: t('apiDocs.apiKeys.update'),
    description: localized('更新当前用户的一把 API Key。注意：未传 ip_whitelist 或 ip_blacklist 会清空对应列表。', 'Update one API key. Important: omitting ip_whitelist or ip_blacklist clears that list.'),
    requestDescription: localized('使用 application/json 请求正文。请显式传入 ip_whitelist 与 ip_blacklist 的当前值或空数组；省略任一字段会清空该列表。', 'Use an application/json request body. Explicitly send current values or empty arrays for ip_whitelist and ip_blacklist; omitting either clears that list.'),
    requestParameters: [
      userAccessKeyHeader(),
      jsonContentTypeHeader(),
      apiKeyPathParameter(),
      requestParameter('name', 'body', false, 'string', localized('非空时更新名称。', 'Updates the name when non-empty.'), { example: 'automation-key-v2' }),
      requestParameter('group_id', 'body', false, 'int64', localized('指定可绑定分组；当前不支持用 null 解绑。', 'Set an available group; null cannot unbind the group.'), { example: '1' }),
      requestParameter('status', 'body', false, 'active | inactive', localized('新的启用状态。', 'New activation status.'), { example: 'inactive' }),
      requestParameter('ip_whitelist', 'body', false, 'string[]', localized('传空数组可清空；省略也会被当前实现清空。请显式传入要保留的值。', 'An empty array clears it; the current implementation also clears it when omitted. Send values to retain.'), { example: '["203.0.113.10"]' }),
      requestParameter('ip_blacklist', 'body', false, 'string[]', localized('传空数组可清空；省略也会被当前实现清空。请显式传入要保留的值。', 'An empty array clears it; the current implementation also clears it when omitted. Send values to retain.'), { example: '[]' }),
      requestParameter('system_prompt', 'body', false, 'string', localized('新的系统提示词内容。', 'New system prompt content.'), { example: 'You are a helpful assistant.' }),
      requestParameter('system_prompt_mode', 'body', false, 'inherit | passthrough | override | append', localized('系统提示词处理模式。', 'System prompt handling mode.'), { example: 'override' }),
      requestParameter('quota', 'body', false, 'number', localized('USD 配额，0 表示不限额。', 'USD quota; 0 means unlimited.'), { example: '20' }),
      requestParameter('expires_at', 'body', false, 'RFC3339 string', localized('传空字符串 "" 清除过期时间；省略或 null 不修改。', 'Send an empty string "" to clear expiration; omitting or null does not change it.'), { example: '2026-08-30T00:00:00Z' }),
      requestParameter('reset_quota', 'body', false, 'boolean', localized('为 true 时清零 quota_used。', 'When true, resets quota_used to zero.'), { example: 'true' }),
      requestParameter('rate_limit_5h', 'body', false, 'number', localized('5 小时窗口的 USD 限额，0 表示不限额。', 'USD limit for the 5-hour window; 0 means unlimited.'), { example: '2' }),
      requestParameter('rate_limit_1d', 'body', false, 'number', localized('1 天窗口的 USD 限额，0 表示不限额。', 'USD limit for the 1-day window; 0 means unlimited.'), { example: '2' }),
      requestParameter('rate_limit_7d', 'body', false, 'number', localized('7 天窗口的 USD 限额，0 表示不限额。', 'USD limit for the 7-day window; 0 means unlimited.'), { example: '2' }),
      requestParameter('reset_rate_limit_usage', 'body', false, 'boolean', localized('为 true 时清零全部时间窗的已用额度。', 'When true, resets usage for all rate-limit windows.'), { example: 'true' })
    ],
    responseParameters: apiKeyResponseFields(),
    requestExample: [
      'curl --request PUT "' + openApiBase.value + '/keys/123" \\',
      '  --header "Content-Type: application/json" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY" \\',
      "  --data '{",
      '    "name": "automation-key-v2",',
      '    "status": "inactive",',
      '    "ip_whitelist": ["203.0.113.10"],',
      '    "ip_blacklist": []',
      "  }'"
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "id": 123,\n    "name": "automation-key-v2",\n    "status": "inactive",\n    "ip_whitelist": ["203.0.113.10"],\n    "ip_blacklist": []\n  }\n}'
  },
  {
    id: 'api-key-delete',
    method: 'DELETE',
    path: '/keys/:id',
    title: t('apiDocs.apiKeys.delete'),
    description: localized('永久删除当前用户的一把 API Key。', 'Permanently delete one API key owned by the current user.'),
    requestDescription: localized('API Key ID 必须作为路径参数传入；不接收 Query 参数或请求正文。删除后不可恢复。', 'Pass the API key ID as a path parameter. No query parameters or request body are accepted. Deletion cannot be undone.'),
    requestParameters: [userAccessKeyHeader(), apiKeyPathParameter()],
    responseParameters: responseEnvelope([
      { name: 'data.message', type: 'string', description: localized('删除成功提示。', 'Deletion success message.'), example: 'API key deleted successfully' }
    ]),
    requestExample: [
      'curl --request DELETE "' + openApiBase.value + '/keys/123" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": {\n    "message": "API key deleted successfully"\n  }\n}'
  },
  {
    id: 'available-groups',
    method: 'GET',
    path: '/groups/available',
    title: t('apiDocs.apiKeys.availableGroups'),
    description: localized('列出当前用户可以绑定到 API Key 的活跃分组。', 'List active groups the current user may bind to an API key.'),
    requestDescription: localized('不接收 Query 参数或请求正文；只需在 Header 中携带用户开发者密钥。', 'No query parameters or request body. Send only the user developer key in the request header.'),
    requestParameters: [userAccessKeyHeader()],
    responseParameters: responseEnvelope([
      { name: 'data', type: 'Group[]', description: localized('可用分组列表。', 'Available group list.'), example: '[{…}]' },
      { name: 'data[].id / name / description', type: 'int64 | string', description: localized('分组标识、名称和描述。', 'Group identifier, name, and description.'), example: '1 / OpenAI' },
      { name: 'data[].platform / status', type: 'string', description: localized('平台和分组状态。', 'Platform and group status.'), example: 'openai / active' },
      { name: 'data[].rate_multiplier', type: 'number', description: localized('费用倍率。', 'Cost multiplier.'), example: '1' },
      { name: 'data[].subscription_type / *_limit_usd', type: 'string | number | null', description: localized('订阅类型与可选的时间额度限制。', 'Subscription type and optional period limits.'), example: 'standard' }
    ]),
    requestExample: [
      'curl --request GET "' + openApiBase.value + '/groups/available" \\',
      '  --header "X-User-Access-Key: YOUR_USER_ACCESS_KEY"'
    ].join('\n'),
    responseExample: '{\n  "code": 0,\n  "message": "success",\n  "data": [{\n    "id": 1,\n    "name": "OpenAI",\n    "platform": "openai",\n    "rate_multiplier": 1,\n    "status": "active",\n    "subscription_type": "standard"\n  }]\n}'
  }
])

function sectionNavigationClass(module: DocumentationModule): string {
  return activeSection.value === module
    ? 'bg-primary-50 text-primary-800 dark:bg-primary-900/25 dark:text-primary-200'
    : 'text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-700 dark:hover:text-white'
}

function endpointNavigationClass(targetId: string): string {
  return activeTarget.value === targetId
    ? 'bg-primary-50 font-semibold text-primary-800 dark:bg-primary-900/25 dark:text-primary-200'
    : 'text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-400 dark:hover:bg-dark-700 dark:hover:text-white'
}

function sectionToggleLabel(module: DocumentationModule): string {
  const section = documentationSections.value.find((item) => item.module === module)
  const action = expandedSections.value[module] ? t('apiDocs.sections.collapse') : t('apiDocs.sections.expand')
  return t('apiDocs.sections.toggleLabel', { action, section: section?.label || '' })
}

function navigationToggleLabel(module: DocumentationModule): string {
  const section = documentationSections.value.find((item) => item.module === module)
  const action = expandedNavigationSections.value[module]
    ? t('apiDocs.sections.collapse')
    : t('apiDocs.sections.expand')
  return t('apiDocs.navigation.toggleModule', { action, module: section?.label || '' })
}

function getSectionTargetId(module: DocumentationModule): string {
  return documentationSections.value.find((section) => section.module === module)?.id || ''
}

function toggleSection(module: DocumentationModule): void {
  expandedSections.value[module] = !expandedSections.value[module]
  activeSection.value = module
  activeTarget.value = getSectionTargetId(module)
}

function toggleNavigationSection(module: DocumentationModule): void {
  expandedNavigationSections.value[module] = !expandedNavigationSections.value[module]
}

function activateSection(module: DocumentationModule, targetId: string): void {
  activeSection.value = module
  activeTarget.value = targetId
  expandedSections.value[module] = true
  expandedNavigationSections.value[module] = true
}

function navigateToTarget(
  module: DocumentationModule,
  targetId: string,
  options: { updateHistory?: boolean; behavior?: 'auto' | 'smooth' } = {}
): void {
  activateSection(module, targetId)

  void nextTick(() => {
    if (typeof document === 'undefined') return

    const target = document.getElementById(targetId)
    if (!target) return

    const reduceMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    const behavior = options.behavior || (reduceMotion ? 'auto' : 'smooth')
    const canScrollIntoView = typeof target.scrollIntoView === 'function'

    if (canScrollIntoView) {
      target.scrollIntoView({
        behavior,
        block: 'start'
      })
    }

    if (options.updateHistory !== false && typeof window !== 'undefined') {
      if (canScrollIntoView && window.history) {
        window.history.replaceState(null, '', '#' + targetId)
      } else {
        window.location.hash = targetId
      }
    }

    const focusTarget = target.matches('section')
      ? target.querySelector<HTMLElement>('[data-docs-heading]')
      : target
    focusTarget?.focus({ preventScroll: true })
  })
}

function observeDocumentationSections(): void {
  if (typeof IntersectionObserver === 'undefined' || typeof document === 'undefined') return

  sectionObserver = new IntersectionObserver((entries) => {
    const nextEntry = entries
      .filter((entry) => entry.isIntersecting)
      .sort((left, right) => left.boundingClientRect.top - right.boundingClientRect.top)[0]

    if (!nextEntry) return

    const target = documentationTargets.value.find((item) => item.id === nextEntry.target.id)
    if (target) {
      activeSection.value = target.module
      activeTarget.value = target.id
    }
  }, {
    rootMargin: '-18% 0px -72% 0px',
    threshold: 0.01
  })

  documentationTargets.value.forEach((target) => {
    const targetElement = document.getElementById(target.id)
    if (targetElement) {
      sectionObserver?.observe(targetElement)
    }
  })
}

function maskAccessKey(value: string): string {
  if (!value) return ''

  const visiblePrefixLength = 8
  const visibleSuffixLength = 4

  if (value.length <= visiblePrefixLength + visibleSuffixLength) {
    return '••••••••'
  }

  return value.slice(0, visiblePrefixLength) + '••••••••' + value.slice(-visibleSuffixLength)
}

function applyAccessKey(data: { key: string; exists: boolean; available?: boolean; created_at?: string | null }): void {
  accessKey.value = data.key || ''
  accessKeyExists.value = data.exists || Boolean(data.key)
  accessKeyAvailable.value = data.available !== false
  accessKeyCreatedAtRaw.value = data.created_at || null
}

async function loadAccessKey(): Promise<void> {
  accessKeyLoading.value = true
  try {
    applyAccessKey(await userAccessKeyAPI.get())
  } catch {
    appStore.showError(t('apiDocs.accessKey.loadFailed'))
  } finally {
    accessKeyLoading.value = false
  }
}

async function generateAccessKey(): Promise<void> {
  generatingAccessKey.value = true
  try {
    applyAccessKey(await userAccessKeyAPI.generate())
    appStore.showSuccess(t('apiDocs.accessKey.generated'))
  } catch {
    appStore.showError(t('apiDocs.accessKey.generateFailed'))
  } finally {
    generatingAccessKey.value = false
  }
}

async function copyAccessKey(): Promise<void> {
  await copySensitiveToClipboard(accessKey.value, t('apiDocs.accessKey.copied'))
}

onMounted(() => {
  const initialHash = typeof window === 'undefined' ? '' : window.location.hash.replace(/^#/, '')
  const initialTarget = documentationTargets.value.find((target) => target.id === initialHash)

  if (initialTarget) {
    navigateToTarget(initialTarget.module, initialTarget.id, {
      updateHistory: false,
      behavior: 'auto'
    })
  }

  observeDocumentationSections()
  void loadAccessKey()
  void appStore.fetchPublicSettings()
})

onBeforeUnmount(() => {
  sectionObserver?.disconnect()
  sectionObserver = null
})
</script>

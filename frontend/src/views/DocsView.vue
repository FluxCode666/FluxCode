<template>
  <div class="flex min-h-screen flex-col bg-[#f5f4f0] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="flex-1 pt-20 sm:pt-24">
      <section class="mx-auto max-w-7xl px-4 pb-16 sm:px-6 sm:pb-20">
        <header class="border-b border-slate-900/10 py-8 dark:border-white/10 sm:grid sm:grid-cols-[minmax(0,1fr)_15rem] sm:items-end sm:gap-12 sm:py-11">
          <div class="max-w-3xl">
            <p class="text-xs font-semibold uppercase tracking-[0.16em] text-teal-700 dark:text-teal-300">客户端配置</p>
            <h1 class="mt-3 text-3xl font-semibold leading-tight text-slate-950 dark:text-white sm:text-4xl">{{ t('home.sections.docsTitle') }}</h1>
            <p class="mt-3 max-w-2xl text-sm leading-6 text-slate-600 dark:text-slate-300 sm:text-base sm:leading-7">
              为常用 AI 客户端生成与当前站点匹配的接入配置。选择客户端后即可复制安装命令、配置文件和排查步骤。
            </p>
          </div>

          <div v-if="activeClient" class="mt-6 border-l-2 border-teal-600 pl-4 dark:border-teal-400 sm:mt-0">
            <p class="text-xs font-medium text-slate-500 dark:text-slate-400">当前指南</p>
            <div class="mt-2 flex items-center gap-2">
              <span class="font-mono text-lg text-teal-700 dark:text-teal-300" aria-hidden="true">{{ activeClient.icon }}</span>
              <p class="font-semibold text-slate-950 dark:text-white">{{ activeClient.shortName }}</p>
            </div>
            <p class="mt-1 font-mono text-xs text-slate-500 dark:text-slate-400">{{ activeClient.protocol }}</p>
          </div>
        </header>

        <section class="border-b border-slate-900/10 py-4 dark:border-white/10" aria-labelledby="client-picker-title">
          <div class="flex items-center justify-between gap-4">
            <h2 id="client-picker-title" class="shrink-0 text-sm font-semibold text-slate-950 dark:text-white">客户端指南</h2>
            <p class="hidden text-xs text-slate-500 dark:text-slate-400 sm:block">{{ clientDocs.length }} 个可用客户端</p>
          </div>

          <div class="scrollbar-hide -mx-4 mt-3 flex gap-2 overflow-x-auto px-4 pb-1 sm:mx-0 sm:px-0">
            <router-link
              v-for="client in clientDocs"
              :key="client.id"
              :to="{ name: 'Docs', params: { clientId: client.id } }"
              :class="[
                'group flex min-w-[13.25rem] items-center gap-3 border px-3 py-2.5 transition-[border-color,background-color,color] duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 sm:min-w-0 sm:flex-1',
                isActiveClient(client.id)
                  ? 'border-teal-700 bg-teal-700 text-white dark:border-teal-300 dark:bg-teal-300 dark:text-teal-950'
                  : 'border-slate-900/10 bg-white/45 text-slate-700 hover:border-slate-900/30 hover:bg-white dark:border-white/10 dark:bg-white/[0.03] dark:text-slate-300 dark:hover:bg-white/[0.07]'
              ]"
              :aria-current="isActiveClient(client.id) ? 'true' : undefined"
            >
              <span class="flex h-8 w-8 shrink-0 items-center justify-center border border-current/20 font-mono text-base font-semibold" aria-hidden="true">{{ client.icon }}</span>
              <span class="min-w-0">
                <span class="block truncate text-sm font-semibold">{{ client.shortName }}</span>
                <span :class="['mt-0.5 block truncate font-mono text-[11px]', isActiveClient(client.id) ? 'text-white/70 dark:text-teal-950/65' : 'text-slate-500 dark:text-slate-400']">{{ client.protocol }}</span>
              </span>
            </router-link>
          </div>
        </section>

        <div class="grid gap-8 py-8 lg:grid-cols-[12.5rem_minmax(0,1fr)] lg:py-10">
          <aside class="hidden lg:block">
            <div class="sticky top-24 border-l border-slate-900/10 pl-4 dark:border-white/10">
              <p class="text-xs font-semibold uppercase tracking-[0.14em] text-slate-500 dark:text-slate-400">目录</p>
              <nav class="mt-3 space-y-1" aria-label="客户端文档目录">
                <router-link
                  v-for="client in clientDocs"
                  :key="client.id"
                  :to="{ name: 'Docs', params: { clientId: client.id } }"
                  :class="[
                    'flex items-center gap-2 border-l-2 py-2 pl-3 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400',
                    isActiveClient(client.id)
                      ? 'border-teal-600 font-semibold text-teal-800 dark:border-teal-400 dark:text-teal-300'
                      : 'border-transparent text-slate-600 hover:border-slate-400 hover:text-slate-950 dark:text-slate-400 dark:hover:border-slate-500 dark:hover:text-white'
                  ]"
                  :aria-current="isActiveClient(client.id) ? 'true' : undefined"
                >
                  <span class="font-mono text-base leading-none" aria-hidden="true">{{ client.icon }}</span>
                  <span class="truncate">{{ client.shortName }}</span>
                </router-link>
              </nav>
            </div>
          </aside>

          <div class="min-w-0">
            <div v-if="activeClient" class="mb-5 flex items-center justify-between border-b border-slate-900/10 pb-3 dark:border-white/10">
              <p class="text-xs font-medium text-slate-500 dark:text-slate-400">{{ activeClient.name }}</p>
              <p class="font-mono text-[11px] text-slate-500 dark:text-slate-400">{{ activeClient.protocol }}</p>
            </div>
            <component
              :is="activeClient.component"
              v-if="activeClient"
              data-testid="client-guide"
              :site-name="siteName"
              :gateway-base-url="gatewayBaseUrl"
              :api-endpoint="apiEndpoint"
              :suggested-model-id="suggestedModelId"
            />

            <section
              v-else
              data-testid="unknown-client-guide"
              class="border-l-2 border-amber-500 bg-amber-50/70 px-6 py-8 dark:border-amber-300 dark:bg-amber-400/[0.08]"
            >
              <p class="text-xs font-semibold uppercase tracking-[0.14em] text-amber-800 dark:text-amber-200">未收录</p>
              <h2 class="mt-3 text-xl font-semibold text-slate-950 dark:text-white">未找到该客户端文档</h2>
              <p class="mt-2 max-w-md text-sm leading-6 text-slate-600 dark:text-slate-300">“{{ requestedClientId }}” 还没有收录。请从已支持的客户端中选择一个继续查看。</p>
              <router-link
                :to="{ name: 'Docs', params: { clientId: defaultClientId } }"
                class="mt-5 inline-flex border border-slate-900 bg-slate-900 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-slate-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 dark:border-white dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200"
              >
                查看 {{ defaultClient.shortName }} 配置
              </router-link>
            </section>
          </div>
        </div>
      </section>
    </main>

    <footer class="border-t border-slate-900/10 bg-white/20 px-6 py-2 dark:border-white/10 dark:bg-dark-900/20">
      <div class="mx-auto max-w-7xl text-center">
        <p class="text-xs text-slate-500 dark:text-slate-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import { clientDocs, findClientDoc } from '@/docs/clientRegistry'
import { resolveOpenAIUseKeyModelId } from '@/utils/openaiUseKeyModel'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

const defaultClientId = 'codex'
const defaultClient = clientDocs.find((client) => client.id === defaultClientId) ?? clientDocs[0]

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')
const currentYear = new Date().getFullYear()
const requestedClientId = computed(() => {
  const param = route.params.clientId
  return typeof param === 'string' ? param : undefined
})
const selectedClientId = computed(() => requestedClientId.value || defaultClient.id)
const activeClient = computed(() => findClientDoc(selectedClientId.value))

const gatewayBaseUrl = computed(() => {
  const raw = (appStore.apiBaseUrl || '').trim()
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const candidate = raw || origin
  return candidate.replace(/\/(?:api\/)?v1\/?$/, '').replace(/\/+$/, '')
})

const apiEndpoint = computed(() => `${gatewayBaseUrl.value}/v1`)
const suggestedModelId = computed(() => resolveOpenAIUseKeyModelId(appStore.openaiUseKeyModelId))

function isActiveClient(clientId: string): boolean {
  return selectedClientId.value === clientId
}

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

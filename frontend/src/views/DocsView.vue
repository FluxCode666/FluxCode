<template>
  <div class="min-h-screen bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="pt-24">
      <section class="mx-auto max-w-7xl px-5 py-10 sm:px-6 sm:py-14">
        <header class="relative overflow-hidden rounded-[32px] border border-black/5 bg-white/70 px-6 py-8 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40 sm:px-9 sm:py-10">
          <div class="pointer-events-none absolute inset-0 opacity-70 dark:opacity-40" aria-hidden="true">
            <div class="absolute -right-20 -top-20 h-64 w-64 rounded-full bg-cyan-200/50 blur-3xl dark:bg-cyan-400/15"></div>
            <div class="absolute bottom-0 left-1/3 h-28 w-2/3 bg-[linear-gradient(90deg,transparent,rgba(20,184,166,0.13),transparent)]"></div>
          </div>
          <div class="relative max-w-3xl">
            <div class="flex items-center gap-3 text-xs font-semibold uppercase tracking-[0.18em] text-teal-700 dark:text-teal-300">
              <span class="h-px w-8 bg-current"></span>
              客户端配置中心
            </div>
            <h1 class="mt-4 text-4xl font-semibold tracking-tight text-gray-950 dark:text-white sm:text-5xl">{{ t('home.sections.docsTitle') }}</h1>
            <p class="mt-4 max-w-2xl text-base leading-7 text-gray-600 dark:text-dark-300">
              为常用 AI 客户端生成与当前站点匹配的接入配置。选择客户端后即可复制安装命令、配置文件和排查步骤。
            </p>
          </div>
        </header>

        <section class="mt-7" aria-labelledby="client-picker-title">
          <div class="mb-3 flex items-end justify-between gap-4">
            <div>
              <h2 id="client-picker-title" class="text-base font-semibold text-gray-950 dark:text-white">选择客户端</h2>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">已收录 {{ clientDocs.length }} 个客户端；后续新增文档会自动出现在这里。</p>
            </div>
            <span class="hidden rounded-full bg-black/[0.04] px-3 py-1.5 text-xs font-medium text-gray-500 dark:bg-white/[0.07] dark:text-dark-300 sm:inline">可分享链接</span>
          </div>

          <div class="grid gap-3 md:grid-cols-2">
            <router-link
              v-for="client in clientDocs"
              :key="client.id"
              :to="{ name: 'Docs', params: { clientId: client.id } }"
              :class="[
                'group relative overflow-hidden rounded-2xl border p-4 transition-[border-color,background-color,box-shadow,transform] duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400',
                isActiveClient(client.id)
                  ? 'border-teal-500/45 bg-teal-50/70 shadow-sm shadow-teal-900/5 dark:border-teal-400/40 dark:bg-teal-400/[0.08]'
                  : 'border-black/5 bg-white/65 hover:-translate-y-0.5 hover:border-teal-400/30 hover:bg-white hover:shadow-md hover:shadow-slate-900/5 dark:border-white/10 dark:bg-dark-900/35 dark:hover:bg-dark-900/60'
              ]"
            >
              <div class="flex items-start gap-4">
                <div
                  :class="[
                    'flex h-11 w-11 shrink-0 items-center justify-center rounded-xl font-mono text-xl font-semibold transition-colors',
                    isActiveClient(client.id)
                      ? 'bg-teal-700 text-white dark:bg-teal-300 dark:text-teal-950'
                      : 'bg-slate-100 text-slate-700 group-hover:bg-teal-100 group-hover:text-teal-800 dark:bg-white/[0.07] dark:text-slate-200 dark:group-hover:bg-teal-400/15 dark:group-hover:text-teal-100'
                  ]"
                  aria-hidden="true"
                >
                  {{ client.icon }}
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex items-center justify-between gap-3">
                    <h3 class="font-semibold text-slate-950 dark:text-white">{{ client.name }}</h3>
                    <svg class="h-4 w-4 shrink-0 text-slate-400 transition-transform group-hover:translate-x-0.5 group-hover:text-teal-600 dark:group-hover:text-teal-300" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M5 12h14m-6-6 6 6-6 6" />
                    </svg>
                  </div>
                  <p class="mt-1.5 text-sm leading-6 text-slate-600 dark:text-slate-300">{{ client.description }}</p>
                  <span class="mt-3 inline-flex rounded-full bg-black/[0.045] px-2.5 py-1 font-mono text-[11px] font-medium text-slate-500 dark:bg-white/[0.08] dark:text-slate-300">{{ client.protocol }}</span>
                </div>
              </div>
            </router-link>
          </div>
        </section>

        <div class="mt-7 grid gap-6 lg:grid-cols-[224px_minmax(0,1fr)]">
          <aside class="hidden lg:block">
            <div class="sticky top-24 rounded-2xl border border-black/5 bg-white/55 p-3 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/35">
              <p class="px-2.5 pb-2 text-xs font-semibold uppercase tracking-[0.14em] text-gray-500 dark:text-dark-400">客户端目录</p>
              <nav class="space-y-1" aria-label="客户端文档目录">
                <router-link
                  v-for="client in clientDocs"
                  :key="client.id"
                  :to="{ name: 'Docs', params: { clientId: client.id } }"
                  :class="[
                    'flex items-center gap-3 rounded-xl px-3 py-2.5 text-sm transition-colors',
                    isActiveClient(client.id)
                      ? 'bg-teal-700 text-white shadow-sm dark:bg-teal-300 dark:text-teal-950'
                      : 'text-slate-600 hover:bg-black/[0.045] hover:text-slate-950 dark:text-slate-300 dark:hover:bg-white/[0.07] dark:hover:text-white'
                  ]"
                >
                  <span class="font-mono text-base leading-none">{{ client.icon }}</span>
                  <span class="font-medium">{{ client.shortName }}</span>
                </router-link>
              </nav>
              <div class="mt-4 rounded-xl border border-dashed border-black/10 px-3 py-3 text-xs leading-5 text-slate-500 dark:border-white/15 dark:text-slate-400">
                新客户端只需注册一个指南组件，即可自动加入卡片与目录。
              </div>
            </div>
          </aside>

          <div>
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
              class="rounded-[24px] border border-dashed border-amber-400/50 bg-amber-50/70 p-8 text-center dark:border-amber-300/30 dark:bg-amber-400/[0.08]"
            >
              <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-amber-100 text-xl text-amber-800 dark:bg-amber-300/15 dark:text-amber-200">?</div>
              <h2 class="mt-4 text-xl font-semibold text-slate-950 dark:text-white">未找到该客户端文档</h2>
              <p class="mx-auto mt-2 max-w-md text-sm leading-6 text-slate-600 dark:text-slate-300">“{{ requestedClientId }}” 还没有收录。请从已支持的客户端中选择一个继续查看。</p>
              <router-link
                :to="{ name: 'Docs', params: { clientId: defaultClientId } }"
                class="mt-5 inline-flex rounded-xl bg-slate-900 px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-slate-700 dark:bg-white dark:text-slate-900 dark:hover:bg-slate-200"
              >
                查看 {{ defaultClient.shortName }} 配置
              </router-link>
            </section>
          </div>
        </div>
      </section>
    </main>
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

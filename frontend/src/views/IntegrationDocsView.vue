<template>
  <div class="min-h-screen bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="pt-24 pb-20">
      <section class="relative overflow-hidden">
        <div class="pointer-events-none absolute inset-0">
          <div class="absolute -top-40 right-[-80px] h-[360px] w-[360px] rounded-full bg-primary-500/15 blur-3xl dark:bg-primary-500/10"></div>
          <div class="absolute -bottom-32 left-[-100px] h-[300px] w-[300px] rounded-full bg-sky-500/10 blur-3xl dark:bg-sky-500/10"></div>
          <div class="absolute inset-0 opacity-[0.28] [background-image:radial-gradient(rgba(0,0,0,0.06)_1px,transparent_1px)] [background-size:22px_22px] dark:opacity-[0.08]"></div>
        </div>

        <div class="relative mx-auto max-w-6xl px-6 py-16 lg:py-20">
          <div class="max-w-3xl">
            <span class="inline-flex items-center rounded-full border border-[#7b6857]/15 bg-white/70 px-4 py-1.5 text-sm font-medium text-[#7b6857] shadow-sm backdrop-blur dark:border-white/10 dark:bg-white/5 dark:text-primary-200">
              {{ t('integrationDocs.hero.badge') }}
            </span>
            <h1 class="mt-6 text-4xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-5xl">
              {{ t('integrationDocs.hero.title') }}
            </h1>
            <p class="mt-4 text-base leading-relaxed text-gray-600 dark:text-dark-300 sm:text-lg">
              {{ t('integrationDocs.hero.subtitle') }}
            </p>
          </div>

          <div class="mt-10 grid gap-4 md:grid-cols-3">
            <div class="rounded-3xl border border-black/5 bg-white/75 p-5 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
              <div class="text-sm font-medium text-gray-500 dark:text-dark-300">
                {{ t('integrationDocs.hero.cards.baseUrl') }}
              </div>
              <div class="mt-3 break-all font-mono text-sm text-gray-900 dark:text-white">
                {{ baseUrl }}
              </div>
            </div>
            <div class="rounded-3xl border border-black/5 bg-white/75 p-5 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
              <div class="text-sm font-medium text-gray-500 dark:text-dark-300">
                {{ t('integrationDocs.hero.cards.auth') }}
              </div>
              <div class="mt-3 font-mono text-sm text-gray-900 dark:text-white">
                Authorization: Bearer {{ apiKeyPlaceholder }}
              </div>
            </div>
            <div class="rounded-3xl border border-black/5 bg-white/75 p-5 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
              <div class="text-sm font-medium text-gray-500 dark:text-dark-300">
                {{ t('integrationDocs.hero.cards.compatibility') }}
              </div>
              <div class="mt-3 text-sm text-gray-900 dark:text-white">
                OpenAI Chat / Responses / Anthropic Messages / Images
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="mx-auto max-w-6xl px-6 py-12 lg:grid lg:grid-cols-[240px_minmax(0,1fr)] lg:gap-8">
        <aside class="mb-8 lg:sticky lg:top-28 lg:mb-0 lg:self-start">
          <div class="rounded-3xl border border-black/5 bg-white/75 p-5 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40">
            <div class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ t('integrationDocs.sidebar.title') }}
            </div>
            <nav class="mt-4 flex flex-col gap-2">
              <a
                v-for="item in pageNav"
                :key="item.id"
                :href="`#${item.id}`"
                class="rounded-2xl px-3 py-2 text-sm text-gray-600 transition-colors hover:bg-black/5 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-white/10 dark:hover:text-white"
              >
                {{ item.label }}
              </a>
            </nav>
          </div>
        </aside>

        <div class="space-y-8">
          <section
            id="overview"
            class="rounded-[32px] border border-black/5 bg-white/75 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40 sm:p-8"
          >
            <div class="max-w-3xl">
              <h2 class="text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">
                {{ t('integrationDocs.overview.title') }}
              </h2>
              <p class="mt-3 text-base leading-relaxed text-gray-600 dark:text-dark-300">
                {{ t('integrationDocs.overview.description') }}
              </p>
            </div>

            <div class="mt-6 rounded-3xl border border-primary-500/15 bg-primary-500/5 p-5 text-sm leading-relaxed text-gray-700 dark:border-primary-400/20 dark:bg-primary-500/10 dark:text-dark-200">
              {{ t('integrationDocs.overview.callout') }}
            </div>

            <div class="mt-8 grid gap-4 md:grid-cols-3">
              <div
                v-for="step in quickstartSteps"
                :key="step.title"
                class="rounded-3xl border border-black/5 bg-neutral-50/80 p-5 dark:border-white/10 dark:bg-dark-950/30"
              >
                <div class="text-sm font-semibold text-[#7b6857] dark:text-primary-300">
                  {{ step.step }}
                </div>
                <div class="mt-2 text-base font-semibold text-gray-900 dark:text-white">
                  {{ step.title }}
                </div>
                <p class="mt-2 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                  {{ step.description }}
                </p>
              </div>
            </div>

            <div class="mt-8 grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
              <div class="rounded-3xl border border-black/5 bg-neutral-50/80 p-5 dark:border-white/10 dark:bg-dark-950/30">
                <div class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('integrationDocs.overview.pathsTitle') }}
                </div>
                <p class="mt-2 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                  {{ t('integrationDocs.overview.pathsDescription') }}
                </p>
                <div class="mt-4 space-y-3">
                  <div
                    v-for="path in standardPaths"
                    :key="path"
                    class="rounded-2xl bg-slate-900 px-4 py-3 font-mono text-sm text-slate-100"
                  >
                    {{ path }}
                  </div>
                </div>
              </div>

              <div class="rounded-3xl border border-black/5 bg-neutral-50/80 p-5 dark:border-white/10 dark:bg-dark-950/30">
                <div class="text-base font-semibold text-gray-900 dark:text-white">
                  {{ t('integrationDocs.overview.notesTitle') }}
                </div>
                <ul class="mt-4 space-y-3 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                  <li v-for="tip in overviewTips" :key="tip" class="flex gap-3">
                    <span class="mt-1 h-2 w-2 rounded-full bg-[#7b6857] dark:bg-primary-300"></span>
                    <span>{{ tip }}</span>
                  </li>
                </ul>
              </div>
            </div>
          </section>

          <section
            v-for="section in protocols"
            :id="section.id"
            :key="section.id"
            class="rounded-[32px] border border-black/5 bg-white/75 p-6 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40 sm:p-8"
          >
            <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div class="max-w-3xl">
                <span class="inline-flex items-center rounded-full border border-[#7b6857]/15 bg-[#7b6857]/8 px-3 py-1 text-xs font-semibold uppercase tracking-[0.18em] text-[#7b6857] dark:border-primary-400/20 dark:bg-primary-500/10 dark:text-primary-200">
                  {{ section.badge }}
                </span>
                <h2 class="mt-4 text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">
                  {{ section.title }}
                </h2>
                <p class="mt-3 text-base leading-relaxed text-gray-600 dark:text-dark-300">
                  {{ section.description }}
                </p>
              </div>

              <div class="rounded-3xl border border-black/5 bg-neutral-50/80 p-4 text-sm dark:border-white/10 dark:bg-dark-950/30 lg:min-w-[220px]">
                <div class="text-xs font-medium uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                  {{ t('integrationDocs.fields.endpoint') }}
                </div>
                <div class="mt-2 font-mono text-sm text-gray-900 dark:text-white">
                  {{ section.endpoint }}
                </div>
                <div class="mt-4 text-xs font-medium uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                  {{ t('integrationDocs.fields.method') }}
                </div>
                <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ section.method }}
                </div>
              </div>
            </div>

            <div class="mt-6 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <div
                v-for="header in section.headers"
                :key="`${section.id}-${header.label}`"
                class="rounded-3xl border border-black/5 bg-neutral-50/80 p-4 dark:border-white/10 dark:bg-dark-950/30"
              >
                <div class="text-xs font-medium uppercase tracking-[0.16em] text-gray-500 dark:text-dark-300">
                  {{ header.label }}
                </div>
                <div class="mt-2 break-all font-mono text-sm text-gray-900 dark:text-white">
                  {{ header.value }}
                </div>
              </div>
            </div>

            <ul class="mt-6 space-y-3 text-sm leading-relaxed text-gray-600 dark:text-dark-300">
              <li v-for="item in section.bullets" :key="item" class="flex gap-3">
                <span class="mt-1 h-2 w-2 rounded-full bg-[#7b6857] dark:bg-primary-300"></span>
                <span>{{ item }}</span>
              </li>
            </ul>

            <div class="mt-8 grid gap-4 xl:grid-cols-2">
              <div>
                <div class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('integrationDocs.fields.exampleCurl') }}
                </div>
                <div class="rounded-3xl bg-slate-900 p-4 text-[14px] text-slate-100">
                  <pre class="overflow-x-auto font-mono leading-relaxed"><code v-text="section.curl"></code></pre>
                </div>
              </div>
              <div>
                <div class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('integrationDocs.fields.exampleJavascript') }}
                </div>
                <div class="rounded-3xl bg-slate-900 p-4 text-[14px] text-slate-100">
                  <pre class="overflow-x-auto font-mono leading-relaxed"><code v-text="section.javascript"></code></pre>
                </div>
              </div>
            </div>
          </section>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import { resolveOpenAIUseKeyModelId } from '@/utils/openaiUseKeyModel'

type HeaderItem = {
  label: string
  value: string
}

type ProtocolSection = {
  id: string
  badge: string
  title: string
  description: string
  method: string
  endpoint: string
  headers: HeaderItem[]
  bullets: string[]
  curl: string
  javascript: string
}

const { t } = useI18n()

const appStore = useAppStore()

const apiKeyPlaceholder = 'sk-your-api-key'
const anthropicVersion = '2023-06-01'

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')

const baseUrl = computed(() => {
  const raw = (appStore.apiBaseUrl || '').trim()
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  const candidate = raw || origin
  return candidate.replace(/\/api\/v1\/?$/, '').replace(/\/+$/, '')
})

const openAIBaseUrl = computed(() => `${baseUrl.value}/v1`)
const openAIModel = computed(() => resolveOpenAIUseKeyModelId(appStore.openaiUseKeyModelId))
const anthropicModel = 'claude-sonnet-4-5'
const imageModel = 'gpt-image-1'

const standardPaths = computed(() => [
  '/v1/chat/completions',
  '/v1/responses',
  '/v1/messages',
  '/v1/images/generations'
])

const quickstartSteps = computed(() => [
  {
    step: '01',
    title: t('integrationDocs.overview.steps.step1.title'),
    description: t('integrationDocs.overview.steps.step1.description')
  },
  {
    step: '02',
    title: t('integrationDocs.overview.steps.step2.title'),
    description: t('integrationDocs.overview.steps.step2.description')
  },
  {
    step: '03',
    title: t('integrationDocs.overview.steps.step3.title'),
    description: t('integrationDocs.overview.steps.step3.description')
  }
])

const overviewTips = computed(() => [
  t('integrationDocs.overview.notes.item1'),
  t('integrationDocs.overview.notes.item2'),
  t('integrationDocs.overview.notes.item3'),
  t('integrationDocs.overview.notes.item4')
])

function stringifyBody(body: unknown): string {
  return JSON.stringify(body, null, 2)
}

function buildCurl(url: string, headers: HeaderItem[], body: unknown): string {
  const headerLines = headers.map((header) => `  -H "${header.value}"`).join(' \\\n')
  return `curl "${url}" \\
${headerLines} \\
  -d '${stringifyBody(body)}'`
}

function buildFetch(url: string, headers: Record<string, string>, body: unknown): string {
  return `const response = await fetch("${url}", {
  method: "POST",
  headers: ${stringifyBody(headers)},
  body: JSON.stringify(${stringifyBody(body)})
})

const data = await response.json()
console.log(data)`
}

const protocols = computed<ProtocolSection[]>(() => {
  const openAIHeaders: HeaderItem[] = [
    {
      label: t('integrationDocs.fields.authHeader'),
      value: `Authorization: Bearer ${apiKeyPlaceholder}`
    },
    {
      label: t('integrationDocs.fields.contentType'),
      value: 'Content-Type: application/json'
    }
  ]

  const anthropicHeaders: HeaderItem[] = [
    {
      label: t('integrationDocs.fields.authHeader'),
      value: `x-api-key: ${apiKeyPlaceholder}`
    },
    {
      label: t('integrationDocs.fields.versionHeader'),
      value: `anthropic-version: ${anthropicVersion}`
    },
    {
      label: t('integrationDocs.fields.contentType'),
      value: 'Content-Type: application/json'
    }
  ]

  const openAIChatBody = {
    model: openAIModel.value,
    messages: [
      { role: 'system', content: 'You are a helpful assistant.' },
      { role: 'user', content: '用一句话介绍 FluxCode。' }
    ],
    temperature: 0.7
  }

  const responsesBody = {
    model: openAIModel.value,
    instructions: 'You are a helpful assistant.',
    input: [
      {
        role: 'user',
        content: [{ type: 'input_text', text: '给我一个接入成功排查清单。' }]
      }
    ]
  }

  const anthropicBody = {
    model: anthropicModel,
    max_tokens: 1024,
    messages: [{ role: 'user', content: '请总结 FluxCode 的主要能力。' }]
  }

  const imageBody = {
    model: imageModel,
    prompt: '一张具有未来感的 API 网关控制台插画，浅色背景，蓝金配色',
    size: '1024x1024',
    response_format: 'url'
  }

  return [
    {
      id: 'openai-chat',
      badge: t('integrationDocs.sections.openaiChat.badge'),
      title: t('integrationDocs.sections.openaiChat.title'),
      description: t('integrationDocs.sections.openaiChat.description'),
      method: 'POST',
      endpoint: '/v1/chat/completions',
      headers: openAIHeaders,
      bullets: [
        t('integrationDocs.sections.openaiChat.bullets.item1'),
        t('integrationDocs.sections.openaiChat.bullets.item2'),
        t('integrationDocs.sections.openaiChat.bullets.item3')
      ],
      curl: buildCurl(`${baseUrl.value}/v1/chat/completions`, openAIHeaders, openAIChatBody),
      javascript: buildFetch(
        `${baseUrl.value}/v1/chat/completions`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        openAIChatBody
      )
    },
    {
      id: 'openai-responses',
      badge: t('integrationDocs.sections.openaiResponses.badge'),
      title: t('integrationDocs.sections.openaiResponses.title'),
      description: t('integrationDocs.sections.openaiResponses.description'),
      method: 'POST',
      endpoint: '/v1/responses',
      headers: openAIHeaders,
      bullets: [
        t('integrationDocs.sections.openaiResponses.bullets.item1'),
        t('integrationDocs.sections.openaiResponses.bullets.item2'),
        t('integrationDocs.sections.openaiResponses.bullets.item3')
      ],
      curl: buildCurl(`${baseUrl.value}/v1/responses`, openAIHeaders, responsesBody),
      javascript: `const clientConfig = {
  apiKey: "${apiKeyPlaceholder}",
  baseURL: "${openAIBaseUrl.value}"
}

${buildFetch(
  `${baseUrl.value}/v1/responses`,
  {
    Authorization: `Bearer ${apiKeyPlaceholder}`,
    'Content-Type': 'application/json'
  },
  responsesBody
)}`,
    },
    {
      id: 'anthropic-messages',
      badge: t('integrationDocs.sections.anthropicMessages.badge'),
      title: t('integrationDocs.sections.anthropicMessages.title'),
      description: t('integrationDocs.sections.anthropicMessages.description'),
      method: 'POST',
      endpoint: '/v1/messages',
      headers: anthropicHeaders,
      bullets: [
        t('integrationDocs.sections.anthropicMessages.bullets.item1'),
        t('integrationDocs.sections.anthropicMessages.bullets.item2'),
        t('integrationDocs.sections.anthropicMessages.bullets.item3')
      ],
      curl: buildCurl(`${baseUrl.value}/v1/messages`, anthropicHeaders, anthropicBody),
      javascript: buildFetch(
        `${baseUrl.value}/v1/messages`,
        {
          'x-api-key': apiKeyPlaceholder,
          'anthropic-version': anthropicVersion,
          'Content-Type': 'application/json'
        },
        anthropicBody
      )
    },
    {
      id: 'openai-images',
      badge: t('integrationDocs.sections.openaiImages.badge'),
      title: t('integrationDocs.sections.openaiImages.title'),
      description: t('integrationDocs.sections.openaiImages.description'),
      method: 'POST',
      endpoint: '/v1/images/generations',
      headers: openAIHeaders,
      bullets: [
        t('integrationDocs.sections.openaiImages.bullets.item1'),
        t('integrationDocs.sections.openaiImages.bullets.item2'),
        t('integrationDocs.sections.openaiImages.bullets.item3')
      ],
      curl: buildCurl(`${baseUrl.value}/v1/images/generations`, openAIHeaders, imageBody),
      javascript: buildFetch(
        `${baseUrl.value}/v1/images/generations`,
        {
          Authorization: `Bearer ${apiKeyPlaceholder}`,
          'Content-Type': 'application/json'
        },
        imageBody
      )
    }
  ]
})

const pageNav = computed(() => [
  { id: 'overview', label: t('integrationDocs.nav.overview') },
  ...protocols.value.map((section) => ({ id: section.id, label: section.title }))
])

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

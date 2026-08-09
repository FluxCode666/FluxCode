<template>
  <div class="min-h-screen bg-[#faf7f2] text-slate-900 dark:bg-dark-950 dark:text-slate-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="relative pb-20 pt-24 sm:pt-28">
      <div class="pointer-events-none absolute inset-x-0 top-0 h-[34rem] overflow-hidden" aria-hidden="true">
        <div class="absolute -right-40 top-0 h-80 w-80 rounded-full bg-primary-300/20 blur-3xl dark:bg-primary-500/10"></div>
        <div class="absolute -left-48 top-32 h-96 w-96 rounded-full bg-amber-200/25 blur-3xl dark:bg-amber-500/5"></div>
        <div class="absolute inset-0 opacity-30 [background-image:linear-gradient(rgba(15,23,42,0.035)_1px,transparent_1px),linear-gradient(90deg,rgba(15,23,42,0.035)_1px,transparent_1px)] [background-size:48px_48px] dark:opacity-[0.08]"></div>
      </div>

      <div class="relative mx-auto w-full max-w-6xl px-5 sm:px-6 lg:px-8">
        <nav class="mb-10 flex items-center gap-2 text-xs font-medium text-slate-500 dark:text-dark-400" aria-label="面包屑">
          <router-link to="/home" class="rounded-md transition-colors hover:text-primary-600 focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400">
            {{ siteName }}
          </router-link>
          <span aria-hidden="true">/</span>
          <span>法律中心</span>
          <span aria-hidden="true">/</span>
          <span class="text-slate-800 dark:text-slate-200">{{ document.shortTitle }}</span>
        </nav>

        <header class="max-w-4xl pb-12 sm:pb-16">
          <div class="mb-5 flex flex-wrap items-center gap-3">
            <span class="inline-flex items-center rounded-full border border-primary-200 bg-primary-50/80 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-primary-700 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-300">
              {{ document.eyebrow }}
            </span>
            <span class="font-mono text-[11px] tracking-[0.14em] text-slate-400 dark:text-dark-500">VER. {{ document.version }}</span>
          </div>
          <h1 class="max-w-3xl text-4xl font-semibold tracking-[-0.035em] text-slate-950 dark:text-white sm:text-5xl lg:text-6xl">
            {{ document.title }}
          </h1>
          <p class="mt-6 max-w-3xl text-base leading-8 text-slate-600 dark:text-dark-300 sm:text-lg">
            {{ renderText(document.description) }}
          </p>
          <div class="mt-8 flex flex-wrap items-center gap-x-6 gap-y-2 border-l-2 border-primary-400 pl-4 text-xs text-slate-500 dark:text-dark-400">
            <span>更新日期：{{ document.updatedAt }}</span>
            <span>适用于网站、控制台与 API 服务</span>
          </div>
        </header>

        <div class="mb-10 border-y border-slate-200/80 py-4 lg:hidden dark:border-white/10">
          <label for="legal-document-select" class="mb-2 block text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-dark-400">
            法律文件
          </label>
          <select
            id="legal-document-select"
            :value="document.path"
            class="w-full rounded-xl border border-slate-200 bg-white/80 px-3 py-2.5 text-sm font-medium text-slate-800 outline-none transition focus:border-primary-400 focus:ring-2 focus:ring-primary-300/30 dark:border-dark-700 dark:bg-dark-900 dark:text-white"
            @change="navigateDocument"
          >
            <option v-for="item in documentNavigation" :key="item.path" :value="item.path">
              {{ item.title }}
            </option>
          </select>
        </div>

        <div class="grid items-start gap-12 lg:grid-cols-[14rem_minmax(0,1fr)] lg:gap-16">
          <aside class="sticky top-24 hidden max-h-[calc(100vh-7rem)] self-start overflow-y-auto pr-5 lg:block">
            <div>
              <h2 class="text-[11px] font-semibold uppercase tracking-[0.2em] text-slate-400 dark:text-dark-500">法律文件</h2>
              <nav class="mt-4 border-l border-slate-200 dark:border-dark-700" aria-label="法律文件">
                <router-link
                  v-for="item in documentNavigation"
                  :key="item.path"
                  :to="item.path"
                  class="-ml-px block border-l px-4 py-2 text-sm leading-5 transition-colors"
                  :class="item.key === document.key
                    ? 'border-primary-500 font-semibold text-primary-700 dark:border-primary-400 dark:text-primary-300'
                    : 'border-transparent text-slate-500 hover:border-slate-300 hover:text-slate-900 dark:text-dark-400 dark:hover:border-dark-500 dark:hover:text-white'"
                >
                  {{ item.shortTitle }}
                </router-link>
              </nav>
            </div>

            <div class="mt-10">
              <h2 class="text-[11px] font-semibold uppercase tracking-[0.2em] text-slate-400 dark:text-dark-500">本页目录</h2>
              <nav class="mt-4 space-y-1" aria-label="本页目录">
                <router-link
                  v-for="section in document.sections"
                  :key="section.id"
                  :to="{ hash: `#${section.id}` }"
                  class="block rounded-lg px-3 py-1.5 text-xs leading-5 transition-colors"
                  :class="activeSectionId === section.id
                    ? 'bg-primary-50 font-medium text-primary-700 dark:bg-primary-500/10 dark:text-primary-300'
                    : 'text-slate-500 hover:bg-black/[0.03] hover:text-slate-900 dark:text-dark-400 dark:hover:bg-white/[0.05] dark:hover:text-white'"
                >
                  {{ section.title }}
                </router-link>
              </nav>
            </div>
          </aside>

          <article class="min-w-0" :aria-labelledby="`${document.key}-document-title`">
            <h2 :id="`${document.key}-document-title`" class="sr-only">{{ document.title }}正文</h2>

            <section
              v-for="section in document.sections"
              :id="section.id"
              :key="section.id"
              :data-legal-section="section.id"
              class="scroll-mt-28 border-t border-slate-200/90 py-10 first:border-t-0 first:pt-0 dark:border-white/10 sm:py-12"
            >
              <h2 class="text-2xl font-semibold tracking-tight text-slate-950 dark:text-white sm:text-[1.7rem]">
                {{ section.title }}
              </h2>

              <div class="mt-5 space-y-4">
                <p
                  v-for="(paragraph, paragraphIndex) in section.paragraphs"
                  :key="`${section.id}-paragraph-${paragraphIndex}`"
                  class="text-[15px] leading-8 text-slate-600 dark:text-dark-300"
                >
                  {{ renderText(paragraph) }}
                </p>
              </div>

              <ul v-if="section.bullets?.length" class="mt-5 space-y-3">
                <li
                  v-for="(bullet, bulletIndex) in section.bullets"
                  :key="`${section.id}-bullet-${bulletIndex}`"
                  class="grid grid-cols-[1rem_minmax(0,1fr)] gap-3 text-[15px] leading-7 text-slate-600 dark:text-dark-300"
                >
                  <span class="mt-[0.68rem] h-1.5 w-1.5 rounded-full bg-primary-400" aria-hidden="true"></span>
                  <span>{{ renderText(bullet) }}</span>
                </li>
              </ul>

              <div v-if="section.subsections?.length" class="mt-8 space-y-8">
                <section v-for="(subsection, subsectionIndex) in section.subsections" :key="`${section.id}-subsection-${subsectionIndex}`">
                  <h3 class="text-base font-semibold text-slate-900 dark:text-slate-100">{{ subsection.title }}</h3>
                  <div class="mt-3 space-y-3">
                    <p
                      v-for="(paragraph, paragraphIndex) in subsection.paragraphs"
                      :key="`${section.id}-subsection-${subsectionIndex}-paragraph-${paragraphIndex}`"
                      class="text-[15px] leading-8 text-slate-600 dark:text-dark-300"
                    >
                      {{ renderText(paragraph) }}
                    </p>
                  </div>
                  <ul v-if="subsection.bullets?.length" class="mt-4 space-y-3">
                    <li
                      v-for="(bullet, bulletIndex) in subsection.bullets"
                      :key="`${section.id}-subsection-${subsectionIndex}-bullet-${bulletIndex}`"
                      class="grid grid-cols-[1rem_minmax(0,1fr)] gap-3 text-[15px] leading-7 text-slate-600 dark:text-dark-300"
                    >
                      <span class="mt-[0.68rem] h-1.5 w-1.5 rounded-full bg-primary-400" aria-hidden="true"></span>
                      <span>{{ renderText(bullet) }}</span>
                    </li>
                  </ul>
                </section>
              </div>

              <div v-if="section.notice" class="mt-7 border-l-2 border-amber-400 bg-amber-50/60 px-5 py-4 dark:bg-amber-500/[0.07]">
                <p class="text-sm leading-7 text-amber-950/75 dark:text-amber-100/80">
                  {{ renderText(section.notice) }}
                </p>
              </div>
            </section>

            <section class="mt-4 border-t border-slate-300 py-10 dark:border-dark-600" aria-labelledby="legal-contact-title">
              <div class="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h2 id="legal-contact-title" class="text-lg font-semibold text-slate-950 dark:text-white">需要帮助？</h2>
                  <p class="mt-2 max-w-xl text-sm leading-7 text-slate-600 dark:text-dark-300">
                    对条款、账户措施、账单或地区可用性存在疑问时，请通过平台公示渠道联系我们，并提供必要的账户与请求信息。
                  </p>
                </div>
                <div class="shrink-0 rounded-xl border border-slate-200 bg-white/60 px-4 py-3 text-sm font-medium text-slate-700 dark:border-dark-700 dark:bg-dark-900/50 dark:text-dark-200">
                  {{ contactInfo || '请在控制台查看客服联系方式' }}
                </div>
              </div>
            </section>
          </article>
        </div>
      </div>
    </main>

    <footer class="border-t border-slate-200/80 bg-white/35 px-6 py-10 dark:border-white/10 dark:bg-dark-900/20">
      <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-5 sm:flex-row">
        <div class="flex items-center gap-3">
          <div class="h-8 w-8 overflow-hidden rounded-lg bg-white shadow-sm ring-1 ring-black/5">
            <img :src="siteLogo || '/logo.png'" alt="" class="h-full w-full object-contain" />
          </div>
          <p class="text-xs text-slate-500 dark:text-dark-400">© {{ currentYear }} {{ siteName }}</p>
        </div>
        <LegalLinkList class="text-xs" />
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import LegalLinkList from '@/components/legal/LegalLinkList.vue'
import {
  getLegalDocument,
  legalDocuments,
  legalDocumentNavigation,
  renderLegalText
} from '@/content/legalDocuments'
import { useAppStore } from '@/stores'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')
const contactInfo = computed(() => appStore.contactInfo || '')
const currentYear = new Date().getFullYear()
const documentNavigation = legalDocumentNavigation
const document = computed(() => getLegalDocument(route.meta.legalDocument) || legalDocuments.terms)
const activeSectionId = ref(document.value.sections[0]?.id || '')

let sectionObserver: IntersectionObserver | null = null

function renderText(text: string): string {
  return renderLegalText(text, siteName.value)
}

function navigateDocument(event: Event): void {
  const path = (event.target as HTMLSelectElement).value
  if (path && path !== route.path) router.push(path)
}

function observeSections(): void {
  sectionObserver?.disconnect()
  if (typeof IntersectionObserver === 'undefined') return

  sectionObserver = new IntersectionObserver(
    (entries) => {
      const visible = entries
        .filter((entry) => entry.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)
      if (visible[0]) activeSectionId.value = (visible[0].target as HTMLElement).id
    },
    { rootMargin: '-18% 0px -68% 0px', threshold: [0, 0.1, 0.5] }
  )

  document.value.sections.forEach((section) => {
    const element = window.document.getElementById(section.id)
    if (element) sectionObserver?.observe(element)
  })
}

watch(
  () => document.value.key,
  async () => {
    activeSectionId.value = document.value.sections[0]?.id || ''
    await nextTick()
    observeSections()
  }
)

onMounted(async () => {
  await appStore.fetchPublicSettings()
  await nextTick()
  observeSections()
})

onBeforeUnmount(() => sectionObserver?.disconnect())
</script>

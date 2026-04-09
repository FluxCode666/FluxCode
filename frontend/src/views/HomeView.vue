<template>
  <div class="min-h-screen bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main id="top" class="pt-24">
      <!-- Hero（参考 HOPCC：居中排版 + 暖色背景） -->
      <section class="relative overflow-hidden">
        <div class="pointer-events-none absolute inset-0">
          <div class="absolute -top-56 right-[-120px] h-[520px] w-[520px] rounded-full bg-primary-500/15 blur-3xl dark:bg-primary-500/10"></div>
          <div class="absolute -bottom-56 left-[-160px] h-[520px] w-[520px] rounded-full bg-blue-500/10 blur-3xl dark:bg-blue-500/10"></div>
          <div class="absolute inset-0 opacity-[0.35] [background-image:radial-gradient(rgba(0,0,0,0.06)_1px,transparent_1px)] [background-size:24px_24px] dark:opacity-[0.08]"></div>
        </div>

        <div class="relative mx-auto max-w-4xl px-6 py-24 text-center sm:py-28">
          <h1 class="text-6xl font-extrabold tracking-tight text-gray-900 dark:text-white sm:text-7xl">
            <span class="relative inline-block">
              <!-- 标题右上渐变光晕 -->
              <span
                aria-hidden="true"
                class="pointer-events-none absolute -right-16 -top-16 h-44 w-44 rounded-full bg-gradient-to-br from-primary-400/45 via-sky-400/25 to-purple-400/45 blur-3xl opacity-90 mix-blend-multiply dark:opacity-60 dark:mix-blend-screen"
              ></span>
              <span
                aria-hidden="true"
                class="pointer-events-none absolute -right-10 -top-10 h-28 w-28 rounded-full bg-gradient-to-tr from-primary-500/20 to-blue-500/20 blur-2xl opacity-90"
              ></span>
              {{ siteName }}
            </span>
          </h1>

          <p class="mt-6 text-3xl font-semibold tracking-tight text-[#7b6857] dark:text-primary-200 sm:text-5xl">
            {{ t('home.hero.headline') }}
          </p>

          <p class="mt-6 text-base leading-relaxed text-gray-600 dark:text-dark-300 sm:text-lg">
            {{ t('home.hero.description') }}
          </p>

          <div class="mt-10 flex flex-col items-center justify-center gap-3 sm:flex-row">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="inline-flex items-center justify-center rounded-full bg-[#7b6857] px-8 py-3 text-sm font-semibold text-white shadow-lg shadow-black/5 transition-colors hover:bg-[#6b5a4a] dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <svg class="ml-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path stroke-linecap="round" stroke-linejoin="round" d="M13.5 4.5L21 12m0 0l-7.5 7.5M21 12H3" />
              </svg>
            </router-link>

            <router-link
              to="/usage"
              class="inline-flex items-center justify-center rounded-full border border-[#7b6857]/30 bg-white/40 px-8 py-3 text-sm font-semibold text-[#7b6857] shadow-sm backdrop-blur transition-colors hover:bg-white/60 dark:border-white/15 dark:bg-white/5 dark:text-white/80 dark:hover:bg-white/10"
            >
              {{ t('home.hero.secondaryCta') }}
            </router-link>
          </div>

          <div class="mt-12 flex flex-wrap items-center justify-center gap-x-8 gap-y-3 text-sm text-gray-700 dark:text-dark-200">
            <div class="flex items-center gap-2">
              <svg class="h-4 w-4 text-[#7b6857] dark:text-primary-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M3 3v18h18" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M7 14l4-4 3 3 6-7" />
              </svg>
              <span class="font-medium">{{ t('home.hero.stats.performance') }}</span>
            </div>
            <div class="flex items-center gap-2">
              <svg class="h-4 w-4 text-[#7b6857] dark:text-primary-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 3l7.5 4.5V12c0 5.25-3.5 9.75-7.5 9.75S4.5 17.25 4.5 12V7.5L12 3z" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M9.5 12.5l1.75 1.75L15.5 10" />
              </svg>
              <span class="font-medium">{{ t('home.hero.stats.control') }}</span>
            </div>
            <div class="flex items-center gap-2">
              <svg class="h-4 w-4 text-[#7b6857] dark:text-primary-300" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 6v6l4 2" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <span class="font-medium">{{ t('home.hero.stats.security') }}</span>
            </div>
          </div>
        </div>
      </section>

      <!-- Features -->
      <section id="features" class="scroll-mt-28 pb-24">
        <div class="mx-auto max-w-6xl px-6">
          <div class="mb-10 text-center">
            <h2 class="text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">
              {{ t('home.sections.featuresTitle') }}
            </h2>
            <p class="mt-3 text-gray-600 dark:text-dark-300">
              {{ t('home.sections.featuresSubtitle') }}
            </p>
          </div>

          <div class="grid gap-6 md:grid-cols-2">
            <div class="group rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur transition-all hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/5 dark:border-white/10 dark:bg-dark-900/40">
              <div class="mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-blue-600 text-white shadow-lg shadow-blue-600/20">
                <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M12 3l7.5 4.5V12c0 5.25-3.5 9.75-7.5 9.75S4.5 17.25 4.5 12V7.5L12 3z"
                  />
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9.5 12.5l1.75 1.75L15.5 10" />
                </svg>
              </div>
              <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.unifiedGateway') }}
              </h3>
              <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
                {{ t('home.features.unifiedGatewayDesc') }}
              </p>
            </div>

            <div class="group rounded-3xl border border-black/5 bg-white/70 p-6 shadow-sm backdrop-blur transition-all hover:-translate-y-0.5 hover:shadow-lg hover:shadow-black/5 dark:border-white/10 dark:bg-dark-900/40">
              <div class="mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-primary-600 text-white shadow-lg shadow-primary-600/20">
                <svg class="h-6 w-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="m3.75 13.5 10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z"
                  />
                </svg>
              </div>
              <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.multiAccount') }}
              </h3>
              <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
                {{ t('home.features.multiAccountDesc') }}
              </p>
            </div>

          </div>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'

const { t } = useI18n()

const appStore = useAppStore()
const authStore = useAuthStore()

// Site settings
const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))

onMounted(() => {
  authStore.checkAuth()
  appStore.fetchPublicSettings()
})
</script>

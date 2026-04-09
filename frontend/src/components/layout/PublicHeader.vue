<template>
  <header class="fixed inset-x-0 top-0 z-50">
    <div
      class="transition-[padding] duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] motion-reduce:transition-none"
      :class="isScrolled ? 'px-4 pt-4' : 'px-0 pt-0'"
    >
      <div
        class="mx-auto w-full origin-top transform-gpu backdrop-blur transition-[max-width,transform,border-radius,background-color,box-shadow,border-color] duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] motion-reduce:transition-none"
        :class="
          isScrolled
            ? 'max-w-5xl rounded-full border border-black/5 bg-[#efe9e1]/80 shadow-lg shadow-black/5 dark:border-white/10 dark:bg-dark-900/60'
            : 'max-w-[100%] rounded-none border border-transparent border-b-black/5 bg-[#faf7f2]/80 shadow-none dark:border-b-white/10 dark:bg-dark-950/60'
        "
      >
        <nav
          class="mx-auto flex max-w-6xl items-center gap-3 px-4 transition-[height] duration-500 ease-[cubic-bezier(0.16,1,0.3,1)] motion-reduce:transition-none sm:px-6"
          :class="isScrolled ? 'h-12' : 'h-14'"
        >
          <!-- Logo -->
          <router-link
            to="/home"
            class="flex shrink-0 items-center gap-3 rounded-xl px-2 py-1.5 transition-colors hover:bg-black/5 dark:hover:bg-white/10"
            aria-label="Home"
            @click="closeMobileMenu"
          >
            <div class="h-8 w-8 overflow-hidden rounded-lg bg-white shadow-sm ring-1 ring-black/5">
              <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
            </div>
            <span class="hidden text-sm font-semibold text-gray-900 dark:text-gray-100 sm:inline">
              {{ siteName }}
            </span>
          </router-link>

          <!-- Desktop Nav -->
          <div class="hidden items-center gap-1 md:flex">
            <!-- <router-link
              :to="{ path: '/home', hash: '#features' }"
              class="rounded-full px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-black/5 hover:text-gray-900 dark:text-dark-200 dark:hover:bg-white/10 dark:hover:text-white"
              @click="closeMobileMenu"
            >
              {{ t('home.nav.features') }}
            </router-link> -->
            <router-link
              to="/pricing"
              class="rounded-full px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-black/5 hover:text-gray-900 dark:text-dark-200 dark:hover:bg-white/10 dark:hover:text-white"
              @click="closeMobileMenu"
            >
              {{ t('home.nav.pricing') }}
            </router-link>
            <router-link
              to="/docs"
              class="rounded-full px-3 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-black/5 hover:text-gray-900 dark:text-dark-200 dark:hover:bg-white/10 dark:hover:text-white"
              @click="closeMobileMenu"
            >
              {{ t('home.nav.docs') }}
            </router-link>
          </div>

          <!-- Right Actions -->
          <div class="ml-auto flex items-center gap-2 sm:gap-3">
            <LocaleSwitcher class="hidden md:block" />

            <button
              @click="toggleTheme"
              class="inline-flex h-9 w-9 items-center justify-center rounded-full text-gray-700 transition-colors hover:bg-black/5 dark:text-dark-200 dark:hover:bg-white/10"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            >
              <svg
                v-if="isDark"
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z"
                />
              </svg>
              <svg
                v-else
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z"
                />
              </svg>
            </button>

            <button
              class="inline-flex h-9 w-9 items-center justify-center rounded-full text-gray-700 transition-colors hover:bg-black/5 dark:text-dark-200 dark:hover:bg-white/10 md:hidden"
              :title="t('home.nav.menu')"
              @click="isMobileMenuOpen = !isMobileMenuOpen"
            >
              <span class="sr-only">{{ t('home.nav.menu') }}</span>
              <svg
                v-if="isMobileMenuOpen"
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
              <svg
                v-else
                class="h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>

            <div class="hidden h-6 w-px bg-black/10 dark:bg-white/10 sm:block"></div>

            <template v-if="isAuthenticated">
              <router-link
                :to="dashboardPath"
                class="inline-flex items-center rounded-full bg-gray-950 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-gray-900 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
                @click="closeMobileMenu"
              >
                {{ t('home.dashboard') }}
              </router-link>
              <div
                class="flex h-9 w-9 items-center justify-center rounded-full bg-white text-xs font-semibold text-gray-900 ring-1 ring-black/5 dark:bg-dark-700 dark:text-white dark:ring-white/10"
                :title="authStore.user?.email || ''"
              >
                {{ userInitial }}
              </div>
            </template>
            <router-link
              v-else
              to="/login"
              class="inline-flex items-center rounded-full bg-gray-950 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-gray-900 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
              @click="closeMobileMenu"
            >
              {{ t('home.login') }}
            </router-link>
          </div>
        </nav>

        <Transition
          enter-active-class="transition duration-200 ease-out"
          enter-from-class="opacity-0 -translate-y-2"
          enter-to-class="opacity-100 translate-y-0"
          leave-active-class="transition duration-150 ease-in"
          leave-from-class="opacity-100 translate-y-0"
          leave-to-class="opacity-0 -translate-y-2"
        >
          <div v-if="isMobileMenuOpen" class="md:hidden">
            <div class="border-t border-black/5 px-4 py-4 dark:border-white/10">
              <div class="flex flex-col gap-2">
                <div class="flex items-center gap-3">
                  <LocaleSwitcher />
                </div>

                <div class="mt-2 grid gap-2">
                  <router-link
                    :to="{ path: '/home', hash: '#features' }"
                    class="rounded-2xl border border-black/5 bg-white/70 px-4 py-3 text-sm font-medium text-gray-800 shadow-sm backdrop-blur transition-colors hover:bg-white/90 dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-100 dark:hover:bg-dark-900/55"
                    @click="closeMobileMenu"
                  >
                    {{ t('home.nav.features') }}
                  </router-link>
                  <router-link
                    to="/pricing"
                    class="rounded-2xl border border-black/5 bg-white/70 px-4 py-3 text-sm font-medium text-gray-800 shadow-sm backdrop-blur transition-colors hover:bg-white/90 dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-100 dark:hover:bg-dark-900/55"
                    @click="closeMobileMenu"
                  >
                    {{ t('home.nav.pricing') }}
                  </router-link>
                  <router-link
                    to="/docs"
                    class="rounded-2xl border border-black/5 bg-white/70 px-4 py-3 text-sm font-medium text-gray-800 shadow-sm backdrop-blur transition-colors hover:bg-white/90 dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-100 dark:hover:bg-dark-900/55"
                    @click="closeMobileMenu"
                  >
                    {{ t('home.nav.docs') }}
                  </router-link>
                </div>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useWindowScroll } from '@vueuse/core'
import { useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

defineProps<{
  siteName: string
  siteLogo: string
}>()

const { t } = useI18n()

const authStore = useAuthStore()

const { y } = useWindowScroll()
const isScrolled = computed(() => y.value > 16)

const isMobileMenuOpen = ref(false)
const closeMobileMenu = () => {
  isMobileMenuOpen.value = false
}

watch(isScrolled, (next) => {
  if (next) closeMobileMenu()
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const user = authStore.user
  if (!user?.email) return ''
  return user.email.charAt(0).toUpperCase()
})

onMounted(() => {
  initTheme()
  authStore.checkAuth()
})
</script>

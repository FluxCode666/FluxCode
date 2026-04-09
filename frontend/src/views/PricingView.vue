<template>
  <div class="min-h-screen bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="pt-24">
      <section class="mx-auto max-w-6xl px-6 py-20">
        <div class="max-w-2xl">
          <h1 class="text-4xl font-semibold tracking-tight text-gray-900 dark:text-white sm:text-5xl">
            {{ t('home.sections.pricingTitle') }}
          </h1>
          <p class="mt-4 text-base leading-relaxed text-gray-600 dark:text-dark-300">
            {{ t('home.sections.pricingSubtitle') }}
          </p>
        </div>

        <div class="mt-12 space-y-10">
          <div v-if="loading" class="flex justify-center py-12">
            <LoadingSpinner size="lg" color="primary" />
          </div>

          <div
            v-else-if="errorMessage"
            class="rounded-3xl border border-red-200 bg-red-50 p-6 text-red-700 dark:border-red-900/40 dark:bg-red-900/10 dark:text-red-200"
          >
            <div class="flex flex-wrap items-center justify-between gap-3">
              <p class="text-sm">{{ errorMessage }}</p>
              <button class="btn btn-secondary" @click="loadPricing">{{ t('common.refresh') }}</button>
            </div>
          </div>

          <div
            v-else-if="!pricingGroups.length"
            class="rounded-3xl border border-black/5 bg-white/70 p-6 text-sm text-gray-600 shadow-sm backdrop-blur dark:border-white/10 dark:bg-dark-900/40 dark:text-dark-300"
          >
            {{ t('common.notAvailable') }}
          </div>

          <section v-else v-for="group in pricingGroups" :key="group.id">
            <div class="flex items-end justify-between gap-4">
              <div class="max-w-3xl">
                <h2 class="text-2xl font-semibold tracking-tight text-gray-900 dark:text-white">
                  {{ group.name }}
                </h2>
                <p
                  v-if="group.description"
                  class="mt-2 text-sm leading-relaxed text-gray-600 dark:text-dark-300"
                >
                  {{ group.description }}
                </p>
              </div>
            </div>

            <div class="mt-6 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
              <div v-for="plan in group.plans || []" :key="plan.id">
                <div
                  :class="[
                    'group relative flex h-full flex-col overflow-hidden rounded-4xl border bg-gradient-glass p-5 shadow-glass backdrop-blur-2xl backdrop-saturate-150 transition-all duration-300',
                    plan.is_featured
                      ? 'border-primary-300/40 bg-white/60 ring-1 ring-primary-500/20 dark:border-primary-500/30 dark:bg-dark-900/40 dark:ring-primary-500/15'
                      : 'border-white/60 bg-white/50 ring-1 ring-black/5 dark:border-white/10 dark:bg-dark-900/30 dark:ring-white/10',
                    'hover:-translate-y-1 hover:shadow-card-hover'
                  ]"
                >
                  <div class="pointer-events-none absolute right-5 top-10 h-24 w-24">
                    <div
                      class="absolute right-8 top-10 z-10 h-20 w-20 rotate-[4deg] rounded-3xl border border-black/10 opacity-55 shadow-sm transition-transform duration-500 group-hover:translate-x-1 group-hover:-translate-y-1 group-hover:rotate-[2deg] dark:border-white/10"
                    ></div>
                    <div
                      class="absolute right-2 top-4 z-20 h-16 w-16 rotate-[-7deg] rounded-3xl border border-black/10 opacity-70 shadow-sm transition-transform duration-500 group-hover:translate-x-1 group-hover:-translate-y-1 group-hover:rotate-[-3deg] dark:border-white/10"
                    ></div>
                    <div
                      class="absolute -right-3 top-0 z-30 h-12 w-12 rotate-[11deg] rounded-2xl border border-black/10 opacity-85 shadow-sm transition-transform duration-500 group-hover:translate-x-1 group-hover:-translate-y-1 group-hover:rotate-[6deg] dark:border-white/10"
                    ></div>
                  </div>

                  <div class="flex items-start justify-between gap-4">
                    <div class="flex items-center gap-4">
                      <div
                        class="flex h-9 w-9 flex-none items-center justify-center rounded-2xl border border-black/10 bg-white shadow-sm transition-transform duration-300 group-hover:scale-[1.02] dark:border-white/10 dark:bg-dark-900/50"
                      >
                        <img v-if="plan.icon_url" :src="plan.icon_url" class="h-5 w-5" alt="" />
                        <span v-else class="text-sm font-semibold text-gray-900 dark:text-white">
                          {{ (plan.name || '').slice(0, 1) }}
                        </span>
                      </div>

                      <div class="min-w-0">
                        <h3 class="truncate text-base font-semibold text-gray-900 dark:text-white">
                          {{ plan.name }}
                        </h3>
                        <p v-if="plan.description" class="mt-1 truncate text-sm text-gray-500 dark:text-dark-400">
                          {{ plan.description }}
                        </p>
                      </div>
                    </div>

                    <div class="flex flex-col items-end gap-2">
                      <span
                        v-if="plan.badge_text"
                        class="inline-flex items-center rounded-full bg-black/5 px-4 py-1 text-xs font-medium text-gray-700 dark:bg-white/10 dark:text-dark-200"
                      >
                        {{ plan.badge_text }}
                      </span>
                      <span
                        v-if="plan.is_featured"
                        class="inline-flex items-center rounded-full bg-primary-600 px-3 py-1 text-xs font-medium text-white shadow shadow-primary-600/20"
                      >
                        {{ t('common.recommended') }}
                      </span>
                    </div>
                  </div>

                  <div class="mt-7 text-center">
                    <div class="text-3xl font-semibold tracking-tight text-gray-900 dark:text-white">
                      {{ formatPriceLine(plan) }}
                    </div>
                    <p v-if="plan.tagline" class="mt-2 text-sm text-gray-500 dark:text-dark-400">
                      {{ plan.tagline }}
                    </p>
                  </div>

                  <div
                    v-if="plan.features?.length"
                    class="mt-5 rounded-3xl border border-white/50 bg-gradient-glass bg-white/35 p-4 shadow-glass-sm backdrop-blur-xl dark:border-white/10 dark:bg-dark-900/25"
                  >
                    <ul class="space-y-2.5">
                      <li
                        v-for="(feature, idx) in plan.features"
                        :key="idx"
                        class="flex items-start gap-3 text-sm text-gray-700 dark:text-dark-200"
                      >
                        <span class="mt-2 h-1.5 w-1.5 flex-none rounded-full bg-blue-500"></span>
                        <span class="leading-relaxed">{{ feature }}</span>
                      </li>
                    </ul>
                  </div>

                  <div class="mt-auto pt-5">
                    <div class="relative" @click.stop>
                      <div
                        v-if="isPurchaseOpen(plan.id)"
                        class="absolute bottom-full left-0 right-0 z-40 mb-3"
                        role="dialog"
                        aria-label="Purchase options"
                      >
                        <div
                          class="rounded-3xl border border-white/60 bg-gradient-glass bg-white/45 p-3 shadow-glass-sm backdrop-blur-xl dark:border-white/10 dark:bg-dark-900/40"
                        >
                          <div class="flex flex-wrap gap-2">
                            <button
                              v-for="(entry, idx) in planPurchaseEntries(plan)"
                              :key="idx"
                              type="button"
                              class="inline-flex items-center rounded-full border border-white/60 bg-white/35 px-3 py-1.5 text-xs font-medium text-gray-800 shadow-glass-sm backdrop-blur-xl transition-colors hover:bg-white/50 dark:border-white/10 dark:bg-dark-900/25 dark:text-dark-100 dark:hover:bg-dark-900/35"
                              :title="entry.value"
                              @click="activatePurchaseEntry(entry)"
                            >
                              {{ entry.type }}
                            </button>
                          </div>
                        </div>
                      </div>

                      <button
                        type="button"
                        class="flex w-full items-center justify-center gap-2 rounded-3xl bg-primary-600 px-4 py-3 text-sm font-semibold text-white shadow shadow-primary-600/20 transition-all duration-300 hover:bg-primary-700 hover:shadow-card-hover disabled:cursor-not-allowed disabled:bg-gray-400 disabled:shadow-none dark:disabled:bg-dark-700"
                        :disabled="!planPurchaseEntries(plan).length"
                        :aria-expanded="isPurchaseOpen(plan.id)"
                        @click="togglePurchase(plan.id)"
                      >
                        {{ t('common.purchase') }}
                        <svg
                          :class="['h-4 w-4 transition-transform duration-300', isPurchaseOpen(plan.id) ? 'rotate-180' : '']"
                          viewBox="0 0 20 20"
                          fill="currentColor"
                          aria-hidden="true"
                        >
                          <path
                            fill-rule="evenodd"
                            d="M5.23 7.21a.75.75 0 011.06.02L10 10.94l3.71-3.71a.75.75 0 111.06 1.06l-4.24 4.24a.75.75 0 01-1.06 0L5.21 8.29a.75.75 0 01.02-1.08z"
                            clip-rule="evenodd"
                          />
                        </svg>
                      </button>
                    </div>
                  </div>
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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { pricingPlansAPI } from '@/api/pricingPlans'
import type { PricingPlan, PricingPlanContactMethod, PricingPlanGroup } from '@/types'
import { useClipboard } from '@/composables/useClipboard'

const { t, locale } = useI18n()

const appStore = useAppStore()

// Site settings
const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')

const pricingGroups = ref<PricingPlanGroup[]>([])
const loading = ref(false)
const errorMessage = ref('')
let abortController: AbortController | null = null
const { copyToClipboard } = useClipboard()
const expandedPurchasePlanId = ref<number | null>(null)

function currencySymbol(currency: string): string {
  const c = (currency || '').toUpperCase()
  if (c === 'USD') return '$'
  if (c === 'CNY' || c === 'RMB') return '¥'
  if (c === 'EUR') return '€'
  if (c === 'GBP') return '£'
  return c ? `${c} ` : ''
}

function formatAmount(amount: number): string {
  if (Number.isInteger(amount)) return String(amount)
  const fixed = amount.toFixed(2)
  return fixed.replace(/\\.00$/, '').replace(/(\\.\\d)0$/, '$1')
}

function formatPriceLine(plan: PricingPlan): string {
  if (plan.price_text) return plan.price_text
  if (plan.price_amount === null || plan.price_amount === undefined) return '—'
  const amount = `${currencySymbol(plan.price_currency)}${formatAmount(plan.price_amount)}`
  const periodLabel = plan.price_period ? formatPeriod(plan.price_period) : ''
  return periodLabel ? `${amount}/${periodLabel}` : amount
}

function formatPeriod(period: string): string {
  const p = (period || '').toLowerCase()
  const isZh = locale.value?.toString().toLowerCase().startsWith('zh')

  const zhMap: Record<string, string> = {
    month: '月',
    year: '年',
    day: '天',
    week: '周',
    once: '一次',
    one_time: '一次'
  }
  const enMap: Record<string, string> = {
    month: 'mo',
    year: 'yr',
    day: 'day',
    week: 'wk',
    once: 'once',
    one_time: 'once'
  }
  return (isZh ? zhMap[p] : enMap[p]) || period
}

function planPurchaseEntries(plan: PricingPlan): PricingPlanContactMethod[] {
  return (plan?.contact_methods || []).filter((m) => (m?.type || '').trim() && (m?.value || '').trim())
}

function togglePurchase(planID: number) {
  expandedPurchasePlanId.value = expandedPurchasePlanId.value === planID ? null : planID
}

function isPurchaseOpen(planID: number): boolean {
  return expandedPurchasePlanId.value === planID
}

function isHTTPURL(value: string): boolean {
  const v = (value || '').trim()
  if (!v) return false
  try {
    const parsed = new URL(v)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

async function activatePurchaseEntry(entry: PricingPlanContactMethod) {
  const value = (entry?.value || '').trim()
  if (!value) return

  if (isHTTPURL(value)) {
    window.open(value, '_blank', 'noopener,noreferrer')
  } else {
    await copyToClipboard(value, t('common.copiedToClipboard'))
  }

  expandedPurchasePlanId.value = null
}

async function loadPricing() {
  abortController?.abort()
  abortController = new AbortController()

  loading.value = true
  errorMessage.value = ''

  try {
    pricingGroups.value = await pricingPlansAPI.listPublicPlanGroups({ signal: abortController.signal })
  } catch (error) {
    errorMessage.value = (error as { message?: string })?.message || t('common.unknown')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  appStore.fetchPublicSettings()
  loadPricing()

  document.addEventListener('click', closePurchasePopover)
})

onBeforeUnmount(() => {
  abortController?.abort()
  document.removeEventListener('click', closePurchasePopover)
})

function closePurchasePopover() {
  expandedPurchasePlanId.value = null
}
</script>

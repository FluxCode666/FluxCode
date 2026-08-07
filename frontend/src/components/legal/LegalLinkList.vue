<template>
  <nav :aria-label="copy.ariaLabel" class="flex flex-wrap items-center justify-center gap-x-3 gap-y-1.5">
    <router-link
      v-for="item in links"
      :key="item.path"
      :to="item.path"
      class="rounded-md text-gray-500 underline-offset-4 transition-colors hover:text-primary-600 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 dark:text-dark-400 dark:hover:text-primary-300"
    >
      {{ isZh ? item.shortTitle : englishLabels[item.key] }}
    </router-link>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { legalDocumentNavigation, type LegalDocumentKey } from '@/content/legalDocuments'

const { locale } = useI18n()
const isZh = computed(() => String(locale.value).toLowerCase().startsWith('zh'))

const links = legalDocumentNavigation
const englishLabels: Record<LegalDocumentKey, string> = {
  terms: 'Terms',
  'usage-policy': 'Usage Policy',
  'supported-regions': 'Supported Regions',
  'service-specific-terms': 'Service Terms'
}

const copy = computed(() => ({
  ariaLabel: isZh.value ? '法律文件' : 'Legal documents'
}))
</script>

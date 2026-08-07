<template>
  <footer
    data-testid="public-footer"
    class="relative z-10 border-t border-gray-200/60 bg-white/20 px-6 py-3 dark:border-white/10 dark:bg-dark-900/20"
  >
    <div class="mx-auto flex max-w-7xl flex-wrap items-center justify-center gap-x-5 gap-y-1 text-center sm:justify-between sm:text-left">
      <p class="text-xs text-gray-500 dark:text-dark-400">
        &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
      </p>
      <div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-1">
        <LegalLinkList class="text-[11px]" />
        <slot />
      </div>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LegalLinkList from '@/components/legal/LegalLinkList.vue'

defineProps<{
  siteName: string
}>()

const { t } = useI18n()
const currentYear = new Date().getFullYear()
</script>

<template>
  <div class="flex min-h-screen flex-col bg-[#faf7f2] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <PublicHeader :site-name="siteName" :site-logo="siteLogo" />

    <main class="mx-auto w-full max-w-6xl flex-1 px-6 pb-20 pt-28">
      <ChannelStatusContent variant="public" />
    </main>

    <PublicFooter :site-name="siteName" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import PublicHeader from '@/components/layout/PublicHeader.vue'
import PublicFooter from '@/components/layout/PublicFooter.vue'
import ChannelStatusContent from '@/components/user/monitor/ChannelStatusContent.vue'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'FluxCode')
const siteLogo = computed(() => appStore.siteLogo || '')

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

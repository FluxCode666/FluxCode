<template>
  <div class="overflow-hidden rounded-2xl border border-slate-800/80 bg-slate-950 shadow-sm shadow-slate-950/15">
    <div class="flex items-center justify-between gap-3 border-b border-white/10 bg-white/[0.035] px-4 py-2.5">
      <span class="font-mono text-xs font-medium tracking-wide text-slate-400">{{ label }}</span>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-xs font-medium text-slate-300 transition-colors hover:bg-white/10 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400"
        :aria-label="copied ? t('common.copied') : t('keys.copyToClipboard')"
        @click="copyCode"
      >
        <svg v-if="copied" class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="m5 12 4 4L19 6" />
        </svg>
        <svg v-else class="h-3.5 w-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <rect x="9" y="9" width="11" height="11" rx="2" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
        </svg>
        <span>{{ copied ? t('common.copied') : t('keys.copyToClipboard') }}</span>
      </button>
    </div>
    <pre class="overflow-x-auto p-4 text-[13px] leading-6 text-slate-100"><code>{{ code }}</code></pre>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'

const props = withDefaults(
  defineProps<{
    code: string
    label?: string
  }>(),
  {
    label: 'terminal'
  }
)

const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const copied = ref(false)
let resetTimer: ReturnType<typeof setTimeout> | null = null

async function copyCode(): Promise<void> {
  const success = await copyToClipboard(props.code, t('common.copiedToClipboard'))
  if (!success) return

  copied.value = true
  if (resetTimer) clearTimeout(resetTimer)
  resetTimer = setTimeout(() => {
    copied.value = false
    resetTimer = null
  }, 1800)
}

onBeforeUnmount(() => {
  if (resetTimer) clearTimeout(resetTimer)
})
</script>

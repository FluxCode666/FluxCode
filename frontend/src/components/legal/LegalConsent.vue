<template>
  <div
    class="rounded-xl border px-4 py-3 transition-colors"
    :class="modelValue
      ? 'border-primary-200 bg-primary-50/70 dark:border-primary-500/30 dark:bg-primary-500/10'
      : 'border-gray-200 bg-gray-50/80 dark:border-dark-700 dark:bg-dark-900/45'"
    data-testid="legal-consent"
  >
    <label :for="inputId" class="flex cursor-pointer items-start gap-3">
      <input
        :id="inputId"
        :checked="modelValue"
        type="checkbox"
        required
        class="mt-0.5 h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-900"
        @change="onChange"
      />
      <span class="text-xs leading-5 text-gray-600 dark:text-dark-300">
        {{ copy.prefix }}
        <template v-for="(item, index) in links" :key="item.path">
          <router-link
            :to="item.path"
            target="_blank"
            class="font-medium text-primary-600 underline-offset-4 transition-colors hover:text-primary-500 hover:underline focus:outline-none focus-visible:ring-2 focus-visible:ring-primary-400 dark:text-primary-300"
            @click.stop
          >
            {{ isZh ? item.shortTitle : englishLabels[item.key] }}
          </router-link>{{ index < links.length - 1 ? copy.separator : '' }}
        </template>
      </span>
    </label>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { legalDocumentNavigation, type LegalDocumentKey } from '@/content/legalDocuments'

withDefaults(defineProps<{
  modelValue: boolean
  inputId?: string
}>(), {
  inputId: 'legal-consent'
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const { locale } = useI18n()
const isZh = computed(() => String(locale.value).toLowerCase().startsWith('zh'))
const links = legalDocumentNavigation

const englishLabels: Record<LegalDocumentKey, string> = {
  terms: 'Terms of Service',
  'usage-policy': 'Usage Policy',
  'supported-regions': 'Supported Regions',
  'service-specific-terms': 'Service-specific Terms'
}

const copy = computed(() => isZh.value
  ? { prefix: '我已阅读并同意', separator: '、' }
  : { prefix: 'I have read and agree to the ', separator: ', ' })

function onChange(event: Event): void {
  emit('update:modelValue', (event.target as HTMLInputElement).checked)
}
</script>

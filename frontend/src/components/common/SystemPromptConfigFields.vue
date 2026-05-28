<template>
  <section class="space-y-3" data-test="system-prompt-config-fields">
    <div>
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ title }}</h3>
      <p v-if="description" class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">
        {{ description }}
      </p>
    </div>

    <div class="grid gap-3 sm:grid-cols-[180px_1fr]">
      <div>
        <label class="input-label">{{ modeLabel }}</label>
        <Select
          :model-value="mode"
          :options="modeOptions"
          :disabled="disabled"
          @update:model-value="updateMode($event as SystemPromptMode)"
        />
      </div>

      <div>
        <label class="input-label">{{ promptLabel }}</label>
        <textarea
          :value="prompt"
          :disabled="promptDisabled"
          rows="4"
          class="input resize-y font-mono text-sm disabled:cursor-not-allowed disabled:bg-gray-50 disabled:text-gray-400 dark:disabled:bg-dark-700 dark:disabled:text-gray-500"
          :placeholder="promptPlaceholder"
          @input="emit('update:prompt', ($event.target as HTMLTextAreaElement).value)"
        />
        <p v-if="promptHint" class="input-hint">{{ promptHint }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Select from '@/components/common/Select.vue'
import type { SystemPromptMode } from '@/types'

type SystemPromptModeOption = {
  value: SystemPromptMode
  label: string
  disabled?: boolean
  [key: string]: unknown
}

const props = defineProps<{
  title: string
  mode: SystemPromptMode
  prompt: string
  modeOptions: SystemPromptModeOption[]
  modeLabel: string
  promptLabel: string
  description?: string
  promptPlaceholder?: string
  promptHint?: string
  disabled?: boolean
}>()

const emit = defineEmits<{
  (event: 'update:mode', value: SystemPromptMode): void
  (event: 'update:prompt', value: string): void
}>()

const promptDisabled = computed(() => props.disabled || props.mode === 'inherit')

function updateMode(mode: SystemPromptMode) {
  emit('update:mode', mode)
  if (mode === 'inherit' && props.prompt !== '') {
    emit('update:prompt', '')
  }
}
</script>

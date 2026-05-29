<template>
  <section class="space-y-3" data-test="system-prompt-config-fields">
    <div v-if="title || description">
      <h3 v-if="title" class="text-sm font-semibold text-gray-900 dark:text-white">{{ title }}</h3>
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
        >
          <template #option="{ option, selected }">
            <div class="flex min-w-0 flex-1 items-center gap-2" data-test="system-prompt-option-row">
              <span class="select-option-label">{{ modeOptionLabel(option) }}</span>
              <Icon
                v-if="selected"
                data-test="system-prompt-option-check"
                name="check"
                size="sm"
                class="flex-shrink-0 text-primary-500"
                :stroke-width="2"
              />
              <HelpTooltip v-if="modeOptionDescription(option)" :content="modeOptionDescription(option)">
                <template #trigger>
                  <span
                    data-test="system-prompt-option-help"
                    class="inline-flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full text-gray-400 transition-colors hover:text-primary-600 dark:text-gray-500 dark:hover:text-primary-400"
                    :title="modeOptionDescription(option)"
                    :aria-label="modeOptionHelpLabel(option)"
                    @click.stop
                    @mousedown.stop
                  >
                    <Icon name="questionCircle" size="sm" :stroke-width="2" />
                  </span>
                </template>
              </HelpTooltip>
            </div>
          </template>
        </Select>
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
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import type { SystemPromptMode } from '@/types'

type SystemPromptModeOption = {
  value: SystemPromptMode
  label: string
  description?: string
  disabled?: boolean
  [key: string]: unknown
}

const props = defineProps<{
  mode: SystemPromptMode
  prompt: string
  modeOptions: SystemPromptModeOption[]
  modeLabel: string
  promptLabel: string
  title?: string
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

function modeOptionLabel(option: unknown): string {
  if (option && typeof option === 'object' && 'label' in option) {
    return String((option as { label?: unknown }).label ?? '')
  }
  return ''
}

function modeOptionDescription(option: unknown): string {
  if (option && typeof option === 'object' && 'description' in option) {
    return String((option as { description?: unknown }).description ?? '')
  }
  return ''
}

function modeOptionHelpLabel(option: unknown): string {
  return `${modeOptionLabel(option)}: ${modeOptionDescription(option)}`
}
</script>

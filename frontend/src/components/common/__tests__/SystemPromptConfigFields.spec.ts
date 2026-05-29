import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

vi.mock('@/components/common/Select.vue', () => ({
  default: {
    name: 'Select',
    props: ['modelValue', 'options', 'disabled'],
    emits: ['update:modelValue'],
    template: `
      <select
        data-test="system-prompt-mode"
        :value="modelValue"
        :disabled="disabled"
        @change="$emit('update:modelValue', $event.target.value)"
      >
        <option v-for="option in options" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
      <div data-test="system-prompt-option-list">
        <div v-for="option in options" :key="'slot-' + option.value" data-test="system-prompt-option">
          <slot name="option" :option="option" :selected="option.value === modelValue">
            {{ option.label }}
          </slot>
        </div>
      </div>
    `,
  },
}))

import SystemPromptConfigFields from '../SystemPromptConfigFields.vue'
import type { SystemPromptMode } from '@/types'

const modeOptions: Array<{ value: SystemPromptMode; label: string; description: string }> = [
  { value: 'inherit', label: '不配置', description: '不在当前层级配置提示词' },
  { value: 'passthrough', label: '透传', description: '请求已有系统提示词时保持原样' },
  { value: 'override', label: '覆盖', description: '使用当前配置替换请求中的系统提示词' },
  { value: 'append', label: '追加', description: '把当前配置追加到请求已有系统提示词前' },
]

describe('SystemPromptConfigFields', () => {
  it('渲染四种模式，并在不配置时禁用提示词输入', () => {
    const wrapper = mount(SystemPromptConfigFields, {
      props: {
        mode: 'inherit',
        prompt: '',
        title: 'OpenAI',
        description: '平台默认提示词',
        modeLabel: '模式',
        promptLabel: '系统提示词',
        promptPlaceholder: '输入系统提示词',
        promptHint: '不配置时继续使用下一级优先级',
        modeOptions,
      },
    })

    expect(wrapper.text()).toContain('OpenAI')
    expect(wrapper.text()).toContain('平台默认提示词')
    expect(wrapper.findAll('option').map((option) => option.text())).toEqual([
      '不配置',
      '透传',
      '覆盖',
      '追加',
    ])
    expect(wrapper.find('textarea').attributes('disabled')).toBeDefined()
  })

  it('切换模式和输入提示词时向父组件同步字段', async () => {
    const wrapper = mount(SystemPromptConfigFields, {
      props: {
        mode: 'override',
        prompt: 'old',
        title: 'API Key',
        modeLabel: '模式',
        promptLabel: '系统提示词',
        modeOptions,
      },
    })

    await wrapper.find('[data-test="system-prompt-mode"]').setValue('append')
    await wrapper.find('textarea').setValue('new prompt')

    expect(wrapper.emitted('update:mode')?.[0]).toEqual(['append'])
    expect(wrapper.emitted('update:prompt')?.[0]).toEqual(['new prompt'])
  })

  it('为每个模式选项渲染问号提示', () => {
    const wrapper = mount(SystemPromptConfigFields, {
      props: {
        mode: 'inherit',
        prompt: '',
        title: 'OpenAI',
        modeLabel: '模式',
        promptLabel: '系统提示词',
        modeOptions,
      },
    })

    const helps = wrapper.findAll('[data-test="system-prompt-option-help"]')

    expect(helps).toHaveLength(4)
    expect(helps.map((help) => help.attributes('title'))).toEqual(modeOptions.map((option) => option.description))
    expect(helps.map((help) => help.attributes('aria-label'))).toEqual(
      modeOptions.map((option) => `${option.label}: ${option.description}`)
    )
  })

  it('将选中对钩渲染在问号提示之前', () => {
    const wrapper = mount(SystemPromptConfigFields, {
      props: {
        mode: 'override',
        prompt: 'system prompt',
        title: 'OpenAI',
        modeLabel: '模式',
        promptLabel: '系统提示词',
        modeOptions,
      },
    })

    const selectedOption = wrapper.findAll('[data-test="system-prompt-option"]')[2]
    const check = selectedOption.find('[data-test="system-prompt-option-check"]')
    const help = selectedOption.find('[data-test="system-prompt-option-help"]')

    expect(check.exists()).toBe(true)
    expect(help.exists()).toBe(true)
    expect(check.element.compareDocumentPosition(help.element) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('未传标题和描述时不渲染字段头部', () => {
    const wrapper = mount(SystemPromptConfigFields, {
      props: {
        mode: 'inherit',
        prompt: '',
        modeLabel: '模式',
        promptLabel: '系统提示词',
        modeOptions,
      },
    })

    expect(wrapper.find('h3').exists()).toBe(false)
  })

  it('切换为不配置时同步清空提示词', async () => {
    const wrapper = mount(SystemPromptConfigFields, {
      props: {
        mode: 'override',
        prompt: 'old prompt',
        title: 'API Key',
        modeLabel: '模式',
        promptLabel: '系统提示词',
        modeOptions,
      },
    })

    await wrapper.find('[data-test="system-prompt-mode"]').setValue('inherit')

    expect(wrapper.emitted('update:mode')?.[0]).toEqual(['inherit'])
    expect(wrapper.emitted('update:prompt')?.[0]).toEqual([''])
  })
})

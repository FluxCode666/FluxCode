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
    `,
  },
}))

import SystemPromptConfigFields from '../SystemPromptConfigFields.vue'
import type { SystemPromptMode } from '@/types'

const modeOptions: Array<{ value: SystemPromptMode; label: string }> = [
  { value: 'inherit', label: '不配置' },
  { value: 'passthrough', label: '透传' },
  { value: 'override', label: '覆盖' },
  { value: 'append', label: '追加' },
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

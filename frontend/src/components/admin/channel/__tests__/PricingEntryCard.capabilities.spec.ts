import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PricingEntryCard from '../PricingEntryCard.vue'
import IntervalRow from '../IntervalRow.vue'
import type { PricingFormEntry } from '../types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (_key: string, fallback?: string) => fallback || _key })
}))

vi.mock('@/api/admin/channels', () => ({
  default: {
    getModelDefaultPricing: vi.fn()
  }
}))

const baseEntry: PricingFormEntry = {
  models: ['claude-sonnet-4'],
  capabilities: ['streaming'],
  billing_mode: 'token',
  input_price: null,
  output_price: null,
  cache_write_price: null,
  cache_read_price: null,
  image_output_price: null,
  per_request_price: null,
  intervals: []
}

describe('PricingEntryCard capabilities', () => {

  it('input-only 区间仅渲染范围与输入价格', () => {
    const wrapper = mount(IntervalRow, {
      props: {
        mode: 'token',
        inputOnly: true,
        interval: {
          min_tokens: 0, max_tokens: 1000, tier_label: '', input_price: 0,
          output_price: 2, cache_write_price: 3, cache_read_price: 4,
          per_request_price: null, sort_order: 0
        }
      },
      global: { stubs: { Icon: true } }
    })
    expect(wrapper.text()).toContain('输入')
    expect(wrapper.text()).not.toContain('输出')
    expect(wrapper.text()).not.toContain('缓存W')
    expect(wrapper.text()).not.toContain('缓存R')
    expect(wrapper.findAll('input')).toHaveLength(3)
  })

  it('embedding 固定 token 输入计价并显示显式零价停用警告', () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: { ...baseEntry, models: ['embed'], billing_mode: 'per_request', input_price: 0 }, platform: 'embedding' },
      global: { stubs: { Icon: true, Select: true, ModelTagInput: true, IntervalRow: true } }
    })
    expect(wrapper.text()).toContain('已停用')
    expect(wrapper.find('[data-testid="embedding-input-price"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="embedding-output-price"]').exists()).toBe(false)
    expect(wrapper.find('.select-stub').exists()).toBe(false)
  })
  it('renders model capability checkboxes', () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: baseEntry, platform: 'anthropic' },
      global: {
        stubs: {
          Icon: true,
          Select: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    expect(wrapper.find('[data-testid="capability-streaming"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-system_prompt"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-function_calling"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-tools"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-json_mode"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-structured_output"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-prompt_cache"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-vision"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-image_generation"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-video_generation"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-audio_input"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="capability-audio_output"]').exists()).toBe(true)
  })

  it('emits updated capabilities when tools is checked', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: baseEntry, platform: 'anthropic' },
      global: {
        stubs: {
          Icon: true,
          Select: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    await wrapper.get('[data-testid="capability-tools"]').setValue(true)

    const updates = wrapper.emitted('update') || []
    expect(updates.at(-1)?.[0]).toMatchObject({
      capabilities: ['streaming', 'tools']
    })
  })

  it('emits updated capabilities when streaming is unchecked after parent re-renders with tools selected', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: baseEntry, platform: 'anthropic' },
      global: {
        stubs: {
          Icon: true,
          Select: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    await wrapper.setProps({
      entry: {
        ...baseEntry,
        capabilities: ['streaming', 'tools']
      }
    })

    await wrapper.get('[data-testid="capability-streaming"]').setValue(false)

    const updates = wrapper.emitted('update') || []
    expect(updates.at(-1)?.[0]).toMatchObject({
      capabilities: ['tools']
    })
  })

  it('emits updated capabilities when prompt cache is checked', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: baseEntry, platform: 'anthropic' },
      global: {
        stubs: {
          Icon: true,
          Select: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    await wrapper.get('[data-testid="capability-prompt_cache"]').setValue(true)

    const updates = wrapper.emitted('update') || []
    expect(updates.at(-1)?.[0]).toMatchObject({
      capabilities: ['streaming', 'prompt_cache']
    })
  })

  it('emits updated capabilities when audio output is checked', async () => {
    const wrapper = mount(PricingEntryCard, {
      props: { entry: baseEntry, platform: 'anthropic' },
      global: {
        stubs: {
          Icon: true,
          Select: true,
          ModelTagInput: true,
          IntervalRow: true
        }
      }
    })

    await wrapper.get('[data-testid="capability-audio_output"]').setValue(true)

    const updates = wrapper.emitted('update') || []
    expect(updates.at(-1)?.[0]).toMatchObject({
      capabilities: ['streaming', 'audio_output']
    })
  })
})

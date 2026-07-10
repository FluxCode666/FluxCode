import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PricingEntryCard from '../PricingEntryCard.vue'
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

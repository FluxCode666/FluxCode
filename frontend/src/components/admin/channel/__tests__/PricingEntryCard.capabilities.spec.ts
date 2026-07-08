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
  capabilities: ['chat'],
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
  it('emits updated capabilities when labels are toggled', async () => {
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

    await wrapper.get('[data-testid="capability-image"]').setValue(true)

    const updates = wrapper.emitted('update') || []
    expect(updates.at(-1)?.[0]).toMatchObject({
      capabilities: ['chat', 'image']
    })
  })
})

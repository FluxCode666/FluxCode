import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import GroupBadge from '../GroupBadge.vue'
import GroupOptionItem from '../GroupOptionItem.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('embedding group color', () => {
  it('uses a dedicated rose badge in standard and subscription groups', () => {
    const standard = mount(GroupBadge, {
      props: { name: 'Embedding', platform: 'embedding', subscriptionType: 'standard' },
      global: { stubs: { PlatformIcon: true } }
    })
    expect(standard.classes()).toContain('bg-rose-50')
    expect(standard.classes()).toContain('text-rose-700')

    const subscription = mount(GroupBadge, {
      props: { name: 'Embedding', platform: 'embedding', subscriptionType: 'subscription' },
      global: { stubs: { PlatformIcon: true } }
    })
    expect(subscription.classes()).toContain('bg-rose-100')
    expect(subscription.classes()).toContain('text-rose-700')
  })

  it('uses the same rose color for the group rate pill', () => {
    const wrapper = mount(GroupOptionItem, {
      props: { name: 'Embedding', platform: 'embedding', rateMultiplier: 1 },
      global: { stubs: { PlatformIcon: true } }
    })
    expect(wrapper.findAll('.bg-rose-50')).toHaveLength(2)
  })
})

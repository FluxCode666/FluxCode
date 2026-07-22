import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('PlatformTypeBadge Agent Identity', () => {
  it('distinguishes Agent Identity from normal OpenAI OAuth', async () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: {
        platform: 'openai',
        type: 'oauth',
        authMode: 'agentIdentity'
      },
      global: {
        stubs: { Icon: true, PlatformIcon: true }
      }
    })

    expect(wrapper.text()).toContain('Agent Identity')
    await wrapper.setProps({ authMode: undefined })
    expect(wrapper.text()).toContain('OAuth')
    expect(wrapper.text()).not.toContain('Agent Identity')
  })
})

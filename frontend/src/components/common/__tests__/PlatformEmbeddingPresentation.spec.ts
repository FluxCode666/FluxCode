import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PlatformIcon from '../PlatformIcon.vue'
import PlatformTypeBadge from '../PlatformTypeBadge.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

describe('embedding platform presentation', () => {
  it('renders the dedicated vector icon', () => {
    const wrapper = mount(PlatformIcon, { props: { platform: 'embedding' } })
    expect(wrapper.find('path[d="M8 7h8M8 17h8M6 9v6M18 9v6"]').exists()).toBe(true)
  })

  it('renders the explicit platform and API key labels', () => {
    const wrapper = mount(PlatformTypeBadge, {
      props: { platform: 'embedding', type: 'apikey' },
      global: { stubs: { Icon: true, PlatformIcon: true } }
    })
    expect(wrapper.text()).toContain('Embedding')
    expect(wrapper.text()).toContain('Key')
  })
})

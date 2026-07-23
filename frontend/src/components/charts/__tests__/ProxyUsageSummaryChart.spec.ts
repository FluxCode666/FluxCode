import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ProxyUsageSummaryChart from '../ProxyUsageSummaryChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('ProxyUsageSummaryChart', () => {
  it('is collapsed by default and emits expand when opened', async () => {
    const wrapper = mount(ProxyUsageSummaryChart, {
      props: {
        items: [],
        title: '代理使用统计'
      },
      global: {
        stubs: {
          Icon: true,
          LoadingSpinner: true
        }
      }
    })

    const toggle = wrapper.get('[data-test="proxy-usage-summary-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[data-test="proxy-usage-summary-content"]').exists()).toBe(false)

    await toggle.trigger('click')

    expect(toggle.attributes('aria-expanded')).toBe('true')
    expect(wrapper.emitted('expand')).toHaveLength(1)
    expect(wrapper.find('[data-test="proxy-usage-summary-content"]').exists()).toBe(true)
  })
})

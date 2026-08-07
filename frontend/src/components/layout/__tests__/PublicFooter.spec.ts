import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import PublicFooter from '@/components/layout/PublicFooter.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key === 'home.footer.allRightsReserved' ? '保留所有权利。' : key,
      locale: { value: 'zh-CN' }
    })
  }
})

describe('PublicFooter', () => {
  it('renders copyright and all public legal documents', async () => {
    const legalPaths = [
      '/legal/terms',
      '/legal/usage-policy',
      '/legal/supported-regions',
      '/legal/service-specific-terms'
    ]
    const router = createRouter({
      history: createMemoryHistory(),
      routes: legalPaths.map((path) => ({
        path,
        component: { template: '<div />' }
      }))
    })
    await router.push('/legal/terms')
    await router.isReady()

    const wrapper = mount(PublicFooter, {
      props: { siteName: 'FluxCode' },
      global: { plugins: [router] }
    })

    expect(wrapper.text()).toContain(`© ${new Date().getFullYear()} FluxCode. 保留所有权利。`)
    expect(wrapper.findAll('nav a').map((link) => link.attributes('href'))).toEqual(legalPaths)
  })
})

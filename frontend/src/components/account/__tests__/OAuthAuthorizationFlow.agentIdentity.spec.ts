import { mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'

import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('OAuthAuthorizationFlow', () => {
  it('shows an accessible browser action after generating an authorization URL', () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        authUrl: 'https://auth.example/authorize?state=test',
        platform: 'openai',
        showCookieOption: false
      },
      global: {
        plugins: [createPinia()],
        stubs: { Icon: true }
      }
    })

    const openBrowser = wrapper.get<HTMLAnchorElement>('[data-testid="open-oauth-url"]')
    expect(openBrowser.text()).toContain('admin.accounts.oauth.openBrowser')
    expect(openBrowser.attributes('href')).toBe('https://auth.example/authorize?state=test')
    expect(openBrowser.attributes('target')).toBe('_blank')
    expect(openBrowser.attributes('rel')).toBe('noopener noreferrer')
  })

  it('shows the import method and emits trimmed auth.json content', async () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'openai',
        showCookieOption: false,
        showRefreshTokenOption: true,
        showAgentIdentityOption: true
      },
      global: {
        plugins: [createPinia()],
        stubs: { Icon: true }
      }
    })

    await wrapper.get('input[value="agent_identity"]').setValue()
    await wrapper.get('textarea').setValue('  {"auth_mode":"agentIdentity"}  ')
    await wrapper.get('button.btn-primary').trigger('click')

    expect(wrapper.emitted('import-agent-identity')).toEqual([
      ['{"auth_mode":"agentIdentity"}']
    ])
  })
})

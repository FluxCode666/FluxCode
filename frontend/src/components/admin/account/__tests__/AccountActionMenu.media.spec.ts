import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { Account } from '@/types'
import AccountActionMenu from '../AccountActionMenu.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const mediaAccount = {
  id: 1,
  name: 'media',
  platform: 'media',
  type: 'apikey',
  credentials: {},
  extra: {},
  concurrency: 1,
  priority: 1,
  status: 'active',
} as Account

describe('AccountActionMenu media isolation', () => {
  it('媒体测试协议未实现前隐藏文本连接测试与定时测试', () => {
    const wrapper = mount(AccountActionMenu, {
      props: {
        show: true,
        account: mediaAccount,
        position: { top: 10, left: 10 },
      },
      global: {
        stubs: { Teleport: true, Icon: true },
      },
    })

    expect(wrapper.text()).not.toContain('admin.accounts.testConnection')
    expect(wrapper.text()).not.toContain('admin.scheduledTests.schedule')
    expect(wrapper.text()).toContain('admin.accounts.viewStats')
  })
})

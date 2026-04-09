import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import PoolMonitorDashboardView from '../PoolMonitorDashboardView.vue'

const {
  getSummary,
  showError
} = vi.hoisted(() => ({
  getSummary: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      getSummary
    }
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('admin PoolMonitorDashboardView', () => {
  beforeEach(() => {
    getSummary.mockReset()
    showError.mockReset()

    getSummary.mockResolvedValue({
      overall: {
        all: 10,
        active: 8,
        inactive: 1,
        expired: 1,
        error: 2,
        banned: 1,
        available: 4,
        manual_unschedulable: 1,
        temp_unschedulable: 1,
        rate_limited: 2,
        overloaded: 1
      },
      platforms: [
        {
          platform: 'openai',
          counts: {
            all: 7,
            active: 6,
            inactive: 1,
            expired: 0,
            error: 1,
            banned: 1,
            available: 3,
            manual_unschedulable: 1,
            temp_unschedulable: 0,
            rate_limited: 1,
            overloaded: 1
          }
        }
      ]
    })
  })

  it('loads summary on mount and switches metrics by platform tab', async () => {
    const wrapper = mount(PoolMonitorDashboardView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          RouterLink: { template: '<a><slot /></a>' },
          StatCard: {
            props: ['title', 'value'],
            template: '<div>{{ title }}:{{ value }}</div>'
          }
        }
      }
    })

    await flushPromises()

    expect(getSummary).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('common.all:10')

    const openaiTab = wrapper.findAll('button.rounded-full')
      .find((node) => node.text() === 'openai')

    expect(openaiTab).toBeTruthy()
    await openaiTab!.trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('common.all:7')
  })
})

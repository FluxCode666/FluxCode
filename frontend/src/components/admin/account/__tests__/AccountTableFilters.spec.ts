import { describe, expect, it, vi } from 'vitest'
import { mount, shallowMount } from '@vue/test-utils'

import AccountTableFilters from '../AccountTableFilters.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

describe('AccountTableFilters', () => {
  it('places filter settings next to search before visible filter controls', () => {
    localStorage.removeItem('account-hidden-filters')
    const wrapper = mount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: {
          platform: '',
          type: '',
          status: '',
          privacy_mode: '',
          schedulable_status: '',
          group: '',
          model: '',
          proxy_ids: [],
          created_start_date: '',
          created_end_date: ''
        },
        groups: [],
        proxies: []
      },
      global: {
        stubs: {
          SearchInput: { template: '<input data-test="account-search" />' },
          Select: { template: '<div data-test="account-filter-select" />' },
          ProxyMultiSelectFilter: { template: '<div data-test="account-proxy-filter" />' },
          DateRangePicker: { template: '<div data-test="account-date-filter" />' },
          Icon: true
        }
      }
    })

    const search = wrapper.get('[data-test="account-search"]')
    const filterSettingsButton = wrapper.findAll('button').find((button) => button.text() === 'admin.accounts.filterSettings')
    const firstVisibleFilter = wrapper.get('[data-test="account-filter-select"]')

    expect(filterSettingsButton).toBeTruthy()
    expect(Boolean(search.element.compareDocumentPosition(filterSettingsButton!.element) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(true)
    expect(Boolean(filterSettingsButton!.element.compareDocumentPosition(firstVisibleFilter.element) & Node.DOCUMENT_POSITION_FOLLOWING)).toBe(true)
  })

  it('分组和模型筛选均启用搜索，并允许查询自定义模型', () => {
    localStorage.removeItem('account-hidden-filters')
    const wrapper = shallowMount(AccountTableFilters, {
      props: {
        searchQuery: '',
        filters: {
          platform: '',
          type: '',
          status: '',
          privacy_mode: '',
          schedulable_status: '',
          group: '',
          model: '',
          proxy_ids: [],
          created_start_date: '',
          created_end_date: ''
        },
        groups: [{ id: 7, name: 'OpenAI 主分组' } as never],
        proxies: []
      }
    })

    const groupSelect = wrapper.findComponent('[data-test="account-group-filter"]')
    const modelSelect = wrapper.findComponent('[data-test="account-model-filter"]')

    expect(groupSelect.exists()).toBe(true)
    expect(groupSelect.props('searchable')).toBe(true)
    expect(modelSelect.exists()).toBe(true)
    expect(modelSelect.props('searchable')).toBe(true)
    expect(modelSelect.props('creatable')).toBe(true)
    expect(modelSelect.props('options')).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ value: '' }),
        expect.objectContaining({ value: 'gpt-5.6-sol' })
      ])
    )
  })
})

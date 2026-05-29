import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import UserMultiSearchSelect from '../UserMultiSearchSelect.vue'

const { listMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: {
      list: listMock,
    },
  },
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}))

describe('UserMultiSearchSelect', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listMock.mockReset()
    document.body.innerHTML = ''
  })

  afterEach(() => {
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('通过邮箱搜索选择用户并只输出用户 ID', async () => {
    listMock.mockResolvedValue({
      items: [{ id: 42, email: 'user@example.com', username: 'user' }],
    })
    const wrapper = mount(UserMultiSearchSelect, {
      props: { modelValue: [], placeholder: 'search' },
      attachTo: document.body,
    })

    await wrapper.get('input').trigger('focus')
    await wrapper.get('input').setValue('user@example.com')
    await vi.runAllTimersAsync()
    await flushPromises()
    const option = document.body.querySelector<HTMLElement>('[data-test="user-multi-search-option"]')
    expect(option).not.toBeNull()
    option?.click()
    await flushPromises()

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[42]])
  })

  it('移除已选择用户', async () => {
    const wrapper = mount(UserMultiSearchSelect, {
      props: {
        modelValue: [42],
        selectedUsers: [{ id: 42, email: 'user@example.com', username: 'user' }],
      },
      attachTo: document.body,
    })

    await wrapper.find('[data-test="user-multi-remove"]').trigger('click')

    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([[]])
  })
})

import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AnnouncementReadStatusDialog from '../AnnouncementReadStatusDialog.vue'

const { getReadStatus, showError } = vi.hoisted(() => ({
  getReadStatus: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    announcements: {
      getReadStatus,
    },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

vi.mock('@/composables/usePersistedPageSize', () => ({
  getPersistedPageSize: () => 20,
}))

const BaseDialogStub = {
  props: ['show', 'title', 'width'],
  emits: ['close'],
  template: '<div><slot /><slot name="footer" /></div>',
}

const SelectStub = {
  props: ['modelValue', 'options'],
  emits: ['update:modelValue', 'change'],
  methods: {
    handleChange(event: Event) {
      const value = (event.target as HTMLSelectElement).value
      this.$emit('update:modelValue', value)
      this.$emit('change', value)
    },
  },
  template: `
    <select data-test="read-status-filter" :value="modelValue" @change="handleChange">
      <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option>
    </select>
  `,
}

describe('AnnouncementReadStatusDialog', () => {
  beforeEach(() => {
    getReadStatus.mockReset()
    showError.mockReset()
    vi.useFakeTimers()
  })

  it('closes by aborting active requests and clearing debounced reloads', async () => {
    let activeSignal: AbortSignal | undefined
    getReadStatus.mockImplementation(async (...args: any[]) => {
      activeSignal = args[4]?.signal
      return new Promise(() => {})
    })

    const wrapper = mount(AnnouncementReadStatusDialog, {
      props: {
        show: false,
        announcementId: 1,
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          DataTable: true,
          Pagination: true,
          Icon: true,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getReadStatus).toHaveBeenCalledTimes(1)
    expect(activeSignal?.aborted).toBe(false)

    const setupState = (wrapper.vm as any).$?.setupState
    setupState.search = 'alice'
    setupState.handleSearch()

    setupState.handleClose()
    await flushPromises()

    expect(activeSignal?.aborted).toBe(true)
    expect(wrapper.emitted('close')).toHaveLength(1)

    vi.advanceTimersByTime(350)
    await flushPromises()

    expect(getReadStatus).toHaveBeenCalledTimes(1)
  })

  it('defaults to all read statuses and reloads when filtering read users', async () => {
    getReadStatus.mockResolvedValue({
      items: [],
      total: 0,
      pages: 0,
      page: 1,
      page_size: 20,
    })

    const wrapper = mount(AnnouncementReadStatusDialog, {
      props: {
        show: false,
        announcementId: 1,
      },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          DataTable: true,
          Pagination: true,
          Icon: true,
          Select: SelectStub,
        },
      },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(getReadStatus).toHaveBeenCalledTimes(1)
    expect(getReadStatus.mock.calls[0][3]).not.toHaveProperty('read_status')

    await wrapper.get('[data-test="read-status-filter"]').setValue('read')
    await flushPromises()

    expect(getReadStatus).toHaveBeenCalledTimes(2)
    expect(getReadStatus.mock.calls[1][1]).toBe(1)
    expect(getReadStatus.mock.calls[1][3]).toMatchObject({ read_status: 'read' })
  })
})

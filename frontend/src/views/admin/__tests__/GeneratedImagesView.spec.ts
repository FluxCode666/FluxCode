import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'

import GeneratedImagesView from '../GeneratedImagesView.vue'

const {
  listGeneratedImages,
  getContentBlob,
  deleteByDateRange,
  searchUsers,
  getAllGroups,
  showSuccess,
  showError,
  createObjectURL,
  revokeObjectURL
} = vi.hoisted(() => ({
  listGeneratedImages: vi.fn(),
  getContentBlob: vi.fn(),
  deleteByDateRange: vi.fn(),
  searchUsers: vi.fn(),
  getAllGroups: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  createObjectURL: vi.fn((blob: Blob) => `blob:image-${blob.size}`),
  revokeObjectURL: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    generatedImages: {
      list: listGeneratedImages,
      getContentBlob,
      deleteByDateRange
    },
    usage: {
      searchUsers
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showSuccess,
    showError
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key)
    })
  }
})

describe('admin GeneratedImagesView', () => {
  const refreshIconPath =
    'M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99'
  const selectStub = defineComponent({
    name: 'Select',
    inheritAttrs: false,
    props: {
      modelValue: {
        type: [String, Number, Boolean],
        default: ''
      },
      options: {
        type: Array,
        default: () => []
      }
    },
    emits: ['update:modelValue'],
    methods: {
      handleChange(event: Event) {
        this.$emit('update:modelValue', (event.target as HTMLSelectElement).value)
      }
    },
    template: `
      <select v-bind="$attrs" :value="modelValue" @change="handleChange">
        <option v-for="option in options" :key="String(option.value)" :value="option.value">{{ option.label }}</option>
      </select>
    `
  })

  const defaultStubs = {
    AppLayout: { template: '<div><slot /></div>' },
    Pagination: true,
    Select: selectStub
  }

  beforeEach(() => {
    ;(window as any).__APP_CONFIG__ = {
      table_default_page_size: 50,
      table_page_size_options: [25, 50, 100]
    }
    listGeneratedImages.mockReset()
    getContentBlob.mockReset()
    deleteByDateRange.mockReset()
    searchUsers.mockReset()
    getAllGroups.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    createObjectURL.mockClear()
    revokeObjectURL.mockClear()

    vi.stubGlobal('URL', {
      createObjectURL,
      revokeObjectURL
    })

    listGeneratedImages.mockResolvedValue({
      items: [
        {
          id: 7,
          provider: 'openai',
          user_id: 11,
          api_key_id: 22,
          account_id: 33,
          user_email: 'artist@example.com',
          api_key_name: 'Gallery Key',
          account_name: 'OpenAI Images',
          account_group_names: ['Images'],
          request_id: 'req_123',
          model: 'gpt-image-1',
          prompt: 'A quiet desk lamp',
          revised_prompt: 'A quiet desk lamp at night',
          response_format: 'url',
          source: 'openai_api_key',
          content_type: 'image/png',
          size_bytes: 2048,
          content_url: '/api/v1/admin/generated-images/7/content',
          created_at: '2026-06-27T12:00:00Z'
        }
      ],
      total: 1,
      page: 1,
      page_size: 50,
      pages: 1
    })
    getContentBlob.mockResolvedValue(new Blob(['image-bytes'], { type: 'image/png' }))
    deleteByDateRange.mockResolvedValue({ deleted_count: 2 })
    searchUsers.mockResolvedValue([{ id: 11, email: 'artist@example.com' }])
    getAllGroups.mockResolvedValue([{ id: 9, name: 'Images', platform: 'openai' }])
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    delete (window as any).__APP_CONFIG__
  })

  it('loads generated images, renders authorized blob thumbnails, and opens preview', async () => {
    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: defaultStubs
      }
    })

    await flushPromises()

    expect(listGeneratedImages).toHaveBeenCalledWith(
      {
        page: 1,
        page_size: 50
      },
      expect.objectContaining({
        signal: expect.any(AbortSignal)
      })
    )
    expect(getContentBlob).toHaveBeenCalledWith(
      7,
      expect.objectContaining({
        signal: expect.any(AbortSignal)
      })
    )

    const thumbnail = wrapper.get('[data-test="generated-image-thumb"]')
    expect(thumbnail.attributes('src')).toBe('blob:image-11')
    expect(wrapper.text()).toContain('openai')
    expect(wrapper.text()).toContain('gpt-image-1')
    expect(wrapper.text()).toContain('A quiet desk lamp')
    expect(wrapper.text()).toContain('artist@example.com')
    expect(wrapper.text()).toContain('Gallery Key')
    expect(wrapper.text()).toContain('OpenAI Images')
    expect(wrapper.text()).toContain('Images')
    expect(wrapper.text()).not.toContain('#11')
    expect(wrapper.text()).not.toContain('#22')
    expect(wrapper.text()).not.toContain('#33')

    await wrapper.get('[data-test="generated-image-card"]').trigger('click')

    expect(wrapper.get('[data-test="generated-image-preview"]').attributes('src')).toBe('blob:image-11')
    expect(wrapper.get('[data-test="generated-image-preview"]').classes()).toEqual(
      expect.arrayContaining(['h-full', 'w-full', 'object-contain'])
    )
    expect(wrapper.text()).toContain('admin.generatedImages.ownership')

    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:image-11')
  })

  it('filters by user email, channel group, and date range', async () => {
    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: defaultStubs
      }
    })

    await flushPromises()

    await wrapper.get('[data-test="generated-images-user-email-search"]').setValue('artist')
    await flushPromises()
    await wrapper.get('[data-test="generated-images-user-option-11"]').trigger('click')
    await wrapper.get('[data-test="generated-images-group-filter"]').setValue('9')
    await wrapper.get('[data-test="generated-images-start-date"]').setValue('2026-06-20')
    await wrapper.get('[data-test="generated-images-end-date"]').setValue('2026-06-27')
    await wrapper.get('[data-test="generated-images-apply-filters"]').trigger('click')
    await flushPromises()

    expect(searchUsers).toHaveBeenCalledWith('artist')
    expect(listGeneratedImages).toHaveBeenLastCalledWith(
      {
        page: 1,
        page_size: 50,
        user_email: 'artist@example.com',
        group_id: 9,
        start_at: '2026-06-20',
        end_at: '2026-06-27'
      },
      expect.objectContaining({
        signal: expect.any(AbortSignal)
      })
    )
  })

  it('aborts stale thumbnail requests when switching pages', async () => {
    let firstContentSignal: AbortSignal | undefined

    listGeneratedImages
      .mockResolvedValueOnce({
        items: [
          {
            id: 7,
            provider: 'openai',
            user_id: 11,
            api_key_id: 22,
            account_id: 33,
            user_email: 'artist@example.com',
            api_key_name: 'Gallery Key',
            account_name: 'OpenAI Images',
            account_group_names: ['Images'],
            request_id: 'req_123',
            model: 'gpt-image-1',
            prompt: 'first page',
            revised_prompt: '',
            response_format: 'url',
            source: 'openai_api_key',
            content_type: 'image/png',
            size_bytes: 2048,
            content_url: '/api/v1/admin/generated-images/7/content',
            created_at: '2026-06-27T12:00:00Z'
          }
        ],
        total: 2,
        page: 1,
        page_size: 50,
        pages: 2
      })
      .mockResolvedValueOnce({
        items: [
          {
            id: 8,
            provider: 'gemini',
            user_id: 11,
            api_key_id: 22,
            account_id: 33,
            user_email: 'artist@example.com',
            api_key_name: 'Gallery Key',
            account_name: 'OpenAI Images',
            account_group_names: ['Images'],
            request_id: 'req_456',
            model: 'gpt-image-1',
            prompt: 'second page',
            revised_prompt: '',
            response_format: 'url',
            source: 'openai_api_key',
            content_type: 'image/png',
            size_bytes: 4096,
            content_url: '/api/v1/admin/generated-images/8/content',
            created_at: '2026-06-27T13:00:00Z'
          }
        ],
        total: 2,
        page: 2,
        page_size: 50,
        pages: 2
      })

    getContentBlob.mockImplementation((id: number, options?: { signal?: AbortSignal }) => {
      if (id === 7) {
        firstContentSignal = options?.signal
        return new Promise<Blob>((_, reject) => {
          options?.signal?.addEventListener('abort', () => reject({ code: 'ERR_CANCELED' }))
        })
      }

      return Promise.resolve(new Blob(['fresh-image-bytes'], { type: 'image/png' }))
    })

    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Pagination: {
            template: '<button data-test="next-page" @click="$emit(\'update:page\', 2)">next</button>',
            emits: ['update:page', 'update:pageSize']
          }
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-test="next-page"]').trigger('click')
    await flushPromises()

    expect(firstContentSignal?.aborted).toBe(true)
    expect(wrapper.get('[data-test="generated-image-thumb"]').attributes('src')).toBe('blob:image-17')
    expect(wrapper.text()).toContain('gemini')
    expect(wrapper.text()).toContain('second page')
  })

  it('uses a refresh icon for the empty-state refresh action', async () => {
    listGeneratedImages.mockResolvedValueOnce({
      items: [],
      total: 0,
      page: 1,
      page_size: 50,
      pages: 0
    })

    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: defaultStubs
      }
    })

    await flushPromises()

    const emptyActionIcon = wrapper.get('.empty-state .btn svg path')
    expect(emptyActionIcon.attributes('d')).toBe(refreshIconPath)
  })

  it('clears generated image rows by selected date range after confirmation', async () => {
    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: {
          ...defaultStubs,
          ConfirmDialog: {
            props: ['show'],
            emits: ['confirm', 'cancel'],
            template: '<button v-if="show" data-test="confirm-cleanup" @click="$emit(\'confirm\')">confirm</button>'
          }
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-test="generated-images-start-date"]').setValue('2026-06-20')
    await wrapper.get('[data-test="generated-images-end-date"]').setValue('2026-06-27')
    await wrapper.get('[data-test="generated-images-open-cleanup"]').trigger('click')
    await wrapper.get('[data-test="confirm-cleanup"]').trigger('click')
    await flushPromises()

    expect(deleteByDateRange).toHaveBeenCalledWith({
      start_at: '2026-06-20',
      end_at: '2026-06-27'
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.generatedImages.cleanupSuccess:{"count":2}')
    expect(listGeneratedImages).toHaveBeenCalledTimes(2)
  })

  it('uses themed controls and configured pagination options', async () => {
    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Pagination: {
            props: {
              pageSizeOptions: {
                type: Array,
                default: () => []
              }
            },
            template: '<div data-test="page-size-options">{{ pageSizeOptions.join(",") }}</div>'
          }
        }
      }
    })

    await flushPromises()

    expect(wrapper.find('[data-test="generated-images-refresh"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('admin.generatedImages.refresh')
    expect(wrapper.find('select[data-test="generated-images-group-filter"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="generated-images-group-filter"] .select-trigger').exists()).toBe(true)
    expect(wrapper.get('[data-test="generated-images-apply-filters"]').text()).toBe('admin.generatedImages.query')
    expect(wrapper.get('[data-test="page-size-options"]').text()).toBe('25,50,100')
  })
})

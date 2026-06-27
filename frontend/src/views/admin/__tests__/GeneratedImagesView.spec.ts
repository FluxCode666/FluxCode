import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import GeneratedImagesView from '../GeneratedImagesView.vue'

const {
  listGeneratedImages,
  getContentBlob,
  searchUsers,
  getAllGroups,
  showError,
  createObjectURL,
  revokeObjectURL
} = vi.hoisted(() => ({
  listGeneratedImages: vi.fn(),
  getContentBlob: vi.fn(),
  searchUsers: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn(),
  createObjectURL: vi.fn((blob: Blob) => `blob:image-${blob.size}`),
  revokeObjectURL: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    generatedImages: {
      list: listGeneratedImages,
      getContentBlob
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

  beforeEach(() => {
    listGeneratedImages.mockReset()
    getContentBlob.mockReset()
    searchUsers.mockReset()
    getAllGroups.mockReset()
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
      page_size: 24,
      pages: 1
    })
    getContentBlob.mockResolvedValue(new Blob(['image-bytes'], { type: 'image/png' }))
    searchUsers.mockResolvedValue([{ id: 11, email: 'artist@example.com' }])
    getAllGroups.mockResolvedValue([{ id: 9, name: 'Images', platform: 'openai' }])
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('loads generated images, renders authorized blob thumbnails, and opens preview', async () => {
    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Pagination: true
        }
      }
    })

    await flushPromises()

    expect(listGeneratedImages).toHaveBeenCalledWith(
      {
        page: 1,
        page_size: 24
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
    expect(wrapper.text()).toContain('admin.generatedImages.ownership')

    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:image-11')
  })

  it('filters by user email, channel group, and date range', async () => {
    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Pagination: true
        }
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
        page_size: 24,
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
        page_size: 24,
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
        page_size: 24,
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
      page_size: 24,
      pages: 0
    })

    const wrapper = mount(GeneratedImagesView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          Pagination: true
        }
      }
    })

    await flushPromises()

    const emptyActionIcon = wrapper.get('.empty-state .btn svg path')
    expect(emptyActionIcon.attributes('d')).toBe(refreshIconPath)
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import GeneratedImagesView from '../GeneratedImagesView.vue'

const {
  listGeneratedImages,
  getContentBlob,
  showError,
  createObjectURL,
  revokeObjectURL
} = vi.hoisted(() => ({
  listGeneratedImages: vi.fn(),
  getContentBlob: vi.fn(),
  showError: vi.fn(),
  createObjectURL: vi.fn((blob: Blob) => `blob:image-${blob.size}`),
  revokeObjectURL: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    generatedImages: {
      list: listGeneratedImages,
      getContentBlob
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
  beforeEach(() => {
    listGeneratedImages.mockReset()
    getContentBlob.mockReset()
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

    await wrapper.get('[data-test="generated-image-card"]').trigger('click')

    expect(wrapper.get('[data-test="generated-image-preview"]').attributes('src')).toBe('blob:image-11')

    wrapper.unmount()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:image-11')
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
})

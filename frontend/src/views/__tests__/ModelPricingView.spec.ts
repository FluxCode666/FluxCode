import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelPricingView from '../ModelPricingView.vue'

const { listModels, getModel, fetchPublicSettings } = vi.hoisted(() => ({
  listModels: vi.fn(),
  getModel: vi.fn(),
  fetchPublicSettings: vi.fn()
}))

vi.mock('@/api/modelPricing', () => ({
  modelPricingAPI: { listModels, getModel },
  default: { listModels, getModel }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: 'FluxCode',
    siteLogo: '',
    fetchPublicSettings
  }),
  useAuthStore: () => ({ isAuthenticated: false, isAdmin: false })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, fallback?: string) => fallback || key
    })
  }
})

describe('ModelPricingView', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    listModels.mockReset()
    getModel.mockReset()
    fetchPublicSettings.mockReset()

    listModels.mockResolvedValue([
      {
        id: 'claude-sonnet-4',
        display_name: 'claude-sonnet-4',
        platform: 'anthropic',
        capabilities: ['chat'],
        supported_group_count: 2,
        official_price: {
          input_price: 0.000003,
          output_price: 0.000015,
          cache_write_price: 0.00000375,
          cache_read_price: 0.0000003,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        }
      }
    ])

    getModel.mockResolvedValue({
      id: 'claude-sonnet-4',
      display_name: 'claude-sonnet-4',
      platform: 'anthropic',
      capabilities: ['chat'],
      supported_group_count: 2,
      official_price: {
        input_price: 0.000003,
        output_price: 0.000015,
        cache_write_price: 0.00000375,
        cache_read_price: 0.0000003,
        image_output_price: 0,
        per_request_price: 0,
        intervals: []
      },
      groups: [
        {
          group_id: 1,
          group_name: '基础组',
          rate_multiplier: 2,
          billing_mode: 'token',
          price: {
            input_price: 0.000006,
            output_price: 0.00003,
            cache_write_price: 0.0000075,
            cache_read_price: 0.0000006,
            image_output_price: 0,
            per_request_price: 0,
            intervals: []
          },
          multipliers: {
            input_price: 2,
            output_price: 2,
            cache_write_price: 2,
            cache_read_price: 2,
            image_output_price: 0,
            per_request_price: 0
          }
        }
      ]
    })
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('renders model cards and loads model detail after click', async () => {
    const wrapper = mount(ModelPricingView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('anthropic')
    expect(wrapper.text()).toContain('2')

    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenCalledWith('claude-sonnet-4', expect.any(Object))
    expect(wrapper.text()).toContain('基础组')
    expect(wrapper.text()).toContain('2.00x')
  })

  it('debounces search for 300ms', async () => {
    const wrapper = mount(ModelPricingView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-testid="model-pricing-search"]').setValue('claude')
    expect(listModels).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(299)
    expect(listModels).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(1)
    await flushPromises()

    expect(listModels).toHaveBeenCalledWith({ q: 'claude', platform: '', capability: '' }, expect.any(Object))
  })

  it('shows query error and retries', async () => {
    listModels.mockRejectedValueOnce(new Error('network'))

    const wrapper = mount(ModelPricingView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('查询异常')
    listModels.mockResolvedValueOnce([])

    await wrapper.get('[data-testid="model-pricing-retry"]').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenCalledTimes(2)
  })

  it('ignores canceled list requests without showing query error', async () => {
    let firstResolve: ((value: unknown) => void) | null = null

    listModels.mockReset()
    listModels
      .mockImplementationOnce((_params, options?: { signal?: AbortSignal }) => new Promise((resolve, reject) => {
        firstResolve = resolve
        options?.signal?.addEventListener('abort', () => {
          const canceledError = Object.assign(new Error('canceled'), { code: 'ERR_CANCELED', name: 'CanceledError' })
          reject(canceledError)
        })
      }))
      .mockImplementationOnce(() => new Promise((resolve) => {
        resolve([
          {
            id: 'claude-sonnet-4',
            display_name: 'claude-sonnet-4',
            platform: 'anthropic',
            capabilities: ['chat'],
            supported_group_count: 2,
            official_price: {
              input_price: 0.000003,
              output_price: 0.000015,
              cache_write_price: 0.00000375,
              cache_read_price: 0.0000003,
              image_output_price: 0,
              per_request_price: 0,
              intervals: []
            }
          }
        ])
      }))

    const wrapper = mount(ModelPricingView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-testid="model-pricing-search"]').setValue('claude')
    await vi.advanceTimersByTimeAsync(300)
    await flushPromises()

    expect(listModels).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).not.toContain('查询异常')

    firstResolve?.([])
    await flushPromises()

    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).not.toContain('查询异常')
  })

  it('shows detail query error and retries successfully', async () => {
    getModel
      .mockRejectedValueOnce(new Error('network'))
      .mockResolvedValueOnce({
        id: 'claude-sonnet-4',
        display_name: 'claude-sonnet-4',
        platform: 'anthropic',
        capabilities: ['chat'],
        supported_group_count: 2,
        official_price: {
          input_price: 0.000003,
          output_price: 0.000015,
          cache_write_price: 0.00000375,
          cache_read_price: 0.0000003,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        },
        groups: [
          {
            group_id: 1,
            group_name: '基础组',
            rate_multiplier: 2,
            billing_mode: 'token',
            price: {
              input_price: 0.000006,
              output_price: 0.00003,
              cache_write_price: 0.0000075,
              cache_read_price: 0.0000006,
              image_output_price: 0,
              per_request_price: 0,
              intervals: []
            },
            multipliers: {
              input_price: 2,
              output_price: 2,
              cache_write_price: 2,
              cache_read_price: 2,
              image_output_price: 0,
              per_request_price: 0
            }
          }
        ]
      })

    const wrapper = mount(ModelPricingView, {
      global: {
        stubs: {
          PublicHeader: true
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('查询异常')
    expect(wrapper.get('[data-testid="model-pricing-detail-retry"]').exists()).toBe(true)

    await wrapper.get('[data-testid="model-pricing-detail-retry"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('基础组')
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelPricingPage from '../ModelPricingPage.vue'

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

const BaseDialogStub = {
  props: ['show', 'title'],
  template: `
    <section v-if="show" data-testid="base-dialog">
      <h3>{{ title }}</h3>
      <slot />
      <slot name="footer" />
    </section>
  `
}

describe('ModelPricingPage', () => {
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
        platforms: ['anthropic'],
        capabilities: ['streaming', 'vision', 'image_generation', 'video_generation', 'audio_input', 'audio_output'],
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
        lowest_group_price: {
          input_price: 0.000006,
          output_price: 0.00003,
          cache_write_price: 0.0000075,
          cache_read_price: 0.0000006,
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
      platforms: ['anthropic'],
      capabilities: ['streaming', 'vision', 'image_generation', 'video_generation', 'audio_input', 'audio_output'],
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
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()

    expect(wrapper.text()).toContain('claude-sonnet-4')
    expect(wrapper.text()).toContain('anthropic')
    expect(wrapper.text()).toContain('视觉理解')
    expect(wrapper.text()).toContain('图片生成')
    expect(wrapper.text()).toContain('视频生成')
    expect(wrapper.text()).toContain('音频输入')
    expect(wrapper.text()).toContain('音频输出')
    expect(wrapper.text()).toContain('2')
    expect(wrapper.find('[data-testid="base-dialog"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="model-card-claude-sonnet-4"]').text()).toContain('$6.00000/M')
    expect(wrapper.get('[data-testid="model-card-claude-sonnet-4"]').text()).toContain('$30.0000/M')

    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenCalledWith('claude-sonnet-4', expect.any(Object))
    expect(wrapper.get('[data-testid="base-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="model-pricing-detail-modal"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('基础组')
    expect(wrapper.text()).toContain('2.00x')
    expect(wrapper.text()).not.toContain('$6.00000/M · 2.00x')
  })

  it('debounces search for 300ms', async () => {
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
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

    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
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
            platforms: ['anthropic'],
            capabilities: ['streaming'],
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
            lowest_group_price: {
              input_price: 0.000006,
              output_price: 0.00003,
              cache_write_price: 0.0000075,
              cache_read_price: 0.0000006,
              image_output_price: 0,
              per_request_price: 0,
              intervals: []
            }
          }
        ])
      }))

    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
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
        platforms: ['anthropic'],
        capabilities: ['streaming'],
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

    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()
    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('查询异常')
    expect(wrapper.get('[data-testid="base-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="model-pricing-detail-retry"]').exists()).toBe(true)

    await wrapper.get('[data-testid="model-pricing-detail-retry"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('基础组')
  })

  it('ignores stale detail responses that resolve after a newer selection', async () => {
    let firstResolve: ((value: unknown) => void) | null = null
    let secondResolve: ((value: unknown) => void) | null = null

    listModels.mockResolvedValue([
      {
        id: 'claude-sonnet-4',
        display_name: 'claude-sonnet-4',
        platform: 'anthropic',
        platforms: ['anthropic'],
        capabilities: ['streaming'],
        supported_group_count: 1,
        official_price: {
          input_price: 0.000003,
          output_price: 0.000015,
          cache_write_price: 0.00000375,
          cache_read_price: 0.0000003,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        },
        lowest_group_price: {
          input_price: 0.000006,
          output_price: 0.00003,
          cache_write_price: 0.0000075,
          cache_read_price: 0.0000006,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        }
      },
      {
        id: 'gpt-4.1',
        display_name: 'gpt-4.1',
        platform: 'openai',
        platforms: ['openai'],
        capabilities: ['streaming'],
        supported_group_count: 1,
        official_price: {
          input_price: 0.000002,
          output_price: 0.000008,
          cache_write_price: 0,
          cache_read_price: 0,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        },
        lowest_group_price: {
          input_price: 0.000002,
          output_price: 0.000008,
          cache_write_price: 0,
          cache_read_price: 0,
          image_output_price: 0,
          per_request_price: 0,
          intervals: []
        }
      }
    ])

    getModel.mockReset()
    getModel
      .mockImplementationOnce(() => new Promise((resolve) => {
        firstResolve = resolve
      }))
      .mockImplementationOnce(() => new Promise((resolve) => {
        secondResolve = resolve
      }))

    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await wrapper.get('[data-testid="model-card-gpt-4.1"]').trigger('click')

    secondResolve?.({
      id: 'gpt-4.1',
      display_name: 'gpt-4.1',
      platform: 'openai',
      platforms: ['openai'],
      capabilities: ['streaming'],
      supported_group_count: 1,
      official_price: {
        input_price: 0.000002,
        output_price: 0.000008,
        cache_write_price: 0,
        cache_read_price: 0,
        image_output_price: 0,
        per_request_price: 0,
        intervals: []
      },
      groups: [
        {
          group_id: 2,
          group_name: 'OpenAI 组',
          rate_multiplier: 1,
          billing_mode: 'token',
          price: {
            input_price: 0.000002,
            output_price: 0.000008,
            cache_write_price: 0,
            cache_read_price: 0,
            image_output_price: 0,
            per_request_price: 0,
            intervals: []
          },
          multipliers: {
            input_price: 1,
            output_price: 1,
            cache_write_price: 0,
            cache_read_price: 0,
            image_output_price: 0,
            per_request_price: 0
          }
        }
      ]
    })
    await flushPromises()

    firstResolve?.({
      id: 'claude-sonnet-4',
      display_name: 'claude-sonnet-4',
      platform: 'anthropic',
      platforms: ['anthropic'],
      capabilities: ['streaming'],
      supported_group_count: 1,
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
    await flushPromises()

    expect(wrapper.text()).toContain('OpenAI 组')
    expect(wrapper.text()).not.toContain('基础组')
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { config, flushPromises, mount, RouterLinkStub } from '@vue/test-utils'

import ModelPricingPage from '../ModelPricingPage.vue'

const { listModels, getModel, listGroups, fetchPublicSettings, copyToClipboard } = vi.hoisted(() => ({
  listModels: vi.fn(),
  getModel: vi.fn(),
  listGroups: vi.fn(),
  fetchPublicSettings: vi.fn(),
  copyToClipboard: vi.fn()
}))

vi.mock('@/api/modelPricing', () => ({
  modelPricingAPI: { listModels, getModel, listGroups },
  default: { listModels, getModel, listGroups }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    siteName: 'FluxCode',
    siteLogo: '',
    fetchPublicSettings
  }),
  useAuthStore: () => ({ isAuthenticated: false, isAdmin: false })
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copyToClipboard
  })
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

const ModelPerformanceTrendChartStub = {
  props: ['metric', 'points', 'range'],
  template: '<div data-testid="model-performance-trend-stub" />'
}

describe('ModelPricingPage', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    config.global.stubs = {
      ...config.global.stubs,
      ModelPerformanceTrendChart: ModelPerformanceTrendChartStub,
      RouterLink: RouterLinkStub
    }
    listModels.mockReset()
    getModel.mockReset()
    listGroups.mockReset()
    fetchPublicSettings.mockReset()
    copyToClipboard.mockReset()
    copyToClipboard.mockResolvedValue(true)

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
        },
        performance: {
          tps: 42.7,
          availability: 99.6,
          average_first_token_ms: 125.5,
          average_request_time_ms: 830.2
        }
      }
    ])

    listGroups.mockResolvedValue([
      { id: 1, name: '基础组', platform: 'anthropic' }
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
      lowest_group_price: {
        input_price: 0.000006,
        output_price: 0.00003,
        cache_write_price: 0.0000075,
        cache_read_price: 0.0000006,
        image_output_price: 0,
        per_request_price: 0,
        intervals: []
      },
      performance: {
        tps: 42.7,
        availability: 99.6,
        average_first_token_ms: 125.5,
        average_request_time_ms: 830.2
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
          },
          performance: {
            tps: 33.3,
            availability: 98.4,
            average_first_token_ms: 150,
            average_request_time_ms: 910
          }
        }
      ],
      performance_trend: [
        {
          bucket_start: '2026-07-19T00:00:00Z',
          average_first_token_ms: 125.5,
          availability: 99.6
        }
      ]
    })
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
    vi.clearAllMocks()
    delete config.global.stubs.ModelPerformanceTrendChart
    delete config.global.stubs.RouterLink
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
    expect(wrapper.find('[data-testid="base-dialog"]').exists()).toBe(false)
    const modelCard = wrapper.get('[data-testid="model-card-claude-sonnet-4"]')
    expect(modelCard.text()).not.toContain('$6.00000/M · 2.00x')
    expect(modelCard.text()).toContain('$6.00000/M')
    expect(modelCard.text()).toContain('$30.0000/M')

    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenCalledWith('claude-sonnet-4', '24h', expect.any(Object))
    expect(wrapper.get('[data-testid="base-dialog"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="model-pricing-detail-modal"]').exists()).toBe(true)
    expect(wrapper.text()).toContain('基础组')
    expect(wrapper.text()).toContain('2.00x')
    expect(wrapper.text()).not.toContain('$6.00000/M · 2.00x')
  })

  it('以胶囊平铺形式提供文本嵌入能力筛选项', async () => {
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()

    const embeddingOption = wrapper.get('[data-testid="model-pricing-capability-option-embedding"]')
    expect(embeddingOption.text()).toContain('文本嵌入')
    expect(embeddingOption.classes()).toContain('rounded-full')
  })

  it('在左侧展开筛选枚举并支持直接切换', async () => {
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()

    const filters = wrapper.get('[data-testid="model-pricing-filters"]')
    expect(filters.text()).toContain('平台')
    expect(filters.text()).toContain('能力')
    expect(filters.text()).toContain('分组')
    expect(filters.text()).toContain('模型')
    expect(filters.get('[data-testid="model-pricing-capability-option-embedding"]').text()).toContain('文本嵌入')
    expect(filters.get('[data-testid="model-pricing-platform-option-anthropic"]').classes()).toContain('rounded-full')

    await filters.get('[data-testid="model-pricing-platform-option-anthropic"]').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenLastCalledWith(
      { q: '', platform: 'anthropic', capability: '', range: '24h' },
      expect.any(Object)
    )

    await filters.get('[data-testid="model-pricing-group-option-1"]').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenLastCalledWith(
      { q: '', platform: 'anthropic', capability: '', group_id: 1, range: '24h' },
      expect.any(Object)
    )
    expect(filters.get('[data-testid="model-pricing-group-option-1"]').attributes('aria-pressed')).toBe('true')

    await filters.get('[data-testid="model-pricing-reset-filters"]').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenLastCalledWith(
      { q: '', platform: '', capability: '', range: '24h' },
      expect.any(Object)
    )

    await filters.get('[data-testid="model-pricing-model-option-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenLastCalledWith('claude-sonnet-4', '24h', expect.any(Object))
    expect(wrapper.get('[data-testid="model-pricing-detail-modal"]').exists()).toBe(true)
  })

  it('copies model id from list cards and detail modal', async () => {
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()

    await wrapper.get('[data-testid="model-copy-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenCalledWith('claude-sonnet-4', '模型 ID 已复制')
    expect(getModel).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="base-dialog"]').exists()).toBe(false)

    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    await wrapper.get('[data-testid="model-detail-copy-id"]').trigger('click')
    await flushPromises()

    expect(copyToClipboard).toHaveBeenLastCalledWith('claude-sonnet-4', '模型 ID 已复制')
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

    expect(listModels).toHaveBeenCalledWith({ q: 'claude', platform: '', capability: '', range: '24h' }, expect.any(Object))
  })

  it('loads pay-as-you-go groups and filters models by selected group', async () => {
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub
        }
      }
    })

    await flushPromises()

    expect(listGroups).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('全部分组')

    await wrapper.get('[data-testid="model-pricing-group-option-1"]').trigger('click')
    await flushPromises()

    expect(listModels).toHaveBeenLastCalledWith(
      { q: '', platform: '', capability: '', group_id: 1, range: '24h' },
      expect.any(Object)
    )
  })

  it('keeps cards at 24 hours and loads only the performance detail when its range changes', async () => {
    const wrapper = mount(ModelPricingPage, {
      global: {
        stubs: {
          PublicHeader: true,
          BaseDialog: BaseDialogStub,
          ModelPerformanceTrendChart: {
            props: ['metric', 'points', 'range'],
            template: '<div :data-testid="`trend-${metric}`">{{ range }} {{ points.length }}</div>'
          }
        }
      }
    })

    await flushPromises()

    const card = wrapper.get('[data-testid="model-card-claude-sonnet-4"]')
    expect(card.get('[data-testid="model-card-latency-claude-sonnet-4"]').text()).toBe('125.5 ms')
    expect(card.get('[data-testid="model-card-tps-claude-sonnet-4"]').text()).toBe('42.7 TPS')
    expect(card.get('[data-testid="model-card-availability-claude-sonnet-4"]').text()).toBe('99.6%')
    expect(wrapper.find('[data-testid="model-pricing-range-24h"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="model-pricing-range-7d"]').exists()).toBe(false)

    const listRequestCount = listModels.mock.calls.length
    await wrapper.get('[data-testid="model-card-claude-sonnet-4"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="model-detail-tab-price"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="model-detail-price-panel"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="model-detail-performance-panel"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="model-detail-range-24h"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="trend-average_first_token_ms"]').exists()).toBe(false)

    await wrapper.get('[data-testid="model-detail-tab-performance"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="model-detail-tab-performance"]').attributes('aria-selected')).toBe('true')
    expect(wrapper.get('[data-testid="model-detail-overall-performance"]').text()).toContain('42.7 TPS')
    expect(wrapper.get('[data-testid="model-detail-group-performance-1"]').text()).toContain('33.3 TPS')
    expect(wrapper.get('[data-testid="trend-average_first_token_ms"]').text()).toBe('24h 1')
    expect(wrapper.get('[data-testid="trend-availability"]').text()).toBe('24h 1')

    await wrapper.get('[data-testid="model-detail-range-7d"]').trigger('click')
    await flushPromises()

    expect(getModel).toHaveBeenLastCalledWith('claude-sonnet-4', '7d', expect.any(Object))
    expect(getModel).toHaveBeenCalledTimes(2)
    expect(listModels).toHaveBeenCalledTimes(listRequestCount)
    expect(wrapper.get('[data-testid="trend-average_first_token_ms"]').text()).toBe('7d 1')
    expect(wrapper.get('[data-testid="trend-availability"]').text()).toBe('7d 1')
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

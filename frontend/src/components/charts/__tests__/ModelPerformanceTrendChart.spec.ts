import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import ModelPerformanceTrendChart from '../ModelPerformanceTrendChart.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (_key: string, fallback?: string) => fallback || _key
    })
  }
})

vi.mock('vue-chartjs', () => ({
  Line: {
    props: ['data', 'options'],
    template: '<div data-testid="performance-trend-chart">{{ JSON.stringify(data) }}</div>'
  }
}))

describe('ModelPerformanceTrendChart', () => {
  const points = [
    {
      bucket_start: '2026-07-19T00:00:00Z',
      average_first_token_ms: 125.25,
      availability: 99.5
    },
    {
      bucket_start: '2026-07-19T01:00:00Z',
      average_first_token_ms: null,
      availability: null
    },
    {
      bucket_start: '2026-07-19T02:00:00Z',
      average_first_token_ms: 80,
      availability: 100
    }
  ]

  it('renders hourly first-token latency points for the selected range without filling empty hours', () => {
    const wrapper = mount(ModelPerformanceTrendChart, {
      props: {
        points,
        metric: 'average_first_token_ms',
        range: '24h'
      }
    })

    const chartData = JSON.parse(wrapper.get('[data-testid="performance-trend-chart"]').text())
    expect(chartData.labels).toHaveLength(3)
    expect(chartData.labels[0]).toMatch(/07-19/)
    expect(chartData.datasets[0].data).toEqual([125.25, null, 80])
    expect(chartData.datasets[0].spanGaps).toBe(false)

    const options = (wrapper.vm as any).$?.setupState.options
    expect(options.plugins.tooltip.callbacks.label({ dataset: { label: '首字时长' }, raw: 125.25 })).toBe('首字时长: 125.3 ms')
  })

  it('renders availability as a percentage and keeps seven-day hourly labels distinct by date', () => {
    const wrapper = mount(ModelPerformanceTrendChart, {
      props: {
        points: [
          ...points,
          {
            bucket_start: '2026-07-20T00:00:00Z',
            average_first_token_ms: 90,
            availability: 98
          }
        ],
        metric: 'availability',
        range: '7d'
      }
    })

    const chartData = JSON.parse(wrapper.get('[data-testid="performance-trend-chart"]').text())
    expect(chartData.labels[0]).toMatch(/07-19/)
    expect(chartData.labels[3]).toMatch(/07-20/)
    expect(chartData.datasets[0].data).toEqual([99.5, null, 100, 98])

    const options = (wrapper.vm as any).$?.setupState.options
    expect(options.plugins.tooltip.callbacks.label({ dataset: { label: '可用率' }, raw: 99.5 })).toBe('可用率: 99.5%')
  })

  it('shows the empty state instead of drawing a zero-value trend when every hour has no sample', () => {
    const wrapper = mount(ModelPerformanceTrendChart, {
      props: {
        points: points.map((point) => ({ ...point, average_first_token_ms: null, availability: null })),
        metric: 'availability',
        range: '24h'
      },
      global: {
        stubs: {
          EmptyState: {
            props: ['title', 'description'],
            template: '<div data-testid="performance-trend-empty">{{ title }} {{ description }}</div>'
          }
        }
      }
    })

    expect(wrapper.find('[data-testid="performance-trend-chart"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="performance-trend-empty"]').text()).toContain('暂无数据')
  })
})

import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import DashboardFireworks from '../DashboardFireworks.vue'

describe('DashboardFireworks', () => {
  it('renders matched side emitters that spray confetti inward', () => {
    const wrapper = mount(DashboardFireworks)

    const leftConfetti = wrapper.findAll('.dashboard-fireworks__confetti--left')
    const rightConfetti = wrapper.findAll('.dashboard-fireworks__confetti--right')

    expect(wrapper.find('.dashboard-fireworks__emitter--left').exists()).toBe(true)
    expect(wrapper.find('.dashboard-fireworks__emitter--right').exists()).toBe(true)
    expect(leftConfetti.length).toBeGreaterThan(20)
    expect(leftConfetti).toHaveLength(rightConfetti.length)
    expect(leftConfetti[0].attributes('style')).toContain('--travel-x:')
    expect(rightConfetti[0].attributes('style')).toContain('--travel-x: -')
  })
})

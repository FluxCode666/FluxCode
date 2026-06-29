import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import DashboardFireworks from '../DashboardFireworks.vue'

describe('DashboardFireworks', () => {
  it('renders launcher-free physics confetti from both sides', () => {
    const wrapper = mount(DashboardFireworks)

    const leftConfetti = wrapper.findAll('.dashboard-fireworks__piece--left')
    const rightConfetti = wrapper.findAll('.dashboard-fireworks__piece--right')

    expect(wrapper.find('.dashboard-fireworks__emitter').exists()).toBe(false)
    expect(wrapper.find('.dashboard-fireworks__flash').exists()).toBe(false)
    expect(leftConfetti.length).toBeGreaterThan(60)
    expect(leftConfetti).toHaveLength(rightConfetti.length)
    expect(leftConfetti[0].attributes('style')).toContain('--p05-x:')
    expect(leftConfetti[0].attributes('style')).toContain('--p100-y:')
    expect(rightConfetti[0].attributes('style')).toContain('--p05-x: -')
  })

  it('keeps the projectile formula in the component source', async () => {
    const source = await import('../DashboardFireworks.vue?raw')

    expect(source.default).toContain('function projectilePoint')
    expect(source.default).toContain('const launchAngle = randRange(random, 56, 66)')
    expect(source.default).toContain('const speed = randRange(random, 760, 1080)')
    expect(source.default).toContain('0.5 * gravity * time * time')
    expect(source.default).not.toContain('dashboard-fireworks__emitter')
  })
})

import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { MediaGenerationSettings } from '@/api/admin/settings'
import MediaGenerationSettingsCard from '../MediaGenerationSettingsCard.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const baseSettings: MediaGenerationSettings = {
  media_sync_wait_timeout_seconds: 240,
  media_sync_timeout_fallback_async_enabled: false,
  media_sync_timeout_billing_policy: 'penalty',
  media_sync_timeout_penalty_ratio: 0.8,
  media_video_storage_mode: 'hybrid',
  media_video_proxy_fallback_enabled: true,
}

describe('MediaGenerationSettingsCard', () => {
  it('提示 0 秒不保证自动转异步，并发出数值 0', async () => {
    const wrapper = mount(MediaGenerationSettingsCard, {
      props: { modelValue: { ...baseSettings } },
    })
    await wrapper.get('[data-test="media-sync-timeout"]').setValue('0')
    const update = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as MediaGenerationSettings
    await wrapper.setProps({ modelValue: update })

    expect(wrapper.text()).toContain('admin.settings.mediaGeneration.timeoutDisabledWarning')
    expect(update).toMatchObject({
      media_sync_wait_timeout_seconds: 0,
      media_sync_timeout_fallback_async_enabled: false,
    })
  })

  it('全额退款策略下隐藏扣费比例，切回惩罚时保留原比例', async () => {
    const wrapper = mount(MediaGenerationSettingsCard, {
      props: { modelValue: { ...baseSettings } },
    })
    await wrapper.get('[data-test="media-timeout-billing-policy"]').setValue('refund')
    const refundUpdate = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as MediaGenerationSettings
    await wrapper.setProps({ modelValue: refundUpdate })
    expect(wrapper.find('[data-test="media-timeout-penalty-ratio"]').exists()).toBe(false)

    await wrapper.get('[data-test="media-timeout-billing-policy"]').setValue('penalty')
    const penaltyUpdate = wrapper.emitted('update:modelValue')?.at(-1)?.[0] as MediaGenerationSettings
    await wrapper.setProps({ modelValue: penaltyUpdate })
    expect(wrapper.get<HTMLInputElement>('[data-test="media-timeout-penalty-ratio"]').element.value).toBe('80')
  })

  it('在 UI 与 API 模型之间转换并钳制扣费比例', async () => {
    const wrapper = mount(MediaGenerationSettingsCard, {
      props: { modelValue: { ...baseSettings } },
    })

    expect(wrapper.get<HTMLInputElement>('[data-test="media-timeout-penalty-ratio"]').element.value).toBe('80')
    await wrapper.get('[data-test="media-timeout-penalty-ratio"]').setValue('125')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({
      media_sync_timeout_penalty_ratio: 1,
    })
    await wrapper.get('[data-test="media-timeout-penalty-ratio"]').setValue('-25')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({
      media_sync_timeout_penalty_ratio: 0,
    })
  })

  it('独立更新异步 fallback 与视频代理 fallback', async () => {
    const wrapper = mount(MediaGenerationSettingsCard, {
      props: { modelValue: { ...baseSettings } },
    })

    await wrapper.get('[data-test="media-fallback-async"]').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({
      media_sync_wait_timeout_seconds: 240,
      media_sync_timeout_fallback_async_enabled: true,
      media_video_proxy_fallback_enabled: true,
    })
    await wrapper.get('[data-test="media-proxy-fallback"]').setValue(false)
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({
      media_sync_timeout_fallback_async_enabled: false,
      media_video_proxy_fallback_enabled: false,
    })
  })
})

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('navigation locale keys', () => {
  it('contains model pricing and pool monitor labels in zh', () => {
    expect(zh.nav.modelPricing).toBe('模型广场')
    expect(zh.nav.poolMonitor).toBe('号池监控')
    expect(zh.nav.channelStatus).toBe('渠道状态')
    expect(zh.home.nav.modelPricing).toBe('模型广场')
    expect(zh.home.nav.channelStatus).toBe('渠道状态')
    expect(zh.home.nav.integrationDocs).toBe('接入文档')
    expect(zh.nav.channelMonitor).toBe('渠道监控')
    expect(zh.nav.channelManagement).toBe('渠道管理')
    expect(zh.nav['pricing' + 'Plans']).toBeUndefined()
  })

  it('contains model pricing and pool monitor labels in en', () => {
    expect(en.nav.modelPricing).toBe('Model Square')
    expect(en.nav.poolMonitor).toBe('Pool Monitor')
    expect(en.nav.channelStatus).toBe('Channel Status')
    expect(en.home.nav.modelPricing).toBe('Model Square')
    expect(en.home.nav.channelStatus).toBe('Channel Status')
    expect(en.home.nav.integrationDocs).toBe('Integration Docs')
    expect(en.nav.channelMonitor).toBe('Channel Monitor')
    expect(en.nav.channelManagement).toBe('Channels')
    expect(en.nav['pricing' + 'Plans']).toBeUndefined()
  })

  it('contains settings labels for channel monitor in both locales', () => {
    expect(zh.admin.settings.channelMonitor.title).toBe('渠道监控')
    expect(en.admin.settings.channelMonitor.title).toBe('Channel Monitor')
  })
})

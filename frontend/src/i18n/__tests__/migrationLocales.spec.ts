import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('migration locale coverage', () => {
  it('contains dashboard chart labels for zh and en', () => {
    expect(zh.admin.dashboard.requestCountTrend).toBe('请求数统计')
    expect(zh.admin.dashboard.proxyUsageSummary).toBe('代理使用统计')
    expect(zh.admin.dashboard.metricLegend).toBe('指标图例')
    expect(zh.admin.dashboard.proxyLegend).toBe('代理 IP 图例')
    expect(zh.admin.dashboard.totalCount).toBe('总次数')
    expect(zh.admin.dashboard.successCount).toBe('成功次数')
    expect(zh.admin.dashboard.failureCount).toBe('失败次数')

    expect(en.admin.dashboard.requestCountTrend).toBe('Request Count Trend')
    expect(en.admin.dashboard.proxyUsageSummary).toBe('Proxy Usage Summary')
    expect(en.admin.dashboard.metricLegend).toBe('Metric Legend')
    expect(en.admin.dashboard.proxyLegend).toBe('Proxy IP Legend')
    expect(en.admin.dashboard.totalCount).toBe('Total Count')
    expect(en.admin.dashboard.successCount).toBe('Success Count')
    expect(en.admin.dashboard.failureCount).toBe('Failure Count')
  })

  it('contains pool monitor meta and page copy for zh and en', () => {
    expect(zh.admin.poolMonitorDashboard.title).toBe('号池监控')
    expect(zh.admin.poolMonitorDashboard.openConfig).toBe('监控配置')
    expect(zh.admin.poolMonitorConfig.title).toBe('号池监控配置')
    expect(zh.admin.poolMonitorConfig.backToDashboard).toBe('返回监控')
    expect(zh.admin.poolMonitor.title).toBe('号池监控配置')

    expect(en.admin.poolMonitorDashboard.title).toBe('Pool Monitor')
    expect(en.admin.poolMonitorDashboard.openConfig).toBe('Monitor Settings')
    expect(en.admin.poolMonitorConfig.title).toBe('Pool Monitor Settings')
    expect(en.admin.poolMonitorConfig.backToDashboard).toBe('Back to Monitor')
    expect(en.admin.poolMonitor.title).toBe('Pool Monitor Settings')
  })

  it('contains proxy count state labels used by scheduling status filters', () => {
    expect(zh.admin.proxies.countStates.available).toBe('可用')
    expect(zh.admin.proxies.countStates.manualUnschedulable).toBe('手动停调度')
    expect(zh.admin.proxies.countStates.tempUnschedulable).toBe('临时异常')
    expect(zh.admin.proxies.countStates.rateLimited).toBe('限流中')
    expect(zh.admin.proxies.countStates.overloaded).toBe('过载中')
    expect(zh.admin.proxies.countStates.expired).toBe('已过期')
    expect(zh.admin.proxies.countStates.inactive).toBe('已停用')
    expect(zh.admin.proxies.countStates.error).toBe('异常')
    expect(zh.admin.proxies.countStates.banned).toBe('封禁')

    expect(en.admin.proxies.countStates.available).toBe('Available')
    expect(en.admin.proxies.countStates.manualUnschedulable).toBe('Manual Off')
    expect(en.admin.proxies.countStates.tempUnschedulable).toBe('Temp Abnormal')
    expect(en.admin.proxies.countStates.rateLimited).toBe('Rate Limited')
    expect(en.admin.proxies.countStates.overloaded).toBe('Overloaded')
    expect(en.admin.proxies.countStates.expired).toBe('Expired')
    expect(en.admin.proxies.countStates.inactive).toBe('Inactive')
    expect(en.admin.proxies.countStates.error).toBe('Error')
    expect(en.admin.proxies.countStates.banned).toBe('Banned')
  })

  it('contains stacked subscription copy for redeem and admin flows', () => {
    expect(zh.redeem.subscriptionExtended).toBe('订阅已延长')
    expect(zh.redeem.subscriptionModeExtend).toBe('延长时长')
    expect(zh.redeem.subscriptionModeStack).toBe('叠加额度')
    expect(zh.redeem.modeGuideTitle).toBe('兑换模式示例（按日维度）')
    expect(zh.redeem.subscriptionChoiceTitle).toBe('订阅兑换方式')

    expect(en.redeem.subscriptionExtended).toBe('Subscription Extended')
    expect(en.redeem.subscriptionModeExtend).toBe('Extend')
    expect(en.redeem.subscriptionModeStack).toBe('Stack')
    expect(en.redeem.modeGuideTitle).toBe('Redeem mode examples (daily view)')
    expect(en.redeem.subscriptionChoiceTitle).toBe('Choose subscription redeem mode')

    expect(zh.admin.subscriptions.assignChoiceTitle).toBe('选择分配方式')
    expect(zh.admin.subscriptions.subscriptionExtended).toBe('订阅延长成功')
    expect(en.admin.subscriptions.assignChoiceTitle).toBe('Choose assignment mode')
    expect(en.admin.subscriptions.subscriptionExtended).toBe(
      'Subscription extended successfully'
    )
  })
})

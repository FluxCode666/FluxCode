import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    isAuthenticated: true,
    isAdmin: true,
    isSimpleMode: false
  })
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    backendModeEnabled: false
  })
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({
    fetch: vi.fn()
  })
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn()
  })
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn()
  })
}))

vi.mock('../title', () => ({
  resolveDocumentTitle: vi.fn().mockReturnValue('FluxCode')
}))

describe('admin route mapping', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  it('keeps pool monitor dashboard and config paths aligned with the old system', async () => {
    const { default: router } = await import('../index')

    const dashboardRoute = router.getRoutes().find((route) => route.name === 'AdminPoolMonitor')
    const configRoute = router.getRoutes().find((route) => route.name === 'AdminPoolMonitorConfig')

    expect(dashboardRoute?.path).toBe('/admin/pool-monitor')
    expect(dashboardRoute?.meta.titleKey).toBe('admin.poolMonitorDashboard.title')
    expect(dashboardRoute?.meta.descriptionKey).toBe('admin.poolMonitorDashboard.description')

    expect(configRoute?.path).toBe('/admin/pool-monitor/config')
    expect(configRoute?.meta.titleKey).toBe('admin.poolMonitorConfig.title')
    expect(configRoute?.meta.descriptionKey).toBe('admin.poolMonitorConfig.description')
    expect(configRoute?.meta.hideInMenu).toBe(true)
  })

  it('registers gated channel monitor admin route', async () => {
    const { default: router } = await import('../index')

    const route = router.getRoutes().find((item) => item.name === 'AdminChannelMonitor')

    expect(route?.path).toBe('/admin/channels/monitor')
    expect(route?.meta.requiresAdmin).toBe(true)
    expect(route?.meta.requiresChannelMonitor).toBe(true)
    expect(route?.meta.titleKey).toBe('admin.channelMonitor.title')
  })

  it('registers public channel status route outside the console', async () => {
    const { default: router } = await import('../index')

    const route = router.getRoutes().find((item) => item.name === 'PublicChannelStatus')

    expect(route?.path).toBe('/channel-status')
    expect(route?.meta.requiresAuth).toBe(false)
    expect(route?.meta.titleKey).toBe('channelStatus.title')
  })

  it('does not register removed legacy admin route', async () => {
    const { default: router } = await import('../index')

    const removedAdminPath = ['/admin', 'pricing' + '-plans'].join('/')
    const oldRoute = router.getRoutes().find((route) => route.path === removedAdminPath)

    expect(oldRoute).toBeUndefined()
  })

  it('registers public model pricing route', async () => {
    const { default: router } = await import('../index')

    const route = router.getRoutes().find((route) => route.path === '/model-pricing')

    expect(route?.meta.requiresAuth).toBe(false)
  })

  it('does not register old public pricing route', async () => {
    const { default: router } = await import('../index')

    const oldRoute = router.getRoutes().find((route) => route.path === '/pricing')

    expect(oldRoute).toBeUndefined()
  })

  it('keeps purchase and order routes registered', async () => {
    const { default: router } = await import('../index')

    expect(router.getRoutes().find((route) => route.path === '/purchase')).toBeDefined()
    expect(router.getRoutes().find((route) => route.path === '/orders')).toBeDefined()
    expect(router.getRoutes().find((route) => route.path === '/admin/orders/plans')).toBeDefined()
  })
})

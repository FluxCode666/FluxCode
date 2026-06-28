const STORAGE_PREFIX = 'dashboard-fireworks-fired'
const DEFAULT_THRESHOLD = 20

type FireworksStorage = Pick<Storage, 'getItem' | 'setItem'>

interface DashboardFireworksOptions {
  enabled: boolean
  threshold?: number | null
  todayActualCost: number
  isMobile: boolean
  userId?: number | string | null
  dateKey?: string
  storage?: FireworksStorage
}

export function getLocalDateKey(date = new Date()): string {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

export function getDashboardFireworksStorageKey(
  userId: number | string | null | undefined,
  dateKey = getLocalDateKey()
): string {
  return `${STORAGE_PREFIX}:${userId ?? 'anonymous'}:${dateKey}`
}

export function shouldTriggerDashboardFireworks(options: DashboardFireworksOptions): boolean {
  if (!options.enabled || options.isMobile) return false

  const threshold = normalizeThreshold(options.threshold)
  if (!Number.isFinite(options.todayActualCost) || options.todayActualCost <= threshold) {
    return false
  }

  const storage = options.storage ?? getBrowserStorage()
  if (!storage) return false

  const key = getDashboardFireworksStorageKey(options.userId, options.dateKey)
  try {
    return storage.getItem(key) !== '1'
  } catch {
    return false
  }
}

export function markDashboardFireworksShown(options: DashboardFireworksOptions): void {
  const storage = options.storage ?? getBrowserStorage()
  if (!storage) return

  const key = getDashboardFireworksStorageKey(options.userId, options.dateKey)
  try {
    storage.setItem(key, '1')
  } catch {
    // Ignore storage failures; the animation should not break the dashboard.
  }
}

function normalizeThreshold(value: number | null | undefined): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
    ? value
    : DEFAULT_THRESHOLD
}

function getBrowserStorage(): FireworksStorage | null {
  if (typeof window === 'undefined') return null
  return window.localStorage
}

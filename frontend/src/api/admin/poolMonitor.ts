import { apiClient } from '../client'

export interface PoolMonitorConfig {
  platform: string
  pool_threshold_enabled: boolean
  proxy_failure_enabled: boolean
  proxy_active_probe_enabled: boolean
  disabled_proxy_schedule_mode: 'direct_without_proxy' | 'exclude_account'
  available_count_threshold: number
  available_ratio_threshold: number
  check_interval_minutes: number
  proxy_probe_interval_minutes: number
  proxy_failure_window_minutes: number
  proxy_failure_threshold: number
  alert_emails: string[]
  alert_cooldown_minutes: number
}

export type PoolMonitorConfigPatch = Partial<
  Omit<PoolMonitorConfig, 'platform'>
>

export async function getPoolMonitorConfig(platform: string): Promise<PoolMonitorConfig> {
  const { data } = await apiClient.get<PoolMonitorConfig>(`/admin/pool-monitor/${platform}`)
  return data
}

export async function updatePoolMonitorConfig(
  platform: string,
  config: PoolMonitorConfigPatch
): Promise<PoolMonitorConfig> {
  const { data } = await apiClient.put<PoolMonitorConfig>(`/admin/pool-monitor/${platform}`, config)
  return data
}

export const poolMonitorAPI = {
  getPoolMonitorConfig,
  updatePoolMonitorConfig
}

export default poolMonitorAPI

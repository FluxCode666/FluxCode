import type { AdminGroup } from '@/types'

export interface FallbackOption {
  value: number | null
  label: string
  [key: string]: unknown
}

export function canEnableFallbackGroup(
  group: Pick<AdminGroup, 'platform' | 'subscription_type'>,
): boolean {
  return (
    (group.platform === 'openai' || group.platform === 'anthropic') &&
    group.subscription_type === 'standard'
  )
}

export function isApiKeyBindableGroup(
  group: Pick<AdminGroup, 'is_fallback_group'>,
): boolean {
  return !group.is_fallback_group
}

export function buildFallbackTargetOptions(
  groups: AdminGroup[],
  current: Pick<AdminGroup, 'id' | 'platform'>,
  noFallbackLabel = 'No Fallback',
): FallbackOption[] {
  const options: FallbackOption[] = [{ value: null, label: noFallbackLabel }]

  for (const group of groups) {
    if (
      group.id !== current.id &&
      group.status === 'active' &&
      group.subscription_type === 'standard' &&
      group.is_fallback_group &&
      group.platform === current.platform &&
      !group.fallback_group_id
    ) {
      options.push({ value: group.id, label: group.name })
    }
  }

  return options
}

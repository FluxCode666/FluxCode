import type { AdminGroup } from '@/types'
export { isApiKeyBindableGroup } from '@/utils/apiKeyGroupSelection'

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

function normalizeFallbackEntryPlatform(platform: string): string {
  return platform === 'codex2api' ? 'openai' : platform
}

export function buildFallbackTargetOptions(
  groups: AdminGroup[],
  current: Pick<AdminGroup, 'id' | 'platform'>,
  noFallbackLabel = 'No Fallback',
): FallbackOption[] {
  const options: FallbackOption[] = [{ value: null, label: noFallbackLabel }]
  const currentPlatform = normalizeFallbackEntryPlatform(current.platform)

  for (const group of groups) {
    if (
      group.id !== current.id &&
      group.status === 'active' &&
      group.subscription_type === 'standard' &&
      group.is_fallback_group &&
      group.platform === currentPlatform &&
      !group.fallback_group_id
    ) {
      options.push({ value: group.id, label: group.name })
    }
  }

  return options
}

export function buildFallbackTargetOptionsForEdit(
  groups: AdminGroup[],
  currentId: number | null | undefined,
  platform: AdminGroup['platform'],
  noFallbackLabel = 'No Fallback',
): FallbackOption[] {
  if (currentId == null) {
    return [{ value: null, label: noFallbackLabel }]
  }

  return buildFallbackTargetOptions(
    groups,
    {
      id: currentId,
      platform,
    },
    noFallbackLabel,
  )
}

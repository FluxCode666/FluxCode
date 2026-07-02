import type { Group } from '@/types'

type FallbackFlagGroup = Pick<Group, 'is_fallback_group'>
type AssignableGroup = Pick<Group, 'subscription_type' | 'status'> & FallbackFlagGroup

export function isApiKeyBindableGroup(group: FallbackFlagGroup): boolean {
  return !group.is_fallback_group
}

export function isAllowedGroupAssignable(group: AssignableGroup): boolean {
  return (
    group.subscription_type === 'standard' &&
    group.status === 'active' &&
    isApiKeyBindableGroup(group)
  )
}

export function filterRebindableApiKeyGroups<T extends FallbackFlagGroup>(
  groups: T[],
): T[] {
  return groups.filter(isApiKeyBindableGroup)
}

export function filterAssignableAllowedGroups<T extends AssignableGroup>(
  groups: T[],
): T[] {
  return groups.filter(isAllowedGroupAssignable)
}

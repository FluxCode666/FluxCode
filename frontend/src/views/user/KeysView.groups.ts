import type { Group } from '@/types'
import { isApiKeyBindableGroup } from '@/utils/apiKeyGroupSelection'

export function isSelectableApiKeyGroup(
  group: Pick<Group, 'is_fallback_group'>,
): boolean {
  return isApiKeyBindableGroup(group)
}

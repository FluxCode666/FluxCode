import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const viewSource = readFileSync(resolve(root, 'views/admin/GroupsView.vue'), 'utf8')
const typesSource = readFileSync(resolve(root, 'types/index.ts'), 'utf8')
const groupsApiSource = readFileSync(resolve(root, 'api/admin/groups.ts'), 'utf8')

describe('GroupsView embedding release contract', () => {
  it('keeps embedding in create, edit, filter, and API group platform types', () => {
    expect(typesSource).toContain("| 'embedding'")
    expect(viewSource.match(/value: 'embedding'/g)).toHaveLength(2)
    expect(viewSource).toContain("label: t('admin.groups.platforms.embedding', 'Embedding')")
    expect(viewSource).toContain("{ value: 'embedding', label: t('admin.groups.platforms.embedding', 'Embedding') }")
    expect(viewSource.match(/group\.platform === 'embedding'|value === 'embedding'/g)).toHaveLength(2)
    expect(viewSource.match(/bg-rose-100 text-rose-700/g)).toHaveLength(2)
  })
})

describe('GroupsView billing type filter contract', () => {
  it('defaults to standard balance billing and forwards the filter to the API', () => {
    expect(viewSource).toContain('v-model="filters.subscription_type"')
    expect(viewSource).toContain('subscription_type: "standard" as SubscriptionType | ""')
    expect(viewSource).toContain('subscription_type: filters.subscription_type || undefined')
    expect(groupsApiSource).toContain("subscription_type?: 'standard' | 'subscription'")
  })
})

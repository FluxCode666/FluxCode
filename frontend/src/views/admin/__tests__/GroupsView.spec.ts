import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const viewSource = readFileSync(resolve(root, 'views/admin/GroupsView.vue'), 'utf8')
const typesSource = readFileSync(resolve(root, 'types/index.ts'), 'utf8')

describe('GroupsView embedding release contract', () => {
  it('keeps embedding in create, edit, filter, and API group platform types', () => {
    expect(typesSource).toContain("| 'embedding'")
    expect(viewSource.match(/value: 'embedding'/g)).toHaveLength(2)
    expect(viewSource).toContain("label: t('admin.groups.platforms.embedding', 'Embedding')")
    expect(viewSource).toContain("{ value: 'embedding', label: t('admin.groups.platforms.embedding', 'Embedding') }")
  })
})

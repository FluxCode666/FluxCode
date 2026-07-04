import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../..')
const viewSource = readFileSync(resolve(root, 'views/admin/GroupsView.vue'), 'utf8')
const typesSource = readFileSync(resolve(root, 'types/index.ts'), 'utf8')

describe('GroupsView OpenAI image generation permission', () => {
  it('binds allow_image_generation in types, forms, edit hydration, and payload', () => {
    expect(typesSource).toContain('allow_image_generation: boolean')
    expect(typesSource).toContain('allow_image_generation?: boolean')
    expect(viewSource).toContain('v-model="createForm.allow_image_generation"')
    expect(viewSource).toContain('v-model="editForm.allow_image_generation"')
    expect(viewSource).toContain('allow_image_generation: false')
    expect(viewSource).toContain('editForm.allow_image_generation = group.allow_image_generation ?? false')
  })
})

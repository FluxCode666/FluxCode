import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const docs = [
  readFileSync(resolve(process.cwd(), '../README.md'), 'utf8'),
  readFileSync(resolve(process.cwd(), '../README_CN.md'), 'utf8')
]

describe('embedding release documentation', () => {
  it.each(docs)('documents the complete public and privacy contract', (doc) => {
    for (const required of [
      'RUN_MODE=simple',
      '/v1/models',
      '/v1/embeddings',
      'Authorization: Bearer',
      'usage.prompt_tokens',
      'client.embeddings.create'
    ]) {
      expect(doc).toContain(required)
    }
    expect(/input|输入/i.test(doc)).toBe(true)
    expect(/vector|向量/i.test(doc)).toBe(true)
  })
})

import { describe, expect, it } from 'vitest'
import zh from '../locales/zh'
import en from '../locales/en'

describe('embedding locale coverage', () => {
  it.each([zh, en])('contains usage, key guidance, and Ops privacy labels', (locale) => {
    expect(locale.usage.embedding).toBeTruthy()
    expect(locale.keys.useKeyModal.embedding.description).toContain('/v1/models')
    expect(locale.keys.useKeyModal.embedding.note).toContain('/v1/embeddings')
    expect(locale.admin.ops.errorLog.requestTypeEmbedding).toBeTruthy()
    expect(locale.admin.ops.errorDetail.requestTypeEmbedding).toBeTruthy()
    expect(locale.admin.ops.errorDetail.contentPreviewUnavailable).toBeTruthy()
  })
})

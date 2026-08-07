import { describe, expect, it } from 'vitest'
import {
  getLegalDocument,
  legalDocumentNavigation,
  legalDocuments,
  renderLegalText
} from '@/content/legalDocuments'

describe('legalDocuments', () => {
  it('exposes the four public legal documents in the expected order', () => {
    expect(legalDocumentNavigation.map((item) => item.path)).toEqual([
      '/legal/terms',
      '/legal/usage-policy',
      '/legal/supported-regions',
      '/legal/service-specific-terms'
    ])
  })

  it.each(Object.values(legalDocuments))('$key has a stable table of contents', (document) => {
    expect(document.sections.length).toBeGreaterThanOrEqual(6)
    expect(new Set(document.sections.map((section) => section.id)).size).toBe(document.sections.length)
    expect(document.sections.every((section) => section.title.trim().length > 0)).toBe(true)
  })

  it('replaces the configured site name without leaking the reference brand', () => {
    const copy = renderLegalText(legalDocuments.terms.description, 'FluxCode')
    expect(copy).toContain('FluxCode')
    expect(copy).not.toContain('AnPin')
  })

  it('does not use invalid document keys or high-risk final-interpretation wording', () => {
    expect(getLegalDocument('unknown')).toBeUndefined()
    expect(JSON.stringify(legalDocuments)).not.toContain('最终解释权')
  })
})

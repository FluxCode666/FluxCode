import { readdirSync, readFileSync } from 'node:fs'
import { dirname, extname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

type LocaleMessages = Record<string, unknown>

const testDir = dirname(fileURLToPath(import.meta.url))
const srcDir = resolve(testDir, '..', '..')
const localeFiles = new Set(['en.ts', 'zh.ts'])
const translationCallPattern =
  /(?:\$t|(?<![\w$.])t|i18n\.global\.t)\(\s*['"]([^'"]+)['"]\s*(?:[),])/g

function walkSourceFiles(dir: string): string[] {
  const entries = readdirSync(dir, { withFileTypes: true })

  return entries.flatMap((entry) => {
    const fullPath = join(dir, entry.name)

    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || entry.name === 'locales') {
        return []
      }

      return walkSourceFiles(fullPath)
    }

    const extension = extname(entry.name)
    if (!['.ts', '.tsx', '.vue'].includes(extension)) {
      return []
    }

    return [fullPath]
  })
}

function collectStaticTranslationKeys(): string[] {
  const keys = new Set<string>()

  for (const filePath of walkSourceFiles(srcDir)) {
    if (localeFiles.has(filePath.split('/').pop() || '')) {
      continue
    }

    const content = readFileSync(filePath, 'utf8')

    for (const match of content.matchAll(translationCallPattern)) {
      const key = match[1]?.trim()
      if (key) {
        keys.add(key)
      }
    }
  }

  return [...keys].sort((left, right) => left.localeCompare(right))
}

function hasLocaleKey(messages: LocaleMessages, key: string): boolean {
  let current: unknown = messages

  for (const segment of key.split('.')) {
    if (!current || typeof current !== 'object' || !(segment in current)) {
      return false
    }

    current = (current as LocaleMessages)[segment]
  }

  return current !== undefined
}

const staticTranslationKeys = collectStaticTranslationKeys()

describe('static locale coverage', () => {
  it('contains all static translation keys in zh locale', () => {
    const missingKeys = staticTranslationKeys.filter((key) => !hasLocaleKey(zh as LocaleMessages, key))

    expect(missingKeys, `zh locale 缺失 ${missingKeys.length} 个 key`).toEqual([])
  })

  it('contains all static translation keys in en locale', () => {
    const missingKeys = staticTranslationKeys.filter((key) => !hasLocaleKey(en as LocaleMessages, key))

    expect(missingKeys, `en locale 缺失 ${missingKeys.length} 个 key`).toEqual([])
  })

  it('does not leave untranslated raw keys inside public source files', () => {
    expect(staticTranslationKeys.length).toBeGreaterThan(0)
    expect(
      staticTranslationKeys.some((key) => key.startsWith('admin.') || key.startsWith('common.'))
    ).toBe(true)
  })
})

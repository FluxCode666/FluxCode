import { describe, expect, it, vi } from 'vitest'

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

import {
  buildModelMappingObject,
  getDefaultModelsForAccount,
  getModelsByPlatform,
  openAIAPIKeyDefaultModels
} from '../useModelWhitelist'

describe('useModelWhitelist', () => {
  it('openai 模型列表包含 GPT-5.5 与 GPT-5.4 官方快照', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.6-sol')
    expect(models).toContain('gpt-5.6-terra')
    expect(models).toContain('gpt-5.6-luna')
    expect(models).toContain('gpt-5.5')
    expect(models).toContain('gpt-5.4')
    expect(models).toContain('gpt-5.4-mini')
    expect(models).toContain('gpt-5.4-nano')
    expect(models).toContain('gpt-5.4-2026-03-05')
  })

  it('includes bare GPT-5.6 and variants', () => {
    const models = getModelsByPlatform('openai')

    expect(models).toContain('gpt-5.6')
    expect(models).toContain('gpt-5.6-sol')
    expect(models).toContain('gpt-5.6-terra')
    expect(models).toContain('gpt-5.6-luna')
  })

  it('OpenAI API Key 账号只默认启用指定模型', () => {
    expect(getDefaultModelsForAccount('openai', 'apikey')).toEqual([
      'gpt-5.4',
      'gpt-5.4-mini',
      'gpt-5.5',
      'gpt-5.6-luna',
      'gpt-5.6-terra',
      'gpt-5.6-sol'
    ])
    expect(getDefaultModelsForAccount('openai', 'apikey')).toBe(openAIAPIKeyDefaultModels)
  })

  it('OpenAI OAuth 账号也默认启用指定模型', () => {
    expect(getDefaultModelsForAccount('openai', 'oauth-based')).toEqual(openAIAPIKeyDefaultModels)
    expect(getDefaultModelsForAccount('openai', 'oauth-based')).not.toEqual(getModelsByPlatform('openai'))
  })

  it('antigravity 模型列表包含图片模型兼容项', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models).toContain('gemini-3-pro-image')
  })

  it('gemini 模型列表包含原生生图模型', () => {
    const models = getModelsByPlatform('gemini')

    expect(models).toContain('gemini-2.5-flash-image')
    expect(models).toContain('gemini-3.1-flash-image')
    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.0-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
  })

  it('embedding 模型列表不回退到 Claude', () => {
    const models = getModelsByPlatform('embedding')
    expect(models).toContain('text-embedding-3-small')
    expect(models.some(model => model.startsWith('claude-'))).toBe(false)
  })

  it('antigravity 模型列表会把新的 Gemini 图片模型排在前面', () => {
    const models = getModelsByPlatform('antigravity')

    expect(models.indexOf('gemini-3.1-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash'))
    expect(models.indexOf('gemini-2.5-flash-image')).toBeLessThan(models.indexOf('gemini-2.5-flash-lite'))
  })

  it('whitelist 模式会忽略通配符条目', () => {
    const mapping = buildModelMappingObject('whitelist', ['claude-*', 'gemini-3.1-flash-image'], [])
    expect(mapping).toEqual({
      'gemini-3.1-flash-image': 'gemini-3.1-flash-image'
    })
  })

  it('whitelist 模式会保留 GPT-5.4 官方快照的精确映射', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-2026-03-05'], [])

    expect(mapping).toEqual({
      'gpt-5.4-2026-03-05': 'gpt-5.4-2026-03-05'
    })
  })

  it('whitelist keeps GPT-5.5 exact mapping', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.5'], [])

    expect(mapping).toEqual({
      'gpt-5.5': 'gpt-5.5'
    })
  })

  it('whitelist keeps GPT-5.4 mini and nano exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.4-mini', 'gpt-5.4-nano'], [])

    expect(mapping).toEqual({
      'gpt-5.4-mini': 'gpt-5.4-mini',
      'gpt-5.4-nano': 'gpt-5.4-nano'
    })
  })

  it('whitelist keeps GPT-5.6 exact mappings', () => {
    const mapping = buildModelMappingObject('whitelist', ['gpt-5.6', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna'], [])

    expect(mapping).toEqual({
      'gpt-5.6': 'gpt-5.6',
      'gpt-5.6-sol': 'gpt-5.6-sol',
      'gpt-5.6-terra': 'gpt-5.6-terra',
      'gpt-5.6-luna': 'gpt-5.6-luna'
    })
  })
})

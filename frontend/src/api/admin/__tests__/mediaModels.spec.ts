import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { MediaModelDefinition, MediaModelDefinitionInput } from '@/types'
import { create, listEnabled, previewRequestMapping, remove, update } from '../mediaModels'

const { getMock, postMock, putMock, deleteMock } = vi.hoisted(() => ({
  getMock: vi.fn(),
  postMock: vi.fn(),
  putMock: vi.fn(),
  deleteMock: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: { get: getMock, post: postMock, put: putMock, delete: deleteMock },
}))

const readyModel: MediaModelDefinition = {
  id: 1,
  model_id: 'grok-2-image',
  vendor: 'xai',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
  adapter_resolution: {
    status: 'ready',
    resolved_adapter: 'xai-image',
    matched_by: 'exact',
    matched_family: '',
    capabilities: {
      operations: ['text_to_image'],
      sync_upstream: true,
      native_async_upstream: false,
      content_fetch: false,
    },
    reason_code: '',
  },
}

const unresolvedModel: MediaModelDefinition = {
  ...readyModel,
  id: 2,
  model_id: 'unknown-image',
  adapter_resolution: {
    status: 'unresolved',
    resolved_adapter: '',
    matched_by: '',
    matched_family: '',
    capabilities: null,
    reason_code: 'MEDIA_ADAPTER_UNRESOLVED',
  },
}

const disabledModel: MediaModelDefinition = {
  ...readyModel,
  id: 3,
  model_id: 'disabled-image',
  enabled: false,
}

const modelInput: MediaModelDefinitionInput = {
  model_id: 'grok-2-image',
  vendor: 'xai',
  media_type: 'image',
  operations: ['text_to_image'],
  constraints: {},
  billing_unit: 'image',
  enabled: true,
  aliases: [],
}

describe('mediaModels API', () => {
  beforeEach(() => {
    getMock.mockReset()
    postMock.mockReset()
    putMock.mockReset()
    deleteMock.mockReset()
  })

  it('只返回已启用且 Adapter 已就绪的媒体模型', async () => {
    getMock.mockResolvedValue({
      data: { items: [readyModel, unresolvedModel, disabledModel] },
    })

    await expect(listEnabled()).resolves.toEqual([readyModel])
  })

  it('创建模型时只发送业务字段', async () => {
    postMock.mockResolvedValue({ data: readyModel })

    await create(modelInput)

    expect(postMock).toHaveBeenCalledWith('/admin/media-models', modelInput)
    expect(postMock.mock.calls[0][1]).not.toHaveProperty('default_adapter')
    expect(postMock.mock.calls[0][1]).not.toHaveProperty('default_async_mode')
  })

  it('更新模型时只发送业务字段', async () => {
    putMock.mockResolvedValue({ data: readyModel })

    await update(readyModel.id, modelInput)

    expect(putMock).toHaveBeenCalledWith(`/admin/media-models/${readyModel.id}`, modelInput)
    expect(putMock.mock.calls[0][1]).not.toHaveProperty('default_adapter')
    expect(putMock.mock.calls[0][1]).not.toHaveProperty('default_async_mode')
  })

  it('通过规范端点删除模型', async () => {
    deleteMock.mockResolvedValue({ data: undefined })

    await remove(readyModel.id)

    expect(deleteMock).toHaveBeenCalledWith(`/admin/media-models/${readyModel.id}`)
  })

  it('通过只读管理员端点预览声明式请求映射', async () => {
    const request = { prompt: 'hello', size: '1024x1024' }
    const mapping = {
      rules: [{ operation: 'rename' as const, source: 'size', target: 'image_size' }],
    }
    const result = { prompt: 'hello', image_size: '1024x1024' }
    postMock.mockResolvedValue({ data: { result } })

    await expect(previewRequestMapping(request, mapping)).resolves.toEqual(result)

    expect(postMock).toHaveBeenCalledWith(
      '/admin/media-models/request-mapping-preview',
      { request, mapping },
    )
  })
})

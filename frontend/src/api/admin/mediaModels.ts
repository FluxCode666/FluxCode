import { apiClient } from '../client'

import type {
  MediaModelDefinition,
  MediaModelDefinitionInput,
  MediaRequestMapping,
} from '@/types'

export interface MediaModelListResponse {
  items: MediaModelDefinition[]
}

export interface GroupMediaModelScopesResponse {
  model_ids: string[]
}

export interface MediaRequestMappingPreviewResponse {
  result: Record<string, unknown>
}

export async function list(): Promise<MediaModelDefinition[]> {
  const { data } = await apiClient.get<MediaModelListResponse>('/admin/media-models')
  return data.items
}

export async function listEnabled(): Promise<MediaModelDefinition[]> {
  const items = await list()
  return items.filter((item) => item.enabled && item.adapter_resolution.status === 'ready')
}

export async function create(input: MediaModelDefinitionInput): Promise<MediaModelDefinition> {
  const { data } = await apiClient.post<MediaModelDefinition>('/admin/media-models', input)
  return data
}

export async function update(
  id: number,
  input: MediaModelDefinitionInput,
): Promise<MediaModelDefinition> {
  const { data } = await apiClient.put<MediaModelDefinition>(`/admin/media-models/${id}`, input)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/media-models/${id}`)
}

export async function previewRequestMapping(
  request: Record<string, unknown>,
  mapping: MediaRequestMapping,
): Promise<Record<string, unknown>> {
  const { data } = await apiClient.post<MediaRequestMappingPreviewResponse>(
    '/admin/media-models/request-mapping-preview',
    { request, mapping },
  )
  return data.result
}

export async function getGroupScopes(groupId: number): Promise<string[]> {
  const { data } = await apiClient.get<GroupMediaModelScopesResponse>(
    `/admin/groups/${groupId}/media-model-scopes`,
  )
  return data.model_ids
}

export async function replaceGroupScopes(groupId: number, modelIds: string[]): Promise<string[]> {
  const { data } = await apiClient.put<GroupMediaModelScopesResponse>(
    `/admin/groups/${groupId}/media-model-scopes`,
    { model_ids: modelIds },
  )
  return data.model_ids
}

export const mediaModelsAPI = {
  list,
  listEnabled,
  create,
  update,
  remove,
  previewRequestMapping,
  getGroupScopes,
  replaceGroupScopes,
}

export default mediaModelsAPI

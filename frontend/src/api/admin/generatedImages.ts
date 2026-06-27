import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface GeneratedImage {
  id: number
  provider: string
  user_id: number
  api_key_id: number
  account_id: number
  request_id: string
  model: string
  prompt: string
  revised_prompt: string
  response_format: string
  source: string
  content_type: string
  size_bytes: number
  content_url: string
  created_at: string
}

export interface GeneratedImagesQuery {
  page?: number
  page_size?: number
}

export async function list(
  params: GeneratedImagesQuery,
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<GeneratedImage>> {
  const { data } = await apiClient.get<PaginatedResponse<GeneratedImage>>('/admin/generated-images', {
    params,
    signal: options?.signal
  })
  return data
}

export async function getContentBlob(id: number, options?: { signal?: AbortSignal }): Promise<Blob> {
  const { data } = await apiClient.get<Blob>(`/admin/generated-images/${id}/content`, {
    responseType: 'blob',
    signal: options?.signal
  })
  return data
}

const generatedImagesAPI = {
  list,
  getContentBlob
}

export default generatedImagesAPI

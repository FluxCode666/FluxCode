import { apiClient } from './client'

export type ModelCapability = 'chat' | 'image' | 'video'

export interface ModelPricingInterval {
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number
  output_price: number
  cache_write_price: number
  cache_read_price: number
  per_request_price: number
}

export interface ModelPricingAmount {
  input_price: number
  output_price: number
  cache_write_price: number
  cache_read_price: number
  image_output_price: number
  per_request_price: number
  intervals: ModelPricingInterval[]
}

export interface ModelPricingMultipliers {
  input_price: number
  output_price: number
  cache_write_price: number
  cache_read_price: number
  image_output_price: number
  per_request_price: number
}

export interface ModelPricingSummary {
  id: string
  display_name: string
  platform: string
  platforms: string[]
  capabilities: ModelCapability[]
  supported_group_count: number
  official_price: ModelPricingAmount
}

export interface ModelPricingGroupPrice {
  group_id: number
  group_name: string
  rate_multiplier: number
  billing_mode: 'token' | 'per_request' | 'image'
  price: ModelPricingAmount
  multipliers: ModelPricingMultipliers
}

export interface ModelPricingDetail extends ModelPricingSummary {
  groups: ModelPricingGroupPrice[]
}

export interface ListModelPricingParams {
  q?: string
  platform?: string
  capability?: ModelCapability | ''
}

export async function listModels(
  params: ListModelPricingParams = {},
  options?: { signal?: AbortSignal }
): Promise<ModelPricingSummary[]> {
  const response = await apiClient.get<ModelPricingSummary[]>('/model-pricing/models', {
    params,
    signal: options?.signal
  })
  return response.data
}

export async function getModel(
  model: string,
  options?: { signal?: AbortSignal }
): Promise<ModelPricingDetail> {
  const response = await apiClient.get<ModelPricingDetail>('/model-pricing/model', {
    params: { model },
    signal: options?.signal
  })
  return response.data
}

export const modelPricingAPI = {
  listModels,
  getModel
}

export default modelPricingAPI

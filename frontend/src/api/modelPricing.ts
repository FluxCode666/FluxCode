import { apiClient } from './client'

export type ModelCapability =
  | 'streaming'
  | 'system_prompt'
  | 'function_calling'
  | 'tools'
  | 'json_mode'
  | 'structured_output'
  | 'prompt_cache'
  | 'vision'
  | 'image_generation'
  | 'video_generation'
  | 'audio_input'
  | 'audio_output'
  | 'embedding'

/** 公开模型性能数据支持的固定观察范围。 */
export type ModelPerformanceRange = '24h' | '7d'

/**
 * 公开模型性能的范围汇总。`null` 表示没有可用于该指标的有效样本，
 * 调用方必须保持为空值而不是将其显示为零。
 */
export interface ModelPerformanceMetrics {
  tps: number | null
  availability: number | null
  average_first_token_ms: number | null
  average_request_time_ms: number | null
}

/** 全模型性能的小时趋势点，时间为 ISO 8601 UTC。 */
export interface ModelPerformanceTrendPoint {
  bucket_start: string
  availability: number | null
  average_first_token_ms: number | null
}

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
  lowest_group_price: ModelPricingAmount
  performance: ModelPerformanceMetrics
}

export interface ModelPricingGroupOption {
  id: number
  name: string
  platform: string
}

export interface ModelPricingGroupPrice {
  group_id: number
  group_name: string
  rate_multiplier: number
  billing_mode: 'token' | 'per_request' | 'image'
  price: ModelPricingAmount
  multipliers: ModelPricingMultipliers
  performance: ModelPerformanceMetrics
}

export interface ModelPricingDetail extends ModelPricingSummary {
  groups: ModelPricingGroupPrice[]
  performance_trend: ModelPerformanceTrendPoint[]
}

export interface ListModelPricingParams {
  q?: string
  platform?: string
  capability?: ModelCapability | ''
  group_id?: number
  range?: ModelPerformanceRange
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

export async function listGroups(
  options?: { signal?: AbortSignal }
): Promise<ModelPricingGroupOption[]> {
  const response = await apiClient.get<ModelPricingGroupOption[]>('/model-pricing/groups', {
    signal: options?.signal
  })
  return response.data
}

export function getModel(model: string, range: ModelPerformanceRange, options?: { signal?: AbortSignal }): Promise<ModelPricingDetail>
export function getModel(model: string, options?: { signal?: AbortSignal }): Promise<ModelPricingDetail>
export async function getModel(
  model: string,
  rangeOrOptions: ModelPerformanceRange | { signal?: AbortSignal } = '24h',
  options?: { signal?: AbortSignal }
): Promise<ModelPricingDetail> {
  const range = typeof rangeOrOptions === 'string' ? rangeOrOptions : '24h'
  const requestOptions = typeof rangeOrOptions === 'string' ? options : rangeOrOptions
  const response = await apiClient.get<ModelPricingDetail>('/model-pricing/model', {
    params: { model, range },
    signal: requestOptions?.signal
  })
  return response.data
}

export const modelPricingAPI = {
  listModels,
  listGroups,
  getModel
}

export default modelPricingAPI

import { apiClient } from '../client'

export type GrowthGranularity = 'day' | 'week' | 'month'

export interface GrowthQueryParams {
  start_date?: string
  end_date?: string
  granularity?: GrowthGranularity
}

export interface GrowthOverview {
  total_users: number
  dau: number
  mau: number
  today_new_users: number
  today_paid_users: number
  month_revenue: number
  arpu: number
  payment_conversion_rate: number
  repurchase_rate: number
}

export interface GrowthUserTrendPoint {
  date: string
  new_registered: number
  new_activated: number
  new_paid: number
}

export interface GrowthUserTrendResponse {
  series: GrowthUserTrendPoint[]
}

export interface GrowthSourceItem {
  source: string
  users: number
}

export interface GrowthSourcesResponse {
  items: GrowthSourceItem[]
}

export interface GrowthSourcePaymentRateItem {
  source: string
  registered_users: number
  paid_users: number
  conversion_rate: number
}

export interface GrowthSourcePaymentRatesResponse {
  items: GrowthSourcePaymentRateItem[]
}

export interface GrowthRetentionCohort {
  date: string
  new_users: number
  retention: Record<string, number>
}

export interface GrowthRetentionMatrix {
  columns: string[]
  cohorts: GrowthRetentionCohort[]
}

export interface GrowthRetentionTrendPoint {
  date: string
  d1: number
  d7: number
  d30: number
}

export interface GrowthRetentionTrendResponse {
  series: GrowthRetentionTrendPoint[]
}

export interface GrowthFunnelStep {
  key: string
  label: string
  users: number
  count: number
  conversion_rate: number
}

export interface GrowthPaymentFunnel {
  steps: GrowthFunnelStep[]
  tracking_ready: boolean
}

export interface GrowthPaymentPlanItem {
  plan_id?: number | null
  plan_name: string
  category: string
  sales: number
  revenue: number
}

export interface GrowthPaymentPlansResponse {
  items: GrowthPaymentPlanItem[]
}

export interface GrowthFirstPaymentBucket {
  bucket: string
  label: string
  users: number
  ratio: number
}

export interface GrowthFirstPaymentResponse {
  items: GrowthFirstPaymentBucket[]
}

export interface GrowthFeatureRankingItem {
  feature: string
  label: string
  uses: number
  users: number
  user_ratio: number
}

export interface GrowthFeatureRankingResponse {
  items: GrowthFeatureRankingItem[]
}

export interface GrowthMetricValue {
  available: boolean
  value: number
}

export interface GrowthSessionMetrics {
  average_turns: GrowthMetricValue
  average_session_duration_seconds: GrowthMetricValue
  average_input_tokens: GrowthMetricValue
  average_output_tokens: GrowthMetricValue
}

export interface GrowthAudienceItem {
  key: string
  label: string
  users: number
  requests: number
  user_ratio: number
}

export interface GrowthAudienceResponse {
  items: GrowthAudienceItem[]
}

async function getOverview(params?: GrowthQueryParams): Promise<GrowthOverview> {
  const { data } = await apiClient.get<GrowthOverview>('/admin/growth/overview', { params })
  return data
}

async function getUserTrend(params?: GrowthQueryParams): Promise<GrowthUserTrendResponse> {
  const { data } = await apiClient.get<GrowthUserTrendResponse>('/admin/growth/users/trend', { params })
  return data
}

async function getUserSources(params?: GrowthQueryParams): Promise<GrowthSourcesResponse> {
  const { data } = await apiClient.get<GrowthSourcesResponse>('/admin/growth/users/sources', { params })
  return data
}

async function getSourcePaymentRates(params?: GrowthQueryParams): Promise<GrowthSourcePaymentRatesResponse> {
  const { data } = await apiClient.get<GrowthSourcePaymentRatesResponse>('/admin/growth/users/source-payment-rates', { params })
  return data
}

async function getRetentionMatrix(params?: GrowthQueryParams): Promise<GrowthRetentionMatrix> {
  const { data } = await apiClient.get<GrowthRetentionMatrix>('/admin/growth/retention/matrix', { params })
  return data
}

async function getRetentionTrend(params?: GrowthQueryParams): Promise<GrowthRetentionTrendResponse> {
  const { data } = await apiClient.get<GrowthRetentionTrendResponse>('/admin/growth/retention/trend', { params })
  return data
}

async function getPaymentFunnel(params?: GrowthQueryParams): Promise<GrowthPaymentFunnel> {
  const { data } = await apiClient.get<GrowthPaymentFunnel>('/admin/growth/payments/funnel', { params })
  return data
}

async function getPaymentPlans(params?: GrowthQueryParams): Promise<GrowthPaymentPlansResponse> {
  const { data } = await apiClient.get<GrowthPaymentPlansResponse>('/admin/growth/payments/plans', { params })
  return data
}

async function getFirstPayment(params?: GrowthQueryParams): Promise<GrowthFirstPaymentResponse> {
  const { data } = await apiClient.get<GrowthFirstPaymentResponse>('/admin/growth/payments/first-payment', { params })
  return data
}

async function getFeatureRanking(params?: GrowthQueryParams): Promise<GrowthFeatureRankingResponse> {
  const { data } = await apiClient.get<GrowthFeatureRankingResponse>('/admin/growth/features/ranking', { params })
  return data
}

async function getSessionMetrics(params?: GrowthQueryParams): Promise<GrowthSessionMetrics> {
  const { data } = await apiClient.get<GrowthSessionMetrics>('/admin/growth/features/session-metrics', { params })
  return data
}

async function getAudienceDevices(params?: GrowthQueryParams): Promise<GrowthAudienceResponse> {
  const { data } = await apiClient.get<GrowthAudienceResponse>('/admin/growth/audience/devices', { params })
  return data
}

async function getAudienceOS(params?: GrowthQueryParams): Promise<GrowthAudienceResponse> {
  const { data } = await apiClient.get<GrowthAudienceResponse>('/admin/growth/audience/os', { params })
  return data
}

async function getAudienceBrowsers(params?: GrowthQueryParams): Promise<GrowthAudienceResponse> {
  const { data } = await apiClient.get<GrowthAudienceResponse>('/admin/growth/audience/browsers', { params })
  return data
}

async function getAudienceClients(params?: GrowthQueryParams): Promise<GrowthAudienceResponse> {
  const { data } = await apiClient.get<GrowthAudienceResponse>('/admin/growth/audience/clients', { params })
  return data
}

export const growthAPI = {
  getOverview,
  getUserTrend,
  getUserSources,
  getSourcePaymentRates,
  getRetentionMatrix,
  getRetentionTrend,
  getPaymentFunnel,
  getPaymentPlans,
  getFirstPayment,
  getFeatureRanking,
  getSessionMetrics,
  getAudienceDevices,
  getAudienceOS,
  getAudienceBrowsers,
  getAudienceClients
}

export default growthAPI

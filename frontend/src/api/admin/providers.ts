import { apiClient } from '../client'

export const PROVIDER_PROTOCOLS = ['chat_completions', 'responses', 'anthropic_messages', 'embeddings'] as const
export type ProviderProtocol = (typeof PROVIDER_PROTOCOLS)[number]
export const PROVIDER_PROTOCOL_DEFAULT_PATHS: Record<ProviderProtocol, string> = {
  chat_completions: '/v1/chat/completions',
  responses: '/v1/responses',
  anthropic_messages: '/v1/messages',
  embeddings: '/v1/embeddings'
}
export type ProviderStatus = 'draft' | 'active' | 'disabled' | 'review_required'
export type ProviderWireProfile = 'canonical_v1' | 'newapi_messages_v1' | 'siliconflow_messages_v1'
export type ProviderFeatureProfile = 'text_v1' | 'stream_text_v1' | 'function_tools_v1' | 'embeddings_v1'

export interface ProviderEndpoint {
  id: number
  protocol: ProviderProtocol
  wire_profile: ProviderWireProfile
  base_url: string
  path: string
  headers: Record<string, string>
  auth_type: string
  enabled: boolean
  version: number
}
export interface ProviderCapability {
  id: number
  logical_model_id: number
  logical_model: string
  logical_model_display: string
  protocol: ProviderProtocol
  upstream_model: string
  wire_profile: ProviderWireProfile
  feature_profile: ProviderFeatureProfile
  endpoint_id: number | null
  enabled: boolean
  version: number
}
export interface Provider {
  id: number
  name: string
  status: ProviderStatus
  allow_protocol_conversion: boolean
  base_url: string
  headers: Record<string, string>
  auth_type: string
  credential_configured: boolean
  group_ids: number[]
  concurrency: number
  rate_multiplier: number
  version: number
  endpoints: ProviderEndpoint[]
  capabilities: ProviderCapability[]
  created_at: string
  updated_at: string
}
export interface ProviderWriteRequest {
  name: string
  base_url: string
  auth_type?: string
  api_key?: string
  clear_api_key?: boolean
  allow_protocol_conversion: boolean
  group_ids: number[]
  concurrency?: number
  rate_multiplier?: number
  endpoints: ProviderEndpointWriteRequest[]
  capabilities: ProviderCapabilityWriteRequest[]
  version?: number
}
export interface ProviderEndpointWriteRequest {
  protocol: ProviderProtocol
  wire_profile?: ProviderWireProfile
  base_url?: string
  path?: string
  headers?: Record<string, string>
  auth_type?: string
  enabled?: boolean
}
export interface ProviderCapabilityWriteRequest {
  logical_model: string
  logical_model_display?: string
  protocol: ProviderProtocol
  upstream_model: string
  wire_profile?: ProviderWireProfile
  feature_profile: ProviderFeatureProfile
  enabled?: boolean
}
export interface GroupProviderCapability {
  provider_id: number
  provider_name: string
  logical_model: string
  ingress_protocol: ProviderProtocol
  upstream_protocol: ProviderProtocol
  tier: 'native' | 'conversion'
  adapter?: string
  adapter_version?: string
  group_priority: number
}
export interface ProviderRouteAttempt {
  trace_id: string
  route_identity: string
  group_id: number
  provider_id: number
  capability_id: number
  endpoint_id: number
  logical_model: string
  upstream_model: string
  ingress_protocol: ProviderProtocol
  upstream_protocol: ProviderProtocol
  tier: 'native' | 'conversion'
  outcome: 'succeeded' | 'failed' | 'rejected'
  status_code: number
  failure_category: string
  upstream_request_id: string
  wire_profile: string
  conversion_used: boolean
  bytes_committed: number
  final_reason: string
  started_at: string
  duration_ms: number
}
export interface GroupRouteSnapshot {
  id: number
  group_id: number
  version: number
  status: 'review_required' | 'approved' | 'active' | 'superseded' | 'draft'
  manifest: Record<string, unknown>
  shadow_diff: Record<string, unknown>
  approved_by: number | null
  approved_at: string | null
  created_at: string
  updated_at: string
}

export async function list(): Promise<Provider[]> { const { data } = await apiClient.get<Provider[]>('/admin/providers'); return data }
export async function getById(id: number): Promise<Provider> { const { data } = await apiClient.get<Provider>(`/admin/providers/${id}`); return data }
export async function create(input: ProviderWriteRequest): Promise<Provider> { const { data } = await apiClient.post<Provider>('/admin/providers', input); return data }
export async function update(id: number, input: ProviderWriteRequest): Promise<Provider> { const { data } = await apiClient.put<Provider>(`/admin/providers/${id}`, input); return data }
export async function test(id: number, input: { capability_id?: number; protocol?: ProviderProtocol; logical_model?: string }) { const { data } = await apiClient.post(`/admin/providers/${id}/test`, input); return data as { status_code: number; duration: number; upstream_request_id: string } }
export async function activate(id: number, version: number): Promise<Provider> { const { data } = await apiClient.post<Provider>(`/admin/providers/${id}/activate`, { version }); return data }
export async function disable(id: number, version: number): Promise<Provider> { const { data } = await apiClient.post<Provider>(`/admin/providers/${id}/disable`, { version }); return data }
export async function listGroupCapabilities(groupId: number): Promise<GroupProviderCapability[]> { const { data } = await apiClient.get<GroupProviderCapability[]>(`/admin/groups/${groupId}/provider-capabilities`); return data }
export async function listRouteAttempts(filters: { group_id?: number; provider_id?: number; logical_model?: string; ingress_protocol?: ProviderProtocol | ''; upstream_protocol?: ProviderProtocol | ''; tier?: 'native' | 'conversion' | ''; outcome?: string; limit?: number } = {}): Promise<ProviderRouteAttempt[]> { const { data } = await apiClient.get<ProviderRouteAttempt[]>('/admin/providers/route-attempts', { params: filters }); return data }
export async function listSnapshots(groupId: number): Promise<GroupRouteSnapshot[]> { const { data } = await apiClient.get<GroupRouteSnapshot[]>(`/admin/groups/${groupId}/provider-route-snapshots`); return data }
export async function createShadowSnapshot(groupId: number): Promise<GroupRouteSnapshot> { const { data } = await apiClient.post<GroupRouteSnapshot>(`/admin/groups/${groupId}/provider-route-snapshots/shadow`); return data }
export async function approveSnapshot(groupId: number, version: number): Promise<GroupRouteSnapshot> { const { data } = await apiClient.post<GroupRouteSnapshot>(`/admin/groups/${groupId}/provider-route-snapshots/${version}/approve`); return data }
export async function activateSnapshot(groupId: number, version: number): Promise<{ group_id: number; active_version: number; previous_version: number }> { const { data } = await apiClient.post(`/admin/groups/${groupId}/provider-route-snapshots/${version}/activate`); return data }
export async function rollbackSnapshot(groupId: number): Promise<{ group_id: number; active_version: number; previous_version: number }> { const { data } = await apiClient.post(`/admin/groups/${groupId}/provider-route-snapshots/rollback`); return data }

export default { list, getById, create, update, test, activate, disable, listGroupCapabilities, listRouteAttempts, listSnapshots, createShadowSnapshot, approveSnapshot, activateSnapshot, rollbackSnapshot }

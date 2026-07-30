/**
 * 用户开发者访问密钥接口。
 *
 * 该密钥用于调用 `/api/v1/openapi` 下的用户级开发者接口，
 * 与当前网页登录使用的 JWT 完全独立。
 */

import { apiClient } from './client'

export interface UserAccessKey {
  key: string
  exists: boolean
  available: boolean
  created_at?: string | null
}

/**
 * 读取当前用户的开发者访问密钥。
 * 已生成过的密钥会由服务端返回完整值，以便用户再次复制。
 */
export async function get(): Promise<UserAccessKey> {
  const { data } = await apiClient.get<UserAccessKey>('/user/access-key')
  return data
}

/**
 * 为当前用户生成开发者访问密钥。
 * 服务端在密钥已存在时返回原有值，不会隐式轮换密钥。
 */
export async function generate(): Promise<UserAccessKey> {
  const { data } = await apiClient.post<UserAccessKey>('/user/access-key')
  return data
}

export const userAccessKeyAPI = {
  get,
  generate
}

export default userAccessKeyAPI

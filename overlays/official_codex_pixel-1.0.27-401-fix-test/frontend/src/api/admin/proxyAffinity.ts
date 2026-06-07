import { apiClient } from '../client'

export interface ProxyAffinitySettings {
  enabled: boolean
  user_owned_enabled: boolean
  admin_accounts_enabled: boolean
  private_accounts_enabled: boolean
  public_accounts_enabled: boolean
  only_approved_public_accounts: boolean
  include_api_key_accounts: boolean
  include_oauth_accounts: boolean
  max_accounts_per_proxy: number
  batch_size: number
  scan_interval_minutes: number
  platforms: string[]
  allow_reassign_when_proxy_down: boolean
  release_when_account_inactive: boolean
}

export interface ProxyAffinityProxyLoad {
  proxy_id: number
  name: string
  protocol: string
  host: string
  port: number
  status: string
  account_count: number
  max_accounts: number
  assignable: boolean
  load_percent: number
  ip_address?: string
  country?: string
  country_code?: string
  quality_status?: string
  quality_grade?: string
}

export interface ProxyAffinityOverview {
  settings: ProxyAffinitySettings
  total_proxies: number
  available_proxies: number
  full_proxies: number
  bound_accounts: number
  unassigned_eligible_accounts: number
  skipped_accounts: number
  average_load: number
  proxy_loads: ProxyAffinityProxyLoad[]
}

export interface ProxyAffinityCandidate {
  account_id: number
  account_name: string
  platform: string
  type: string
  share_mode: string
  share_status: string
  account_level: string
  owner_user_id?: number
}

export interface ProxyAffinityAssignment {
  candidate: ProxyAffinityCandidate
  proxy_id?: number
  proxy_name?: string
  action: 'assigned' | 'released' | 'skipped' | 'failed'
  reason: string
  dry_run: boolean
}

export interface ProxyAffinityAssignRequest {
  dry_run?: boolean
  limit?: number
  platforms?: string[]
}

export interface ProxyAffinityAssignResult {
  dry_run: boolean
  scanned: number
  assigned: number
  released: number
  skipped: number
  assignments: ProxyAffinityAssignment[]
}

export async function getSettings(): Promise<ProxyAffinitySettings> {
  const { data } = await apiClient.get<ProxyAffinitySettings>('/admin/proxy-affinity/settings')
  return data
}

export async function updateSettings(payload: ProxyAffinitySettings): Promise<ProxyAffinitySettings> {
  const { data } = await apiClient.put<ProxyAffinitySettings>('/admin/proxy-affinity/settings', payload)
  return data
}

export async function getOverview(): Promise<ProxyAffinityOverview> {
  const { data } = await apiClient.get<ProxyAffinityOverview>('/admin/proxy-affinity/overview')
  return data
}

export async function assign(payload: ProxyAffinityAssignRequest): Promise<ProxyAffinityAssignResult> {
  const { data } = await apiClient.post<ProxyAffinityAssignResult>('/admin/proxy-affinity/assign', payload)
  return data
}

export const proxyAffinityAPI = {
  getSettings,
  updateSettings,
  getOverview,
  assign
}

export default proxyAffinityAPI

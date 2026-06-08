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
  strategy: 'least_loaded' | 'weighted_least_loaded'
  max_stored_events: number
  paused_proxy_ids: number[]
  proxy_weights: Record<number, number>
  pre_validation_enabled: boolean
  enforce_validation_proxy: boolean
  include_pending_accounts: boolean
  release_on_validation_failure: boolean
  retry_with_new_proxy_on_failure: boolean
  max_pre_validation_retries: number
  fallback_when_no_proxy: 'wait' | 'direct' | 'reject'
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
  paused: boolean
  weight: number
  effective_load: number
  reason?: string
}

export interface ProxyAffinityOverview {
  settings: ProxyAffinitySettings
  total_proxies: number
  available_proxies: number
  full_proxies: number
  bound_accounts: number
  unassigned_eligible_accounts: number
  pre_validation_accounts: number
  waiting_proxy_accounts: number
  validation_failed_accounts: number
  skipped_accounts: number
  average_load: number
  proxy_loads: ProxyAffinityProxyLoad[]
  bound_account_details: ProxyAffinityAccountBinding[]
  pending_accounts: ProxyAffinityPendingAccount[]
  recent_events: ProxyAffinityEvent[]
  last_run_at?: string
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

export interface ProxyAffinityAccountBinding extends ProxyAffinityCandidate {
  proxy_id: number
  proxy_name?: string
  proxy_host?: string
  proxy_port?: number
  assigned_at?: string
  assigned_by?: string
  assign_reason?: string
  phase?: string
  last_test_at?: string
  last_test_error?: string
  health_status: 'healthy' | 'proxy_down' | 'proxy_paused' | 'proxy_missing' | 'account_ineligible' | string
  health_reason?: string
}

export interface ProxyAffinityPendingAccount extends ProxyAffinityCandidate {
  reason: string
  phase?: string
  last_test_at?: string
  last_test_error?: string
}

export interface ProxyAffinityEvent {
  id: string
  occurred_at: string
  source: string
  action: string
  account_id?: number
  account_name?: string
  proxy_id?: number
  proxy_name?: string
  reason?: string
  dry_run: boolean
  details?: Record<string, unknown>
}

export interface ProxyAffinityAssignRequest {
  dry_run?: boolean
  limit?: number
  platforms?: string[]
}

export type ProxyAffinityPrebindRequest = ProxyAffinityAssignRequest

export interface ProxyAffinityAssignResult {
  dry_run: boolean
  scanned: number
  assigned: number
  released: number
  skipped: number
  assignments: ProxyAffinityAssignment[]
}

export interface ProxyAffinityBindRequest {
  account_id: number
  proxy_id: number
  dry_run?: boolean
  reason?: string
}

export interface ProxyAffinityReleaseRequest {
  account_id: number
  dry_run?: boolean
  reason?: string
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

export async function prebind(payload: ProxyAffinityPrebindRequest): Promise<ProxyAffinityAssignResult> {
  const { data } = await apiClient.post<ProxyAffinityAssignResult>('/admin/proxy-affinity/prebind', payload)
  return data
}

export async function bindAccount(payload: ProxyAffinityBindRequest): Promise<ProxyAffinityAssignment> {
  const { data } = await apiClient.post<ProxyAffinityAssignment>('/admin/proxy-affinity/bind', payload)
  return data
}

export async function releaseAccount(payload: ProxyAffinityReleaseRequest): Promise<ProxyAffinityAssignment> {
  const { data } = await apiClient.post<ProxyAffinityAssignment>('/admin/proxy-affinity/release', payload)
  return data
}

export const proxyAffinityAPI = {
  getSettings,
  updateSettings,
  getOverview,
  assign,
  prebind,
  bindAccount,
  releaseAccount
}

export default proxyAffinityAPI

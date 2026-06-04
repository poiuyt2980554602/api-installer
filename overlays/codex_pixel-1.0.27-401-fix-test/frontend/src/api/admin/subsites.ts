import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface Subsite {
  id: number
  subsite_id: string
  name: string
  public_url: string
  region: string
  capabilities: string[]
  status: string
  max_qps: number
  max_concurrency: number
  version: string
  last_heartbeat_at?: string
  health_score: number
  last_seen_ip: string
  metadata?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface AccountLease {
  id: number
  lease_id: string
  subsite_id: string
  account_id: number
  group_id?: number | null
  group_name?: string | null
  account_name?: string | null
  platform: string
  status: string
  max_concurrency: number
  max_requests: number
  max_tokens: number
  used_requests: number
  used_tokens: number
  assigned_at: string
  expires_at: string
  renewed_at?: string
  released_at?: string
  created_at: string
  updated_at: string
}

export interface SubsiteForwardSiteStat {
  subsite_id: string
  name: string
  status: string
  effective_status: string
  load_level: string
  health_score: number
  last_heartbeat_at?: string
  active_requests: number
  queued_usage: number
  qps: number
  cpu_percent: number
  memory_bytes: number
  active_leases: number
  expiring_leases_24h: number
  events_24h: number
  failures_24h: number
  avg_latency_ms_24h: number
  p95_latency_ms_24h: number
  p99_latency_ms_24h: number
  success_rate_24h: number
  forwarded_tokens_24h: number
  forwarded_cost_24h: number
  cache_read_tokens_24h: number
  cache_hit_ratio_24h: number
  avg_first_token_ms_24h: number
  affinities: number
  locked_affinities: number
  circuit_open: boolean
  circuit_reason: string
  cooldown_until?: string
}

export interface SubsiteCircuitBreaker {
  id: number
  scope: string
  target_id: string
  subsite_id: string
  account_id?: number
  lease_id?: string
  reason: string
  failures: number
  cooldown_until: string
  last_error: string
  created_at: string
  updated_at: string
}

export interface SubsiteRelayLeaseStat {
  group_id: number
  group_name: string
  platform: string
  scope: string
  required_level: string
  active_leases: number
  assigned_subsites: number
  expiring_leases_1h: number
  expiring_leases_24h: number
}

export interface SubsiteRelayPoolStat {
  group_id: number
  group_name: string
  platform: string
  scope: string
  required_level: string
  total_accounts: number
  relay_eligible_accounts: number
  master_direct_accounts: number
  local_only_accounts: number
  schedulable_accounts: number
  unschedulable_accounts: number
  pending_accounts: number
  suspended_accounts: number
  rate_limited_accounts: number
  temp_blocked_accounts: number
  expired_accounts: number
  unknown_level_accounts: number
  level_mismatch_accounts: number
  proxy_bound_accounts: number
  proxy_missing_accounts: number
  leased_accounts: number
  unleased_accounts: number
  assigned_subsites: number
  active_leases: number
  blocked_reason: string
}

export interface SubsiteRelayAccountStat {
  account_id: number
  account_name: string
  platform: string
  account_level: string
  share_mode: string
  share_status: string
  status: string
  schedulable: boolean
  group_id: number
  group_name: string
  group_scope: string
  required_level: string
  route_policy: string
  route_resolved: string
  route_reason: string
  proxy_id?: number
  proxy_name?: string
  proxy_protocol?: string
  proxy_host?: string
  proxy_port?: number
  distributed: boolean
  distributable: boolean
  subsite_id?: string
  subsite_name?: string
  lease_id?: string
  lease_status?: string
  reason_code: string
  reason: string
  updated_at: string
  lease_expires_at?: string
}

export interface SubsiteRelayConfigCheck {
  code: string
  status: string
  severity: string
  message: string
}

export interface SubsiteRelayAutomationSummary {
  ready: boolean
  config_ok: boolean
  online_subsites: number
  public_pool_accounts: number
  private_pool_accounts: number
  schedulable_accounts: number
  leased_accounts: number
  unleased_accounts: number
  pending_accounts: number
  blocked_accounts: number
}

export interface SubsiteRelayAdvice {
  code: string
  severity: string
  message: string
}

export interface SubsiteRelayDistributionSkip {
  account_id: number
  account_name: string
  group_id: number
  group_name: string
  reason_code: string
  reason: string
}

export interface SubsiteRelayDistributionRunResult {
  created_leases: AccountLease[]
  skipped_accounts: SubsiteRelayDistributionSkip[]
  released_invalid_leases: number
  online_subsites: number
  candidate_accounts: number
  created_count: number
  skipped_count: number
}

export interface SubsiteForwardStats {
  total_affinities: number
  locked_affinities: number
  active_affinities: number
  account_affinities: number
  events_24h: number
  failures_24h: number
  failovers_24h: number
  circuit_open: number
  total_subsites: number
  online_subsites: number
  degraded_subsites: number
  offline_subsites: number
  active_leases: number
  expiring_leases_24h: number
  avg_latency_ms_24h: number
  p95_latency_ms_24h: number
  p99_latency_ms_24h: number
  success_rate_24h: number
  forwarded_tokens_24h: number
  forwarded_cost_24h: number
  cache_read_tokens_24h: number
  cache_hit_ratio_24h: number
  avg_first_token_ms_24h: number
  mode: SubsiteForwardMode
  by_subsite: SubsiteForwardSiteStat[]
  circuit_breakers: SubsiteCircuitBreaker[]
  lease_distribution: SubsiteRelayLeaseStat[]
  pool_distribution: SubsiteRelayPoolStat[]
  account_distribution: SubsiteRelayAccountStat[]
  configuration_checks: SubsiteRelayConfigCheck[]
  automation_summary: SubsiteRelayAutomationSummary
  recommendations: SubsiteRelayAdvice[]
}

export type SubsiteForwardMode = 'local' | 'forward' | 'direct'

export interface SubsiteForwardAffinity {
  id: number
  affinity_key: string
  affinity_type: string
  subsite_id: string
  lease_id?: string
  account_id?: number
  api_key_id?: number
  user_id?: number
  group_id?: number
  model: string
  session_id: string
  source: string
  locked: boolean
  hits: number
  last_reason: string
  last_error: string
  expires_at: string
  last_used_at: string
  created_at: string
  updated_at: string
}

export interface SubsiteForwardEvent {
  id: number
  request_id: string
  affinity_key: string
  subsite_id: string
  attempted_subsite_id: string
  fallback_from: string
  lease_id?: string
  account_id?: number
  api_key_id?: number
  user_id?: number
  group_id?: number
  model: string
  session_id: string
  method: string
  path: string
  status_code: number
  latency_ms: number
  request_bytes: number
  response_bytes: number
  reason: string
  outcome: string
  error: string
  metadata?: Record<string, unknown>
  created_at: string
}

export interface UpsertForwardAffinityRequest {
  affinity_key: string
  affinity_type?: string
  subsite_id: string
  lease_id?: string
  account_id?: number
  api_key_id?: number
  user_id?: number
  group_id?: number
  model?: string
  session_id?: string
  ttl_seconds?: number
  locked?: boolean
}

export interface CreateSubsiteRequest {
  subsite_id?: string
  name: string
  public_url: string
  region?: string
  capabilities?: string[]
  max_qps?: number
  max_concurrency?: number
  version?: string
}

export interface CreateSubsiteResult {
  subsite: Subsite
  secret: string
}

export interface ResetSubsiteSecretResult {
  subsite: Subsite
  secret: string
}

export interface UpdateSubsiteRequest {
  name?: string
  public_url?: string
  region?: string
  capabilities?: string[]
  max_qps?: number
  max_concurrency?: number
  version?: string
  metadata?: Record<string, unknown>
}

export interface CreateLeaseRequest {
  account_id: number
  group_id: number
  max_concurrency?: number
  max_requests?: number
  max_tokens?: number
  ttl_seconds?: number
}

export interface RenewLeaseRequest {
  ttl_seconds: number
}

export interface UpdateLeaseRequest {
  max_concurrency?: number
  max_requests?: number
  max_tokens?: number
}

export async function list(
  page = 1,
  pageSize = 20,
  filters?: { status?: string; search?: string }
): Promise<PaginatedResponse<Subsite>> {
  const { data } = await apiClient.get<PaginatedResponse<Subsite>>('/admin/subsites', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    }
  })
  return data
}

export async function create(payload: CreateSubsiteRequest): Promise<CreateSubsiteResult> {
  const { data } = await apiClient.post<CreateSubsiteResult>('/admin/subsites', payload)
  return data
}

export async function update(subsiteID: string, payload: UpdateSubsiteRequest): Promise<Subsite> {
  const { data } = await apiClient.patch<Subsite>(`/admin/subsites/${subsiteID}`, payload)
  return data
}

export async function activate(subsiteID: string): Promise<{ status: string }> {
  const { data } = await apiClient.post<{ status: string }>(`/admin/subsites/${subsiteID}/activate`)
  return data
}

export async function pause(subsiteID: string): Promise<{ status: string }> {
  const { data } = await apiClient.post<{ status: string }>(`/admin/subsites/${subsiteID}/pause`)
  return data
}

export async function resume(subsiteID: string): Promise<{ status: string }> {
  const { data } = await apiClient.post<{ status: string }>(`/admin/subsites/${subsiteID}/resume`)
  return data
}

export async function resetSecret(subsiteID: string): Promise<ResetSubsiteSecretResult> {
  const { data } = await apiClient.post<ResetSubsiteSecretResult>(`/admin/subsites/${subsiteID}/reset-secret`)
  return data
}

export async function listLeases(
  subsiteID: string,
  page = 1,
  pageSize = 20
): Promise<PaginatedResponse<AccountLease>> {
  const { data } = await apiClient.get<PaginatedResponse<AccountLease>>(`/admin/subsites/${subsiteID}/leases`, {
    params: {
      page,
      page_size: pageSize
    }
  })
  return data
}

export async function listLeaseActiveAccountIds(subsiteID: string): Promise<number[]> {
  const { data } = await apiClient.get<{ account_ids: number[] }>(`/admin/subsites/${subsiteID}/leases/active-account-ids`)
  return data.account_ids || []
}

export async function createLease(subsiteID: string, payload: CreateLeaseRequest): Promise<AccountLease> {
  const { data } = await apiClient.post<AccountLease>(`/admin/subsites/${subsiteID}/leases`, payload)
  return data
}

export async function drainLease(subsiteID: string, leaseID: string): Promise<AccountLease> {
  const { data } = await apiClient.post<AccountLease>(`/admin/subsites/${subsiteID}/leases/${leaseID}/drain`)
  return data
}

export async function releaseLease(subsiteID: string, leaseID: string): Promise<AccountLease> {
  const { data } = await apiClient.post<AccountLease>(`/admin/subsites/${subsiteID}/leases/${leaseID}/release`)
  return data
}

export async function renewLease(subsiteID: string, leaseID: string, payload: RenewLeaseRequest): Promise<AccountLease> {
  const { data } = await apiClient.post<AccountLease>(`/admin/subsites/${subsiteID}/leases/${leaseID}/renew`, payload)
  return data
}

export async function updateLease(subsiteID: string, leaseID: string, payload: UpdateLeaseRequest): Promise<AccountLease> {
  const { data } = await apiClient.patch<AccountLease>(`/admin/subsites/${subsiteID}/leases/${leaseID}`, payload)
  return data
}

export async function deleteLease(subsiteID: string, leaseID: string): Promise<void> {
  await apiClient.delete(`/admin/subsites/${subsiteID}/leases/${leaseID}`)
}

export async function forwardStats(): Promise<SubsiteForwardStats> {
  const { data } = await apiClient.get<SubsiteForwardStats>('/admin/subsites/forward-stats')
  return data
}

export async function autoDistribute(): Promise<SubsiteRelayDistributionRunResult> {
  const { data } = await apiClient.post<SubsiteRelayDistributionRunResult>('/admin/subsites/auto-distribute')
  return data
}

export async function updateForwardMode(mode: SubsiteForwardMode): Promise<{ mode: SubsiteForwardMode }> {
  const { data } = await apiClient.put<{ mode: SubsiteForwardMode }>('/admin/subsites/forward-mode', { mode })
  return data
}

export async function listForwardAffinities(
  page = 1,
  pageSize = 20,
  filters?: { subsite_id?: string; search?: string; locked?: boolean; api_key_id?: number; account_id?: number }
): Promise<PaginatedResponse<SubsiteForwardAffinity>> {
  const { data } = await apiClient.get<PaginatedResponse<SubsiteForwardAffinity>>('/admin/subsites/forward-affinities', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    }
  })
  return data
}

export async function upsertForwardAffinity(payload: UpsertForwardAffinityRequest): Promise<SubsiteForwardAffinity> {
  const { data } = await apiClient.post<SubsiteForwardAffinity>('/admin/subsites/forward-affinities', payload)
  return data
}

export async function deleteForwardAffinity(affinityID: number): Promise<void> {
  await apiClient.delete(`/admin/subsites/forward-affinities/${affinityID}`)
}

export async function listForwardEvents(
  page = 1,
  pageSize = 20,
  filters?: { subsite_id?: string; outcome?: string; search?: string }
): Promise<PaginatedResponse<SubsiteForwardEvent>> {
  const { data } = await apiClient.get<PaginatedResponse<SubsiteForwardEvent>>('/admin/subsites/forward-events', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    }
  })
  return data
}

export default {
  list,
  create,
  update,
  activate,
  pause,
  resume,
  resetSecret,
  listLeases,
  listLeaseActiveAccountIds,
  createLease,
  drainLease,
  releaseLease,
  renewLease,
  updateLease,
  deleteLease,
  forwardStats,
  autoDistribute,
  updateForwardMode,
  listForwardAffinities,
  upsertForwardAffinity,
  deleteForwardAffinity,
  listForwardEvents
}

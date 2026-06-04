package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

type SubsiteForwardAffinity struct {
	ID         int64      `json:"id"`
	Key        string     `json:"affinity_key"`
	Type       string     `json:"affinity_type"`
	SubsiteID  string     `json:"subsite_id"`
	LeaseID    string     `json:"lease_id,omitempty"`
	AccountID  int64      `json:"account_id,omitempty"`
	APIKeyID   int64      `json:"api_key_id,omitempty"`
	UserID     int64      `json:"user_id,omitempty"`
	GroupID    int64      `json:"group_id,omitempty"`
	Model      string     `json:"model"`
	SessionID  string     `json:"session_id"`
	Source     string     `json:"source"`
	Locked     bool       `json:"locked"`
	Hits       int64      `json:"hits"`
	LastReason string     `json:"last_reason"`
	LastError  string     `json:"last_error"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt time.Time  `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
}

type SubsiteForwardEvent struct {
	ID                 int64          `json:"id"`
	RequestID          string         `json:"request_id"`
	AffinityKey        string         `json:"affinity_key"`
	SubsiteID          string         `json:"subsite_id"`
	AttemptedSubsiteID string         `json:"attempted_subsite_id"`
	FallbackFrom       string         `json:"fallback_from"`
	LeaseID            string         `json:"lease_id,omitempty"`
	AccountID          int64          `json:"account_id,omitempty"`
	APIKeyID           int64          `json:"api_key_id,omitempty"`
	UserID             int64          `json:"user_id,omitempty"`
	GroupID            int64          `json:"group_id,omitempty"`
	Model              string         `json:"model"`
	SessionID          string         `json:"session_id"`
	Method             string         `json:"method"`
	Path               string         `json:"path"`
	StatusCode         int            `json:"status_code"`
	LatencyMS          int64          `json:"latency_ms"`
	RequestBytes       int64          `json:"request_bytes"`
	ResponseBytes      int64          `json:"response_bytes"`
	Reason             string         `json:"reason"`
	Outcome            string         `json:"outcome"`
	Error              string         `json:"error"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at"`
}

type SubsiteForwardStats struct {
	TotalAffinities     int64                         `json:"total_affinities"`
	LockedAffinity      int64                         `json:"locked_affinities"`
	ActiveAffinity      int64                         `json:"active_affinities"`
	AccountAffinity     int64                         `json:"account_affinities"`
	Events24h           int64                         `json:"events_24h"`
	Failures24h         int64                         `json:"failures_24h"`
	Failovers24h        int64                         `json:"failovers_24h"`
	CircuitOpen         int64                         `json:"circuit_open"`
	TotalSubsites       int64                         `json:"total_subsites"`
	OnlineSubsites      int64                         `json:"online_subsites"`
	DegradedSubsites    int64                         `json:"degraded_subsites"`
	OfflineSubsites     int64                         `json:"offline_subsites"`
	ActiveLeases        int64                         `json:"active_leases"`
	ExpiringLeases24h   int64                         `json:"expiring_leases_24h"`
	AvgLatencyMS24h     float64                       `json:"avg_latency_ms_24h"`
	P95LatencyMS24h     float64                       `json:"p95_latency_ms_24h"`
	P99LatencyMS24h     float64                       `json:"p99_latency_ms_24h"`
	SuccessRate24h      float64                       `json:"success_rate_24h"`
	ForwardedTokens24h  int64                         `json:"forwarded_tokens_24h"`
	ForwardedCost24h    float64                       `json:"forwarded_cost_24h"`
	CacheReadTokens24h  int64                         `json:"cache_read_tokens_24h"`
	CacheHitRatio24h    float64                       `json:"cache_hit_ratio_24h"`
	AvgFirstTokenMS24h  float64                       `json:"avg_first_token_ms_24h"`
	Mode                string                        `json:"mode"`
	BySubsite           []SubsiteForwardSiteStat      `json:"by_subsite"`
	CircuitBreakers     []SubsiteCircuitBreaker       `json:"circuit_breakers"`
	LeaseDistribution   []SubsiteRelayLeaseStat       `json:"lease_distribution"`
	PoolDistribution    []SubsiteRelayPoolStat        `json:"pool_distribution"`
	AccountDistribution []SubsiteRelayAccountStat     `json:"account_distribution"`
	ConfigChecks        []SubsiteRelayConfigCheck     `json:"configuration_checks"`
	Automation          SubsiteRelayAutomationSummary `json:"automation_summary"`
	Recommendations     []SubsiteRelayAdvice          `json:"recommendations"`
}

type SubsiteRelayLeaseStat struct {
	GroupID           int64  `json:"group_id"`
	GroupName         string `json:"group_name"`
	Platform          string `json:"platform"`
	Scope             string `json:"scope"`
	RequiredLevel     string `json:"required_level"`
	ActiveLeases      int64  `json:"active_leases"`
	AssignedSubsites  int64  `json:"assigned_subsites"`
	ExpiringLeases1h  int64  `json:"expiring_leases_1h"`
	ExpiringLeases24h int64  `json:"expiring_leases_24h"`
}

type SubsiteRelayPoolStat struct {
	GroupID               int64  `json:"group_id"`
	GroupName             string `json:"group_name"`
	Platform              string `json:"platform"`
	Scope                 string `json:"scope"`
	RequiredLevel         string `json:"required_level"`
	TotalAccounts         int64  `json:"total_accounts"`
	RelayEligibleAccounts int64  `json:"relay_eligible_accounts"`
	MasterDirectAccounts  int64  `json:"master_direct_accounts"`
	LocalOnlyAccounts     int64  `json:"local_only_accounts"`
	SchedulableAccounts   int64  `json:"schedulable_accounts"`
	UnschedulableAccounts int64  `json:"unschedulable_accounts"`
	PendingAccounts       int64  `json:"pending_accounts"`
	SuspendedAccounts     int64  `json:"suspended_accounts"`
	RateLimitedAccounts   int64  `json:"rate_limited_accounts"`
	TempBlockedAccounts   int64  `json:"temp_blocked_accounts"`
	ExpiredAccounts       int64  `json:"expired_accounts"`
	UnknownLevelAccounts  int64  `json:"unknown_level_accounts"`
	LevelMismatchAccounts int64  `json:"level_mismatch_accounts"`
	ProxyBoundAccounts    int64  `json:"proxy_bound_accounts"`
	ProxyMissingAccounts  int64  `json:"proxy_missing_accounts"`
	LeasedAccounts        int64  `json:"leased_accounts"`
	UnleasedAccounts      int64  `json:"unleased_accounts"`
	AssignedSubsites      int64  `json:"assigned_subsites"`
	ActiveLeases          int64  `json:"active_leases"`
	BlockedReason         string `json:"blocked_reason"`
}

type SubsiteRelayAccountStat struct {
	AccountID      int64      `json:"account_id"`
	AccountName    string     `json:"account_name"`
	Platform       string     `json:"platform"`
	AccountLevel   string     `json:"account_level"`
	ShareMode      string     `json:"share_mode"`
	ShareStatus    string     `json:"share_status"`
	Status         string     `json:"status"`
	Schedulable    bool       `json:"schedulable"`
	GroupID        int64      `json:"group_id"`
	GroupName      string     `json:"group_name"`
	GroupScope     string     `json:"group_scope"`
	RequiredLevel  string     `json:"required_level"`
	RoutePolicy    string     `json:"route_policy"`
	RouteResolved  string     `json:"route_resolved"`
	RouteReason    string     `json:"route_reason"`
	ProxyID        int64      `json:"proxy_id,omitempty"`
	ProxyName      string     `json:"proxy_name,omitempty"`
	ProxyProtocol  string     `json:"proxy_protocol,omitempty"`
	ProxyHost      string     `json:"proxy_host,omitempty"`
	ProxyPort      int        `json:"proxy_port,omitempty"`
	Distributed    bool       `json:"distributed"`
	Distributable  bool       `json:"distributable"`
	SubsiteID      string     `json:"subsite_id,omitempty"`
	SubsiteName    string     `json:"subsite_name,omitempty"`
	LeaseID        string     `json:"lease_id,omitempty"`
	LeaseStatus    string     `json:"lease_status,omitempty"`
	ReasonCode     string     `json:"reason_code"`
	Reason         string     `json:"reason"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
}

type SubsiteRelayConfigCheck struct {
	Code     string `json:"code"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type SubsiteRelayAutomationSummary struct {
	Ready               bool  `json:"ready"`
	ConfigOK            bool  `json:"config_ok"`
	OnlineSubsites      int64 `json:"online_subsites"`
	PublicPoolAccounts  int64 `json:"public_pool_accounts"`
	PrivatePoolAccounts int64 `json:"private_pool_accounts"`
	SchedulableAccounts int64 `json:"schedulable_accounts"`
	LeasedAccounts      int64 `json:"leased_accounts"`
	UnleasedAccounts    int64 `json:"unleased_accounts"`
	PendingAccounts     int64 `json:"pending_accounts"`
	BlockedAccounts     int64 `json:"blocked_accounts"`
}

type SubsiteRelayAdvice struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type SubsiteRelayDistributionRunResult struct {
	CreatedLeases         []AccountLease                 `json:"created_leases"`
	SkippedAccounts       []SubsiteRelayDistributionSkip `json:"skipped_accounts"`
	ReleasedInvalidLeases int64                          `json:"released_invalid_leases"`
	OnlineSubsites        int                            `json:"online_subsites"`
	CandidateAccounts     int                            `json:"candidate_accounts"`
	CreatedCount          int                            `json:"created_count"`
	SkippedCount          int                            `json:"skipped_count"`
}

type SubsiteRelayDistributionSkip struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	GroupID     int64  `json:"group_id"`
	GroupName   string `json:"group_name"`
	ReasonCode  string `json:"reason_code"`
	Reason      string `json:"reason"`
}

type SubsiteForwardCleanupResult struct {
	ExpiredAffinities int64 `json:"expired_affinities"`
	ExpiredBreakers   int64 `json:"expired_breakers"`
	DeletedEvents     int64 `json:"deleted_events"`
	DeletedSamples    int64 `json:"deleted_samples"`
}

type SubsiteForwardSiteStat struct {
	SubsiteID          string     `json:"subsite_id"`
	Name               string     `json:"name"`
	Status             string     `json:"status"`
	EffectiveStatus    string     `json:"effective_status"`
	LoadLevel          string     `json:"load_level"`
	HealthScore        int        `json:"health_score"`
	LastHeartbeatAt    *time.Time `json:"last_heartbeat_at,omitempty"`
	ActiveRequests     int        `json:"active_requests"`
	QueuedUsage        int        `json:"queued_usage"`
	QPS                float64    `json:"qps"`
	CPUPercent         float64    `json:"cpu_percent"`
	MemoryBytes        int64      `json:"memory_bytes"`
	ActiveLeases       int64      `json:"active_leases"`
	ExpiringLeases24h  int64      `json:"expiring_leases_24h"`
	Events24h          int64      `json:"events_24h"`
	Failures24h        int64      `json:"failures_24h"`
	AvgLatencyMS24h    float64    `json:"avg_latency_ms_24h"`
	P95LatencyMS24h    float64    `json:"p95_latency_ms_24h"`
	P99LatencyMS24h    float64    `json:"p99_latency_ms_24h"`
	SuccessRate24h     float64    `json:"success_rate_24h"`
	ForwardedTokens24h int64      `json:"forwarded_tokens_24h"`
	ForwardedCost24h   float64    `json:"forwarded_cost_24h"`
	CacheReadTokens24h int64      `json:"cache_read_tokens_24h"`
	CacheHitRatio24h   float64    `json:"cache_hit_ratio_24h"`
	AvgFirstTokenMS24h float64    `json:"avg_first_token_ms_24h"`
	Affinities         int64      `json:"affinities"`
	LockedAffinity     int64      `json:"locked_affinities"`
	CircuitOpen        bool       `json:"circuit_open"`
	CircuitReason      string     `json:"circuit_reason"`
	CooldownUntil      *time.Time `json:"cooldown_until,omitempty"`
}

type SubsiteCircuitBreaker struct {
	ID            int64     `json:"id"`
	Scope         string    `json:"scope"`
	TargetID      string    `json:"target_id"`
	SubsiteID     string    `json:"subsite_id"`
	AccountID     int64     `json:"account_id,omitempty"`
	LeaseID       string    `json:"lease_id,omitempty"`
	Reason        string    `json:"reason"`
	Failures      int       `json:"failures"`
	CooldownUntil time.Time `json:"cooldown_until"`
	LastError     string    `json:"last_error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UpsertSubsiteForwardAffinityInput struct {
	Key        string
	Type       string
	SubsiteID  string
	LeaseID    string
	AccountID  int64
	APIKeyID   int64
	UserID     int64
	GroupID    int64
	Model      string
	SessionID  string
	Source     string
	Locked     bool
	LastReason string
	LastError  string
	ExpiresAt  time.Time
}

type ListSubsiteForwardAffinitiesFilter struct {
	SubsiteID string
	APIKeyID  int64
	AccountID int64
	Search    string
	Locked    *bool
}

type ListSubsiteForwardEventsFilter struct {
	SubsiteID string
	Outcome   string
	Search    string
}

type SubsiteForwardRepository interface {
	GetForwardAffinity(ctx context.Context, key string) (*SubsiteForwardAffinity, error)
	UpsertForwardAffinity(ctx context.Context, input UpsertSubsiteForwardAffinityInput) (*SubsiteForwardAffinity, error)
	DeleteForwardAffinity(ctx context.Context, id int64) error
	ListForwardAffinities(ctx context.Context, params pagination.PaginationParams, filter ListSubsiteForwardAffinitiesFilter) ([]SubsiteForwardAffinity, *pagination.PaginationResult, error)
	RecordForwardEvent(ctx context.Context, event *SubsiteForwardEvent) error
	ListForwardEvents(ctx context.Context, params pagination.PaginationParams, filter ListSubsiteForwardEventsFilter) ([]SubsiteForwardEvent, *pagination.PaginationResult, error)
	ForwardStats(ctx context.Context) (*SubsiteForwardStats, error)
	ForwardRouteStats(ctx context.Context) ([]SubsiteForwardSiteStat, error)
	ListRelayLeaseDistribution(ctx context.Context) ([]SubsiteRelayLeaseStat, error)
	ListRelayPoolDistribution(ctx context.Context) ([]SubsiteRelayPoolStat, error)
	ListRelayAccountDistribution(ctx context.Context) ([]SubsiteRelayAccountStat, error)
	ListRelayConfigChecks(ctx context.Context) ([]SubsiteRelayConfigCheck, error)
	ListActiveCircuitBreakers(ctx context.Context) ([]SubsiteCircuitBreaker, error)
	CleanupForwardState(ctx context.Context, now time.Time) (*SubsiteForwardCleanupResult, error)
}

func (s *SubsiteService) GetForwardAffinity(ctx context.Context, key string) (*SubsiteForwardAffinity, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, errors.New("subsite forward repository is not initialized")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrSubsiteInvalidInput
	}
	return s.forwardRepo.GetForwardAffinity(ctx, key)
}

func (s *SubsiteService) UpsertForwardAffinity(ctx context.Context, input UpsertSubsiteForwardAffinityInput) (*SubsiteForwardAffinity, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, errors.New("subsite forward repository is not initialized")
	}
	input.Key = strings.TrimSpace(input.Key)
	input.SubsiteID = strings.TrimSpace(input.SubsiteID)
	input.LeaseID = strings.TrimSpace(input.LeaseID)
	input.Model = strings.TrimSpace(input.Model)
	input.SessionID = strings.TrimSpace(input.SessionID)
	input.LastReason = strings.TrimSpace(input.LastReason)
	input.LastError = strings.TrimSpace(input.LastError)
	input.Type = normalizeForwardAffinityType(input.Type)
	input.Source = normalizeForwardAffinitySource(input.Source)
	if input.Key == "" || input.SubsiteID == "" {
		return nil, ErrSubsiteInvalidInput
	}
	if input.ExpiresAt.IsZero() {
		input.ExpiresAt = time.Now().Add(12 * time.Hour)
	}
	return s.forwardRepo.UpsertForwardAffinity(ctx, input)
}

func (s *SubsiteService) DeleteForwardAffinity(ctx context.Context, id int64) error {
	if s == nil || s.forwardRepo == nil {
		return errors.New("subsite forward repository is not initialized")
	}
	if id <= 0 {
		return ErrSubsiteInvalidInput
	}
	return s.forwardRepo.DeleteForwardAffinity(ctx, id)
}

func (s *SubsiteService) ListForwardAffinities(ctx context.Context, params pagination.PaginationParams, filter ListSubsiteForwardAffinitiesFilter) ([]SubsiteForwardAffinity, *pagination.PaginationResult, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, nil, errors.New("subsite forward repository is not initialized")
	}
	filter.SubsiteID = strings.TrimSpace(filter.SubsiteID)
	filter.Search = strings.TrimSpace(filter.Search)
	return s.forwardRepo.ListForwardAffinities(ctx, params, filter)
}

func (s *SubsiteService) RecordForwardEvent(ctx context.Context, event *SubsiteForwardEvent) error {
	if s == nil || s.forwardRepo == nil {
		return errors.New("subsite forward repository is not initialized")
	}
	if event == nil {
		return ErrSubsiteInvalidInput
	}
	event.RequestID = strings.TrimSpace(event.RequestID)
	event.AffinityKey = strings.TrimSpace(event.AffinityKey)
	event.SubsiteID = strings.TrimSpace(event.SubsiteID)
	event.AttemptedSubsiteID = strings.TrimSpace(event.AttemptedSubsiteID)
	event.FallbackFrom = strings.TrimSpace(event.FallbackFrom)
	event.LeaseID = strings.TrimSpace(event.LeaseID)
	event.Model = strings.TrimSpace(event.Model)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.Method = strings.ToUpper(strings.TrimSpace(event.Method))
	event.Path = strings.TrimSpace(event.Path)
	event.Reason = strings.TrimSpace(event.Reason)
	event.Outcome = normalizeForwardOutcome(event.Outcome)
	event.Error = strings.TrimSpace(event.Error)
	event.Metadata = normalizeSubsiteMap(event.Metadata)
	return s.forwardRepo.RecordForwardEvent(ctx, event)
}

func (s *SubsiteService) ListForwardEvents(ctx context.Context, params pagination.PaginationParams, filter ListSubsiteForwardEventsFilter) ([]SubsiteForwardEvent, *pagination.PaginationResult, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, nil, errors.New("subsite forward repository is not initialized")
	}
	filter.SubsiteID = strings.TrimSpace(filter.SubsiteID)
	filter.Outcome = strings.TrimSpace(filter.Outcome)
	filter.Search = strings.TrimSpace(filter.Search)
	return s.forwardRepo.ListForwardEvents(ctx, params, filter)
}

func (s *SubsiteService) ForwardStats(ctx context.Context) (*SubsiteForwardStats, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, errors.New("subsite forward repository is not initialized")
	}
	stats, err := s.forwardRepo.ForwardStats(ctx)
	if err != nil {
		return nil, err
	}
	stats.Recommendations = BuildSubsiteRelayAdvice(stats)
	return stats, nil
}

func (s *SubsiteService) ForwardRouteStats(ctx context.Context) ([]SubsiteForwardSiteStat, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, errors.New("subsite forward repository is not initialized")
	}
	return s.forwardRepo.ForwardRouteStats(ctx)
}

func (s *SubsiteService) ListActiveCircuitBreakers(ctx context.Context) ([]SubsiteCircuitBreaker, error) {
	if s == nil || s.forwardRepo == nil {
		return nil, errors.New("subsite forward repository is not initialized")
	}
	return s.forwardRepo.ListActiveCircuitBreakers(ctx)
}

func BuildSubsiteRelayAdvice(stats *SubsiteForwardStats) []SubsiteRelayAdvice {
	if stats == nil {
		return []SubsiteRelayAdvice{}
	}
	items := make([]SubsiteRelayAdvice, 0)
	if stats.TotalSubsites == 0 {
		items = append(items, SubsiteRelayAdvice{Code: "NO_SUBSITE", Severity: "critical", Message: "还没有配置子站。请先新增并激活至少一个子站。"})
	} else if stats.OnlineSubsites == 0 {
		items = append(items, SubsiteRelayAdvice{Code: "NO_ONLINE_SUBSITE", Severity: "critical", Message: "没有在线子站。请检查子站 Agent 心跳、主站地址和子站公网地址。"})
	}
	if stats.ActiveLeases == 0 {
		items = append(items, SubsiteRelayAdvice{Code: "NO_ACTIVE_LEASE", Severity: "warning", Message: "当前没有已分发账号。可以点击“立即自动分发”，也可以等待首次 API 请求自动创建租约。"})
	}
	if stats.Events24h >= 10 && stats.SuccessRate24h < 0.95 {
		items = append(items, SubsiteRelayAdvice{Code: "LOW_SUCCESS_RATE", Severity: "warning", Message: "最近 24 小时转发成功率低于 95%。建议查看转发事件和熔断记录。"})
	}
	if stats.Events24h >= 10 && stats.CacheHitRatio24h < 0.10 {
		items = append(items, SubsiteRelayAdvice{Code: "LOW_CACHE_HIT", Severity: "info", Message: "缓存命中率低于 10%。长对话最好保持同一会话路由到同一子站和账号。"})
	}
	if stats.ExpiringLeases24h > 0 {
		items = append(items, SubsiteRelayAdvice{Code: "LEASE_EXPIRING", Severity: "info", Message: "有租约将在 24 小时内过期。过期后系统会重新分发或由请求触发新租约。"})
	}
	if stats.ActiveLeases == 0 && stats.Automation.PublicPoolAccounts == 0 && stats.Automation.PrivatePoolAccounts == 0 {
		items = append(items, SubsiteRelayAdvice{Code: "NO_RELAY_POOL_ACCOUNT", Severity: "warning", Message: "没有账号进入可转发分组。请确认账号已审核通过，并绑定到对应等级/模式的分组。"})
	}
	for _, check := range stats.ConfigChecks {
		if check.Status == "ok" || check.Severity == "" {
			continue
		}
		items = append(items, SubsiteRelayAdvice{Code: check.Code, Severity: check.Severity, Message: check.Message})
	}
	for _, pool := range stats.PoolDistribution {
		if pool.UnknownLevelAccounts > 0 && strings.TrimSpace(pool.RequiredLevel) != "" {
			items = append(items, SubsiteRelayAdvice{
				Code:     "POOL_HAS_UNKNOWN_LEVEL_" + strings.ToUpper(pool.Platform) + "_" + strings.ToUpper(pool.RequiredLevel),
				Severity: "warning",
				Message:  fmt.Sprintf("%s 有 %d 个账号等级未知。账号虽然可能已审核通过，但为了避免错价计费，不会自动进入有等级要求的价格池。请重新校验等级或修复等级识别。", displayRelayPool(pool), pool.UnknownLevelAccounts),
			})
		}
		if pool.TotalAccounts > 0 && pool.SchedulableAccounts == 0 {
			message := fmt.Sprintf("%s 有账号，但没有一个可分发。请检查审核状态、共享模式、限流、账号状态和等级。", displayRelayPool(pool))
			if pool.BlockedReason != "" {
				message = fmt.Sprintf("%s 暂不可分发：%s。", displayRelayPool(pool), pool.BlockedReason)
			}
			items = append(items, SubsiteRelayAdvice{
				Code:     "POOL_NOT_SCHEDULABLE_" + strings.ToUpper(pool.Platform) + "_" + strings.ToUpper(pool.RequiredLevel),
				Severity: "warning",
				Message:  message,
			})
		}
		if pool.SchedulableAccounts > 0 && pool.UnleasedAccounts > 0 && stats.OnlineSubsites > 0 {
			items = append(items, SubsiteRelayAdvice{
				Code:     "POOL_HAS_UNLEASED_" + strings.ToUpper(pool.Platform) + "_" + strings.ToUpper(pool.RequiredLevel),
				Severity: "info",
				Message:  fmt.Sprintf("%s 有 %d 个可分发账号尚未进入子站。点击“立即自动分发”可以马上创建租约。", displayRelayPool(pool), pool.UnleasedAccounts),
			})
		}
	}
	for _, pool := range stats.PoolDistribution {
		if pool.ProxyMissingAccounts <= 0 {
			continue
		}
		items = append(items, SubsiteRelayAdvice{
			Code:     "POOL_MISSING_PROXY_" + strings.ToUpper(pool.Platform) + "_" + strings.ToUpper(pool.RequiredLevel),
			Severity: "info",
			Message:  fmt.Sprintf("%s 有 %d 个可进子站账号尚未绑定代理。系统会保持已绑定代理不变；未绑定账号自动分发时会尽量绑定负载最低的活跃代理。", displayRelayPool(pool), pool.ProxyMissingAccounts),
		})
	}
	return items
}

func displayRelayLevel(level string) string {
	level = strings.TrimSpace(level)
	if level == "" {
		return "不限等级"
	}
	switch level {
	case AccountLevelFree:
		return "Free 免费"
	case AccountLevelPlus:
		return "Plus 会员"
	case AccountLevelPro:
		return "Pro 专业"
	case AccountLevelTeam:
		return "Team 团队"
	default:
		return level
	}
}

func displayRelayPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case PlatformOpenAI:
		return "OpenAI"
	case PlatformAnthropic:
		return "Claude"
	case PlatformGemini:
		return "Gemini"
	case PlatformAntigravity:
		return "Antigravity"
	default:
		if strings.TrimSpace(platform) == "" {
			return "未知平台"
		}
		return platform
	}
}

func displayRelayScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case GroupScopeUserPrivate:
		return "用户私有池"
	case GroupScopePublic, "":
		return "公有池"
	default:
		return scope
	}
}

func displayRelayPool(pool SubsiteRelayPoolStat) string {
	return fmt.Sprintf("%s / %s / %s", displayRelayPlatform(pool.Platform), displayRelayScope(pool.Scope), displayRelayLevel(pool.RequiredLevel))
}

func (s *SubsiteService) SignRouteHint(ctx context.Context, subsiteID, method, path, timestamp, nonce, preferredLeaseID string, preferredAccountID int64) (string, error) {
	if s == nil || s.repo == nil || s.encryptor == nil {
		return "", errors.New("subsite service dependencies are nil")
	}
	secret, err := s.decryptSecret(ctx, subsiteID)
	if err != nil {
		return "", err
	}
	return SignSubsiteRouteHint(secret, method, path, timestamp, nonce, preferredLeaseID, preferredAccountID), nil
}

func SignSubsiteRouteHint(secret, method, path, timestamp, nonce, preferredLeaseID string, preferredAccountID int64) string {
	canonical := strings.Join([]string{
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(path),
		strings.TrimSpace(timestamp),
		strings.TrimSpace(nonce),
		strings.TrimSpace(preferredLeaseID),
		fmt.Sprint(preferredAccountID),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeForwardAffinityType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "api_key", "account", "manual":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "session"
	}
}

func normalizeForwardAffinitySource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "manual", "fallback", "imported":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

func normalizeForwardOutcome(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "failed", "no_candidate", "fallback", "client_error", "upstream_error":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "success"
	}
}

func SubsiteLoadLevelForStats(stat SubsiteForwardSiteStat) string {
	if stat.CircuitOpen || stat.EffectiveStatus == "offline" {
		return "offline"
	}
	if stat.ActiveRequests >= 100 || stat.QueuedUsage >= 100 || stat.CPUPercent >= 90 || stat.HealthScore < 50 {
		return "critical"
	}
	if stat.ActiveRequests >= 50 || stat.QueuedUsage >= 50 || stat.CPUPercent >= 75 || stat.HealthScore < 80 {
		return "high"
	}
	if stat.ActiveRequests >= 15 || stat.QueuedUsage >= 15 || stat.CPUPercent >= 55 {
		return "medium"
	}
	return "low"
}

func SubsiteEffectiveStatusForStats(stat SubsiteForwardSiteStat, now time.Time) string {
	if stat.CircuitOpen {
		return "circuit_open"
	}
	if stat.Status != SubsiteStatusActive {
		return stat.Status
	}
	if stat.LastHeartbeatAt == nil || now.Sub(*stat.LastHeartbeatAt) > 3*time.Minute {
		return "offline"
	}
	if stat.HealthScore < 80 {
		return "degraded"
	}
	return "online"
}

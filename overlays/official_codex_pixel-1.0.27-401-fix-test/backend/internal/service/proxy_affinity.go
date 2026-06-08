package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

const (
	SettingKeyProxyAffinityConfig    = "proxy_affinity_config"
	SettingKeyProxyAffinityLastRunAt = "proxy_affinity_last_run_at"
	SettingKeyProxyAffinityEvents    = "proxy_affinity_events"

	proxyAffinityDefaultScanIntervalMinutes = 5
	proxyAffinityDefaultBatchSize           = 100
	proxyAffinityMaxBatchSize               = 1000
	proxyAffinityDefaultMaxEvents           = 200
	proxyAffinityMaxStoredEvents            = 500

	proxyAffinityStrategyLeastLoaded         = "least_loaded"
	proxyAffinityStrategyWeightedLeastLoaded = "weighted_least_loaded"
	proxyAffinityFallbackWait                = "wait"
	proxyAffinityFallbackDirect              = "direct"
	proxyAffinityFallbackReject              = "reject"

	proxyAffinityExtraSource        = "proxy_affinity_source"
	proxyAffinityExtraAssignedAt    = "proxy_affinity_assigned_at"
	proxyAffinityExtraReason        = "proxy_affinity_reason"
	proxyAffinityExtraProxyID       = "proxy_affinity_proxy_id"
	proxyAffinityExtraReleasedAt    = "proxy_affinity_released_at"
	proxyAffinityExtraReleaseReason = "proxy_affinity_release_reason"
	proxyAffinityExtraPhase         = "proxy_affinity_phase"
	proxyAffinityExtraValidationAt  = "proxy_affinity_last_validation_at"
	proxyAffinityExtraValidationErr = "proxy_affinity_last_validation_error"
	proxyAffinityExtraAttempts      = "proxy_affinity_validation_attempts"

	proxyAffinityPhasePreValidation    = "pre_validation"
	proxyAffinityPhaseValidated        = "validated"
	proxyAffinityPhaseValidationFailed = "validation_failed"
	proxyAffinityPhaseWaitingProxy     = "waiting_proxy"
)

var ErrProxyAffinityNoProxyAvailable = errors.New("proxy affinity validation requires a proxy but no proxy is available")

type ProxyAffinitySettings struct {
	Enabled                    bool          `json:"enabled"`
	UserOwnedEnabled           bool          `json:"user_owned_enabled"`
	AdminAccountsEnabled       bool          `json:"admin_accounts_enabled"`
	PrivateAccountsEnabled     bool          `json:"private_accounts_enabled"`
	PublicAccountsEnabled      bool          `json:"public_accounts_enabled"`
	OnlyApprovedPublicAccounts bool          `json:"only_approved_public_accounts"`
	IncludeAPIKeyAccounts      bool          `json:"include_api_key_accounts"`
	IncludeOAuthAccounts       bool          `json:"include_oauth_accounts"`
	MaxAccountsPerProxy        int64         `json:"max_accounts_per_proxy"`
	BatchSize                  int           `json:"batch_size"`
	ScanIntervalMinutes        int           `json:"scan_interval_minutes"`
	Platforms                  []string      `json:"platforms"`
	AllowReassignWhenProxyDown bool          `json:"allow_reassign_when_proxy_down"`
	ReleaseWhenAccountInactive bool          `json:"release_when_account_inactive"`
	Strategy                   string        `json:"strategy"`
	MaxStoredEvents            int           `json:"max_stored_events"`
	PausedProxyIDs             []int64       `json:"paused_proxy_ids"`
	ProxyWeights               map[int64]int `json:"proxy_weights"`
	PreValidationEnabled       bool          `json:"pre_validation_enabled"`
	EnforceValidationProxy     bool          `json:"enforce_validation_proxy"`
	IncludePendingAccounts     bool          `json:"include_pending_accounts"`
	ReleaseOnValidationFailure bool          `json:"release_on_validation_failure"`
	RetryWithNewProxyOnFailure bool          `json:"retry_with_new_proxy_on_failure"`
	MaxPreValidationRetries    int           `json:"max_pre_validation_retries"`
	FallbackWhenNoProxy        string        `json:"fallback_when_no_proxy"`
}

type ProxyAffinityOverview struct {
	Settings                   ProxyAffinitySettings         `json:"settings"`
	TotalProxies               int                           `json:"total_proxies"`
	AvailableProxies           int                           `json:"available_proxies"`
	FullProxies                int                           `json:"full_proxies"`
	BoundAccounts              int64                         `json:"bound_accounts"`
	UnassignedEligibleAccounts int64                         `json:"unassigned_eligible_accounts"`
	PreValidationAccounts      int64                         `json:"pre_validation_accounts"`
	WaitingProxyAccounts       int64                         `json:"waiting_proxy_accounts"`
	ValidationFailedAccounts   int64                         `json:"validation_failed_accounts"`
	SkippedAccounts            int64                         `json:"skipped_accounts"`
	AverageLoad                float64                       `json:"average_load"`
	ProxyLoads                 []ProxyAffinityProxyLoad      `json:"proxy_loads"`
	BoundAccountDetails        []ProxyAffinityAccountBinding `json:"bound_account_details"`
	PendingAccounts            []ProxyAffinityPendingAccount `json:"pending_accounts"`
	RecentEvents               []ProxyAffinityEvent          `json:"recent_events"`
	LastRunAt                  string                        `json:"last_run_at,omitempty"`
}

type ProxyAffinityProxyLoad struct {
	ProxyID       int64   `json:"proxy_id"`
	Name          string  `json:"name"`
	Protocol      string  `json:"protocol"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Status        string  `json:"status"`
	AccountCount  int64   `json:"account_count"`
	MaxAccounts   int64   `json:"max_accounts"`
	Assignable    bool    `json:"assignable"`
	LoadPercent   float64 `json:"load_percent"`
	IPAddress     string  `json:"ip_address,omitempty"`
	Country       string  `json:"country,omitempty"`
	CountryCode   string  `json:"country_code,omitempty"`
	QualityStatus string  `json:"quality_status,omitempty"`
	QualityGrade  string  `json:"quality_grade,omitempty"`
	Paused        bool    `json:"paused"`
	Weight        int     `json:"weight"`
	EffectiveLoad float64 `json:"effective_load"`
	Reason        string  `json:"reason,omitempty"`
}

type ProxyAffinityCandidate struct {
	AccountID    int64  `json:"account_id"`
	AccountName  string `json:"account_name"`
	Platform     string `json:"platform"`
	Type         string `json:"type"`
	ShareMode    string `json:"share_mode"`
	ShareStatus  string `json:"share_status"`
	AccountLevel string `json:"account_level"`
	OwnerUserID  *int64 `json:"owner_user_id,omitempty"`
}

type ProxyAffinityAssignment struct {
	Candidate ProxyAffinityCandidate `json:"candidate"`
	ProxyID   int64                  `json:"proxy_id,omitempty"`
	ProxyName string                 `json:"proxy_name,omitempty"`
	Action    string                 `json:"action"`
	Reason    string                 `json:"reason"`
	DryRun    bool                   `json:"dry_run"`
}

type ProxyAffinityAccountBinding struct {
	ProxyAffinityCandidate
	ProxyID      int64  `json:"proxy_id"`
	ProxyName    string `json:"proxy_name,omitempty"`
	ProxyHost    string `json:"proxy_host,omitempty"`
	ProxyPort    int    `json:"proxy_port,omitempty"`
	AssignedAt   string `json:"assigned_at,omitempty"`
	AssignedBy   string `json:"assigned_by,omitempty"`
	AssignReason string `json:"assign_reason,omitempty"`
	Phase        string `json:"phase,omitempty"`
	LastTestAt   string `json:"last_test_at,omitempty"`
	LastTestErr  string `json:"last_test_error,omitempty"`
	HealthStatus string `json:"health_status"`
	HealthReason string `json:"health_reason,omitempty"`
}

type ProxyAffinityPendingAccount struct {
	ProxyAffinityCandidate
	Reason      string `json:"reason"`
	Phase       string `json:"phase,omitempty"`
	LastTestAt  string `json:"last_test_at,omitempty"`
	LastTestErr string `json:"last_test_error,omitempty"`
}

type ProxyAffinityEvent struct {
	ID          string         `json:"id"`
	OccurredAt  string         `json:"occurred_at"`
	Source      string         `json:"source"`
	Action      string         `json:"action"`
	AccountID   int64          `json:"account_id,omitempty"`
	AccountName string         `json:"account_name,omitempty"`
	ProxyID     int64          `json:"proxy_id,omitempty"`
	ProxyName   string         `json:"proxy_name,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	DryRun      bool           `json:"dry_run"`
	Details     map[string]any `json:"details,omitempty"`
}

type ProxyAffinityAssignRequest struct {
	DryRun    bool     `json:"dry_run"`
	Limit     int      `json:"limit"`
	Platforms []string `json:"platforms"`
}

type ProxyAffinityBindRequest struct {
	AccountID int64  `json:"account_id"`
	ProxyID   int64  `json:"proxy_id"`
	DryRun    bool   `json:"dry_run"`
	Reason    string `json:"reason"`
}

type ProxyAffinityReleaseRequest struct {
	AccountID int64  `json:"account_id"`
	DryRun    bool   `json:"dry_run"`
	Reason    string `json:"reason"`
}

type ProxyAffinityPrebindRequest struct {
	DryRun    bool     `json:"dry_run"`
	Limit     int      `json:"limit"`
	Platforms []string `json:"platforms"`
}

type ProxyAffinityValidationResult struct {
	Success bool
	Error   string
}

type ProxyAffinityAssignResult struct {
	DryRun      bool                      `json:"dry_run"`
	Scanned     int                       `json:"scanned"`
	Assigned    int                       `json:"assigned"`
	Released    int                       `json:"released"`
	Skipped     int                       `json:"skipped"`
	Assignments []ProxyAffinityAssignment `json:"assignments"`
}

type ProxyAffinityService struct {
	settingRepo SettingRepository
	proxyRepo   ProxyRepository
	accountRepo AccountRepository

	timingWheel *TimingWheelService
	mu          sync.Mutex
}

func NewProxyAffinityService(settingRepo SettingRepository, proxyRepo ProxyRepository, accountRepo AccountRepository) *ProxyAffinityService {
	return &ProxyAffinityService{
		settingRepo: settingRepo,
		proxyRepo:   proxyRepo,
		accountRepo: accountRepo,
	}
}

func ProvideProxyAffinityService(
	settingRepo SettingRepository,
	proxyRepo ProxyRepository,
	accountRepo AccountRepository,
	timingWheel *TimingWheelService,
) *ProxyAffinityService {
	svc := NewProxyAffinityService(settingRepo, proxyRepo, accountRepo)
	svc.SetTimingWheel(timingWheel)
	svc.Start()
	return svc
}

func (s *ProxyAffinityService) SetTimingWheel(timingWheel *TimingWheelService) {
	s.timingWheel = timingWheel
}

func (s *ProxyAffinityService) Start() {
	if s == nil || s.timingWheel == nil {
		return
	}
	s.timingWheel.ScheduleRecurring("proxy_affinity:assign_unassigned", time.Minute, s.maintenanceTick)
}

func (s *ProxyAffinityService) Stop() {
	if s == nil || s.timingWheel == nil {
		return
	}
	s.timingWheel.Cancel("proxy_affinity:assign_unassigned")
}

func DefaultProxyAffinitySettings() ProxyAffinitySettings {
	return ProxyAffinitySettings{
		Enabled:                    false,
		UserOwnedEnabled:           true,
		AdminAccountsEnabled:       true,
		PrivateAccountsEnabled:     true,
		PublicAccountsEnabled:      true,
		OnlyApprovedPublicAccounts: true,
		IncludeAPIKeyAccounts:      true,
		IncludeOAuthAccounts:       true,
		MaxAccountsPerProxy:        0,
		BatchSize:                  proxyAffinityDefaultBatchSize,
		ScanIntervalMinutes:        proxyAffinityDefaultScanIntervalMinutes,
		Platforms:                  []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity},
		AllowReassignWhenProxyDown: false,
		ReleaseWhenAccountInactive: false,
		Strategy:                   proxyAffinityStrategyWeightedLeastLoaded,
		MaxStoredEvents:            proxyAffinityDefaultMaxEvents,
		PausedProxyIDs:             []int64{},
		ProxyWeights:               map[int64]int{},
		PreValidationEnabled:       true,
		EnforceValidationProxy:     true,
		IncludePendingAccounts:     true,
		ReleaseOnValidationFailure: true,
		RetryWithNewProxyOnFailure: false,
		MaxPreValidationRetries:    1,
		FallbackWhenNoProxy:        proxyAffinityFallbackWait,
	}
}

func (s *ProxyAffinityService) GetSettings(ctx context.Context) (ProxyAffinitySettings, error) {
	settings := DefaultProxyAffinitySettings()
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyProxyAffinityConfig)
	if err != nil {
		if infraerrors.IsNotFound(err) {
			return settings, nil
		}
		return settings, fmt.Errorf("get proxy affinity settings: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return settings, nil
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		slog.Warn("failed to parse proxy affinity settings, using defaults", "error", err)
		return DefaultProxyAffinitySettings(), nil
	}
	settings = applyProxyAffinityDefaultsForMissingFields(raw, settings)
	return normalizeProxyAffinitySettings(settings), nil
}

func (s *ProxyAffinityService) UpdateSettings(ctx context.Context, settings ProxyAffinitySettings) (ProxyAffinitySettings, error) {
	settings = normalizeProxyAffinitySettings(settings)
	data, err := json.Marshal(settings)
	if err != nil {
		return settings, fmt.Errorf("marshal proxy affinity settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyProxyAffinityConfig, string(data)); err != nil {
		return settings, fmt.Errorf("save proxy affinity settings: %w", err)
	}
	return settings, nil
}

func (s *ProxyAffinityService) GetOverview(ctx context.Context) (*ProxyAffinityOverview, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}

	proxyLoads, err := s.loadProxyLoads(ctx, settings)
	if err != nil {
		return nil, err
	}
	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active accounts: %w", err)
	}
	proxyByID := proxyAffinityProxyLoadMap(proxyLoads)

	var bound, eligible, preValidation, waitingProxy, validationFailed, skipped int64
	boundDetails := make([]ProxyAffinityAccountBinding, 0)
	pendingAccounts := make([]ProxyAffinityPendingAccount, 0)
	for i := range accounts {
		account := &accounts[i]
		phase := proxyAffinityPhaseFromAccount(account)
		switch phase {
		case proxyAffinityPhasePreValidation:
			preValidation++
		case proxyAffinityPhaseWaitingProxy:
			waitingProxy++
		case proxyAffinityPhaseValidationFailed:
			validationFailed++
		}
		if account.ProxyID != nil {
			bound++
			boundDetails = append(boundDetails, s.accountBindingFromAccount(account, proxyByID, settings))
			continue
		}
		candidate := proxyAffinityCandidateFromAccount(account)
		if ok, reason := s.isEligibleAccount(account, settings); ok {
			eligible++
			pendingAccounts = append(pendingAccounts, ProxyAffinityPendingAccount{
				ProxyAffinityCandidate: candidate,
				Reason:                 "符合规则，等待自动或手动分配",
				Phase:                  phase,
				LastTestAt:             proxyAffinityExtraString(account, proxyAffinityExtraValidationAt),
				LastTestErr:            proxyAffinityExtraString(account, proxyAffinityExtraValidationErr),
			})
		} else {
			skipped++
			if len(pendingAccounts) < 200 {
				pendingAccounts = append(pendingAccounts, ProxyAffinityPendingAccount{
					ProxyAffinityCandidate: candidate,
					Reason:                 reason,
					Phase:                  phase,
					LastTestAt:             proxyAffinityExtraString(account, proxyAffinityExtraValidationAt),
					LastTestErr:            proxyAffinityExtraString(account, proxyAffinityExtraValidationErr),
				})
			}
		}
	}

	var totalLoad int64
	available := 0
	full := 0
	for i := range proxyLoads {
		totalLoad += proxyLoads[i].AccountCount
		if proxyLoads[i].Assignable {
			available++
		}
		if settings.MaxAccountsPerProxy > 0 && proxyLoads[i].AccountCount >= settings.MaxAccountsPerProxy {
			full++
		}
	}
	averageLoad := 0.0
	if len(proxyLoads) > 0 {
		averageLoad = float64(totalLoad) / float64(len(proxyLoads))
	}
	events, _ := s.getRecentEvents(ctx)
	lastRunAt := ""
	if raw, err := s.settingRepo.GetValue(ctx, SettingKeyProxyAffinityLastRunAt); err == nil {
		lastRunAt = strings.TrimSpace(raw)
	}

	return &ProxyAffinityOverview{
		Settings:                   settings,
		TotalProxies:               len(proxyLoads),
		AvailableProxies:           available,
		FullProxies:                full,
		BoundAccounts:              bound,
		UnassignedEligibleAccounts: eligible,
		PreValidationAccounts:      preValidation,
		WaitingProxyAccounts:       waitingProxy,
		ValidationFailedAccounts:   validationFailed,
		SkippedAccounts:            skipped,
		AverageLoad:                averageLoad,
		ProxyLoads:                 proxyLoads,
		BoundAccountDetails:        boundDetails,
		PendingAccounts:            pendingAccounts,
		RecentEvents:               events,
		LastRunAt:                  lastRunAt,
	}, nil
}

func (s *ProxyAffinityService) AssignUnassigned(ctx context.Context, req ProxyAffinityAssignRequest) (*ProxyAffinityAssignResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Platforms) > 0 {
		settings.Platforms = normalizeProxyAffinityPlatforms(req.Platforms)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = settings.BatchSize
	}
	if limit <= 0 {
		limit = proxyAffinityDefaultBatchSize
	}
	if limit > proxyAffinityMaxBatchSize {
		limit = proxyAffinityMaxBatchSize
	}

	proxyLoads, err := s.loadProxyLoads(ctx, settings)
	if err != nil {
		return nil, err
	}

	result := &ProxyAffinityAssignResult{
		DryRun:      req.DryRun,
		Assignments: []ProxyAffinityAssignment{},
	}
	released, releaseEvents, err := s.releaseInvalidBindings(ctx, settings, proxyLoads, req.DryRun)
	if err != nil {
		return nil, err
	}
	result.Released = released
	result.Assignments = append(result.Assignments, releaseEvents...)
	if released > 0 && !req.DryRun {
		proxyLoads, err = s.loadProxyLoads(ctx, settings)
		if err != nil {
			return nil, err
		}
	}

	assignable := make([]ProxyAffinityProxyLoad, 0, len(proxyLoads))
	for _, proxyLoad := range proxyLoads {
		if proxyLoad.Assignable {
			assignable = append(assignable, proxyLoad)
		}
	}

	if len(assignable) == 0 {
		if len(result.Assignments) > 0 && !req.DryRun {
			_ = s.appendAssignmentEvents(ctx, result.Assignments, "auto")
		}
		return result, nil
	}

	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unassigned accounts: %w", err)
	}
	sort.SliceStable(accounts, func(i, j int) bool {
		return accounts[i].ID < accounts[j].ID
	})

	for i := range accounts {
		if result.Assigned >= limit {
			break
		}
		if accounts[i].ProxyID != nil {
			continue
		}
		result.Scanned++
		account := &accounts[i]
		candidate := proxyAffinityCandidateFromAccount(account)
		if ok, reason := s.isEligibleAccount(account, settings); !ok {
			result.Skipped++
			result.Assignments = append(result.Assignments, ProxyAffinityAssignment{
				Candidate: candidate,
				Action:    "skipped",
				Reason:    reason,
				DryRun:    req.DryRun,
			})
			continue
		}

		chosen, ok := chooseProxyAffinityTarget(assignable, settings)
		if !ok {
			result.Skipped++
			result.Assignments = append(result.Assignments, ProxyAffinityAssignment{
				Candidate: candidate,
				Action:    "skipped",
				Reason:    "没有可分配的代理或代理已达到上限",
				DryRun:    req.DryRun,
			})
			continue
		}

		assignment := ProxyAffinityAssignment{
			Candidate: candidate,
			ProxyID:   chosen.ProxyID,
			ProxyName: chosen.Name,
			Action:    "assigned",
			Reason:    proxyAffinityAssignmentReason(settings),
			DryRun:    req.DryRun,
		}
		if !req.DryRun {
			if err := s.assignAccountToProxy(ctx, account, chosen.ProxyID, "auto_assign"); err != nil {
				result.Skipped++
				assignment.Action = "failed"
				assignment.Reason = err.Error()
				result.Assignments = append(result.Assignments, assignment)
				continue
			}
		}

		result.Assigned++
		result.Assignments = append(result.Assignments, assignment)
		for idx := range assignable {
			if assignable[idx].ProxyID == chosen.ProxyID {
				assignable[idx].AccountCount++
				assignable[idx].Assignable = settings.MaxAccountsPerProxy <= 0 || assignable[idx].AccountCount < settings.MaxAccountsPerProxy
				break
			}
		}
	}

	if len(result.Assignments) > 0 && !req.DryRun {
		_ = s.appendAssignmentEvents(ctx, result.Assignments, "auto")
	}
	return result, nil
}

func (s *ProxyAffinityService) BindAccount(ctx context.Context, req ProxyAffinityBindRequest) (*ProxyAffinityAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.AccountID <= 0 {
		return nil, infraerrors.BadRequest("PROXY_AFFINITY_ACCOUNT_REQUIRED", "account_id is required")
	}
	if req.ProxyID <= 0 {
		return nil, infraerrors.BadRequest("PROXY_AFFINITY_PROXY_REQUIRED", "proxy_id is required")
	}
	account, err := s.accountRepo.GetByID(ctx, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account for proxy affinity bind: %w", err)
	}
	proxy, err := s.proxyRepo.GetByID(ctx, req.ProxyID)
	if err != nil {
		return nil, fmt.Errorf("get proxy for proxy affinity bind: %w", err)
	}
	assignment := &ProxyAffinityAssignment{
		Candidate: proxyAffinityCandidateFromAccount(account),
		ProxyID:   proxy.ID,
		ProxyName: proxy.Name,
		Action:    "assigned",
		Reason:    strings.TrimSpace(req.Reason),
		DryRun:    req.DryRun,
	}
	if assignment.Reason == "" {
		assignment.Reason = "管理员手动绑定代理"
	}
	if account.ProxyID != nil {
		assignment.Action = "skipped"
		assignment.ProxyID = *account.ProxyID
		assignment.Reason = "账号已经绑定代理，请先释放原绑定"
		return assignment, nil
	}
	if proxy.Status != StatusActive {
		assignment.Action = "skipped"
		assignment.Reason = "代理状态不是 active，不能绑定"
		return assignment, nil
	}
	if !req.DryRun {
		if err := s.assignAccountToProxy(ctx, account, proxy.ID, "manual_bind"); err != nil {
			assignment.Action = "failed"
			assignment.Reason = err.Error()
		}
		_ = s.appendAssignmentEvents(ctx, []ProxyAffinityAssignment{*assignment}, "manual")
	}
	return assignment, nil
}

func (s *ProxyAffinityService) ReleaseAccount(ctx context.Context, req ProxyAffinityReleaseRequest) (*ProxyAffinityAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.AccountID <= 0 {
		return nil, infraerrors.BadRequest("PROXY_AFFINITY_ACCOUNT_REQUIRED", "account_id is required")
	}
	account, err := s.accountRepo.GetByID(ctx, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("get account for proxy affinity release: %w", err)
	}
	assignment := &ProxyAffinityAssignment{
		Candidate: proxyAffinityCandidateFromAccount(account),
		Action:    "released",
		Reason:    strings.TrimSpace(req.Reason),
		DryRun:    req.DryRun,
	}
	if assignment.Reason == "" {
		assignment.Reason = "管理员手动释放代理绑定"
	}
	if account.ProxyID == nil {
		assignment.Action = "skipped"
		assignment.Reason = "账号当前没有代理绑定"
		return assignment, nil
	}
	assignment.ProxyID = *account.ProxyID
	if proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && proxy != nil {
		assignment.ProxyName = proxy.Name
	}
	if !req.DryRun {
		if err := s.releaseAccountProxy(ctx, account, assignment.Reason); err != nil {
			assignment.Action = "failed"
			assignment.Reason = err.Error()
		}
		_ = s.appendAssignmentEvents(ctx, []ProxyAffinityAssignment{*assignment}, "manual")
	}
	return assignment, nil
}

func (s *ProxyAffinityService) PrebindPendingAccounts(ctx context.Context, req ProxyAffinityPrebindRequest) (*ProxyAffinityAssignResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || !settings.PreValidationEnabled {
		return &ProxyAffinityAssignResult{DryRun: req.DryRun, Assignments: []ProxyAffinityAssignment{}}, nil
	}
	if len(req.Platforms) > 0 {
		settings.Platforms = normalizeProxyAffinityPlatforms(req.Platforms)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = settings.BatchSize
	}
	if limit <= 0 {
		limit = proxyAffinityDefaultBatchSize
	}
	if limit > proxyAffinityMaxBatchSize {
		limit = proxyAffinityMaxBatchSize
	}

	proxyLoads, err := s.loadProxyLoads(ctx, settings)
	if err != nil {
		return nil, err
	}
	assignable := proxyAffinityAssignableLoads(proxyLoads)
	result := &ProxyAffinityAssignResult{DryRun: req.DryRun, Assignments: []ProxyAffinityAssignment{}}
	if len(assignable) == 0 {
		return result, nil
	}

	accounts, err := s.accountRepo.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active accounts for proxy affinity prebind: %w", err)
	}
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	for i := range accounts {
		if result.Assigned >= limit {
			break
		}
		account := &accounts[i]
		if account.ProxyID != nil {
			continue
		}
		result.Scanned++
		candidate := proxyAffinityCandidateFromAccount(account)
		if ok, reason := s.isPreValidationEligibleAccount(account, settings); !ok {
			result.Skipped++
			result.Assignments = append(result.Assignments, ProxyAffinityAssignment{
				Candidate: candidate,
				Action:    "skipped",
				Reason:    reason,
				DryRun:    req.DryRun,
			})
			continue
		}
		chosen, ok := chooseProxyAffinityTarget(assignable, settings)
		if !ok {
			result.Skipped++
			result.Assignments = append(result.Assignments, ProxyAffinityAssignment{
				Candidate: candidate,
				Action:    "skipped",
				Reason:    "没有可用于校验前绑定的代理",
				DryRun:    req.DryRun,
			})
			continue
		}
		assignment := ProxyAffinityAssignment{
			Candidate: candidate,
			ProxyID:   chosen.ProxyID,
			ProxyName: chosen.Name,
			Action:    "assigned",
			Reason:    "校验前预绑定代理，账号检测将使用该代理出口",
			DryRun:    req.DryRun,
		}
		if !req.DryRun {
			if err := s.assignAccountToProxy(ctx, account, chosen.ProxyID, "pre_validation"); err != nil {
				result.Skipped++
				assignment.Action = "failed"
				assignment.Reason = err.Error()
				result.Assignments = append(result.Assignments, assignment)
				continue
			}
		}
		result.Assigned++
		result.Assignments = append(result.Assignments, assignment)
		for idx := range assignable {
			if assignable[idx].ProxyID == chosen.ProxyID {
				assignable[idx].AccountCount++
				assignable[idx].EffectiveLoad = float64(assignable[idx].AccountCount) / float64(maxProxyAffinityWeight(assignable[idx].Weight))
				assignable[idx].Assignable = settings.MaxAccountsPerProxy <= 0 || assignable[idx].AccountCount < settings.MaxAccountsPerProxy
				break
			}
		}
	}
	if len(result.Assignments) > 0 && !req.DryRun {
		_ = s.appendAssignmentEvents(ctx, result.Assignments, "pre_validation")
	}
	return result, nil
}

func (s *ProxyAffinityService) EnsurePreValidationBinding(ctx context.Context, accountID int64) (*Account, *ProxyAffinityAssignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return account, nil, err
	}
	if !settings.Enabled || !settings.PreValidationEnabled {
		return account, nil, nil
	}
	if account.ProxyID != nil {
		proxy, proxyErr := s.proxyRepo.GetByID(ctx, *account.ProxyID)
		if proxyErr != nil || proxy == nil || proxy.Status != StatusActive {
			reason := "校验前发现已绑定代理不可用，释放后重新选择代理"
			if err := s.releaseAccountProxy(ctx, account, reason); err != nil {
				return account, nil, err
			}
			account, err = s.accountRepo.GetByID(ctx, accountID)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if account.ProxyID != nil {
		if proxyAffinityPhaseFromAccount(account) == "" {
			if err := s.setProxyAffinityPhase(ctx, account, proxyAffinityPhasePreValidation, ""); err != nil {
				return account, nil, err
			}
			account, _ = s.accountRepo.GetByID(ctx, accountID)
		}
		return account, &ProxyAffinityAssignment{
			Candidate: proxyAffinityCandidateFromAccount(account),
			ProxyID:   int64Value(account.ProxyID),
			Action:    "skipped",
			Reason:    "账号已绑定代理，校验将复用现有代理",
		}, nil
	}
	if ok, reason := s.isPreValidationEligibleAccount(account, settings); !ok {
		return account, &ProxyAffinityAssignment{
			Candidate: proxyAffinityCandidateFromAccount(account),
			Action:    "skipped",
			Reason:    reason,
		}, nil
	}
	proxyLoads, err := s.loadProxyLoads(ctx, settings)
	if err != nil {
		return account, nil, err
	}
	assignable := proxyAffinityAssignableLoads(proxyLoads)
	chosen, ok := chooseProxyAffinityTarget(assignable, settings)
	if !ok {
		reason := "没有可用于校验前绑定的代理"
		_ = s.setProxyAffinityPhase(ctx, account, proxyAffinityPhaseWaitingProxy, reason)
		assignment := &ProxyAffinityAssignment{
			Candidate: proxyAffinityCandidateFromAccount(account),
			Action:    "skipped",
			Reason:    reason,
		}
		if settings.EnforceValidationProxy && settings.FallbackWhenNoProxy != proxyAffinityFallbackDirect {
			return account, assignment, ErrProxyAffinityNoProxyAvailable
		}
		return account, assignment, nil
	}
	if err := s.assignAccountToProxy(ctx, account, chosen.ProxyID, "pre_validation"); err != nil {
		return account, nil, err
	}
	assignment := &ProxyAffinityAssignment{
		Candidate: proxyAffinityCandidateFromAccount(account),
		ProxyID:   chosen.ProxyID,
		ProxyName: chosen.Name,
		Action:    "assigned",
		Reason:    "校验前预绑定代理，账号检测将使用该代理出口",
	}
	_ = s.appendAssignmentEvents(ctx, []ProxyAffinityAssignment{*assignment}, "pre_validation")
	account, err = s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, assignment, err
	}
	return account, assignment, nil
}

func (s *ProxyAffinityService) HandleValidationResult(ctx context.Context, accountID int64, result ProxyAffinityValidationResult) error {
	if s == nil || accountID <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.Enabled || !settings.PreValidationEnabled {
		return nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if result.Success {
		return s.updateProxyAffinityExtra(ctx, account, map[string]any{
			proxyAffinityExtraPhase:         proxyAffinityPhaseValidated,
			proxyAffinityExtraValidationAt:  now.Format(time.RFC3339),
			proxyAffinityExtraValidationErr: "",
		})
	}

	errMsg := strings.TrimSpace(result.Error)
	if errMsg == "" {
		errMsg = "account validation failed"
	}
	attempts := intFromAny(account.Extra[proxyAffinityExtraAttempts]) + 1
	if settings.ReleaseOnValidationFailure && account.ProxyID != nil && proxyAffinityShouldReleaseOnValidationFailure(errMsg) {
		reason := "账号校验失败，释放校验前代理绑定"
		if releaseErr := s.releaseAccountProxy(ctx, account, reason); releaseErr != nil {
			return releaseErr
		}
		account, err = s.accountRepo.GetByID(ctx, accountID)
		if err != nil {
			return err
		}
	}
	return s.updateProxyAffinityExtra(ctx, account, map[string]any{
		proxyAffinityExtraPhase:         proxyAffinityPhaseValidationFailed,
		proxyAffinityExtraValidationAt:  now.Format(time.RFC3339),
		proxyAffinityExtraValidationErr: errMsg,
		proxyAffinityExtraAttempts:      attempts,
	})
}

func (s *ProxyAffinityService) maintenanceTick() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	settings, err := s.GetSettings(ctx)
	if err != nil {
		slog.Warn("proxy affinity maintenance failed to load settings", "error", err)
		return
	}
	if !settings.Enabled {
		return
	}
	now := time.Now().UTC()
	if settings.ScanIntervalMinutes > 1 {
		lastRaw, err := s.settingRepo.GetValue(ctx, SettingKeyProxyAffinityLastRunAt)
		if err == nil {
			if last, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(lastRaw)); parseErr == nil {
				if now.Sub(last) < time.Duration(settings.ScanIntervalMinutes)*time.Minute {
					return
				}
			}
		}
	}

	prebindResult, prebindErr := s.PrebindPendingAccounts(ctx, ProxyAffinityPrebindRequest{
		DryRun: false,
		Limit:  settings.BatchSize,
	})
	if prebindErr != nil {
		slog.Warn("proxy affinity maintenance pre-validation binding failed", "error", prebindErr)
	}

	result, err := s.AssignUnassigned(ctx, ProxyAffinityAssignRequest{
		DryRun: false,
		Limit:  settings.BatchSize,
	})
	if err != nil {
		slog.Warn("proxy affinity maintenance assignment failed", "error", err)
		return
	}
	_ = s.settingRepo.Set(ctx, SettingKeyProxyAffinityLastRunAt, now.Format(time.RFC3339))
	prebound := 0
	if prebindResult != nil {
		prebound = prebindResult.Assigned
	}
	if prebound > 0 || result.Assigned > 0 || result.Released > 0 {
		slog.Info("proxy affinity maintenance completed", "prebound", prebound, "assigned", result.Assigned, "released", result.Released, "scanned", result.Scanned)
	}
}

func (s *ProxyAffinityService) loadProxyLoads(ctx context.Context, settings ProxyAffinitySettings) ([]ProxyAffinityProxyLoad, error) {
	proxies, err := s.proxyRepo.ListActiveWithAccountCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active proxies: %w", err)
	}
	paused := proxyAffinityIDSet(settings.PausedProxyIDs)
	out := make([]ProxyAffinityProxyLoad, 0, len(proxies))
	for _, proxy := range proxies {
		weight := settings.ProxyWeights[proxy.ID]
		if weight <= 0 {
			weight = 1
		}
		isPaused := false
		if _, ok := paused[proxy.ID]; ok {
			isPaused = true
		}
		assignable := proxy.Status == StatusActive && !isPaused && (settings.MaxAccountsPerProxy <= 0 || proxy.AccountCount < settings.MaxAccountsPerProxy)
		reason := ""
		switch {
		case proxy.Status != StatusActive:
			reason = "代理状态不是 active"
		case isPaused:
			reason = "代理已暂停自动分配"
		case settings.MaxAccountsPerProxy > 0 && proxy.AccountCount >= settings.MaxAccountsPerProxy:
			reason = "代理已达到账号上限"
		}
		load := ProxyAffinityProxyLoad{
			ProxyID:       proxy.ID,
			Name:          proxy.Name,
			Protocol:      proxy.Protocol,
			Host:          proxy.Host,
			Port:          proxy.Port,
			Status:        proxy.Status,
			AccountCount:  proxy.AccountCount,
			MaxAccounts:   settings.MaxAccountsPerProxy,
			Assignable:    assignable,
			IPAddress:     proxy.IPAddress,
			Country:       proxy.Country,
			CountryCode:   proxy.CountryCode,
			QualityStatus: proxy.QualityStatus,
			QualityGrade:  proxy.QualityGrade,
			Paused:        isPaused,
			Weight:        weight,
			EffectiveLoad: float64(proxy.AccountCount) / float64(weight),
			Reason:        reason,
		}
		if settings.MaxAccountsPerProxy > 0 {
			load.LoadPercent = float64(proxy.AccountCount) * 100 / float64(settings.MaxAccountsPerProxy)
		}
		out = append(out, load)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].EffectiveLoad == out[j].EffectiveLoad {
			return out[i].ProxyID < out[j].ProxyID
		}
		return out[i].EffectiveLoad < out[j].EffectiveLoad
	})
	return out, nil
}

func proxyAffinityAssignableLoads(loads []ProxyAffinityProxyLoad) []ProxyAffinityProxyLoad {
	assignable := make([]ProxyAffinityProxyLoad, 0, len(loads))
	for _, load := range loads {
		if load.Assignable {
			assignable = append(assignable, load)
		}
	}
	return assignable
}

func (s *ProxyAffinityService) isEligibleAccount(account *Account, settings ProxyAffinitySettings) (bool, string) {
	if account == nil {
		return false, "账号为空"
	}
	if account.ProxyID != nil {
		return false, "账号已经绑定代理"
	}
	if !account.IsSchedulable() {
		return false, "账号当前不可调度"
	}
	if !proxyAffinityPlatformAllowed(account.Platform, settings.Platforms) {
		return false, "账号平台不在代理亲和调度范围内"
	}
	switch account.Type {
	case AccountTypeAPIKey:
		if !settings.IncludeAPIKeyAccounts {
			return false, "API Key 账号未开启参与"
		}
	case AccountTypeOAuth, AccountTypeSetupToken:
		if !settings.IncludeOAuthAccounts {
			return false, "OAuth 账号未开启参与"
		}
	default:
		return false, "账号类型不支持代理自动分配"
	}
	if account.OwnerUserID == nil {
		if !settings.AdminAccountsEnabled {
			return false, "管理员账号未开启参与"
		}
		return true, ""
	}
	if !settings.UserOwnedEnabled {
		return false, "用户上传账号未开启参与"
	}
	shareMode := NormalizeAccountShareMode(account.ShareMode)
	if shareMode == AccountShareModePublic {
		if !settings.PublicAccountsEnabled {
			return false, "公有账号未开启参与"
		}
		if settings.OnlyApprovedPublicAccounts && NormalizeAccountShareStatus(account.ShareStatus) != AccountShareStatusApproved {
			return false, "公有账号尚未审核通过"
		}
		return true, ""
	}
	if !settings.PrivateAccountsEnabled {
		return false, "私有账号未开启参与"
	}
	return true, ""
}

func (s *ProxyAffinityService) isPreValidationEligibleAccount(account *Account, settings ProxyAffinitySettings) (bool, string) {
	if account == nil {
		return false, "账号为空"
	}
	if account.ProxyID != nil {
		return false, "账号已经绑定代理"
	}
	if !account.IsSchedulable() {
		return false, "账号当前不可调度"
	}
	if !proxyAffinityPlatformAllowed(account.Platform, settings.Platforms) {
		return false, "账号平台不在代理亲和调度范围内"
	}
	switch account.Type {
	case AccountTypeAPIKey:
		if !settings.IncludeAPIKeyAccounts {
			return false, "API Key 账号未开启参与"
		}
	case AccountTypeOAuth, AccountTypeSetupToken:
		if !settings.IncludeOAuthAccounts {
			return false, "OAuth 账号未开启参与"
		}
	default:
		return false, "账号类型不支持代理自动分配"
	}
	if account.OwnerUserID == nil {
		if !settings.AdminAccountsEnabled {
			return false, "管理员账号未开启参与"
		}
		return true, ""
	}
	if !settings.UserOwnedEnabled {
		return false, "用户上传账号未开启参与"
	}
	shareMode := NormalizeAccountShareMode(account.ShareMode)
	if shareMode == AccountShareModePublic {
		if !settings.PublicAccountsEnabled {
			return false, "公有账号未开启参与"
		}
		shareStatus := NormalizeAccountShareStatus(account.ShareStatus)
		if shareStatus == AccountShareStatusApproved {
			return true, ""
		}
		if settings.IncludePendingAccounts && shareStatus == AccountShareStatusPending {
			return true, ""
		}
		return false, "公有账号尚未审核通过"
	}
	if !settings.PrivateAccountsEnabled {
		return false, "私有账号未开启参与"
	}
	return true, ""
}

func (s *ProxyAffinityService) releaseInvalidBindings(ctx context.Context, settings ProxyAffinitySettings, proxyLoads []ProxyAffinityProxyLoad, dryRun bool) (int, []ProxyAffinityAssignment, error) {
	if !settings.ReleaseWhenAccountInactive && !settings.AllowReassignWhenProxyDown {
		return 0, nil, nil
	}

	activeProxyIDs := make(map[int64]struct{}, len(proxyLoads))
	for _, proxyLoad := range proxyLoads {
		if proxyLoad.Status == StatusActive {
			activeProxyIDs[proxyLoad.ProxyID] = struct{}{}
		}
	}

	accounts, err := s.listBoundAccounts(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("list bound accounts for proxy affinity release: %w", err)
	}

	events := []ProxyAffinityAssignment{}
	released := 0
	for i := range accounts {
		account := &accounts[i]
		if account.ProxyID == nil {
			continue
		}

		shouldRelease := false
		reason := ""
		if settings.AllowReassignWhenProxyDown {
			if _, ok := activeProxyIDs[*account.ProxyID]; !ok {
				shouldRelease = true
				reason = "当前绑定的代理不可用，释放后重新分配"
			}
		}
		if !shouldRelease && settings.ReleaseWhenAccountInactive {
			if ok, ineligibleReason := s.isEligibleBoundAccount(account, settings); !ok {
				shouldRelease = true
				reason = ineligibleReason
			}
		}
		if !shouldRelease {
			continue
		}

		event := ProxyAffinityAssignment{
			Candidate: proxyAffinityCandidateFromAccount(account),
			ProxyID:   *account.ProxyID,
			Action:    "released",
			Reason:    reason,
			DryRun:    dryRun,
		}
		if !dryRun {
			if err := s.releaseAccountProxy(ctx, account, reason); err != nil {
				event.Action = "failed"
				event.Reason = err.Error()
				events = append(events, event)
				continue
			}
		}
		released++
		events = append(events, event)
	}
	return released, events, nil
}

func (s *ProxyAffinityService) listBoundAccounts(ctx context.Context) ([]Account, error) {
	const pageSize = 1000
	out := []Account{}
	for page := 1; ; page++ {
		accounts, result, err := s.accountRepo.ListWithFilters(
			ctx,
			pagination.PaginationParams{Page: page, PageSize: pageSize, SortBy: "id", SortOrder: pagination.SortOrderAsc},
			"", "", "", "", "", 0, 0, "",
		)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if account.ProxyID != nil {
				out = append(out, account)
			}
		}
		if result == nil || int64(page*pageSize) >= result.Total || len(accounts) == 0 {
			break
		}
	}
	return out, nil
}

func (s *ProxyAffinityService) isEligibleBoundAccount(account *Account, settings ProxyAffinitySettings) (bool, string) {
	if account == nil {
		return false, "账号为空"
	}
	if !account.IsActive() {
		return false, "账号已停用或异常，释放代理绑定"
	}
	if !account.Schedulable {
		return false, "账号已被手动设为不可调度，释放代理绑定"
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !time.Now().Before(*account.ExpiresAt) {
		return false, "账号已过期并自动暂停，释放代理绑定"
	}
	if !proxyAffinityPlatformAllowed(account.Platform, settings.Platforms) {
		return false, "账号平台不在代理亲和调度范围内，释放代理绑定"
	}
	switch account.Type {
	case AccountTypeAPIKey:
		if !settings.IncludeAPIKeyAccounts {
			return false, "API Key 账号未开启参与，释放代理绑定"
		}
	case AccountTypeOAuth, AccountTypeSetupToken:
		if !settings.IncludeOAuthAccounts {
			return false, "OAuth 账号未开启参与，释放代理绑定"
		}
	default:
		return false, "账号类型不支持代理亲和调度，释放代理绑定"
	}
	if account.OwnerUserID == nil {
		if !settings.AdminAccountsEnabled {
			return false, "管理员账号未开启参与，释放代理绑定"
		}
		return true, ""
	}
	if !settings.UserOwnedEnabled {
		return false, "用户上传账号未开启参与，释放代理绑定"
	}
	shareMode := NormalizeAccountShareMode(account.ShareMode)
	if shareMode == AccountShareModePublic {
		if !settings.PublicAccountsEnabled {
			return false, "公有账号未开启参与，释放代理绑定"
		}
		if settings.OnlyApprovedPublicAccounts && NormalizeAccountShareStatus(account.ShareStatus) != AccountShareStatusApproved {
			phase := proxyAffinityPhaseFromAccount(account)
			if settings.IncludePendingAccounts && NormalizeAccountShareStatus(account.ShareStatus) == AccountShareStatusPending &&
				(phase == proxyAffinityPhasePreValidation || phase == proxyAffinityPhaseValidated) {
				return true, ""
			}
			return false, "公有账号尚未审核通过，释放代理绑定"
		}
		return true, ""
	}
	if !settings.PrivateAccountsEnabled {
		return false, "私有账号未开启参与，释放代理绑定"
	}
	return true, ""
}

func (s *ProxyAffinityService) assignAccountToProxy(ctx context.Context, account *Account, proxyID int64, reason string) error {
	if account == nil {
		return ErrAccountNilInput
	}
	if account.ProxyID != nil {
		return nil
	}
	extra := make(map[string]any, len(account.Extra)+4)
	for k, v := range account.Extra {
		extra[k] = v
	}
	extra[proxyAffinityExtraSource] = "proxy_affinity"
	extra[proxyAffinityExtraAssignedAt] = time.Now().UTC().Format(time.RFC3339)
	extra[proxyAffinityExtraReason] = reason
	extra[proxyAffinityExtraProxyID] = proxyID
	if reason == "pre_validation" {
		extra[proxyAffinityExtraPhase] = proxyAffinityPhasePreValidation
	} else if proxyAffinityStringFromAny(extra[proxyAffinityExtraPhase]) == "" {
		extra[proxyAffinityExtraPhase] = proxyAffinityPhaseValidated
	}
	account.Extra = extra
	account.ProxyID = &proxyID
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return fmt.Errorf("绑定代理失败: %w", err)
	}
	return nil
}

func (s *ProxyAffinityService) releaseAccountProxy(ctx context.Context, account *Account, reason string) error {
	if account == nil {
		return ErrAccountNilInput
	}
	if account.ProxyID == nil {
		return nil
	}
	extra := make(map[string]any, len(account.Extra)+2)
	for k, v := range account.Extra {
		extra[k] = v
	}
	extra[proxyAffinityExtraReleasedAt] = time.Now().UTC().Format(time.RFC3339)
	extra[proxyAffinityExtraReleaseReason] = reason
	account.Extra = extra
	account.ProxyID = nil
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return fmt.Errorf("释放代理绑定失败: %w", err)
	}
	return nil
}

func (s *ProxyAffinityService) setProxyAffinityPhase(ctx context.Context, account *Account, phase, reason string) error {
	updates := map[string]any{
		proxyAffinityExtraPhase: phase,
	}
	if strings.TrimSpace(reason) != "" {
		updates[proxyAffinityExtraValidationErr] = strings.TrimSpace(reason)
	}
	return s.updateProxyAffinityExtra(ctx, account, updates)
}

func (s *ProxyAffinityService) updateProxyAffinityExtra(ctx context.Context, account *Account, updates map[string]any) error {
	if account == nil {
		return ErrAccountNilInput
	}
	extra := make(map[string]any, len(account.Extra)+len(updates))
	for k, v := range account.Extra {
		extra[k] = v
	}
	for k, v := range updates {
		extra[k] = v
	}
	account.Extra = extra
	if err := s.accountRepo.Update(ctx, account); err != nil {
		return fmt.Errorf("更新代理亲和状态失败: %w", err)
	}
	return nil
}

func (s *ProxyAffinityService) accountBindingFromAccount(account *Account, proxyByID map[int64]ProxyAffinityProxyLoad, settings ProxyAffinitySettings) ProxyAffinityAccountBinding {
	binding := ProxyAffinityAccountBinding{
		ProxyAffinityCandidate: proxyAffinityCandidateFromAccount(account),
		HealthStatus:           "healthy",
	}
	if account == nil || account.ProxyID == nil {
		return binding
	}
	binding.ProxyID = *account.ProxyID
	if proxy, ok := proxyByID[*account.ProxyID]; ok {
		binding.ProxyName = proxy.Name
		binding.ProxyHost = proxy.Host
		binding.ProxyPort = proxy.Port
		if proxy.Status != StatusActive {
			binding.HealthStatus = "proxy_down"
			binding.HealthReason = "代理状态不是 active"
		} else if proxy.Paused {
			binding.HealthStatus = "proxy_paused"
			binding.HealthReason = "代理已暂停自动分配，当前绑定仍会保持"
		}
	} else {
		binding.HealthStatus = "proxy_missing"
		binding.HealthReason = "绑定的代理不存在或不可用"
	}
	if ok, reason := s.isEligibleBoundAccount(account, settings); !ok {
		binding.HealthStatus = "account_ineligible"
		binding.HealthReason = reason
	}
	if account.Extra != nil {
		binding.AssignedAt = proxyAffinityStringFromAny(account.Extra[proxyAffinityExtraAssignedAt])
		binding.AssignedBy = proxyAffinityStringFromAny(account.Extra[proxyAffinityExtraSource])
		binding.AssignReason = proxyAffinityStringFromAny(account.Extra[proxyAffinityExtraReason])
		binding.Phase = proxyAffinityStringFromAny(account.Extra[proxyAffinityExtraPhase])
		binding.LastTestAt = proxyAffinityStringFromAny(account.Extra[proxyAffinityExtraValidationAt])
		binding.LastTestErr = proxyAffinityStringFromAny(account.Extra[proxyAffinityExtraValidationErr])
	}
	return binding
}

func chooseProxyAffinityTarget(loads []ProxyAffinityProxyLoad, settings ProxyAffinitySettings) (ProxyAffinityProxyLoad, bool) {
	candidates := make([]ProxyAffinityProxyLoad, 0, len(loads))
	for _, load := range loads {
		if load.Assignable {
			candidates = append(candidates, load)
		}
	}
	if len(candidates) == 0 {
		return ProxyAffinityProxyLoad{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if settings.Strategy == proxyAffinityStrategyLeastLoaded {
			if candidates[i].AccountCount == candidates[j].AccountCount {
				return candidates[i].ProxyID < candidates[j].ProxyID
			}
			return candidates[i].AccountCount < candidates[j].AccountCount
		}
		if candidates[i].EffectiveLoad == candidates[j].EffectiveLoad {
			return candidates[i].ProxyID < candidates[j].ProxyID
		}
		return candidates[i].EffectiveLoad < candidates[j].EffectiveLoad
	})
	return candidates[0], true
}

func proxyAffinityAssignmentReason(settings ProxyAffinitySettings) string {
	if settings.Strategy == proxyAffinityStrategyLeastLoaded {
		return "选择当前账号数最少的可用代理"
	}
	return "按代理权重选择有效负载最低的可用代理"
}

func proxyAffinityCandidateFromAccount(account *Account) ProxyAffinityCandidate {
	if account == nil {
		return ProxyAffinityCandidate{}
	}
	return ProxyAffinityCandidate{
		AccountID:    account.ID,
		AccountName:  account.Name,
		Platform:     account.Platform,
		Type:         account.Type,
		ShareMode:    NormalizeAccountShareMode(account.ShareMode),
		ShareStatus:  NormalizeAccountShareStatus(account.ShareStatus),
		AccountLevel: NormalizeAccountLevel(account.AccountLevel),
		OwnerUserID:  account.OwnerUserID,
	}
}

func (s *ProxyAffinityService) appendAssignmentEvents(ctx context.Context, assignments []ProxyAffinityAssignment, source string) error {
	if len(assignments) == 0 {
		return nil
	}
	events, err := s.getRecentEvents(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, assignment := range assignments {
		if assignment.DryRun {
			continue
		}
		event := ProxyAffinityEvent{
			ID:          fmt.Sprintf("%d-%d-%s", now.UnixNano(), assignment.Candidate.AccountID, assignment.Action),
			OccurredAt:  now.Format(time.RFC3339),
			Source:      source,
			Action:      assignment.Action,
			AccountID:   assignment.Candidate.AccountID,
			AccountName: assignment.Candidate.AccountName,
			ProxyID:     assignment.ProxyID,
			ProxyName:   assignment.ProxyName,
			Reason:      assignment.Reason,
			DryRun:      assignment.DryRun,
		}
		events = append([]ProxyAffinityEvent{event}, events...)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		settings = DefaultProxyAffinitySettings()
	}
	maxEvents := settings.MaxStoredEvents
	if maxEvents <= 0 {
		maxEvents = proxyAffinityDefaultMaxEvents
	}
	if maxEvents > proxyAffinityMaxStoredEvents {
		maxEvents = proxyAffinityMaxStoredEvents
	}
	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	data, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("marshal proxy affinity events: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyProxyAffinityEvents, string(data))
}

func (s *ProxyAffinityService) getRecentEvents(ctx context.Context) ([]ProxyAffinityEvent, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyProxyAffinityEvents)
	if err != nil {
		if infraerrors.IsNotFound(err) {
			return []ProxyAffinityEvent{}, nil
		}
		return nil, fmt.Errorf("get proxy affinity events: %w", err)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []ProxyAffinityEvent{}, nil
	}
	var events []ProxyAffinityEvent
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		slog.Warn("failed to parse proxy affinity events, clearing invalid cache", "error", err)
		return []ProxyAffinityEvent{}, nil
	}
	return events, nil
}

func proxyAffinityProxyLoadMap(loads []ProxyAffinityProxyLoad) map[int64]ProxyAffinityProxyLoad {
	out := make(map[int64]ProxyAffinityProxyLoad, len(loads))
	for _, load := range loads {
		out[load.ProxyID] = load
	}
	return out
}

func proxyAffinityIDSet(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

func proxyAffinityStringFromAny(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return ""
	}
}

func intFromAny(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	default:
		return 0
	}
}

func int64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func maxProxyAffinityWeight(weight int) int {
	if weight <= 0 {
		return 1
	}
	return weight
}

func proxyAffinityExtraString(account *Account, key string) string {
	if account == nil || account.Extra == nil {
		return ""
	}
	return proxyAffinityStringFromAny(account.Extra[key])
}

func proxyAffinityPhaseFromAccount(account *Account) string {
	return proxyAffinityExtraString(account, proxyAffinityExtraPhase)
}

func proxyAffinityShouldReleaseOnValidationFailure(errMsg string) bool {
	normalized := strings.ToLower(strings.TrimSpace(errMsg))
	if normalized == "" {
		return true
	}
	temporaryMarkers := []string{
		"429",
		"rate limit",
		"rate_limit",
		"temporarily",
		"timeout",
		"context deadline",
		"deadline exceeded",
		"connection reset",
		"connection refused",
		"no such host",
		"proxyconnect",
		"tls handshake timeout",
		"too many requests",
	}
	for _, marker := range temporaryMarkers {
		if strings.Contains(normalized, marker) {
			return false
		}
	}
	return true
}

func applyProxyAffinityDefaultsForMissingFields(raw string, settings ProxyAffinitySettings) ProxyAffinitySettings {
	var present map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &present); err != nil {
		return settings
	}
	defaults := DefaultProxyAffinitySettings()
	if _, ok := present["pre_validation_enabled"]; !ok {
		settings.PreValidationEnabled = defaults.PreValidationEnabled
	}
	if _, ok := present["enforce_validation_proxy"]; !ok {
		settings.EnforceValidationProxy = defaults.EnforceValidationProxy
	}
	if _, ok := present["include_pending_accounts"]; !ok {
		settings.IncludePendingAccounts = defaults.IncludePendingAccounts
	}
	if _, ok := present["release_on_validation_failure"]; !ok {
		settings.ReleaseOnValidationFailure = defaults.ReleaseOnValidationFailure
	}
	if _, ok := present["retry_with_new_proxy_on_failure"]; !ok {
		settings.RetryWithNewProxyOnFailure = defaults.RetryWithNewProxyOnFailure
	}
	if _, ok := present["max_pre_validation_retries"]; !ok {
		settings.MaxPreValidationRetries = defaults.MaxPreValidationRetries
	}
	if _, ok := present["fallback_when_no_proxy"]; !ok {
		settings.FallbackWhenNoProxy = defaults.FallbackWhenNoProxy
	}
	return settings
}

func normalizeProxyAffinitySettings(settings ProxyAffinitySettings) ProxyAffinitySettings {
	defaults := DefaultProxyAffinitySettings()
	if settings.BatchSize <= 0 {
		settings.BatchSize = proxyAffinityDefaultBatchSize
	}
	if settings.BatchSize > proxyAffinityMaxBatchSize {
		settings.BatchSize = proxyAffinityMaxBatchSize
	}
	if settings.ScanIntervalMinutes <= 0 {
		settings.ScanIntervalMinutes = proxyAffinityDefaultScanIntervalMinutes
	}
	if settings.ScanIntervalMinutes > 1440 {
		settings.ScanIntervalMinutes = 1440
	}
	if settings.MaxAccountsPerProxy < 0 {
		settings.MaxAccountsPerProxy = 0
	}
	switch strings.TrimSpace(settings.Strategy) {
	case proxyAffinityStrategyLeastLoaded, proxyAffinityStrategyWeightedLeastLoaded:
		settings.Strategy = strings.TrimSpace(settings.Strategy)
	default:
		settings.Strategy = proxyAffinityStrategyWeightedLeastLoaded
	}
	if settings.MaxStoredEvents <= 0 {
		settings.MaxStoredEvents = proxyAffinityDefaultMaxEvents
	}
	if settings.MaxStoredEvents > proxyAffinityMaxStoredEvents {
		settings.MaxStoredEvents = proxyAffinityMaxStoredEvents
	}
	settings.Platforms = normalizeProxyAffinityPlatforms(settings.Platforms)
	if len(settings.Platforms) == 0 {
		settings.Platforms = defaults.Platforms
	}
	settings.PausedProxyIDs = normalizeProxyAffinityIDs(settings.PausedProxyIDs)
	settings.ProxyWeights = normalizeProxyAffinityWeights(settings.ProxyWeights)
	if strings.TrimSpace(settings.FallbackWhenNoProxy) == "" {
		settings.FallbackWhenNoProxy = defaults.FallbackWhenNoProxy
	}
	switch strings.ToLower(strings.TrimSpace(settings.FallbackWhenNoProxy)) {
	case proxyAffinityFallbackWait, proxyAffinityFallbackDirect, proxyAffinityFallbackReject:
		settings.FallbackWhenNoProxy = strings.ToLower(strings.TrimSpace(settings.FallbackWhenNoProxy))
	default:
		settings.FallbackWhenNoProxy = proxyAffinityFallbackWait
	}
	if settings.MaxPreValidationRetries < 0 {
		settings.MaxPreValidationRetries = 0
	}
	if settings.MaxPreValidationRetries > 10 {
		settings.MaxPreValidationRetries = 10
	}
	return settings
}

func normalizeProxyAffinityIDs(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeProxyAffinityWeights(weights map[int64]int) map[int64]int {
	out := map[int64]int{}
	for id, weight := range weights {
		if id <= 0 {
			continue
		}
		if weight <= 0 {
			continue
		}
		if weight > 100 {
			weight = 100
		}
		out[id] = weight
	}
	return out
}

func normalizeProxyAffinityPlatforms(platforms []string) []string {
	allowed := map[string]struct{}{
		PlatformOpenAI:      {},
		PlatformAnthropic:   {},
		PlatformGemini:      {},
		PlatformAntigravity: {},
	}
	seen := make(map[string]struct{}, len(platforms))
	out := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		platform = strings.ToLower(strings.TrimSpace(platform))
		if platform == "" {
			continue
		}
		if _, ok := allowed[platform]; !ok {
			continue
		}
		if _, ok := seen[platform]; ok {
			continue
		}
		seen[platform] = struct{}{}
		out = append(out, platform)
	}
	sort.Strings(out)
	return out
}

func proxyAffinityPlatformAllowed(platform string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	platform = strings.ToLower(strings.TrimSpace(platform))
	for _, item := range allowed {
		if item == platform {
			return true
		}
	}
	return false
}

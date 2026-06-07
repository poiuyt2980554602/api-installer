package service

import (
	"context"
	"encoding/json"
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

	proxyAffinityDefaultScanIntervalMinutes = 5
	proxyAffinityDefaultBatchSize           = 100
	proxyAffinityMaxBatchSize               = 1000

	proxyAffinityExtraSource        = "proxy_affinity_source"
	proxyAffinityExtraAssignedAt    = "proxy_affinity_assigned_at"
	proxyAffinityExtraReason        = "proxy_affinity_reason"
	proxyAffinityExtraProxyID       = "proxy_affinity_proxy_id"
	proxyAffinityExtraReleasedAt    = "proxy_affinity_released_at"
	proxyAffinityExtraReleaseReason = "proxy_affinity_release_reason"
)

type ProxyAffinitySettings struct {
	Enabled                    bool     `json:"enabled"`
	UserOwnedEnabled           bool     `json:"user_owned_enabled"`
	AdminAccountsEnabled       bool     `json:"admin_accounts_enabled"`
	PrivateAccountsEnabled     bool     `json:"private_accounts_enabled"`
	PublicAccountsEnabled      bool     `json:"public_accounts_enabled"`
	OnlyApprovedPublicAccounts bool     `json:"only_approved_public_accounts"`
	IncludeAPIKeyAccounts      bool     `json:"include_api_key_accounts"`
	IncludeOAuthAccounts       bool     `json:"include_oauth_accounts"`
	MaxAccountsPerProxy        int64    `json:"max_accounts_per_proxy"`
	BatchSize                  int      `json:"batch_size"`
	ScanIntervalMinutes        int      `json:"scan_interval_minutes"`
	Platforms                  []string `json:"platforms"`
	AllowReassignWhenProxyDown bool     `json:"allow_reassign_when_proxy_down"`
	ReleaseWhenAccountInactive bool     `json:"release_when_account_inactive"`
}

type ProxyAffinityOverview struct {
	Settings                   ProxyAffinitySettings    `json:"settings"`
	TotalProxies               int                      `json:"total_proxies"`
	AvailableProxies           int                      `json:"available_proxies"`
	FullProxies                int                      `json:"full_proxies"`
	BoundAccounts              int64                    `json:"bound_accounts"`
	UnassignedEligibleAccounts int64                    `json:"unassigned_eligible_accounts"`
	SkippedAccounts            int64                    `json:"skipped_accounts"`
	AverageLoad                float64                  `json:"average_load"`
	ProxyLoads                 []ProxyAffinityProxyLoad `json:"proxy_loads"`
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

type ProxyAffinityAssignRequest struct {
	DryRun    bool     `json:"dry_run"`
	Limit     int      `json:"limit"`
	Platforms []string `json:"platforms"`
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

	var bound, eligible, skipped int64
	for i := range accounts {
		account := &accounts[i]
		if account.ProxyID != nil {
			bound++
			continue
		}
		if ok, _ := s.isEligibleAccount(account, settings); ok {
			eligible++
		} else {
			skipped++
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

	return &ProxyAffinityOverview{
		Settings:                   settings,
		TotalProxies:               len(proxyLoads),
		AvailableProxies:           available,
		FullProxies:                full,
		BoundAccounts:              bound,
		UnassignedEligibleAccounts: eligible,
		SkippedAccounts:            skipped,
		AverageLoad:                averageLoad,
		ProxyLoads:                 proxyLoads,
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

		chosen, ok := chooseProxyAffinityTarget(assignable)
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
			Reason:    "选择当前账号数最少的可用代理",
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

	return result, nil
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

	result, err := s.AssignUnassigned(ctx, ProxyAffinityAssignRequest{
		DryRun: false,
		Limit:  settings.BatchSize,
	})
	if err != nil {
		slog.Warn("proxy affinity maintenance assignment failed", "error", err)
		return
	}
	_ = s.settingRepo.Set(ctx, SettingKeyProxyAffinityLastRunAt, now.Format(time.RFC3339))
	if result.Assigned > 0 || result.Released > 0 {
		slog.Info("proxy affinity maintenance completed", "assigned", result.Assigned, "released", result.Released, "scanned", result.Scanned)
	}
}

func (s *ProxyAffinityService) loadProxyLoads(ctx context.Context, settings ProxyAffinitySettings) ([]ProxyAffinityProxyLoad, error) {
	proxies, err := s.proxyRepo.ListActiveWithAccountCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active proxies: %w", err)
	}
	out := make([]ProxyAffinityProxyLoad, 0, len(proxies))
	for _, proxy := range proxies {
		load := ProxyAffinityProxyLoad{
			ProxyID:       proxy.ID,
			Name:          proxy.Name,
			Protocol:      proxy.Protocol,
			Host:          proxy.Host,
			Port:          proxy.Port,
			Status:        proxy.Status,
			AccountCount:  proxy.AccountCount,
			MaxAccounts:   settings.MaxAccountsPerProxy,
			Assignable:    proxy.Status == StatusActive && (settings.MaxAccountsPerProxy <= 0 || proxy.AccountCount < settings.MaxAccountsPerProxy),
			IPAddress:     proxy.IPAddress,
			Country:       proxy.Country,
			CountryCode:   proxy.CountryCode,
			QualityStatus: proxy.QualityStatus,
			QualityGrade:  proxy.QualityGrade,
		}
		if settings.MaxAccountsPerProxy > 0 {
			load.LoadPercent = float64(proxy.AccountCount) * 100 / float64(settings.MaxAccountsPerProxy)
		}
		out = append(out, load)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AccountCount == out[j].AccountCount {
			return out[i].ProxyID < out[j].ProxyID
		}
		return out[i].AccountCount < out[j].AccountCount
	})
	return out, nil
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

func chooseProxyAffinityTarget(loads []ProxyAffinityProxyLoad) (ProxyAffinityProxyLoad, bool) {
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
		if candidates[i].AccountCount == candidates[j].AccountCount {
			return candidates[i].ProxyID < candidates[j].ProxyID
		}
		return candidates[i].AccountCount < candidates[j].AccountCount
	})
	return candidates[0], true
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

func normalizeProxyAffinitySettings(settings ProxyAffinitySettings) ProxyAffinitySettings {
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
	settings.Platforms = normalizeProxyAffinityPlatforms(settings.Platforms)
	if len(settings.Platforms) == 0 {
		settings.Platforms = DefaultProxyAffinitySettings().Platforms
	}
	return settings
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

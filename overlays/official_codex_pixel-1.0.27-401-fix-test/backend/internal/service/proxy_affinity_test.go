package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestProxyAffinityAssignUnassignedUsesLeastLoadedProxy(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "pending-public", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePublic, ShareStatus: AccountShareStatusPending},
			{ID: 2, Name: "private", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved},
			{ID: 3, Name: "already-bound", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, ProxyID: ptrInt64(10)},
		},
	}
	proxyRepo := &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{
			{Proxy: Proxy{ID: 10, Name: "busy", Status: StatusActive}, AccountCount: 5},
			{Proxy: Proxy{ID: 11, Name: "idle", Status: StatusActive}, AccountCount: 1},
		},
	}
	settingRepo := &proxyAffinitySettingRepoStub{}
	svc := NewProxyAffinityService(settingRepo, proxyRepo, accountRepo)
	_, err := svc.UpdateSettings(ctx, DefaultProxyAffinitySettings())
	require.NoError(t, err)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, result.Assigned)
	require.Equal(t, 1, len(accountRepo.updated))
	require.NotNil(t, accountRepo.updated[0].ProxyID)
	require.Equal(t, int64(11), *accountRepo.updated[0].ProxyID)
	require.Equal(t, "proxy_affinity", accountRepo.updated[0].Extra[proxyAffinityExtraSource])
}

func TestProxyAffinityDryRunDoesNotUpdateAccounts(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "private", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 10, Name: "proxy", Status: StatusActive}}},
	}, accountRepo)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{DryRun: true, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, result.Assigned)
	require.Empty(t, accountRepo.updated)
}

func TestProxyAffinityReleaseInactiveBoundAccounts(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "manual-unschedulable", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved, ProxyID: ptrInt64(10)},
			{ID: 2, Name: "healthy-bound", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved, ProxyID: ptrInt64(10)},
			{ID: 3, Name: "unassigned", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 10, Name: "proxy", Status: StatusActive}, AccountCount: 2}},
	}, accountRepo)
	settings := DefaultProxyAffinitySettings()
	settings.ReleaseWhenAccountInactive = true
	_, err := svc.UpdateSettings(ctx, settings)
	require.NoError(t, err)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, result.Released)
	require.Equal(t, 1, result.Assigned)
	require.Len(t, accountRepo.updated, 2)
	require.Nil(t, accountRepo.updated[0].ProxyID)
	require.Equal(t, "proxy_affinity", accountRepo.updated[1].Extra[proxyAffinityExtraSource])
}

func TestProxyAffinityDryRunReleaseDoesNotUpdateAccounts(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "bound-to-missing-proxy", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved, ProxyID: ptrInt64(99)},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 10, Name: "proxy", Status: StatusActive}}},
	}, accountRepo)
	settings := DefaultProxyAffinitySettings()
	settings.AllowReassignWhenProxyDown = true
	_, err := svc.UpdateSettings(ctx, settings)
	require.NoError(t, err)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{DryRun: true, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, result.Released)
	require.Empty(t, accountRepo.updated)
	require.Len(t, result.Assignments, 1)
	require.Equal(t, "released", result.Assignments[0].Action)
}

func TestProxyAffinityWeightedLeastLoadedUsesProxyWeight(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "private", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{
			{Proxy: Proxy{ID: 10, Name: "low-weight", Status: StatusActive}, AccountCount: 2},
			{Proxy: Proxy{ID: 11, Name: "high-weight", Status: StatusActive}, AccountCount: 3},
		},
	}, accountRepo)
	settings := DefaultProxyAffinitySettings()
	settings.ProxyWeights = map[int64]int{10: 1, 11: 10}
	_, err := svc.UpdateSettings(ctx, settings)
	require.NoError(t, err)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 1, result.Assigned)
	require.Len(t, accountRepo.updated, 1)
	require.Equal(t, int64(11), *accountRepo.updated[0].ProxyID)
}

func TestProxyAffinityPausedProxyIsNotAssignable(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "private", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{
			{Proxy: Proxy{ID: 10, Name: "paused", Status: StatusActive}, AccountCount: 0},
			{Proxy: Proxy{ID: 11, Name: "active", Status: StatusActive}, AccountCount: 5},
		},
	}, accountRepo)
	settings := DefaultProxyAffinitySettings()
	settings.PausedProxyIDs = []int64{10}
	_, err := svc.UpdateSettings(ctx, settings)
	require.NoError(t, err)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{Limit: 1})
	require.NoError(t, err)
	require.Equal(t, 1, result.Assigned)
	require.Equal(t, int64(11), *accountRepo.updated[0].ProxyID)
}

func TestProxyAffinityReleaseDoesNotReleaseTemporaryRateLimitedAccount(t *testing.T) {
	ctx := context.Background()
	resetAt := time.Now().Add(time.Hour)
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "rate-limited", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved, ProxyID: ptrInt64(10), RateLimitResetAt: &resetAt},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 10, Name: "proxy", Status: StatusActive}, AccountCount: 1}},
	}, accountRepo)
	settings := DefaultProxyAffinitySettings()
	settings.ReleaseWhenAccountInactive = true
	_, err := svc.UpdateSettings(ctx, settings)
	require.NoError(t, err)

	result, err := svc.AssignUnassigned(ctx, ProxyAffinityAssignRequest{Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 0, result.Released)
	require.Empty(t, accountRepo.updated)
}

func TestProxyAffinityManualBindAndRelease(t *testing.T) {
	ctx := context.Background()
	accountRepo := &proxyAffinityAccountRepoStub{
		accounts: []Account{
			{ID: 1, Name: "private", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, OwnerUserID: ptrInt64(7), ShareMode: AccountShareModePrivate, ShareStatus: AccountShareStatusApproved},
		},
	}
	svc := NewProxyAffinityService(&proxyAffinitySettingRepoStub{}, &proxyAffinityProxyRepoStub{
		proxies: []ProxyWithAccountCount{{Proxy: Proxy{ID: 10, Name: "proxy", Status: StatusActive}, AccountCount: 0}},
	}, accountRepo)

	bound, err := svc.BindAccount(ctx, ProxyAffinityBindRequest{AccountID: 1, ProxyID: 10})
	require.NoError(t, err)
	require.Equal(t, "assigned", bound.Action)
	require.Len(t, accountRepo.updated, 1)
	require.Equal(t, int64(10), *accountRepo.updated[0].ProxyID)

	accountRepo.accounts[0].ProxyID = ptrInt64(10)
	released, err := svc.ReleaseAccount(ctx, ProxyAffinityReleaseRequest{AccountID: 1})
	require.NoError(t, err)
	require.Equal(t, "released", released.Action)
	require.Len(t, accountRepo.updated, 2)
	require.Nil(t, accountRepo.updated[1].ProxyID)
}

func ptrInt64(v int64) *int64 {
	return &v
}

type proxyAffinitySettingRepoStub struct {
	values map[string]string
}

func (r *proxyAffinitySettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	if r.values == nil {
		return nil, ErrSettingNotFound
	}
	v, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (r *proxyAffinitySettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (r *proxyAffinitySettingRepoStub) Set(ctx context.Context, key, value string) error {
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}

func (r *proxyAffinitySettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if r.values != nil {
			if v, ok := r.values[key]; ok {
				out[key] = v
			}
		}
	}
	return out, nil
}

func (r *proxyAffinitySettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	for key, value := range settings {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *proxyAffinitySettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *proxyAffinitySettingRepoStub) Delete(ctx context.Context, key string) error {
	delete(r.values, key)
	return nil
}

type proxyAffinityProxyRepoStub struct {
	proxies []ProxyWithAccountCount
}

func (r *proxyAffinityProxyRepoStub) Create(ctx context.Context, proxy *Proxy) error { return nil }
func (r *proxyAffinityProxyRepoStub) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	for _, proxy := range r.proxies {
		if proxy.ID == id {
			return &proxy.Proxy, nil
		}
	}
	return nil, ErrProxyNotFound
}
func (r *proxyAffinityProxyRepoStub) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	return nil, nil
}
func (r *proxyAffinityProxyRepoStub) Update(ctx context.Context, proxy *Proxy) error { return nil }
func (r *proxyAffinityProxyRepoStub) Delete(ctx context.Context, id int64) error     { return nil }
func (r *proxyAffinityProxyRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *proxyAffinityProxyRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *proxyAffinityProxyRepoStub) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *proxyAffinityProxyRepoStub) ListActive(ctx context.Context) ([]Proxy, error) {
	out := make([]Proxy, 0, len(r.proxies))
	for _, proxy := range r.proxies {
		out = append(out, proxy.Proxy)
	}
	return out, nil
}
func (r *proxyAffinityProxyRepoStub) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	return append([]ProxyWithAccountCount(nil), r.proxies...), nil
}
func (r *proxyAffinityProxyRepoStub) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	return false, nil
}
func (r *proxyAffinityProxyRepoStub) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	return 0, nil
}
func (r *proxyAffinityProxyRepoStub) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	return nil, nil
}

type proxyAffinityAccountRepoStub struct {
	accounts []Account
	updated  []*Account
}

func (r *proxyAffinityAccountRepoStub) Create(ctx context.Context, account *Account) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) GetByID(ctx context.Context, id int64) (*Account, error) {
	for i := range r.accounts {
		if r.accounts[i].ID == id {
			return &r.accounts[i], nil
		}
	}
	return nil, ErrAccountNotFound
}
func (r *proxyAffinityAccountRepoStub) GetByIDs(ctx context.Context, ids []int64) ([]*Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ExistsByID(ctx context.Context, id int64) (bool, error) {
	return true, nil
}
func (r *proxyAffinityAccountRepoStub) GetByCRSAccountID(ctx context.Context, crsAccountID string) (*Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) FindByExtraField(ctx context.Context, key string, value any) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListCRSAccountIDs(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) Update(ctx context.Context, account *Account) error {
	cp := *account
	if account.Extra != nil {
		cp.Extra = map[string]any{}
		for k, v := range account.Extra {
			cp.Extra[k] = v
		}
	}
	r.updated = append(r.updated, &cp)
	return nil
}
func (r *proxyAffinityAccountRepoStub) Delete(ctx context.Context, id int64) error { return nil }
func (r *proxyAffinityAccountRepoStub) List(ctx context.Context, params pagination.PaginationParams) ([]Account, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListWithFilters(ctx context.Context, params pagination.PaginationParams, platform, accountType, status, search, ownerSearch string, groupID, proxyID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	out := append([]Account(nil), r.accounts...)
	total := int64(len(out))
	offset := params.Offset()
	limit := params.Limit()
	if offset >= len(out) {
		return []Account{}, &pagination.PaginationResult{Total: total, Page: params.Page, PageSize: limit}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], &pagination.PaginationResult{Total: total, Page: params.Page, PageSize: limit}, nil
}
func (r *proxyAffinityAccountRepoStub) ListByGroup(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListActive(ctx context.Context) ([]Account, error) {
	return append([]Account(nil), r.accounts...), nil
}
func (r *proxyAffinityAccountRepoStub) ListByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) UpdateLastUsed(ctx context.Context, id int64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) BatchUpdateLastUsed(ctx context.Context, updates map[int64]time.Time) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) SetError(ctx context.Context, id int64, errorMsg string) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ClearError(ctx context.Context, id int64) error { return nil }
func (r *proxyAffinityAccountRepoStub) SetSchedulable(ctx context.Context, id int64, schedulable bool) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) AutoPauseExpiredAccounts(ctx context.Context, now time.Time) (int64, error) {
	return 0, nil
}
func (r *proxyAffinityAccountRepoStub) BindGroups(ctx context.Context, accountID int64, groupIDs []int64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulable(ctx context.Context) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableByGroupID(ctx context.Context, groupID int64) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableByGroupIDAndPlatform(ctx context.Context, groupID int64, platform string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableByGroupIDAndPlatforms(ctx context.Context, groupID int64, platforms []string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableUngroupedByPlatform(ctx context.Context, platform string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) ListSchedulableUngroupedByPlatforms(ctx context.Context, platforms []string) ([]Account, error) {
	return nil, nil
}
func (r *proxyAffinityAccountRepoStub) SetRateLimited(ctx context.Context, id int64, resetAt time.Time) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) SetModelRateLimit(ctx context.Context, id int64, scope string, resetAt time.Time) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) SetOverloaded(ctx context.Context, id int64, until time.Time) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) SetTempUnschedulable(ctx context.Context, id int64, until time.Time, reason string) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ClearTempUnschedulable(ctx context.Context, id int64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ClearRateLimit(ctx context.Context, id int64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ClearAntigravityQuotaScopes(ctx context.Context, id int64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ClearModelRateLimits(ctx context.Context, id int64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) UpdateSessionWindow(ctx context.Context, id int64, start, end *time.Time, status string) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) BulkUpdate(ctx context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	return 0, nil
}
func (r *proxyAffinityAccountRepoStub) IncrementQuotaUsed(ctx context.Context, id int64, amount float64) error {
	return nil
}
func (r *proxyAffinityAccountRepoStub) ResetQuotaUsed(ctx context.Context, id int64) error {
	return nil
}

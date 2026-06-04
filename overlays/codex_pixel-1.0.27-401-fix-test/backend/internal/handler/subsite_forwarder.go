package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	subsiteForwardAffinityTTL      = 12 * time.Hour
	subsiteForwardMaxBodyBytes     = 64 << 20
	subsiteForwardHealthyWindow    = 3 * time.Minute
	subsiteForwardRouteSnapshotTTL = 1500 * time.Millisecond
	subsiteForwardRouteStaleTTL    = 10 * time.Second
	subsiteForwardAccountRouteTTL  = 5 * time.Second
	subsiteForwardHeader           = "X-Sub2API-Master-Forwarded"
	subsiteForwardTargetHeader     = "X-Sub2API-Forward-Target"
	subsiteForwardAffinityHeader   = "X-Sub2API-Forward-Affinity"
	subsiteForwardReasonHeader     = "X-Sub2API-Forward-Reason"
	subsiteForwardLeaseHeader      = "X-Sub2API-Lease-ID"
	subsiteForwardAccountHeader    = "X-Sub2API-Account-ID"
	subsiteForwardRequestIDHeader  = "X-Sub2API-Request-ID"
	subsiteForwardNoCandidateCode  = "SUBSITE_FORWARD_NO_CANDIDATE"
	subsiteForwardMasterDirectKey  = "subsite_forward_master_direct"
	subsiteForwardMasterDirectWhy  = "subsite_forward_master_direct_reason"
)

var errSubsiteForwardNoCandidate = errors.New("no healthy subsite is available for forwarding")

type SubsiteForwarder struct {
	subsiteService *service.SubsiteService
	settingService *service.SettingService
	accountRepo    service.AccountRepository
	client         *http.Client
	affinity       *subsiteAffinityStore
	routes         *subsiteRouteSnapshotStore
	accountRoutes  *subsiteAccountRouteStore
}

type subsiteForwardContext struct {
	Key              string
	Type             string
	Platform         string
	Model            string
	Session          string
	APIKeyID         int64
	UserID           int64
	GroupID          int64
	AccountID        int64
	LeaseID          string
	SubsiteRequestID string
	Strict           bool
	Diagnostics      map[string]any
}

type subsiteRouteCandidate struct {
	subsite service.Subsite
	load    service.SubsiteForwardSiteStat
}

type subsiteAccountRouteEligibility struct {
	HasRelay        bool
	HasMasterDirect bool
	HasLocalOnly    bool
	TotalAccounts   int
	Reason          string
	LoadedAt        time.Time
}

func NewSubsiteForwarder(subsiteService *service.SubsiteService, settingService *service.SettingService, accountRepo service.AccountRepository) *SubsiteForwarder {
	return &SubsiteForwarder{
		subsiteService: subsiteService,
		settingService: settingService,
		accountRepo:    accountRepo,
		client: &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          512,
				MaxIdleConnsPerHost:   128,
				MaxConnsPerHost:       256,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
		affinity:      newSubsiteAffinityStore(subsiteForwardAffinityTTL),
		routes:        newSubsiteRouteSnapshotStore(subsiteForwardRouteSnapshotTTL, subsiteForwardRouteStaleTTL),
		accountRoutes: newSubsiteAccountRouteStore(subsiteForwardAccountRouteTTL),
	}
}

func (f *SubsiteForwarder) Mode(ctx context.Context, fallback string) string {
	if f == nil || f.settingService == nil {
		return service.NormalizeSubsiteForwardMode(fallback)
	}
	return f.settingService.GetSubsiteForwardMode(ctx, fallback)
}

func (f *SubsiteForwarder) ForwardGatewayRequest(c *gin.Context) bool {
	if f == nil || f.subsiteService == nil {
		return false
	}
	if strings.EqualFold(c.GetHeader(subsiteForwardHeader), "1") {
		writeSubsiteForwardError(c, http.StatusBadGateway, "SUBSITE_FORWARD_LOOP", "subsite forward loop detected")
		return true
	}

	routeCtx := f.forwardContext(c, nil)
	if ok, reason := f.shouldUseMasterDirect(c.Request.Context(), routeCtx); ok {
		c.Set(subsiteForwardMasterDirectKey, true)
		c.Set(subsiteForwardMasterDirectWhy, reason)
		return false
	}

	body, err := readForwardBody(c)
	if err != nil {
		writeSubsiteForwardError(c, http.StatusBadRequest, "SUBSITE_FORWARD_BODY", err.Error())
		return true
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	forwardCtx := f.forwardContext(c, body)
	excluded := make(map[string]struct{})
	fallbackFrom := ""
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		candidate, reason, err := f.selectSubsite(c.Request.Context(), forwardCtx, excluded)
		if err != nil {
			if recordErr := f.recordForwardEvent(c, forwardCtx, nil, fallbackFrom, reason, "no_candidate", 0, 0, len(body), 0, err); recordErr != nil {
				log.Printf("[subsite-forwarder] record no_candidate event failed: %v", recordErr)
			}
			writeSubsiteForwardError(c, http.StatusServiceUnavailable, subsiteForwardNoCandidateCode, err.Error())
			return true
		}
		if candidate == nil {
			if recordErr := f.recordForwardEvent(c, forwardCtx, nil, fallbackFrom, reason, "no_candidate", 0, 0, len(body), 0, errSubsiteForwardNoCandidate); recordErr != nil {
				log.Printf("[subsite-forwarder] record no_candidate event failed: %v", recordErr)
			}
			writeSubsiteForwardError(c, http.StatusServiceUnavailable, subsiteForwardNoCandidateCode, errSubsiteForwardNoCandidate.Error())
			return true
		}

		proxyResult, err := f.proxy(c, candidate, body, reason, forwardCtx)
		if err == nil {
			outcome := forwardOutcomeForProxyResult(proxyResult, fallbackFrom)
			var eventErr error
			if outcome == "success" || outcome == "fallback" {
				if affinityErr := f.storeAffinity(c.Request.Context(), forwardCtx, candidate, reason, ""); affinityErr != nil {
					log.Printf("[subsite-forwarder] store affinity failed: %v", affinityErr)
				}
			} else {
				eventErr = fmt.Errorf("subsite returned %d", proxyResult.StatusCode)
			}
			if recordErr := f.recordForwardEvent(c, forwardCtx, candidate, fallbackFrom, reason, outcome, proxyResult.StatusCode, proxyResult.LatencyMS, len(body), proxyResult.ResponseBytes, eventErr); recordErr != nil {
				log.Printf("[subsite-forwarder] record %s event failed: %v", outcome, recordErr)
			}
			return true
		}
		lastErr = err
		outcome := "failed"
		if proxyResult.RetryableSelectionFailure {
			outcome = "no_candidate"
			if proxyResult.SelectionFailureCode != "" {
				reason = proxyResult.SelectionFailureCode
			}
		}
		if recordErr := f.recordForwardEvent(c, forwardCtx, candidate, fallbackFrom, reason, outcome, proxyResult.StatusCode, proxyResult.LatencyMS, len(body), proxyResult.ResponseBytes, err); recordErr != nil {
			log.Printf("[subsite-forwarder] record failed event failed: %v", recordErr)
		}
		if c.Writer.Written() {
			return true
		}
		forwardCtx.LeaseID = ""
		forwardCtx.AccountID = 0
		excluded[candidate.SubsiteID] = struct{}{}
		fallbackFrom = candidate.SubsiteID
	}
	if lastErr == nil {
		lastErr = errSubsiteForwardNoCandidate
	}
	writeSubsiteForwardError(c, http.StatusBadGateway, "SUBSITE_FORWARD_FAILED", lastErr.Error())
	return true
}

func SubsiteForwardMasterDirect(c *gin.Context) (bool, string) {
	if c == nil {
		return false, ""
	}
	enabled, _ := c.Get(subsiteForwardMasterDirectKey)
	if enabled != true {
		return false, ""
	}
	reason, _ := c.Get(subsiteForwardMasterDirectWhy)
	return true, strings.TrimSpace(fmt.Sprint(reason))
}

func (f *SubsiteForwarder) shouldUseMasterDirect(ctx context.Context, forwardCtx *subsiteForwardContext) (bool, string) {
	if f == nil || f.accountRepo == nil || forwardCtx == nil || forwardCtx.GroupID <= 0 || strings.TrimSpace(forwardCtx.Platform) == "" {
		return false, ""
	}
	eligibility, err := f.accountRouteEligibility(ctx, forwardCtx.GroupID, forwardCtx.Platform)
	if err != nil {
		log.Printf("[subsite-forwarder] account route eligibility failed; using subsite forward path: group=%d platform=%s err=%v", forwardCtx.GroupID, forwardCtx.Platform, err)
		return false, ""
	}
	if eligibility == nil {
		return false, ""
	}
	if eligibility.HasRelay {
		return false, ""
	}
	if eligibility.HasMasterDirect {
		return true, eligibility.Reason
	}
	return false, ""
}

func (f *SubsiteForwarder) accountRouteEligibility(ctx context.Context, groupID int64, platform string) (*subsiteAccountRouteEligibility, error) {
	if f == nil || f.accountRepo == nil {
		return nil, nil
	}
	if f.accountRoutes == nil {
		f.accountRoutes = newSubsiteAccountRouteStore(subsiteForwardAccountRouteTTL)
	}
	key := fmt.Sprintf("%d:%s", groupID, strings.ToLower(strings.TrimSpace(platform)))
	return f.accountRoutes.Get(ctx, key, func(loadCtx context.Context) (*subsiteAccountRouteEligibility, error) {
		accounts, err := f.accountRepo.ListByGroup(loadCtx, groupID)
		if err != nil {
			return nil, err
		}
		eligibility := &subsiteAccountRouteEligibility{
			LoadedAt: time.Now(),
		}
		for i := range accounts {
			account := &accounts[i]
			if !strings.EqualFold(strings.TrimSpace(account.Platform), strings.TrimSpace(platform)) {
				continue
			}
			eligibility.TotalAccounts++
			switch service.AccountSubsiteRoutePolicyResolved(account) {
			case service.AccountSubsiteRoutePolicySubsiteRelay:
				if account.IsSchedulable() {
					eligibility.HasRelay = true
				}
			case service.AccountSubsiteRoutePolicyMasterDirect:
				eligibility.HasMasterDirect = true
			case service.AccountSubsiteRoutePolicyLocalOnly:
				eligibility.HasLocalOnly = true
			}
		}
		switch {
		case eligibility.HasRelay:
			eligibility.Reason = "账号池包含可进入子站的官方账号，使用子站转发"
		case eligibility.HasMasterDirect:
			eligibility.Reason = "账号池只有主站直连账号，跳过子站转发"
		case eligibility.HasLocalOnly:
			eligibility.Reason = "账号池只有仅主站本地账号"
		default:
			eligibility.Reason = "账号池没有可调度账号"
		}
		return eligibility, nil
	})
}

func (f *SubsiteForwarder) selectSubsite(ctx context.Context, forwardCtx *subsiteForwardContext, excluded map[string]struct{}) (*service.Subsite, string, error) {
	snapshot, err := f.routeSnapshot(ctx)
	if err != nil {
		return nil, "", err
	}
	blockedSubsites, blockedAccounts, blockedLeases := activeCircuitMaps(snapshot.breakers)
	now := time.Now()
	excludedSubsites := sortedStringSet(excluded)
	ineligibleSubsites := make([]string, 0)
	capacityLimitedSubsites := make([]string, 0)
	capabilityMismatchSubsites := make([]string, 0)
	active := make([]subsiteRouteCandidate, 0, len(snapshot.items))
	for _, item := range snapshot.items {
		if !isForwardableSubsite(item, now) {
			ineligibleSubsites = append(ineligibleSubsites, item.SubsiteID)
			continue
		}
		if !subsiteSupportsForwardPlatform(item, forwardCtx.Platform) {
			capabilityMismatchSubsites = append(capabilityMismatchSubsites, item.SubsiteID)
			continue
		}
		if _, skip := excluded[item.SubsiteID]; skip {
			continue
		}
		if _, skip := blockedSubsites[item.SubsiteID]; skip {
			continue
		}
		load := snapshot.loadBySubsite[item.SubsiteID]
		if isSubsiteAtRelayCapacity(item, load) {
			capacityLimitedSubsites = append(capacityLimitedSubsites, item.SubsiteID)
			continue
		}
		active = append(active, subsiteRouteCandidate{
			subsite: item,
			load:    load,
		})
	}
	diagnostics := map[string]any{
		"total_subsites":                  len(snapshot.items),
		"candidate_subsites":              len(active),
		"ineligible_subsite_ids":          sortedStrings(ineligibleSubsites),
		"capability_mismatch_subsite_ids": sortedStrings(capabilityMismatchSubsites),
		"capacity_limited_subsite_ids":    sortedStrings(capacityLimitedSubsites),
		"retry_excluded_subsite_ids":      excludedSubsites,
		"circuit_blocked_subsite_ids":     sortedStringSet(blockedSubsites),
		"circuit_blocked_account_ids":     sortedInt64Set(blockedAccounts),
		"circuit_blocked_lease_ids":       sortedStringSet(blockedLeases),
		"affinity_key":                    forwardCtx.Key,
		"api_key_id":                      forwardCtx.APIKeyID,
		"user_id":                         forwardCtx.UserID,
		"group_id":                        forwardCtx.GroupID,
		"platform":                        forwardCtx.Platform,
		"requested_model":                 forwardCtx.Model,
		"loaded_at":                       snapshot.loadedAt.Format(time.RFC3339Nano),
	}
	forwardCtx.Diagnostics = diagnostics
	if len(active) == 0 {
		return nil, "", errSubsiteForwardNoCandidate
	}
	if persisted, err := f.subsiteService.GetForwardAffinity(ctx, forwardCtx.Key); err == nil && persisted != nil {
		if persisted.AccountID > 0 {
			if _, blocked := blockedAccounts[persisted.AccountID]; blocked {
				diagnostics["db_affinity_blocked_account_id"] = persisted.AccountID
				persisted.AccountID = 0
				persisted.LeaseID = ""
			}
		}
		if strings.TrimSpace(persisted.LeaseID) != "" {
			if _, blocked := blockedLeases[persisted.LeaseID]; blocked {
				diagnostics["db_affinity_blocked_lease_id"] = persisted.LeaseID
				persisted.LeaseID = ""
				persisted.AccountID = 0
			}
		}
		for i := range active {
			if active[i].subsite.SubsiteID == persisted.SubsiteID {
				forwardCtx.AccountID = persisted.AccountID
				forwardCtx.LeaseID = persisted.LeaseID
				forwardCtx.Strict = persisted.Locked
				diagnostics["selected_subsite_id"] = active[i].subsite.SubsiteID
				diagnostics["selected_reason"] = "db_affinity"
				diagnostics["strict_route_hint"] = persisted.Locked
				return &active[i].subsite, "db_affinity", nil
			}
		}
		diagnostics["db_affinity_miss_subsite_id"] = persisted.SubsiteID
	} else if err != nil {
		diagnostics["db_affinity_error"] = err.Error()
	}
	if id, ok := f.affinity.Get(forwardCtx.Key); ok {
		for i := range active {
			if active[i].subsite.SubsiteID == id {
				diagnostics["selected_subsite_id"] = active[i].subsite.SubsiteID
				diagnostics["selected_reason"] = "memory_affinity"
				return &active[i].subsite, "memory_affinity", nil
			}
		}
		diagnostics["memory_affinity_miss_subsite_id"] = id
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].load.ActiveRequests != active[j].load.ActiveRequests {
			return active[i].load.ActiveRequests < active[j].load.ActiveRequests
		}
		if active[i].load.QueuedUsage != active[j].load.QueuedUsage {
			return active[i].load.QueuedUsage < active[j].load.QueuedUsage
		}
		if active[i].load.QPS != active[j].load.QPS {
			return active[i].load.QPS < active[j].load.QPS
		}
		if active[i].load.ActiveLeases != active[j].load.ActiveLeases {
			return active[i].load.ActiveLeases < active[j].load.ActiveLeases
		}
		if active[i].subsite.HealthScore != active[j].subsite.HealthScore {
			return active[i].subsite.HealthScore > active[j].subsite.HealthScore
		}
		if active[i].subsite.LastHeartbeatAt == nil {
			return false
		}
		if active[j].subsite.LastHeartbeatAt == nil {
			return true
		}
		return active[i].subsite.LastHeartbeatAt.After(*active[j].subsite.LastHeartbeatAt)
	})
	bestScore := active[0].subsite.HealthScore
	bestLoad := active[0].load.ActiveRequests
	bestQueue := active[0].load.QueuedUsage
	bestQPS := active[0].load.QPS
	bestLeases := active[0].load.ActiveLeases
	top := active[:0]
	for _, item := range active {
		if item.subsite.HealthScore == bestScore && item.load.ActiveRequests == bestLoad && item.load.QueuedUsage == bestQueue && item.load.QPS == bestQPS && item.load.ActiveLeases == bestLeases {
			top = append(top, item)
		}
	}
	selected := top[rand.Intn(len(top))]
	diagnostics["selected_subsite_id"] = selected.subsite.SubsiteID
	diagnostics["selected_reason"] = "least_load"
	diagnostics["selected_active_requests"] = selected.load.ActiveRequests
	diagnostics["selected_queued_usage"] = selected.load.QueuedUsage
	diagnostics["selected_qps"] = selected.load.QPS
	diagnostics["selected_active_leases"] = selected.load.ActiveLeases
	diagnostics["selected_health_score"] = selected.subsite.HealthScore
	return &selected.subsite, "least_load", nil
}

func (f *SubsiteForwarder) routeSnapshot(ctx context.Context) (*subsiteRouteSnapshot, error) {
	if f == nil || f.subsiteService == nil {
		return nil, errors.New("subsite forwarder dependencies are nil")
	}
	if f.routes == nil {
		f.routes = newSubsiteRouteSnapshotStore(subsiteForwardRouteSnapshotTTL, subsiteForwardRouteStaleTTL)
	}
	return f.routes.Get(ctx, func(ctx context.Context) (*subsiteRouteSnapshot, error) {
		items, _, err := f.subsiteService.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 1000}, service.ListSubsitesFilter{})
		if err != nil {
			return nil, err
		}
		breakers, err := f.subsiteService.ListActiveCircuitBreakers(ctx)
		if err != nil {
			log.Printf("[subsite-forwarder] list circuit breakers failed; routing without circuit filter: %v", err)
			breakers = nil
		}
		loadBySubsite := make(map[string]service.SubsiteForwardSiteStat)
		stats, err := f.subsiteService.ForwardRouteStats(ctx)
		if err != nil {
			log.Printf("[subsite-forwarder] forward stats failed; routing without load stats: %v", err)
		}
		for _, item := range stats {
			loadBySubsite[item.SubsiteID] = item
		}
		return &subsiteRouteSnapshot{
			items:         items,
			breakers:      breakers,
			loadBySubsite: loadBySubsite,
			loadedAt:      time.Now(),
		}, nil
	})
}

type subsiteProxyResult struct {
	StatusCode                int
	LatencyMS                 int64
	ResponseBytes             int64
	ClientError               bool
	RetryableSelectionFailure bool
	SelectionFailureCode      string
}

type subsiteHTTPStatusError struct {
	statusCode int
	body       string
}

func (e *subsiteHTTPStatusError) Error() string {
	if e == nil {
		return "subsite returned an error status"
	}
	if e.body != "" {
		return fmt.Sprintf("subsite returned %d: %s", e.statusCode, e.body)
	}
	return fmt.Sprintf("subsite returned %d", e.statusCode)
}

type subsiteAuthorizationSelectionError struct {
	statusCode int
	code       string
	body       string
}

func (e *subsiteAuthorizationSelectionError) Error() string {
	if e == nil {
		return "subsite authorization selection failed"
	}
	if e.code != "" {
		return fmt.Sprintf("subsite authorization selection failed: %s", e.code)
	}
	return fmt.Sprintf("subsite authorization selection failed with status %d", e.statusCode)
}

func (f *SubsiteForwarder) proxy(c *gin.Context, subsite *service.Subsite, body []byte, reason string, forwardCtx *subsiteForwardContext) (subsiteProxyResult, error) {
	targetURL, err := buildSubsiteForwardURL(subsite.PublicURL, c.Request.URL)
	if err != nil {
		return subsiteProxyResult{}, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, bytes.NewReader(body))
	if err != nil {
		return subsiteProxyResult{}, err
	}
	copyForwardHeaders(req.Header, c.Request.Header)
	if requestID := requestIDFromContextOrHeader(c); requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	req.Header.Set(subsiteForwardHeader, "1")
	req.Header.Set("X-Forwarded-Host", c.Request.Host)
	req.Header.Set("X-Forwarded-Proto", forwardedProto(c))
	req.Header.Set("X-Forwarded-For", c.ClientIP())
	if forwardCtx.AccountID > 0 {
		req.Header.Set(service.SubsiteRouteHeaderPreferredAccountID, fmt.Sprint(forwardCtx.AccountID))
	}
	if forwardCtx.LeaseID != "" {
		req.Header.Set(service.SubsiteRouteHeaderPreferredLeaseID, forwardCtx.LeaseID)
	}
	if forwardCtx.Strict {
		req.Header.Set(service.SubsiteRouteHeaderPreferredStrict, "1")
	}
	if forwardCtx.AccountID > 0 || forwardCtx.LeaseID != "" {
		timestamp := time.Now().UTC().Format(time.RFC3339)
		nonce := requestShapeHash(c.Request.Method, c.Request.URL.RequestURI(), fmt.Sprintf("%s:%d", subsite.SubsiteID, time.Now().UnixNano()), forwardCtx.APIKeyID, forwardCtx.UserID, forwardCtx.GroupID)
		signature, err := f.subsiteService.SignRouteHint(c.Request.Context(), subsite.SubsiteID, c.Request.Method, c.Request.URL.RequestURI(), timestamp, nonce, forwardCtx.LeaseID, forwardCtx.AccountID)
		if err != nil {
			return subsiteProxyResult{}, fmt.Errorf("sign subsite route hint: %w", err)
		}
		req.Header.Set(service.SubsiteRouteHeaderTimestamp, timestamp)
		req.Header.Set(service.SubsiteRouteHeaderNonce, nonce)
		req.Header.Set(service.SubsiteRouteHeaderSignature, signature)
	}

	started := time.Now()
	resp, err := f.client.Do(req)
	if err != nil {
		return subsiteProxyResult{}, err
	}
	defer resp.Body.Close()

	if leaseID := strings.TrimSpace(resp.Header.Get(subsiteForwardLeaseHeader)); leaseID != "" {
		forwardCtx.LeaseID = leaseID
	}
	if requestID := strings.TrimSpace(resp.Header.Get(subsiteForwardRequestIDHeader)); requestID != "" {
		forwardCtx.SubsiteRequestID = requestID
	}
	if accountID := parseForwardAccountID(resp.Header.Get(subsiteForwardAccountHeader)); accountID > 0 {
		forwardCtx.AccountID = accountID
	}
	latencyMS := time.Since(started).Milliseconds()
	if shouldFailoverSubsiteStatus(resp.StatusCode) {
		payload, _ := io.ReadAll(http.MaxBytesReader(c.Writer, resp.Body, 1<<20))
		if code, retryable := classifySubsiteAuthorizationSelectionFailure(resp.StatusCode, payload); code != "" {
			if retryable {
				return subsiteProxyResult{
						StatusCode:                resp.StatusCode,
						LatencyMS:                 latencyMS,
						ResponseBytes:             int64(len(payload)),
						RetryableSelectionFailure: true,
						SelectionFailureCode:      code,
					}, &subsiteAuthorizationSelectionError{
						statusCode: resp.StatusCode,
						code:       code,
						body:       strings.TrimSpace(string(payload)),
					}
			}
			copyResponseHeaders(c.Writer.Header(), resp.Header)
			c.Header(subsiteForwardTargetHeader, subsite.SubsiteID)
			c.Header(subsiteForwardReasonHeader, reason)
			c.Header(subsiteForwardAffinityHeader, "miss")
			c.Header("X-Sub2API-Forward-Latency-MS", fmt.Sprint(latencyMS))
			c.Status(resp.StatusCode)
			n, writeErr := copySubsiteResponse(c, bytes.NewReader(payload), resp.Header.Get("Content-Type"))
			if n == 0 {
				n = int64(len(payload))
			}
			return subsiteProxyResult{StatusCode: resp.StatusCode, LatencyMS: latencyMS, ResponseBytes: n, ClientError: true, SelectionFailureCode: code}, writeErr
		}
		return subsiteProxyResult{StatusCode: resp.StatusCode, LatencyMS: latencyMS, ResponseBytes: int64(len(payload))}, &subsiteHTTPStatusError{
			statusCode: resp.StatusCode,
			body:       strings.TrimSpace(string(payload)),
		}
	}
	copyResponseHeaders(c.Writer.Header(), resp.Header)
	c.Header(subsiteForwardTargetHeader, subsite.SubsiteID)
	c.Header(subsiteForwardReasonHeader, reason)
	if reason == "affinity" || reason == "memory_affinity" || reason == "db_affinity" {
		c.Header(subsiteForwardAffinityHeader, "hit")
	} else {
		c.Header(subsiteForwardAffinityHeader, "miss")
	}
	c.Header("X-Sub2API-Forward-Latency-MS", fmt.Sprint(latencyMS))
	c.Status(resp.StatusCode)
	n, err := copySubsiteResponse(c, resp.Body, resp.Header.Get("Content-Type"))
	return subsiteProxyResult{StatusCode: resp.StatusCode, LatencyMS: latencyMS, ResponseBytes: n}, err
}

func (f *SubsiteForwarder) forwardContext(c *gin.Context, body []byte) *subsiteForwardContext {
	apiKeyID := int64(0)
	userID := int64(0)
	groupID := int64(0)
	platform := platformForForwardPath(c.Request.URL.Path)
	if apiKey, ok := middleware.GetAPIKeyFromContext(c); ok && apiKey != nil {
		apiKeyID = apiKey.ID
		userID = apiKey.UserID
		if apiKey.GroupID != nil {
			groupID = *apiKey.GroupID
		}
		if apiKey.Group != nil && strings.TrimSpace(apiKey.Group.Platform) != "" {
			platform = strings.TrimSpace(apiKey.Group.Platform)
		}
	}
	model := extractForwardModel(body)
	session := extractForwardSession(body, c.Request.Header)
	affinityType := "session"
	if session == "" {
		affinityType = "api_key"
		session = stableModelAffinityScope(c.Request.Method, c.Request.URL.Path, model)
	}
	return &subsiteForwardContext{
		Key:      fmt.Sprintf("%s:api:%d:user:%d:group:%d:platform:%s:model:%s:session:%s", affinityType, apiKeyID, userID, groupID, platform, model, session),
		Type:     affinityType,
		Platform: platform,
		Model:    model,
		Session:  session,
		APIKeyID: apiKeyID,
		UserID:   userID,
		GroupID:  groupID,
	}
}

func (f *SubsiteForwarder) storeAffinity(ctx context.Context, forwardCtx *subsiteForwardContext, subsite *service.Subsite, reason, lastError string) error {
	if f == nil || f.subsiteService == nil || subsite == nil || forwardCtx.Key == "" {
		return nil
	}
	f.affinity.Set(forwardCtx.Key, subsite.SubsiteID)
	_, err := f.subsiteService.UpsertForwardAffinity(ctx, service.UpsertSubsiteForwardAffinityInput{
		Key:        forwardCtx.Key,
		Type:       forwardCtx.Type,
		SubsiteID:  subsite.SubsiteID,
		LeaseID:    forwardCtx.LeaseID,
		AccountID:  forwardCtx.AccountID,
		APIKeyID:   forwardCtx.APIKeyID,
		UserID:     forwardCtx.UserID,
		GroupID:    forwardCtx.GroupID,
		Model:      forwardCtx.Model,
		SessionID:  forwardCtx.Session,
		Source:     "auto",
		LastReason: reason,
		LastError:  lastError,
		ExpiresAt:  time.Now().Add(subsiteForwardAffinityTTL),
	})
	return err
}

func (f *SubsiteForwarder) recordForwardEvent(c *gin.Context, forwardCtx *subsiteForwardContext, subsite *service.Subsite, fallbackFrom, reason, outcome string, statusCode int, latencyMS int64, requestBytes int, responseBytes int64, eventErr error) error {
	if f == nil || f.subsiteService == nil || c == nil {
		return nil
	}
	if forwardCtx == nil {
		forwardCtx = &subsiteForwardContext{}
	}
	subsiteID := ""
	if subsite != nil {
		subsiteID = subsite.SubsiteID
	}
	errText := ""
	if eventErr != nil {
		errText = eventErr.Error()
	}
	metadata := map[string]any{
		"query":              c.Request.URL.RawQuery,
		"forward_trace":      requestIDFromContextOrHeader(c),
		"subsite_request_id": forwardCtx.SubsiteRequestID,
		"client_request":     c.GetHeader("X-Client-Request-ID"),
	}
	if forwardCtx.Diagnostics != nil {
		metadata["route"] = forwardCtx.Diagnostics
	}
	return f.subsiteService.RecordForwardEvent(c.Request.Context(), &service.SubsiteForwardEvent{
		RequestID:          requestIDFromContextOrHeader(c),
		AffinityKey:        forwardCtx.Key,
		SubsiteID:          subsiteID,
		AttemptedSubsiteID: subsiteID,
		FallbackFrom:       fallbackFrom,
		LeaseID:            forwardCtx.LeaseID,
		AccountID:          forwardCtx.AccountID,
		APIKeyID:           forwardCtx.APIKeyID,
		UserID:             forwardCtx.UserID,
		GroupID:            forwardCtx.GroupID,
		Model:              forwardCtx.Model,
		SessionID:          forwardCtx.Session,
		Method:             c.Request.Method,
		Path:               c.Request.URL.Path,
		StatusCode:         statusCode,
		LatencyMS:          latencyMS,
		RequestBytes:       int64(requestBytes),
		ResponseBytes:      responseBytes,
		Reason:             reason,
		Outcome:            outcome,
		Error:              errText,
		Metadata:           metadata,
	})
}

func requestIDFromContextOrHeader(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if c.Request != nil {
		if requestID, ok := c.Request.Context().Value(ctxkey.RequestID).(string); ok && strings.TrimSpace(requestID) != "" {
			return strings.TrimSpace(requestID)
		}
	}
	for _, key := range []string{"X-Request-ID", "X-Request-Id"} {
		if value := strings.TrimSpace(c.GetHeader(key)); value != "" {
			return value
		}
	}
	return ""
}

func isForwardableSubsite(subsite service.Subsite, now time.Time) bool {
	if subsite.Status != service.SubsiteStatusActive {
		return false
	}
	if strings.TrimSpace(subsite.PublicURL) == "" {
		return false
	}
	if subsite.LastHeartbeatAt == nil {
		return false
	}
	return now.Sub(*subsite.LastHeartbeatAt) <= subsiteForwardHealthyWindow
}

func subsiteSupportsForwardPlatform(subsite service.Subsite, platform string) bool {
	platform = strings.ToLower(strings.TrimSpace(platform))
	if platform == "" || len(subsite.Capabilities) == 0 {
		return true
	}
	for _, capability := range subsite.Capabilities {
		value := strings.ToLower(strings.TrimSpace(capability))
		switch value {
		case platform, "all", "gateway", "relay", "subsite_relay":
			return true
		case "openai", "anthropic", "gemini", "antigravity":
			if value == platform {
				return true
			}
		}
	}
	return false
}

func isSubsiteAtRelayCapacity(subsite service.Subsite, load service.SubsiteForwardSiteStat) bool {
	if subsite.MaxConcurrency > 0 && load.ActiveRequests >= subsite.MaxConcurrency {
		return true
	}
	if subsite.MaxQPS > 0 && load.QPS >= float64(subsite.MaxQPS) {
		return true
	}
	return false
}

func activeCircuitMaps(breakers []service.SubsiteCircuitBreaker) (map[string]struct{}, map[int64]struct{}, map[string]struct{}) {
	blockedSubsites := make(map[string]struct{})
	blockedAccounts := make(map[int64]struct{})
	blockedLeases := make(map[string]struct{})
	for _, breaker := range breakers {
		switch breaker.Scope {
		case "subsite":
			targetID := strings.TrimSpace(breaker.SubsiteID)
			if targetID == "" {
				targetID = strings.TrimSpace(breaker.TargetID)
			}
			if targetID != "" {
				blockedSubsites[targetID] = struct{}{}
			}
		case "account":
			if breaker.AccountID > 0 {
				blockedAccounts[breaker.AccountID] = struct{}{}
				continue
			}
			if id := parseForwardAccountID(breaker.TargetID); id > 0 {
				blockedAccounts[id] = struct{}{}
			}
		case "lease":
			targetID := strings.TrimSpace(breaker.LeaseID)
			if targetID == "" {
				targetID = strings.TrimSpace(breaker.TargetID)
			}
			if targetID != "" {
				blockedLeases[targetID] = struct{}{}
			}
		}
	}
	return blockedSubsites, blockedAccounts, blockedLeases
}

func sortedStringSet(values map[string]struct{}) []string {
	if len(values) == 0 {
		return []string{}
	}
	items := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	return sortedStrings(items)
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	return items
}

func sortedInt64Set(values map[int64]struct{}) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	items := make([]int64, 0, len(values))
	for value := range values {
		if value > 0 {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

func forwardOutcomeForStatus(statusCode int, fallbackFrom string) string {
	if shouldFailoverSubsiteStatus(statusCode) {
		return "failed"
	}
	if statusCode >= http.StatusBadRequest {
		return "client_error"
	}
	if fallbackFrom != "" {
		return "fallback"
	}
	return "success"
}

func forwardOutcomeForProxyResult(result subsiteProxyResult, fallbackFrom string) string {
	if result.ClientError {
		return "client_error"
	}
	return forwardOutcomeForStatus(result.StatusCode, fallbackFrom)
}

func shouldFailoverSubsiteStatus(statusCode int) bool {
	return statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden ||
		statusCode == http.StatusTooManyRequests ||
		statusCode >= http.StatusInternalServerError
}

func classifySubsiteAuthorizationSelectionFailure(statusCode int, body []byte) (string, bool) {
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return "", false
	}
	text := string(body)
	if !strings.Contains(text, "SUBSITE_AUTHORIZE_FAILED") {
		return "", false
	}
	for _, code := range []string{
		"SUBSITE_NO_ACCOUNT_LEASE",
		"SUBSITE_LEASE_CAPACITY_EXCEEDED",
		"ACCOUNT_LEASE_NOT_FOUND",
		"ACCOUNT_LEASE_CONFLICT",
	} {
		if strings.Contains(text, code) {
			return code, true
		}
	}
	for _, code := range []string{
		"SUBSITE_GROUP_REQUIRED",
		"SUBSITE_MODEL_MISMATCH",
		"QUOTA_RESERVATION_INSUFFICIENT_FUNDS",
		"QUOTA_RESERVATION_COST_REQUIRED",
		"SUBSCRIPTION_LIMIT",
	} {
		if strings.Contains(text, code) {
			return code, false
		}
	}
	return "SUBSITE_AUTHORIZE_FAILED", false
}

func platformForForwardPath(path string) string {
	switch {
	case isForwardOpenAIModelsEndpoint(path):
		return service.PlatformOpenAI
	case strings.HasPrefix(path, "/v1beta/"):
		return service.PlatformGemini
	case strings.Contains(path, "/chat/completions"), strings.Contains(path, "/responses"), strings.Contains(path, "/images/"):
		return service.PlatformOpenAI
	case strings.Contains(path, "/backend-api/codex/"):
		return service.PlatformOpenAI
	default:
		return service.PlatformAnthropic
	}
}

func isForwardOpenAIModelsEndpoint(path string) bool {
	normalized := strings.TrimRight(strings.TrimSpace(path), "/")
	return normalized == "/v1/models" || normalized == "/models"
}

func readForwardBody(c *gin.Context) ([]byte, error) {
	if c.Request.Body == nil {
		return nil, nil
	}
	reader := http.MaxBytesReader(c.Writer, c.Request.Body, subsiteForwardMaxBodyBytes)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func buildSubsiteForwardURL(base string, original *url.URL) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(base), "/"))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid subsite public url: %s", base)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + original.EscapedPath()
	parsed.RawQuery = original.RawQuery
	return parsed.String(), nil
}

func copyForwardHeaders(dst, src http.Header) {
	for key, values := range src {
		if shouldSkipForwardHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if shouldSkipResponseHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copySubsiteResponse(c *gin.Context, reader io.Reader, contentType string) (int64, error) {
	if isForwardStreamingContent(contentType) {
		header := c.Writer.Header()
		if strings.TrimSpace(contentType) != "" {
			header.Set("Content-Type", contentType)
		}
		header.Set("Cache-Control", "no-cache")
		header.Set("X-Accel-Buffering", "no")
		header.Del("Content-Length")
		c.Writer.WriteHeaderNow()
		c.Writer.Flush()
		return copyWithFlush(c.Writer, reader)
	}
	return io.Copy(c.Writer, reader)
}

func copyWithFlush(writer gin.ResponseWriter, reader io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			m, writeErr := writer.Write(buf[:n])
			written += int64(m)
			if writeErr != nil {
				return written, writeErr
			}
			if m != n {
				return written, io.ErrShortWrite
			}
			writer.Flush()
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			return written, readErr
		}
	}
}

func isForwardStreamingContent(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}

func shouldSkipForwardHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "host", "content-length",
		"x-sub2api-preferred-account-id", "x-sub2api-preferred-lease-id", "x-sub2api-preferred-strict", "x-sub2api-route-signature", "x-sub2api-route-timestamp", "x-sub2api-route-nonce":
		return true
	default:
		return false
	}
}

func shouldSkipResponseHeader(key string) bool {
	switch strings.ToLower(key) {
	case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "content-length",
		"x-sub2api-account-id", "x-sub2api-lease-id", "x-sub2api-request-id":
		return true
	default:
		return false
	}
}

func parseForwardAccountID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var id int64
	if _, err := fmt.Sscan(value, &id); err != nil || id <= 0 {
		return 0
	}
	return id
}

func forwardedProto(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); value != "" {
		return value
	}
	if c.Request.TLS != nil {
		return "https"
	}
	return "http"
}

func extractForwardModel(body []byte) string {
	var payload map[string]any
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil {
		return ""
	}
	if model, ok := payload["model"].(string); ok {
		return strings.TrimSpace(model)
	}
	return ""
}

func extractForwardSession(body []byte, header http.Header) string {
	keys := []string{
		"previous_response_id",
		"conversation_id",
		"session_id",
		"thread_id",
		"response_id",
		"parent_response_id",
		"metadata.conversation_id",
		"metadata.session_id",
		"metadata.thread_id",
	}
	var payload map[string]any
	if len(body) > 0 && json.Unmarshal(body, &payload) == nil {
		for _, key := range keys {
			if value := stringFromNestedMap(payload, strings.Split(key, ".")); value != "" {
				return key + ":" + value
			}
		}
	}
	for _, key := range []string{
		"session_id",
		"conversation_id",
		"thread_id",
		"OpenAI-Session-ID",
		"OpenAI-Conversation-ID",
		"x-session-id",
		"x-conversation-id",
		"x-thread-id",
	} {
		if value := strings.TrimSpace(header.Get(key)); value != "" {
			return key + ":" + value
		}
	}
	return ""
}

func stringFromNestedMap(root map[string]any, path []string) string {
	if len(path) == 0 {
		return ""
	}
	var current any = root
	for _, part := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = obj[part]
	}
	value, ok := current.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func requestShapeHash(method, path, model string, apiKeyID, userID, groupID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d|%d|%d", method, path, model, apiKeyID, userID, groupID)))
	return hex.EncodeToString(sum[:12])
}

func stableModelAffinityScope(method, path, model string) string {
	normalizedPath := strings.TrimRight(strings.TrimSpace(path), "/")
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	if model == "" {
		model = "models"
	}
	return fmt.Sprintf("%s:%s:%s", strings.ToUpper(strings.TrimSpace(method)), normalizedPath, model)
}

func writeSubsiteForwardError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

type subsiteAffinityStore struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]subsiteAffinityEntry
}

type subsiteAffinityEntry struct {
	subsiteID string
	expiresAt time.Time
}

func newSubsiteAffinityStore(ttl time.Duration) *subsiteAffinityStore {
	return &subsiteAffinityStore{ttl: ttl, entries: make(map[string]subsiteAffinityEntry)}
}

func (s *subsiteAffinityStore) Get(key string) (string, bool) {
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			s.mu.Lock()
			delete(s.entries, key)
			s.mu.Unlock()
		}
		return "", false
	}
	return entry.subsiteID, true
}

func (s *subsiteAffinityStore) Set(key, subsiteID string) {
	if key == "" || subsiteID == "" {
		return
	}
	s.mu.Lock()
	s.entries[key] = subsiteAffinityEntry{subsiteID: subsiteID, expiresAt: time.Now().Add(s.ttl)}
	s.mu.Unlock()
}

type subsiteRouteSnapshotStore struct {
	mu       sync.RWMutex
	ttl      time.Duration
	staleTTL time.Duration
	snapshot *subsiteRouteSnapshot
	expires  time.Time
	staleAt  time.Time
}

type subsiteRouteSnapshot struct {
	items         []service.Subsite
	breakers      []service.SubsiteCircuitBreaker
	loadBySubsite map[string]service.SubsiteForwardSiteStat
	loadedAt      time.Time
}

func newSubsiteRouteSnapshotStore(ttl, staleTTL time.Duration) *subsiteRouteSnapshotStore {
	return &subsiteRouteSnapshotStore{ttl: ttl, staleTTL: staleTTL}
}

func (s *subsiteRouteSnapshotStore) Get(ctx context.Context, load func(context.Context) (*subsiteRouteSnapshot, error)) (*subsiteRouteSnapshot, error) {
	if s == nil || load == nil {
		return nil, errors.New("subsite route snapshot store is not initialized")
	}
	now := time.Now()
	s.mu.RLock()
	if s.snapshot != nil && now.Before(s.expires) {
		snapshot := s.snapshot
		s.mu.RUnlock()
		return snapshot, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	now = time.Now()
	if s.snapshot != nil && now.Before(s.expires) {
		return s.snapshot, nil
	}
	snapshot, err := load(ctx)
	if err != nil {
		if s.snapshot != nil && now.Before(s.staleAt) {
			return s.snapshot, nil
		}
		return nil, err
	}
	if snapshot == nil {
		return nil, errSubsiteForwardNoCandidate
	}
	if snapshot.loadBySubsite == nil {
		snapshot.loadBySubsite = make(map[string]service.SubsiteForwardSiteStat)
	}
	if snapshot.loadedAt.IsZero() {
		snapshot.loadedAt = now
	}
	s.snapshot = snapshot
	s.expires = snapshot.loadedAt.Add(s.ttl)
	s.staleAt = snapshot.loadedAt.Add(s.ttl + s.staleTTL)
	return snapshot, nil
}

type subsiteAccountRouteStore struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]subsiteAccountRouteEntry
}

type subsiteAccountRouteEntry struct {
	value     *subsiteAccountRouteEligibility
	expiresAt time.Time
}

func newSubsiteAccountRouteStore(ttl time.Duration) *subsiteAccountRouteStore {
	return &subsiteAccountRouteStore{ttl: ttl, entries: make(map[string]subsiteAccountRouteEntry)}
}

func (s *subsiteAccountRouteStore) Get(ctx context.Context, key string, load func(context.Context) (*subsiteAccountRouteEligibility, error)) (*subsiteAccountRouteEligibility, error) {
	if s == nil || strings.TrimSpace(key) == "" || load == nil {
		return nil, nil
	}
	now := time.Now()
	s.mu.RLock()
	entry, ok := s.entries[key]
	s.mu.RUnlock()
	if ok && entry.value != nil && now.Before(entry.expiresAt) {
		return entry.value, nil
	}

	value, err := load(ctx)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	if value.LoadedAt.IsZero() {
		value.LoadedAt = now
	}
	s.mu.Lock()
	s.entries[key] = subsiteAccountRouteEntry{value: value, expiresAt: now.Add(s.ttl)}
	s.mu.Unlock()
	return value, nil
}

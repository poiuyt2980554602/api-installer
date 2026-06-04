package admin

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type SubsiteHandler struct {
	subsiteService *service.SubsiteService
	leaseService   *service.AccountLeaseService
	settingService *service.SettingService
}

func NewSubsiteHandler(subsiteService *service.SubsiteService, leaseService *service.AccountLeaseService, settingService *service.SettingService) *SubsiteHandler {
	return &SubsiteHandler{subsiteService: subsiteService, leaseService: leaseService, settingService: settingService}
}

func (h *SubsiteHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.subsiteService.List(c.Request.Context(), pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}, service.ListSubsitesFilter{
		Status: strings.TrimSpace(c.Query("status")),
		Search: strings.TrimSpace(c.Query("search")),
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.PaginatedWithResult(c, items, &response.PaginationResult{
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
		Pages:    result.Pages,
	})
}

func (h *SubsiteHandler) ForwardStats(c *gin.Context) {
	stats, err := h.subsiteService.ForwardStats(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	stats.Mode = h.forwardMode(c.Request.Context())
	response.Success(c, stats)
}

func (h *SubsiteHandler) AutoDistributeRelayAccounts(c *gin.Context) {
	if h == nil || h.leaseService == nil {
		response.BadRequest(c, "lease service is not initialized")
		return
	}
	result, err := h.leaseService.AutoDistributeRelayAccounts(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *SubsiteHandler) forwardMode(ctx context.Context) string {
	fallback := os.Getenv("SUBSITE_FORWARD_MODE")
	if h == nil || h.settingService == nil {
		return service.NormalizeSubsiteForwardMode(fallback)
	}
	return h.settingService.GetSubsiteForwardMode(ctx, fallback)
}

func (h *SubsiteHandler) UpdateForwardMode(c *gin.Context) {
	if h == nil || h.settingService == nil {
		response.BadRequest(c, "setting service is not initialized")
		return
	}
	var input struct {
		Mode string `json:"mode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	mode, err := h.settingService.SetSubsiteForwardMode(c.Request.Context(), input.Mode)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"mode": mode})
}

func (h *SubsiteHandler) ListForwardAffinities(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	var locked *bool
	if value := strings.TrimSpace(c.Query("locked")); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			response.BadRequest(c, "locked must be true or false")
			return
		}
		locked = &parsed
	}
	apiKeyID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("api_key_id")), 10, 64)
	accountID, _ := strconv.ParseInt(strings.TrimSpace(c.Query("account_id")), 10, 64)
	items, result, err := h.subsiteService.ListForwardAffinities(c.Request.Context(), pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}, service.ListSubsiteForwardAffinitiesFilter{
		SubsiteID: strings.TrimSpace(c.Query("subsite_id")),
		APIKeyID:  apiKeyID,
		AccountID: accountID,
		Search:    strings.TrimSpace(c.Query("search")),
		Locked:    locked,
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.PaginatedWithResult(c, items, &response.PaginationResult{
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
		Pages:    result.Pages,
	})
}

func (h *SubsiteHandler) UpsertForwardAffinity(c *gin.Context) {
	var input struct {
		Key        string `json:"affinity_key" binding:"required"`
		Type       string `json:"affinity_type"`
		SubsiteID  string `json:"subsite_id" binding:"required"`
		LeaseID    string `json:"lease_id"`
		AccountID  int64  `json:"account_id"`
		APIKeyID   int64  `json:"api_key_id"`
		UserID     int64  `json:"user_id"`
		GroupID    int64  `json:"group_id"`
		Model      string `json:"model"`
		SessionID  string `json:"session_id"`
		TTLSeconds int    `json:"ttl_seconds"`
		Locked     bool   `json:"locked"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ttl := input.TTLSeconds
	if ttl <= 0 {
		ttl = 7 * 24 * 3600
	}
	affinity, err := h.subsiteService.UpsertForwardAffinity(c.Request.Context(), service.UpsertSubsiteForwardAffinityInput{
		Key:        input.Key,
		Type:       input.Type,
		SubsiteID:  input.SubsiteID,
		LeaseID:    input.LeaseID,
		AccountID:  input.AccountID,
		APIKeyID:   input.APIKeyID,
		UserID:     input.UserID,
		GroupID:    input.GroupID,
		Model:      input.Model,
		SessionID:  input.SessionID,
		Source:     "manual",
		Locked:     input.Locked,
		LastReason: "manual",
		ExpiresAt:  time.Now().Add(time.Duration(ttl) * time.Second),
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, affinity)
}

func (h *SubsiteHandler) DeleteForwardAffinity(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("affinity_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid affinity id")
		return
	}
	if response.ErrorFrom(c, h.subsiteService.DeleteForwardAffinity(c.Request.Context(), id)) {
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *SubsiteHandler) ListForwardEvents(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	items, result, err := h.subsiteService.ListForwardEvents(c.Request.Context(), pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	}, service.ListSubsiteForwardEventsFilter{
		SubsiteID: strings.TrimSpace(c.Query("subsite_id")),
		Outcome:   strings.TrimSpace(c.Query("outcome")),
		Search:    strings.TrimSpace(c.Query("search")),
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.PaginatedWithResult(c, items, &response.PaginationResult{
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
		Pages:    result.Pages,
	})
}

func (h *SubsiteHandler) Create(c *gin.Context) {
	var input service.CreateSubsiteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.subsiteService.Create(c.Request.Context(), input)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Created(c, result)
}

func (h *SubsiteHandler) Update(c *gin.Context) {
	var input service.UpdateSubsiteInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	result, err := h.subsiteService.Update(c.Request.Context(), c.Param("id"), input)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *SubsiteHandler) Activate(c *gin.Context) {
	if response.ErrorFrom(c, h.subsiteService.Activate(c.Request.Context(), c.Param("id"))) {
		return
	}
	response.Success(c, gin.H{"status": service.SubsiteStatusActive})
}

func (h *SubsiteHandler) Pause(c *gin.Context) {
	if response.ErrorFrom(c, h.subsiteService.Pause(c.Request.Context(), c.Param("id"))) {
		return
	}
	response.Success(c, gin.H{"status": service.SubsiteStatusMaintenance})
}

func (h *SubsiteHandler) Resume(c *gin.Context) {
	if response.ErrorFrom(c, h.subsiteService.Resume(c.Request.Context(), c.Param("id"))) {
		return
	}
	response.Success(c, gin.H{"status": service.SubsiteStatusActive})
}

func (h *SubsiteHandler) ResetSecret(c *gin.Context) {
	result, err := h.subsiteService.ResetSecret(c.Request.Context(), c.Param("id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *SubsiteHandler) ListLeases(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	leases, result, err := h.leaseService.ListBySubsitePaginated(c.Request.Context(), c.Param("id"), pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.PaginatedWithResult(c, leases, &response.PaginationResult{
		Total:    result.Total,
		Page:     result.Page,
		PageSize: result.PageSize,
		Pages:    result.Pages,
	})
}

func (h *SubsiteHandler) ListLeaseActiveAccountIDs(c *gin.Context) {
	accountIDs, err := h.leaseService.ListActiveAccountIDsBySubsite(c.Request.Context(), c.Param("id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"account_ids": accountIDs})
}

func (h *SubsiteHandler) CreateLease(c *gin.Context) {
	var input struct {
		GroupID        int64      `json:"group_id" binding:"required"`
		AccountID      int64      `json:"account_id" binding:"required"`
		MaxConcurrency int        `json:"max_concurrency"`
		MaxRequests    int        `json:"max_requests"`
		MaxTokens      int64      `json:"max_tokens"`
		ExpiresAt      *time.Time `json:"expires_at"`
		TTLSeconds     int        `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	expiresAt := input.ExpiresAt
	if expiresAt == nil && input.TTLSeconds > 0 {
		t := time.Now().Add(time.Duration(input.TTLSeconds) * time.Second)
		expiresAt = &t
	}
	lease, err := h.leaseService.Create(c.Request.Context(), service.CreateAccountLeaseInput{
		SubsiteID:      c.Param("id"),
		GroupID:        input.GroupID,
		AccountID:      input.AccountID,
		MaxConcurrency: input.MaxConcurrency,
		MaxRequests:    input.MaxRequests,
		MaxTokens:      input.MaxTokens,
		ExpiresAt:      expiresAt,
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Created(c, lease)
}

func (h *SubsiteHandler) DrainLease(c *gin.Context) {
	lease, err := h.leaseService.DrainForSubsite(c.Request.Context(), c.Param("id"), c.Param("lease_id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, lease)
}

func (h *SubsiteHandler) ReleaseLease(c *gin.Context) {
	lease, err := h.leaseService.ReleaseForSubsite(c.Request.Context(), c.Param("id"), c.Param("lease_id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, lease)
}

func (h *SubsiteHandler) RenewLease(c *gin.Context) {
	var input struct {
		ExpiresAt  *time.Time `json:"expires_at"`
		TTLSeconds int        `json:"ttl_seconds"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var expiresAt time.Time
	if input.ExpiresAt != nil {
		expiresAt = *input.ExpiresAt
	} else if input.TTLSeconds > 0 {
		expiresAt = time.Now().Add(time.Duration(input.TTLSeconds) * time.Second)
	} else {
		response.BadRequest(c, "expires_at or ttl_seconds is required")
		return
	}
	lease, err := h.leaseService.Renew(c.Request.Context(), service.RenewAccountLeaseInput{
		SubsiteID: c.Param("id"),
		LeaseID:   c.Param("lease_id"),
		ExpiresAt: expiresAt,
	})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, lease)
}

func (h *SubsiteHandler) UpdateLease(c *gin.Context) {
	var input struct {
		MaxConcurrency int   `json:"max_concurrency"`
		MaxRequests    int   `json:"max_requests"`
		MaxTokens      int64 `json:"max_tokens"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	lease, err := h.leaseService.UpdateLimitsForSubsite(
		c.Request.Context(),
		c.Param("id"),
		c.Param("lease_id"),
		input.MaxConcurrency,
		input.MaxRequests,
		input.MaxTokens,
	)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, lease)
}

func (h *SubsiteHandler) DeleteLease(c *gin.Context) {
	lease, err := h.leaseService.DeleteForSubsite(c.Request.Context(), c.Param("id"), c.Param("lease_id"))
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, lease)
}

package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProxyAffinityHandler struct {
	proxyAffinityService *service.ProxyAffinityService
}

func NewProxyAffinityHandler(proxyAffinityService *service.ProxyAffinityService) *ProxyAffinityHandler {
	return &ProxyAffinityHandler{proxyAffinityService: proxyAffinityService}
}

func (h *ProxyAffinityHandler) GetSettings(c *gin.Context) {
	settings, err := h.proxyAffinityService.GetSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *ProxyAffinityHandler) UpdateSettings(c *gin.Context) {
	var req service.ProxyAffinitySettings
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	settings, err := h.proxyAffinityService.UpdateSettings(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

func (h *ProxyAffinityHandler) GetOverview(c *gin.Context) {
	overview, err := h.proxyAffinityService.GetOverview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *ProxyAffinityHandler) Assign(c *gin.Context) {
	var req service.ProxyAffinityAssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if raw := strings.TrimSpace(c.Query("dry_run")); raw != "" {
		if dryRun, err := strconv.ParseBool(raw); err == nil {
			req.DryRun = dryRun
		}
	}
	result, err := h.proxyAffinityService.AssignUnassigned(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ProxyAffinityHandler) Prebind(c *gin.Context) {
	var req service.ProxyAffinityPrebindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if raw := strings.TrimSpace(c.Query("dry_run")); raw != "" {
		if dryRun, err := strconv.ParseBool(raw); err == nil {
			req.DryRun = dryRun
		}
	}
	result, err := h.proxyAffinityService.PrebindPendingAccounts(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ProxyAffinityHandler) BindAccount(c *gin.Context) {
	var req service.ProxyAffinityBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if raw := strings.TrimSpace(c.Query("dry_run")); raw != "" {
		if dryRun, err := strconv.ParseBool(raw); err == nil {
			req.DryRun = dryRun
		}
	}
	result, err := h.proxyAffinityService.BindAccount(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ProxyAffinityHandler) ReleaseAccount(c *gin.Context) {
	var req service.ProxyAffinityReleaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if raw := strings.TrimSpace(c.Query("dry_run")); raw != "" {
		if dryRun, err := strconv.ParseBool(raw); err == nil {
			req.DryRun = dryRun
		}
	}
	result, err := h.proxyAffinityService.ReleaseAccount(c.Request.Context(), req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

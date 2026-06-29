package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/amll-dev/amll-hub/backend/internal/middleware"
	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/amll-dev/amll-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// SyncHandler 同步相关 handler
type SyncHandler struct {
	svc *service.SyncService
}

func NewSyncHandler(svc *service.SyncService) *SyncHandler {
	return &SyncHandler{svc: svc}
}

// Trigger POST /api/v1/sync
// 触发同步任务
func (h *SyncHandler) Trigger(c *gin.Context) {
	// triggered_by 默认 api，可由 header X-Triggered-By 覆盖（github_action / cron）
	triggeredBy := c.GetHeader("X-Triggered-By")
	if triggeredBy == "" {
		triggeredBy = "api"
	}
	triggeredBy = strings.ToLower(triggeredBy)
	switch triggeredBy {
	case "api", "cron", "github_action":
	default:
		triggeredBy = "api"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultTimeout)
	defer cancel()

	result, err := h.svc.TriggerSync(ctx, triggeredBy)
	if err != nil {
		pkg.Fail(c, http.StatusBadGateway, 502, "触发同步失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":                 200,
		"status":               result.Status,
		"message":              result.Message,
		"requestId":            result.RequestID,
		"previousCommit":       result.PreviousCommit,
		"targetCommit":         result.TargetCommit,
		"startedAt":            result.StartedAt,
		"queuePosition":        result.QueuePosition,
		"currentSyncRequestId": result.CurrentSyncReqID,
		"currentSyncStartedAt": result.CurrentSyncStart,
		"lastSyncedCommit":     result.LastSyncedCommit,
		"lastSyncedAt":         result.LastSyncedAt,
	})
}

// Status GET /api/v1/sync/status
func (h *SyncHandler) Status(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultTimeout)
	defer cancel()

	result, err := h.svc.GetStatus(ctx)
	if err != nil {
		pkg.InternalError(c, "查询同步状态失败")
		return
	}

	resp := gin.H{
		"code":    200,
		"syncing": result.Syncing,
	}
	if result.Syncing {
		resp["startedAt"] = result.StartedAt
		if result.Progress != nil {
			resp["progress"] = result.Progress
		}
	} else {
		resp["lastSyncedAt"] = result.LastSyncedAt
		resp["lastSyncedCommit"] = result.LastSyncedCommit
	}
	c.JSON(http.StatusOK, resp)
}

// _ 防止 unused
var _ = middleware.GetRequestID

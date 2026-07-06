package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/amll-dev/amll-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// OnlineSearchHandler 在线搜索 handler
type OnlineSearchHandler struct {
	svc *service.OnlineSearchService
}

func NewOnlineSearchHandler(svc *service.OnlineSearchService) *OnlineSearchHandler {
	return &OnlineSearchHandler{svc: svc}
}

// Search GET /api/v1/online-search?q=&platform=&limit=
func (h *OnlineSearchHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	platform := strings.TrimSpace(c.Query("platform"))
	limit := pkg.Clamp(pkg.ParseInt(c.Query("limit"), 5), 1, 20)

	if q == "" {
		pkg.BadRequest(c, "q 参数必填")
		return
	}

	switch platform {
	case "ncm", "qq", "kugou":
	default:
		pkg.BadRequest(c, "platform 参数非法（可选: ncm, qq, kugou）")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	result, err := h.svc.Search(ctx, q, platform, limit)
	if err != nil {
		pkg.Fail(c, http.StatusBadGateway, 502, "在线搜索失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": result,
	})
}

// GetSong GET /api/v1/online-song?platform=&songId=
func (h *OnlineSearchHandler) GetSong(c *gin.Context) {
	platform := strings.TrimSpace(c.Query("platform"))
	songID := strings.TrimSpace(c.Query("songId"))

	switch platform {
	case "ncm", "qq", "kugou":
	default:
		pkg.BadRequest(c, "platform 参数非法（可选: ncm, qq, kugou）")
		return
	}

	if songID == "" {
		pkg.BadRequest(c, "songId 参数必填")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	result, err := h.svc.GetSong(ctx, platform, songID)
	if err != nil {
		pkg.Fail(c, http.StatusBadGateway, 502, "获取歌曲详情失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": result,
	})
}

// GetLyric GET /api/v1/online-lyric?platform=&songId=
func (h *OnlineSearchHandler) GetLyric(c *gin.Context) {
	platform := strings.TrimSpace(c.Query("platform"))
	songID := strings.TrimSpace(c.Query("songId"))

	switch platform {
	case "ncm", "qq", "kugou":
	default:
		pkg.BadRequest(c, "platform 参数非法（可选: ncm, qq, kugou）")
		return
	}

	if songID == "" {
		pkg.BadRequest(c, "songId 参数必填")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	result, err := h.svc.GetLyric(ctx, platform, songID)
	if err != nil {
		pkg.Fail(c, http.StatusBadGateway, 502, "获取歌词失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": result,
	})
}

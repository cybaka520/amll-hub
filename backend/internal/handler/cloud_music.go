package handler

import (
	"context"
	"strings"

	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/amll-dev/amll-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
	logrus "github.com/sirupsen/logrus"
)

// CloudMusicHandler 网易云解析 handler
type CloudMusicHandler struct {
	svc *service.CloudMusicService
}

func NewCloudMusicHandler(svc *service.CloudMusicService) *CloudMusicHandler {
	return &CloudMusicHandler{svc: svc}
}

// Search GET /api/v1/ncm/search?q=&limit=
// limit 默认 10，范围 1-100
func (h *CloudMusicHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		pkg.BadRequest(c, "q 参数必填")
		return
	}
	limit := pkg.Clamp(pkg.ParseInt(c.Query("limit"), 10), 1, 100)

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	result, err := h.svc.Search(ctx, q, limit)
	if err != nil {
		logrus.WithError(err).
			WithField("q", q).
			WithField("limit", limit).
			Warn("cloud music search failed")
		pkg.Fail(c, 502, 502, "搜索失败: "+err.Error())
		return
	}
	c.JSON(200, gin.H{"code": 200, "data": result})
}

// ParseMusic GET /api/v1/ncm/parse-music?songId=&level=
// level 默认 exhigh，可选: standard/exhigh/lossless/hires/jyeffect/jymaster/sky/dolby
func (h *CloudMusicHandler) ParseMusic(c *gin.Context) {
	songID := strings.TrimSpace(c.Query("songId"))
	if songID == "" {
		pkg.BadRequest(c, "songId 参数必填")
		return
	}
	level := strings.TrimSpace(c.DefaultQuery("level", "exhigh"))

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	result, err := h.svc.ParseMusic(ctx, songID, level)
	if err != nil {
		logrus.WithError(err).
			WithField("songId", songID).
			WithField("level", level).
			Warn("cloud music parse-music failed")
		pkg.Fail(c, 502, 502, "解析单曲失败: "+err.Error())
		return
	}
	c.JSON(200, gin.H{"code": 200, "data": result})
}

// ParsePlaylist GET /api/v1/ncm/parse-playlist?playlistId=
func (h *CloudMusicHandler) ParsePlaylist(c *gin.Context) {
	playlistID := strings.TrimSpace(c.Query("playlistId"))
	if playlistID == "" {
		pkg.BadRequest(c, "playlistId 参数必填")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	result, err := h.svc.ParsePlaylist(ctx, playlistID)
	if err != nil {
		logrus.WithError(err).
			WithField("playlistId", playlistID).
			Warn("cloud music parse-playlist failed")
		pkg.Fail(c, 502, 502, "解析歌单失败: "+err.Error())
		return
	}
	c.JSON(200, gin.H{"code": 200, "data": result})
}

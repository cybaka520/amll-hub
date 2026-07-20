package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/amll-dev/amll-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
	logrus "github.com/sirupsen/logrus"
)

// LyricsHandler 歌词获取 handler
type LyricsHandler struct {
	svc   *service.LyricsService
	nfSvc *service.NotFoundService
}

func NewLyricsHandler(svc *service.LyricsService, nfSvc *service.NotFoundService) *LyricsHandler {
	return &LyricsHandler{svc: svc, nfSvc: nfSvc}
}

// GetLyrics GET /api/v1/:folder/:filename
// 直接返回 TTML 原始字节流，支持 Range 请求
func (h *LyricsHandler) GetLyrics(c *gin.Context) {
	folder := c.Param("folder")
	filename := c.Param("filename")

	if !pkg.IsValidFolder(folder) || filename == "" {
		c.Status(http.StatusNotFound)
		return
	}

	// 平台 ID 查询时，去掉末尾的 .ttml 后缀
	// 防呆设计兼容 /ncm-lyrics/114514 和 /ncm-lyrics/114514.ttml 两种写法
	if folder != "raw-lyrics" {
		filename = strings.TrimSuffix(filename, ".ttml")
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	// 1. 解析 MinioPath
	resolved, err := h.svc.ResolveLyric(ctx, folder, filename)
	if err != nil {
		if errors.Is(err, service.ErrLyricNotFound) {
			// platform_id 对于非 raw-lyrics 请求，filename 已经被去掉了 .ttml 后缀
			platformID := filename
			if folder == "raw-lyrics" {
				platformID = strings.TrimSuffix(filename, ".ttml")
			}
			c.JSON(http.StatusNotFound, gin.H{
				"error":       "lyric not found",
				"folder":      folder,
				"filename":    filename,
				"platform":    strings.TrimSuffix(folder, "-lyrics"),
				"platform_id": platformID,
			})

			// 异步记录无歌词（仅对平台歌词端点生效，raw-lyrics 不记录）
			if folder != "raw-lyrics" && h.nfSvc != nil {
				platform := service.ParseFolderToPlatform(folder)
				clientIP := GetClientIP(c)
				// 使用独立 context，避免被请求 context 取消
				go func() {
					nfCtx, nfCancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer nfCancel()
					h.nfSvc.HandleNotFoundRequest(nfCtx, platform, filename, clientIP)
				}()
			}
			return
		}
		logrus.WithError(err).Error("resolve lyric failed")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	rangeHeader := c.GetHeader("Range")

	// 2. 设置基础响应头
	c.Header("Content-Type", "application/xml; charset=utf-8")
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	if resolved.ETag != "" {
		c.Header("ETag", resolved.ETag)
	}
	c.Header("Accept-Ranges", "bytes")

	// 3. 流式返回
	status, contentRange, contentLength, err := h.svc.StreamLyric(ctx, resolved.MinioPath, rangeHeader, func(_ int64, reader io.Reader) error {
		_, err := io.Copy(c.Writer, reader)
		return err
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidRange) {
			c.Header("Content-Range", "bytes */"+itoa(resolved.Size))
			c.Status(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		if status == 0 {
			c.Status(http.StatusInternalServerError)
		}
		return
	}

	if contentRange != "" {
		c.Header("Content-Range", contentRange)
	}
	c.Header("Content-Length", itoa(contentLength))
	c.Status(status)

	// 歌词流式返回成功后：异步检查是否在排行榜中，如果在则删除
	if folder != "raw-lyrics" && h.nfSvc != nil {
		platform := service.ParseFolderToPlatform(folder)
		go func() {
			nfCtx, nfCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer nfCancel()
			h.nfSvc.CheckAndDeleteOnLyricResolved(nfCtx, platform, filename)
		}()
	}
}

// itoa int64 -> string，避免引入 strconv 包名的歧义
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

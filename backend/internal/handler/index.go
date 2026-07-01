package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/amll-dev/amll-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
	logrus "github.com/sirupsen/logrus"
)

// IndexHandler 索引文件下载 handler
type IndexHandler struct {
	svc *service.IndexService
}

func NewIndexHandler(svc *service.IndexService) *IndexHandler {
	return &IndexHandler{svc: svc}
}

func (h *IndexHandler) GetIndex(c *gin.Context) {
	filePath := c.Param("path")
	filePath = strings.TrimPrefix(filePath, "/")

	if filePath == "" || !isValidIndexPath(filePath) {
		c.Status(http.StatusNotFound)
		return
	}

	minioKey := "index/" + filePath

	ctx, cancel := context.WithTimeout(c.Request.Context(), longTimeout)
	defer cancel()

	obj, err := h.svc.GetIndexFile(ctx, minioKey)
	if err != nil {
		if errors.Is(err, service.ErrIndexNotFound) {
			c.Status(http.StatusNotFound)
			return
		}
		logrus.WithError(err).Error("get index file failed")
		c.Status(http.StatusInternalServerError)
		return
	}
	defer func() { _ = obj.Close() }()

	c.Header("Content-Type", indexContentType(filePath))
	c.Header("Cache-Control", "public, max-age=300")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, obj); err != nil {
		logrus.WithError(err).Error("stream index file failed")
	}
}

// isValidIndexPath 白名单校验，防止路径遍历
func isValidIndexPath(path string) bool {
	allowed := []string{
		"metadata/raw-lyrics-index.jsonl",
		"raw-lyrics/raw-lyrics.zip",
		"ncm-lyrics/index.jsonl",
		"qq-lyrics/index.jsonl",
		"spotify-lyrics/index.jsonl",
		"am-lyrics/index.jsonl",
	}
	for _, a := range allowed {
		if path == a {
			return true
		}
	}
	return false
}

func indexContentType(path string) string {
	if strings.HasSuffix(path, ".zip") {
		return "application/zip"
	}
	return "application/x-ndjson; charset=utf-8"
}

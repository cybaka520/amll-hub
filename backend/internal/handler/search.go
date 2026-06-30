package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/amll-dev/amll-hub/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// SearchHandler 搜索 handler
type SearchHandler struct {
	svc *service.SearchService
}

func NewSearchHandler(svc *service.SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

// Search GET /api/v1/search?q=&field=&limit=&offset=
func (h *SearchHandler) Search(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	field := strings.TrimSpace(c.DefaultQuery("field", "all"))
	limit := pkg.Clamp(pkg.ParseInt(c.Query("limit"), 20), 1, 100)
	offset := pkg.Clamp(pkg.ParseInt(c.Query("offset"), 0), 0, 100000)

	// 校验 field
	switch field {
	case "all", "song", "artist", "album", "id", "lyric", "author":
	default:
		pkg.BadRequest(c, "field 参数非法")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), defaultTimeout)
	defer cancel()

	result, err := h.svc.Search(ctx, service.SearchRequest{
		Query:  q,
		Field:  field,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		pkg.Fail(c, http.StatusBadGateway, 502, "搜索失败: "+err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": result,
	})
}

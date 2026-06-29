package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/amll-dev/amll-hub/backend/internal/config"
	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/meilisearch/meilisearch-go"
)

// SearchService 搜索服务
type SearchService struct {
	cfg    *config.Config
	client *meilisearch.Client
}

func NewSearchService(cfg *config.Config, client *meilisearch.Client) *SearchService {
	return &SearchService{cfg: cfg, client: client}
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query  string
	Field  string // all, song, artist, album, id, lyric
	Limit  int
	Offset int
}

// SearchHitResult 单条命中
type SearchHitResult struct {
	ID              string            `json:"id"`
	MusicNames      []string          `json:"musicNames"`
	Artists         []string          `json:"artists"`
	Albums          []string          `json:"albums"`
	PlatformIds     map[string]string `json:"platformIds"`
	RawLyricFile    string            `json:"rawLyricFile"`
	WordCount       int               `json:"wordCount"`
	LineCount       int               `json:"lineCount"`
	CommitTimestamp *int64            `json:"commitTimestamp,omitempty"`
}

// SearchResult 搜索结果
type SearchResult struct {
	Hits             []SearchHitResult `json:"hits"`
	TotalHits        int64             `json:"totalHits"`
	Limit            int               `json:"limit"`
	Offset           int               `json:"offset"`
	ProcessingTimeMs int               `json:"processingTimeMs"`
}

// Search 执行搜索
func (s *SearchService) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	if req.Query == "" {
		return &SearchResult{
			Hits:   []SearchHitResult{},
			Limit:  req.Limit,
			Offset: req.Offset,
		}, nil
	}

	index := s.client.Index(s.cfg.MeiliSearch.Index)

	// field=id：使用 filter 精确匹配
	if req.Field == "id" {
		return s.searchByExactID(ctx, index, req)
	}

	// 其它场景使用 attributesToSearchOn 限定搜索字段
	searchable := searchOnFields(req.Field)
	req2 := meilisearch.SearchRequest{
		Query:                req.Query,
		ShowRankingScore:     false,
		AttributesToSearchOn: searchable,
		AttributesToRetrieve: []string{"*"},
		Limit:                int64(req.Limit),
		Offset:               int64(req.Offset),
		// 按提交时间戳降序：最新版本排最前，旧版本也返回但排在后面
		Sort: []string{"commitTimestamp:desc"},
	}

	resp, err := index.Search(req2.Query, &req2)
	if err != nil {
		return nil, fmt.Errorf("meilisearch search: %w", err)
	}

	hits := make([]SearchHitResult, 0, len(resp.Hits))
	for _, raw := range resp.Hits {
		hits = append(hits, convertHit(raw))
	}

	return &SearchResult{
		Hits:             hits,
		TotalHits:        resp.EstimatedTotalHits,
		Limit:            req.Limit,
		Offset:           req.Offset,
		ProcessingTimeMs: int(resp.ProcessingTimeMs),
	}, nil
}

// searchByExactID 使用 filter 精确匹配所有平台 ID 字段
func (s *SearchService) searchByExactID(ctx context.Context, index *meilisearch.Index, req SearchRequest) (*SearchResult, error) {
	escaped := meiliEscape(req.Query)
	filter := fmt.Sprintf(
		`platformIds_ncm = "%s" OR platformIds_qq = "%s" OR platformIds_spotify = "%s" OR platformIds_apple = "%s"`,
		escaped, escaped, escaped, escaped,
	)
	req2 := meilisearch.SearchRequest{
		Query:                "",
		Filter:               filter,
		AttributesToRetrieve: []string{"*"},
		Limit:                int64(req.Limit),
		Offset:               int64(req.Offset),
		// 按提交时间戳降序：同一平台 ID 的多个版本中最新版排最前
		Sort: []string{"commitTimestamp:desc"},
	}

	resp, err := index.Search(req2.Query, &req2)
	if err != nil {
		return nil, fmt.Errorf("meilisearch search by id: %w", err)
	}

	hits := make([]SearchHitResult, 0, len(resp.Hits))
	for _, raw := range resp.Hits {
		hits = append(hits, convertHit(raw))
	}
	return &SearchResult{
		Hits:             hits,
		TotalHits:        resp.EstimatedTotalHits,
		Limit:            req.Limit,
		Offset:           req.Offset,
		ProcessingTimeMs: int(resp.ProcessingTimeMs),
	}, nil
}

// searchOnFields 根据字段返回 attributesToSearchOn 列表
func searchOnFields(field string) []string {
	switch field {
	case "song":
		return []string{"musicNames", "musicNamesPinyin"}
	case "artist":
		return []string{"artists", "artistsPinyin"}
	case "album":
		return []string{"albums", "albumsPinyin"}
	case "lyric":
		return []string{"lyricText"}
	case "", "all":
		return []string{
			"musicNames", "musicNamesPinyin",
			"artists", "artistsPinyin",
			"albums", "albumsPinyin",
			"lyricText",
			"platformIds_ncm", "platformIds_qq", "platformIds_spotify", "platformIds_apple",
		}
	}
	return []string{"*"}
}

// convertHit 把 map[string]interface{} 转换为 SearchHitResult
func convertHit(raw interface{}) SearchHitResult {
	m, ok := raw.(map[string]interface{})
	if !ok {
		return SearchHitResult{}
	}
	hit := SearchHitResult{
		PlatformIds: map[string]string{},
	}
	if v, ok := m["id"].(string); ok {
		hit.ID = v
	}
	hit.MusicNames = toStringSlice(m["musicNames"])
	hit.Artists = toStringSlice(m["artists"])
	hit.Albums = toStringSlice(m["albums"])
	if v, ok := m["rawLyricFile"].(string); ok {
		hit.RawLyricFile = v
	}
	if v, ok := m["platformIds_ncm"].(string); ok && v != "" {
		hit.PlatformIds["ncm"] = v
	}
	if v, ok := m["platformIds_qq"].(string); ok && v != "" {
		hit.PlatformIds["qq"] = v
	}
	if v, ok := m["platformIds_spotify"].(string); ok && v != "" {
		hit.PlatformIds["spotify"] = v
	}
	if v, ok := m["platformIds_apple"].(string); ok && v != "" {
		hit.PlatformIds["apple"] = v
	}
	if v, ok := toFloat(m["wordCount"]); ok {
		hit.WordCount = int(v)
	}
	if v, ok := toFloat(m["lineCount"]); ok {
		hit.LineCount = int(v)
	}
	if v, ok := toFloat(m["commitTimestamp"]); ok {
		ts := int64(v)
		hit.CommitTimestamp = &ts
	}
	return hit
}

func toStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// meiliEscape 转义 filter 字符串中的特殊字符（仅做基础双引号转义）
func meiliEscape(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '"' || r == '\\' {
			out = append(out, '\\')
		}
		out = append(out, r)
	}
	return string(out)
}

// _ 防 errors unused
var _ = errors.New
var _ = pkg.IsValidPlatform

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// errNoData 表示接口正常返回但没有数据（例如今天没有推荐）。
var errNoData = errors.New("no data")

const (
	searchCacheTTL = 60 * time.Second
	dailyCacheTTL  = 5 * time.Minute
	latestCacheTTL = 5 * time.Minute
)

type amllClient struct {
	base string
	http *http.Client

	mu       sync.Mutex
	search   map[string]searchEntry
	daily    map[string]dailyEntry
	latest   latestEntry
	latestAt time.Time
}

type searchEntry struct {
	data *searchResult
	exp  time.Time
}

type dailyEntry struct {
	data *dailyRec
	exp  time.Time
}

type latestEntry struct {
	data []latestSong
	exp  time.Time
}

func newAMLLClient(base string) *amllClient {
	return &amllClient{
		base:   strings.TrimRight(base, "/"),
		http:   &http.Client{Timeout: 10 * time.Second},
		search: make(map[string]searchEntry),
		daily:  make(map[string]dailyEntry),
	}
}

type searchResult struct {
	Hits      []searchHit `json:"hits"`
	TotalHits int64       `json:"totalHits"`
}

type searchHit struct {
	ID                    string              `json:"id"`
	MusicNames            []string            `json:"musicNames"`
	Artists               []string            `json:"artists"`
	Albums                []string            `json:"albums"`
	PlatformIDs           map[string][]string `json:"platformIds"`
	RawLyricFile          string              `json:"rawLyricFile"`
	LyricSnippet          string              `json:"lyricSnippet"`
	TtmlAuthorGithubLogin string              `json:"ttmlAuthorGithubLogin"`
}

type dailyRec struct {
	Date      string `json:"date"`
	SongName  string `json:"songName"`
	Artist    string `json:"artist"`
	CoverKey  string `json:"coverKey"`
	NcmID     string `json:"ncmId"`
	QQID      string `json:"qqId"`
	Comment   string `json:"comment"`
	LikeCount int    `json:"likeCount"`
}

type latestSong struct {
	ID       int64  `json:"id"`
	SongID   int64  `json:"songId"`
	NcmID    string `json:"ncmId"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	CoverURL string `json:"coverUrl"`
}

func (c *amllClient) Search(ctx context.Context, query, field string, limit int) (*searchResult, error) {
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	if field == "" {
		field = "all"
	}

	key := strings.Join([]string{query, field, strconv.Itoa(limit)}, "\x00")
	c.mu.Lock()
	if e, ok := c.search[key]; ok && time.Now().Before(e.exp) {
		c.mu.Unlock()
		return e.data, nil
	}
	c.mu.Unlock()

	q := url.Values{}
	q.Set("q", query)
	q.Set("field", field)
	q.Set("limit", strconv.Itoa(limit))

	var out searchResult
	if err := c.getJSON(ctx, "/api/v1/search?"+q.Encode(), &out); err != nil {
		return nil, err
	}

	c.mu.Lock()
	if len(c.search) > 256 {
		now := time.Now()
		for k, e := range c.search {
			if now.After(e.exp) {
				delete(c.search, k)
			}
		}
	}
	c.search[key] = searchEntry{data: &out, exp: time.Now().Add(searchCacheTTL)}
	c.mu.Unlock()

	return &out, nil
}

func (c *amllClient) Today(ctx context.Context) (*dailyRec, error) {
	return c.dailyCached(ctx, "today", "/api/v1/daily-recommendations/today")
}

func (c *amllClient) ByDate(ctx context.Context, date string) (*dailyRec, error) {
	return c.dailyCached(ctx, date, "/api/v1/daily-recommendations/date/"+url.PathEscape(date))
}

func (c *amllClient) dailyCached(ctx context.Context, key, path string) (*dailyRec, error) {
	c.mu.Lock()
	if e, ok := c.daily[key]; ok && time.Now().Before(e.exp) {
		c.mu.Unlock()
		if e.data == nil {
			return nil, errNoData
		}
		return e.data, nil
	}
	c.mu.Unlock()

	var rec dailyRec
	err := c.getJSON(ctx, path, &rec)
	if errors.Is(err, errNoData) {
		c.mu.Lock()
		c.daily[key] = dailyEntry{data: nil, exp: time.Now().Add(dailyCacheTTL)}
		c.mu.Unlock()
		return nil, errNoData
	}
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.daily[key] = dailyEntry{data: &rec, exp: time.Now().Add(dailyCacheTTL)}
	c.mu.Unlock()
	return &rec, nil
}

func (c *amllClient) Latest(ctx context.Context, limit int) ([]latestSong, error) {
	c.mu.Lock()
	if c.latest.data != nil && time.Now().Before(c.latestAt) {
		data := c.latest.data
		c.mu.Unlock()
		return data, nil
	}
	c.mu.Unlock()

	var list []latestSong
	if err := c.getJSON(ctx, "/api/v1/latest-songs", &list); err != nil {
		return nil, err
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}

	c.mu.Lock()
	c.latest = latestEntry{data: list}
	c.latestAt = time.Now().Add(latestCacheTTL)
	c.mu.Unlock()
	return list, nil
}

func (c *amllClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "AMLLHub-QQBot/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var env struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return err
	}
	if env.Code != http.StatusOK {
		return fmt.Errorf("接口错误 %d: %s", env.Code, env.Message)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return errNoData
	}
	return json.Unmarshal(env.Data, out)
}

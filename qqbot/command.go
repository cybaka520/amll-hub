package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type commandKind int

const (
	cmdHelp commandKind = iota
	cmdSearch
	cmdToday
	cmdByDate
	cmdLatest
)

type command struct {
	kind  commandKind
	query string
	field string
	date  string
}

type reply struct {
	Text     string
	ImageURL string
}

const helpText = `AMLL Hub 指令：
搜索 <关键词> / 点歌 <关键词> —— 搜索歌曲与歌词
歌词 <关键词> —— 只搜歌词文本
每日推荐 / 今日推荐 —— 今天的推荐
推荐 2026-11-20 —— 查看指定日期
最新 / 最新收录 —— 最近收录的歌曲`

var mentionRe = regexp.MustCompile(`<@!?\d+>`)

func cleanContent(s string) string {
	s = mentionRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, "/／!！")
	return strings.TrimSpace(s)
}

func parseCommand(text string) command {
	s := strings.TrimSpace(text)
	if s == "" {
		return command{kind: cmdHelp}
	}

	switch strings.ToLower(s) {
	case "help", "帮助", "指令", "菜单":
		return command{kind: cmdHelp}
	case "推荐", "每日推荐", "今日推荐", "今天推荐", "daily":
		return command{kind: cmdToday}
	case "最新", "最新收录", "最近更新", "latest":
		return command{kind: cmdLatest}
	}

	// 搜索前缀
	for _, p := range []string{"搜索", "搜歌词", "查歌词", "找歌词", "点歌", "search"} {
		if strings.HasPrefix(strings.ToLower(s), p) {
			rest := strings.TrimSpace(s[len(p):])
			if rest == "" {
				return command{kind: cmdHelp}
			}
			field := "all"
			if p == "搜歌词" || p == "查歌词" || p == "找歌词" {
				field = "lyric"
			}
			return command{kind: cmdSearch, query: rest, field: field}
		}
	}

	// 推荐 <日期>
	if strings.HasPrefix(s, "推荐") {
		rest := strings.TrimSpace(strings.TrimPrefix(s, "推荐"))
		if date, ok := normalizeDate(rest, time.Local); ok {
			return command{kind: cmdByDate, date: date}
		}
	}

	return command{kind: cmdSearch, query: s, field: "all"}
}

// normalizeDate 支持 2026-11-20 / 2026/11/20 / 11-20 / 11月20日 / 今天 / 昨天。
func normalizeDate(raw string, loc *time.Location) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}

	now := time.Now().In(loc)
	switch s {
	case "今天", "今日":
		return now.Format("2006-01-02"), true
	case "昨天", "昨日":
		return now.AddDate(0, 0, -1).Format("2006-01-02"), true
	}

	s = strings.ReplaceAll(s, "日", "")
	s = strings.ReplaceAll(s, "年", "-")
	s = strings.ReplaceAll(s, "月", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.Trim(s, "-")

	for _, layout := range []string{"2006-01-02", "2006-1-2"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	for _, layout := range []string{"01-02", "1-2"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			t = time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

func (b *bot) buildReply(ctx context.Context, cmd command) reply {
	switch cmd.kind {
	case cmdHelp:
		return reply{Text: helpText}

	case cmdToday:
		rec, err := b.amll.Today(ctx)
		if errors.Is(err, errNoData) {
			return reply{Text: "今天还没有每日推荐，晚点再来看看吧～"}
		}
		if err != nil {
			log.Printf("查询今日推荐失败: %v", err)
			return reply{Text: "查询每日推荐失败，请稍后再试"}
		}
		return reply{Text: formatDaily(rec, b.cfg.webBase), ImageURL: b.coverURL(rec.CoverKey)}

	case cmdByDate:
		rec, err := b.amll.ByDate(ctx, cmd.date)
		if errors.Is(err, errNoData) {
			return reply{Text: fmt.Sprintf("%s 没有每日推荐。", cmd.date)}
		}
		if err != nil {
			log.Printf("查询 %s 推荐失败: %v", cmd.date, err)
			return reply{Text: "查询每日推荐失败，请稍后再试"}
		}
		return reply{Text: formatDaily(rec, b.cfg.webBase), ImageURL: b.coverURL(rec.CoverKey)}

	case cmdLatest:
		list, err := b.amll.Latest(ctx, 5)
		if err != nil {
			log.Printf("查询最新收录失败: %v", err)
			return reply{Text: "查询最新收录失败，请稍后再试"}
		}
		return reply{Text: formatLatest(list, b.cfg.webBase)}

	case cmdSearch:
		query := strings.TrimSpace(cmd.query)
		if query == "" {
			return reply{Text: helpText}
		}
		res, err := b.amll.Search(ctx, query, cmd.field, 5)
		if err != nil {
			log.Printf("搜索「%s」失败: %v", query, err)
			return reply{Text: "搜索失败，请稍后再试"}
		}
		return reply{Text: formatSearch(res, query, b.cfg.webBase)}
	}

	return reply{Text: helpText}
}

func (b *bot) coverURL(key string) string {
	if key == "" {
		return ""
	}
	return b.cfg.apiBase + "/api/v1/daily-recommendations/image/" + url.PathEscape(key) + "?w=640"
}

func formatDaily(rec *dailyRec, webBase string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【每日推荐】%s\n", rec.Date)
	fmt.Fprintf(&b, "《%s》 - %s\n", rec.SongName, rec.Artist)
	if rec.Comment != "" {
		fmt.Fprintf(&b, "%s\n", strings.TrimSpace(rec.Comment))
	}
	if rec.LikeCount > 0 {
		fmt.Fprintf(&b, "❤ %d\n", rec.LikeCount)
	}
	fmt.Fprintf(&b, "详情：%s/daily?date=%s", webBase, url.QueryEscape(rec.Date))
	return b.String()
}

func formatSearch(res *searchResult, query, webBase string) string {
	if res == nil || len(res.Hits) == 0 {
		return fmt.Sprintf("没有找到「%s」相关的歌词。", query)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "「%s」共 %d 条，显示前 %d 条：\n", query, res.TotalHits, len(res.Hits))
	for i, h := range res.Hits {
		name := firstOr(h.MusicNames, "未知歌曲")
		artist := strings.Join(h.Artists, " / ")
		if artist == "" {
			artist = "未知歌手"
		}
		fmt.Fprintf(&b, "%d. %s - %s\n", i+1, name, artist)
		if album := firstOr(h.Albums, ""); album != "" {
			fmt.Fprintf(&b, "   专辑：%s\n", album)
		}
		if h.LyricSnippet != "" {
			fmt.Fprintf(&b, "   歌词：%s\n", oneLine(h.LyricSnippet, 60))
		}
		if h.RawLyricFile != "" {
			fmt.Fprintf(&b, "   %s/lyric/%s\n", webBase, url.PathEscape(h.RawLyricFile))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatLatest(list []latestSong, webBase string) string {
	if len(list) == 0 {
		return "暂无最新收录数据。"
	}

	var b strings.Builder
	b.WriteString("最新收录：\n")
	for i, s := range list {
		fmt.Fprintf(&b, "%d. %s - %s\n", i+1, s.Title, s.Artist)
		if s.NcmID != "" {
			fmt.Fprintf(&b, "   %s/online-lyric/ncm/%s\n", webBase, url.PathEscape(s.NcmID))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func firstOr(list []string, def string) string {
	for _, v := range list {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return def
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	return truncateRunes(s, max)
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}

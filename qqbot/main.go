// QQ 机器人适配层：把 AMLL Hub 的搜索、每日推荐、最新收录接入 QQ 官方机器人。
//
// 只依赖 AMLL Hub 的公开只读接口，不需要改动后端。
// 通过 QQ Bot API v2 的 WebSocket 长连接接收群 @ 消息与私聊消息，
// 因此不需要公网 HTTPS 回调地址。
package main

import (
	"bufio"
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type config struct {
	appID        string
	clientSecret string
	sandbox      bool
	apiBase      string // AMLL Hub API 地址，末尾不带 /
	webBase      string // 站点地址，用于拼详情链接
	intents      int
	location     *time.Location
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[qqbot] ")

	loadDotEnv(".env")

	cfg := loadConfig()
	if cfg.appID == "" || cfg.clientSecret == "" {
		log.Fatal("缺少 QQ_BOT_APP_ID / QQ_BOT_CLIENT_SECRET，请查看 qqbot/.env.example")
	}
	// 让「今天 / 昨天 / 推荐 11-20」按站点时区计算
	time.Local = cfg.location

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b := newBot(cfg)
	log.Printf("启动：api=%s web=%s sandbox=%v", cfg.apiBase, cfg.webBase, cfg.sandbox)
	if err := b.run(ctx); err != nil {
		log.Fatalf("机器人退出: %v", err)
	}
	log.Println("已停止")
}

func loadConfig() config {
	loc, err := time.LoadLocation(envOr("TZ", "Asia/Shanghai"))
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}

	intents := intentGroupAndC2C
	if v := strings.TrimSpace(os.Getenv("QQ_BOT_INTENTS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			intents = n
		}
	}

	return config{
		appID:        strings.TrimSpace(os.Getenv("QQ_BOT_APP_ID")),
		clientSecret: strings.TrimSpace(os.Getenv("QQ_BOT_CLIENT_SECRET")),
		sandbox:      envBool("QQ_BOT_SANDBOX"),
		apiBase:      strings.TrimRight(envOr("AMLL_API_BASE", "https://amlldb.bikonoo.com"), "/"),
		webBase:      strings.TrimRight(envOr("AMLL_WEB_BASE", "https://amlldb.bikonoo.com"), "/"),
		intents:      intents,
		location:     loc,
	}
}

func newBot(cfg config) *bot {
	return &bot{
		cfg:  cfg,
		http: &http.Client{Timeout: 10 * time.Second},
		tokens: &tokenManager{
			appID:  cfg.appID,
			secret: cfg.clientSecret,
			http:   &http.Client{Timeout: 10 * time.Second},
		},
		amll: newAMLLClient(cfg.apiBase),
	}
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// loadDotEnv 读取同目录下的 .env，已存在的环境变量优先。
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}

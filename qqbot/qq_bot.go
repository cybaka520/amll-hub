package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// GROUP_AND_C2C_EVENT：群 @ 消息 + 单聊消息
	intentGroupAndC2C = 1 << 25

	maxMessageRunes = 1800
)

type bot struct {
	cfg    config
	http   *http.Client
	tokens *tokenManager
	amll   *amllClient

	writeMu sync.Mutex // gorilla/websocket 只允许一个写者
}

// messageEvent 是 GROUP_AT_MESSAGE_CREATE / C2C_MESSAGE_CREATE 的 d 字段。
type messageEvent struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	GroupOpenID string `json:"group_openid"`
	Author      struct {
		ID           string `json:"id"`
		MemberOpenID string `json:"member_openid"`
		UserOpenID   string `json:"user_openid"`
	} `json:"author"`
}

func (e *messageEvent) userOpenID() string {
	if e.Author.UserOpenID != "" {
		return e.Author.UserOpenID
	}
	return e.Author.ID
}

// run 维持长连接，断线后按指数退避重连。
func (b *bot) run(ctx context.Context) error {
	backoff := time.Second
	for {
		err := b.session(ctx)
		if ctx.Err() != nil {
			return nil
		}
		log.Printf("连接断开: %v，%s 后重连", err, backoff)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

// session 建立一次 WebSocket 会话直到断开。
func (b *bot) session(ctx context.Context) error {
	token, err := b.tokens.get(ctx)
	if err != nil {
		return fmt.Errorf("获取 access token: %w", err)
	}

	gwURL, err := b.gatewayURL(ctx, token)
	if err != nil {
		return fmt.Errorf("获取 gateway: %w", err)
	}

	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, gwURL, nil)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			resp.Body.Close()
			return fmt.Errorf("连接 gateway 失败: %w (%s)", err, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("连接 gateway 失败: %w", err)
	}
	defer conn.Close()

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-sctx.Done()
		conn.Close()
	}()

	writeJSON := func(v any) error {
		b.writeMu.Lock()
		defer b.writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(v)
	}

	// 第一条消息必须是 op 10 Hello
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	var hello struct {
		Op int `json:"op"`
		D  struct {
			HeartbeatInterval int `json:"heartbeat_interval"`
		} `json:"d"`
	}
	if _, msg, err := conn.ReadMessage(); err != nil {
		return fmt.Errorf("读取 Hello 失败: %w", err)
	} else if err := json.Unmarshal(msg, &hello); err != nil {
		return fmt.Errorf("解析 Hello 失败: %w", err)
	}
	if hello.Op != 10 {
		return fmt.Errorf("期待 op 10 Hello，收到 op %d", hello.Op)
	}

	interval := time.Duration(hello.D.HeartbeatInterval) * time.Millisecond
	if interval <= 0 {
		interval = 30 * time.Second
	}

	if err := writeJSON(map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":      "QQBot " + token,
			"intents":    b.cfg.intents,
			"shard":      []int{0, 1},
			"properties": map[string]string{},
		},
	}); err != nil {
		return fmt.Errorf("发送 Identify 失败: %w", err)
	}

	var (
		lastSeq atomic.Int64
		acked   atomic.Bool
	)
	acked.Store(true)

	hbDone := make(chan struct{})
	defer close(hbDone)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-hbDone:
				return
			case <-sctx.Done():
				return
			case <-t.C:
				if !acked.Swap(false) {
					log.Println("心跳未确认，主动重连")
					conn.Close()
					return
				}
				var d any
				if seq := lastSeq.Load(); seq > 0 {
					d = seq
				}
				if err := writeJSON(map[string]any{"op": 1, "d": d}); err != nil {
					conn.Close()
					return
				}
			}
		}
	}()

	for {
		// 正常情况下心跳 ACK 会持续刷新读超时
		_ = conn.SetReadDeadline(time.Now().Add(interval*2 + 30*time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var ev struct {
			Op int             `json:"op"`
			S  int64           `json:"s"`
			T  string          `json:"t"`
			D  json.RawMessage `json:"d"`
		}
		if err := json.Unmarshal(msg, &ev); err != nil {
			continue
		}
		if ev.S > 0 {
			lastSeq.Store(ev.S)
		}

		switch ev.Op {
		case 0:
			if ev.T == "READY" {
				log.Println("已就绪")
				continue
			}
			b.dispatch(sctx, ev.T, ev.D)
		case 11:
			acked.Store(true)
		case 7:
			return errors.New("服务端要求重连")
		case 9:
			return errors.New("会话失效，重新 Identify")
		case 10:
			// 重连后的 Hello，忽略
		}
	}
}

func (b *bot) dispatch(ctx context.Context, eventType string, raw json.RawMessage) {
	switch eventType {
	case "GROUP_AT_MESSAGE_CREATE", "C2C_MESSAGE_CREATE":
	default:
		return
	}

	var ev messageEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		log.Printf("解析消息事件失败: %v", err)
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("处理消息 panic: %v", r)
			}
		}()

		hctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := b.handleMessage(hctx, eventType, &ev); err != nil {
			log.Printf("处理消息失败: %v", err)
		}
	}()
}

func (b *bot) handleMessage(ctx context.Context, eventType string, ev *messageEvent) error {
	isGroup := eventType == "GROUP_AT_MESSAGE_CREATE"
	cmd := parseCommand(cleanContent(ev.Content))
	reply := b.buildReply(ctx, cmd)

	seq := 1
	if reply.Text != "" {
		if err := b.sendText(ctx, ev, isGroup, reply.Text, seq); err != nil {
			return fmt.Errorf("发送文本: %w", err)
		}
		seq++
	}
	if reply.ImageURL != "" {
		if err := b.sendImage(ctx, ev, isGroup, reply.ImageURL, seq); err != nil {
			log.Printf("发送封面失败: %v", err)
		}
	}
	return nil
}

func (b *bot) sendText(ctx context.Context, ev *messageEvent, isGroup bool, text string, seq int) error {
	return b.sendMessage(ctx, ev, isGroup, map[string]any{
		"content":  truncateRunes(text, maxMessageRunes),
		"msg_type": 0,
		"msg_id":   ev.ID,
		"msg_seq":  seq,
	})
}

// sendImage 先上传图片拿到 file_info，再发富媒体消息。
func (b *bot) sendImage(ctx context.Context, ev *messageEvent, isGroup bool, imageURL string, seq int) error {
	fileInfo, err := b.uploadImage(ctx, ev, isGroup, imageURL)
	if err != nil {
		return err
	}
	return b.sendMessage(ctx, ev, isGroup, map[string]any{
		"msg_type": 7,
		"media":    map[string]string{"file_info": fileInfo},
		"msg_id":   ev.ID,
		"msg_seq":  seq,
	})
}

func (b *bot) uploadImage(ctx context.Context, ev *messageEvent, isGroup bool, imageURL string) (string, error) {
	var endpoint string
	if isGroup {
		endpoint = b.apiURL("/v2/groups/" + url.PathEscape(ev.GroupOpenID) + "/files")
	} else {
		endpoint = b.apiURL("/v2/users/" + url.PathEscape(ev.userOpenID()) + "/files")
	}

	body, err := b.postJSON(ctx, endpoint, map[string]any{
		"file_type":    1,
		"url":          imageURL,
		"srv_send_msg": false,
	})
	if err != nil {
		return "", err
	}

	var out struct {
		FileInfo string `json:"file_info"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	if out.FileInfo == "" {
		return "", errors.New("上传接口未返回 file_info")
	}
	return out.FileInfo, nil
}

func (b *bot) sendMessage(ctx context.Context, ev *messageEvent, isGroup bool, payload map[string]any) error {
	var endpoint string
	if isGroup {
		if ev.GroupOpenID == "" {
			return errors.New("群消息缺少 group_openid")
		}
		endpoint = b.apiURL("/v2/groups/" + url.PathEscape(ev.GroupOpenID) + "/messages")
	} else {
		endpoint = b.apiURL("/v2/users/" + url.PathEscape(ev.userOpenID()) + "/messages")
	}
	_, err := b.postJSON(ctx, endpoint, payload)
	return err
}

func (b *bot) postJSON(ctx context.Context, endpoint string, payload any) ([]byte, error) {
	token, err := b.tokens.get(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "QQBot "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (b *bot) gatewayURL(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.apiURL("/gateway"), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "QQBot "+token)

	resp, err := b.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var out struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.URL == "" {
		return "", errors.New("gateway 响应缺少 url")
	}
	return out.URL, nil
}

func (b *bot) apiURL(path string) string {
	base := "https://api.sgroup.qq.com"
	if b.cfg.sandbox {
		base = "https://sandbox.api.sgroup.qq.com"
	}
	return base + path
}

// tokenManager 缓存 app access token，过期前自动刷新。
type tokenManager struct {
	appID  string
	secret string
	http   *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (t *tokenManager) get(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && time.Now().Before(t.expiresAt) {
		return t.token, nil
	}

	body, _ := json.Marshal(map[string]string{
		"appId":        t.appID,
		"clientSecret": t.secret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://bots.qq.com/app/getAppAccessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}

	var out struct {
		AccessToken string      `json:"access_token"`
		ExpiresIn   json.Number `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("获取 token 失败: %s", strings.TrimSpace(string(data)))
	}

	ttl, _ := out.ExpiresIn.Int64()
	if ttl <= 60 {
		ttl = 7200
	}
	t.token = out.AccessToken
	// 提前一分钟刷新，避免临界过期
	t.expiresAt = time.Now().Add(time.Duration(ttl)*time.Second - time.Minute)
	return t.token, nil
}

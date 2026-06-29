# TTML 歌词服务 API - 项目指令（最终定稿版）

## 1. 项目概述

构建一个面向 `amll-ttml-db` GitHub 仓库的歌词服务 API。该仓库采用 CC0 协议，提供标准化的 TTML 格式歌词文件。本服务作为该仓库的二次封装层，提供自动同步、歌词获取、多维度搜索等能力。

**核心目标：**
- 自动同步 GitHub 仓库歌词文件到本地存储
- 提供按音乐平台 ID 直接获取原始 TTML 文件的 API
- 提供歌曲名、艺术家、专辑、平台 ID、歌词内容的多维度搜索（含拼音搜索）
- 所有歌词文件不做任何二次加工，原样返回
- 支持 HTTP Range 请求（断点续传/预览）

---

## 2. 技术栈

### 2.1 整体架构

| 层级 | 技术 | 版本/说明 |
|------|------|-----------|
| 用户层 | React + Vite + TypeScript | 19.x / 6.x / 5.x |
| 认证层 | **Casdoor** | 架构预留，**第一阶段暂不实现代码** |
| API 网关层 | **Go + Gin** | 1.22+ / 1.9+ |
| 同步 Worker | **Rust + Tokio** | 1.80+ |
| 搜索层 | MeiliSearch | 最新稳定版 |
| 消息层 | RabbitMQ | 3.12+ |
| 数据层 | PostgreSQL | 15+ |
| 文件层 | MinIO | 最新稳定版 |
| 缓存层 | Redis | 7+ |

### 2.2 语言分工说明

| 服务 | 语言 | 职责 | 原因 |
|------|------|------|------|
| **API 服务** | Go | HTTP 接口（歌词获取、搜索、批量查询、同步触发、状态查询） | 开发效率高，HTTP 生态成熟，快速响应高并发请求 |
| **同步 Worker** | Rust | 消费 RabbitMQ 队列，执行同步任务（下载、解析、入库、索引） | 内存安全、长时间运行稳定、高并发 IO 性能极致 |

**两者通过 RabbitMQ 消息队列 + PostgreSQL/MinIO/MeiliSearch/Redis 共享数据，无直接通信。**

---

## 3. 架构设计

### 3.1 整体架构图

```
用户层 (React 19 + Vite + TypeScript)
    │
    ▼
认证层 (Casdoor) ←── 架构预留，第一阶段不接入
    │
    ▼
┌─────────────────────────────────────────┐
│         Go 服务 (API 网关层)             │
│  ┌─────────┐ ┌─────────┐ ┌───────────┐ │
│  │ 歌词获取 │ │  搜索   │ │ 批量查询  │ │
│  │  /sync  │ │ /search │ │  /batch   │ │
│  │ 同步状态 │ │  /stats │ │           │ │
│  └─────────┘ └─────────┘ └───────────┘ │
│  技术：Gin + GORM + Redis + MinIO       │
└─────────────────────────────────────────┘
                    │
                    ▼
            ┌──────────────┐
            │   RabbitMQ    │
            │  sync_queue   │
            └──────────────┘
                    │
                    ▼
┌─────────────────────────────────────────┐
│      Rust 服务 (同步 Worker)             │
│  ┌─────────────────────────────────┐   │
│  │ 消费队列 → 下载文件 → 解析 TTML  │   │
│  │ → 提取拼音 → 写入 PG → 更新      │   │
│  │ MeiliSearch 索引                  │   │
│  └─────────────────────────────────┘   │
│  技术：Tokio + SeaORM + reqwest + lapin │
└─────────────────────────────────────────┘
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
   PostgreSQL   MinIO      MeiliSearch
   (元数据)    (文件)       (搜索索引)
        │           │           │
        └───────────┴───────────┘
                    │
                   Redis
                  (缓存/锁)
```

### 3.2 分层职责说明

**用户层：**
- 提供歌词搜索、展示、获取的 Web 界面
- 调用后端 API 获取数据

**认证层 (Casdoor) - 暂不实现：**
- 预留 SSO 统一认证能力
- 支持 JWT 签发与校验
- 第一阶段所有 API 均为公开访问，不强制认证
- 代码中预留 JWT Middleware 接口，但不启用

**Go API 服务：**
- 路由注册与参数校验
- 注入 `X-Request-ID` 请求追踪 ID
- 基础限流（全局级别）
- 错误统一封装与日志记录
- **HTTP Range 请求解析与响应**
- 同步任务入队（`POST /api/v1/sync`）
- 同步状态查询（查 PostgreSQL `sync_progress` 表）
- 歌词获取（从 MinIO 流式返回）
- 搜索（查 MeiliSearch）
- 批量查询（查 PostgreSQL）
- 词库统计（查 PostgreSQL）

**Rust 同步 Worker：**
- 消费 RabbitMQ `sync_queue`
- 获取 Redis 分布式锁
- 下载 `raw-lyrics-index.jsonl` 并解析
- 对比本地索引，计算差异
- 并发下载文件到 MinIO（goroutine 池 → Tokio Semaphore）
- 解析 TTML 提取纯文本和拼音
- 写入 PostgreSQL（事务）
- 批量更新 MeiliSearch 索引
- 更新同步状态、历史记录、实时进度
- 释放锁，检查队列继续消费

**消息层 (RabbitMQ)：**
- 解耦 Go API 服务与 Rust Worker
- 支持同步任务排队与串行消费
- 死信队列 (DLQ) 处理失败文件

**搜索层 (MeiliSearch)：**
- Go 查询，Rust 写入索引
- 歌词纯文本全文索引
- 歌曲名、艺术家、专辑名索引
- **拼音字段索引（中文搜索优化）**
- 平台 ID 精确过滤

**数据层 (PostgreSQL)：**
- Go 和 Rust 共用同一数据库
- 存储歌曲、艺术家、专辑、平台映射等关系型元数据
- 存储同步状态、历史记录、**实时进度**
- 作为系统唯一真相源 (Source of Truth)

**文件层 (MinIO)：**
- Rust 写入文件，Go 读取文件
- 按平台分文件夹存储
- 支持通过路径直接获取对象流
- **支持 Range 请求（断点续传）**

**缓存层 (Redis)：**
- Go 读取缓存（热点查询）
- Rust 写入分布式锁（防止并发同步）
- 同步任务状态缓存（辅助）

---

## 4. 数据模型设计

### 4.1 数据库选型说明
- 使用 **PostgreSQL** 作为唯一关系型数据库
- 使用 **golang-migrate** 管理 Schema 变更（Go 端执行）
- **禁止使用 GORM AutoMigrate 上生产环境**
- Go 使用 **GORM v2**，Rust 使用 **SeaORM**
- 两边模型定义需手动同步，以 `migrations/` 目录的 SQL 为准

### 4.2 核心表结构

#### songs（歌曲主表）

```sql
CREATE TABLE songs (
    id              BIGSERIAL PRIMARY KEY,
    music_name      JSONB NOT NULL DEFAULT '[]',        -- 多语言歌曲名，如 ["宜", "Yi"]
    album           JSONB NOT NULL DEFAULT '[]',        -- 专辑名，如 ["宜"]
    isrc            VARCHAR(20),                        -- ISRC 编码
    raw_lyric_file  VARCHAR(255) NOT NULL UNIQUE,       -- raw-lyrics 文件名，唯一标识
    minio_path      VARCHAR(500) NOT NULL,             -- MinIO 存储路径
    lyric_text      TEXT,                               -- 歌词纯文本（用于辅助搜索/预览）
    ttml_author_github        VARCHAR(50),             -- TTML 作者 GitHub ID
    ttml_author_github_login  VARCHAR(100),             -- TTML 作者 GitHub 用户名
    word_count      INT DEFAULT 0,                      -- 歌词字数统计
    line_count      INT DEFAULT 0,                      -- 歌词行数统计
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,      -- 软删除标记（预留）
    deleted_at      TIMESTAMPTZ,                       -- 删除时间
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_songs_music_name ON songs USING GIN(music_name);
CREATE INDEX idx_songs_album ON songs USING GIN(album);
CREATE INDEX idx_songs_raw_lyric_file ON songs(raw_lyric_file);
```

#### artists（艺术家表）

```sql
CREATE TABLE artists (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_artists_name ON artists(name);
```

#### song_artists（歌曲-艺术家关联表）

```sql
CREATE TABLE song_artists (
    song_id     BIGINT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    artist_id   BIGINT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    PRIMARY KEY (song_id, artist_id)
);
```

#### platform_mappings（平台 ID 映射表）

```sql
CREATE TABLE platform_mappings (
    id          BIGSERIAL PRIMARY KEY,
    song_id     BIGINT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    platform    VARCHAR(20) NOT NULL,                    -- ncm, qq, spotify, apple
    platform_id VARCHAR(100) NOT NULL,                   -- 平台音乐 ID
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(song_id, platform),
    UNIQUE(platform, platform_id)
);

CREATE INDEX idx_platform_mappings_platform ON platform_mappings(platform, platform_id);
CREATE INDEX idx_platform_mappings_song ON platform_mappings(song_id);
```

#### sync_state（同步状态表）

```sql
CREATE TABLE sync_state (
    key   VARCHAR(50) PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO sync_state (key, value) VALUES ('last_synced_commit', '');
INSERT INTO sync_state (key, value) VALUES ('last_synced_at', '');
```

#### sync_history（同步历史表）

```sql
CREATE TABLE sync_history (
    id              BIGSERIAL PRIMARY KEY,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    previous_commit VARCHAR(40),
    target_commit   VARCHAR(40) NOT NULL,
    status          VARCHAR(20) NOT NULL,               -- running, success, failed
    added_count     INT NOT NULL DEFAULT 0,
    updated_count   INT NOT NULL DEFAULT 0,
    deleted_count   INT NOT NULL DEFAULT 0,
    error_message   TEXT,
    triggered_by    VARCHAR(20) NOT NULL,               -- api, cron, github_action
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sync_history_time ON sync_history(started_at DESC);
```

#### sync_progress（同步进度表）

```sql
CREATE TABLE sync_progress (
    id              BIGSERIAL PRIMARY KEY,
    sync_history_id BIGINT NOT NULL REFERENCES sync_history(id) ON DELETE CASCADE,
    total           INT NOT NULL DEFAULT 0,
    downloaded      INT NOT NULL DEFAULT 0,
    failed          INT NOT NULL DEFAULT 0,
    current_file    VARCHAR(255),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sync_progress_history ON sync_progress(sync_history_id);
```

### 4.3 数据存储分工总结

| 数据类型 | 存储位置 | 说明 |
|----------|----------|------|
| 歌曲/艺术家/专辑/平台映射关系 | PostgreSQL | 关系型数据，Go 和 Rust 共用 |
| 同步状态、Commit Hash、历史记录、实时进度 | PostgreSQL | 唯一真相源，Go 查 Rust 写 |
| 原始 TTML XML 文件 | MinIO | Rust 写入，Go 流式返回，不做任何加工 |
| 歌词纯文本（搜索用） | MeiliSearch | Rust 写入索引，Go 查询 |
| 拼音字段（搜索用） | MeiliSearch | Rust 写入索引，Go 查询 |
| 分布式锁 | Redis | Rust 写入 |
| 热点缓存 | Redis | Go 读取 |

---

## 5. Go API 服务详细设计

### 5.1 技术选型

| 组件 | 库/Crate | 说明 |
|------|----------|------|
| HTTP 框架 | `gin-gonic/gin` | 路由、中间件、参数绑定 |
| ORM | `gorm.io/gorm` + `gorm.io/driver/postgres` | 数据库操作 |
| 数据库连接池 | 内置 | GORM 管理 |
| Redis | `github.com/redis/go-redis/v9` | 缓存读取 |
| MinIO | `github.com/minio/minio-go/v7` | 文件读取、Range 请求 |
| MeiliSearch | `github.com/meilisearch/meilisearch-go` | 搜索查询 |
| RabbitMQ | `github.com/rabbitmq/amqp091-go` | 同步任务入队 |
| 配置 | `github.com/spf13/viper` | 环境变量 + 配置文件 |
| 日志 | `github.com/sirupsen/logrus` 或 `go.uber.org/zap` | JSON 结构化日志 |
| 拼音 | `github.com/mozillazg/go-pinyin` | 预留（Rust Worker 已处理，Go 端不需要） |

### 5.2 代码结构

```
/ttml-api-go
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── handler/
│   │   ├── sync.go          # POST /sync, GET /sync/status
│   │   ├── lyrics.go        # GET /{folder}/{filename}
│   │   ├── search.go        # GET /search
│   │   ├── batch.go         # POST /batch
│   │   └── stats.go         # GET /stats
│   ├── service/
│   │   ├── sync_service.go
│   │   ├── lyrics_service.go
│   │   ├── search_service.go
│   │   └── stats_service.go
│   ├── repository/
│   │   ├── song_repo.go
│   │   ├── artist_repo.go
│   │   ├── platform_repo.go
│   │   ├── sync_repo.go
│   │   └── sync_progress_repo.go
│   ├── model/
│   │   ├── song.go
│   │   ├── sync.go
│   │   ├── sync_progress.go
│   │   └── meilisearch.go
│   ├── infrastructure/
│   │   ├── postgres.go
│   │   ├── redis.go
│   │   ├── minio.go
│   │   ├── rabbitmq.go
│   │   └── meilisearch.go
│   ├── middleware/
│   │   ├── request_id.go
│   │   ├── logger.go
│   │   ├── recovery.go
│   │   └── range.go         # Range 请求解析
│   └── pkg/
│       ├── validator.go
│       └── response.go
├── migrations/
│   ├── 001_create_tables.up.sql
│   ├── 001_create_tables.down.sql
│   ├── 002_add_sync_progress.up.sql
│   └── 002_add_sync_progress.down.sql
├── docker-compose.yml
├── Dockerfile
└── README.md
```

### 5.3 关键接口实现

#### 歌词获取（含 Range 支持）

```go
func (h *LyricsHandler) GetLyrics(c *gin.Context) {
    folder := c.Param("folder")
    filename := c.Param("filename")

    // 查 PG 获取 minio_path
    song, err := h.songRepo.GetByPlatform(folder, filename)
    if err != nil {
        c.Status(404)
        return
    }

    // 解析 Range 头
    rangeHeader := c.GetHeader("Range")

    // 从 MinIO 获取对象（支持 Range）
    object, err := h.minioClient.GetObject(c, bucket, song.MinioPath, minio.GetObjectOptions{})
    if err != nil {
        c.Status(500)
        return
    }
    defer object.Close()

    // 设置响应头
    c.Header("Content-Type", "application/xml; charset=utf-8")
    c.Header("Cache-Control", "public, max-age=31536000, immutable")

    if rangeHeader != "" {
        // 解析 bytes=start-end，设置 Content-Range，返回 206
        // ...
        c.Status(206)
    } else {
        c.Status(200)
    }

    io.Copy(c.Writer, object)
}
```

#### 同步状态查询（从 PG 读）

```go
func (h *SyncHandler) GetStatus(c *gin.Context) {
    // 查 sync_progress 表获取最新进度
    progress, err := h.syncProgressRepo.GetLatest()
    if err != nil {
        // 无同步记录，返回空闲状态
        c.JSON(200, gin.H{"syncing": false})
        return
    }

    // 查 sync_history 确认状态
    history, _ := h.syncRepo.GetHistory(progress.SyncHistoryID)

    c.JSON(200, gin.H{
        "syncing": history.Status == "running",
        "started_at": history.StartedAt,
        "progress": gin.H{
            "total": progress.Total,
            "downloaded": progress.Downloaded,
            "failed": progress.Failed,
            "current_file": progress.CurrentFile,
        },
    })
}
```

---

## 6. Rust 同步 Worker 详细设计

### 6.1 技术选型

| 组件 | 库/Crate | 说明 |
|------|----------|------|
| 异步运行时 | `tokio` | 并发调度、网络 IO |
| HTTP 框架 | `axum` | 可选，用于健康检查端点 |
| ORM | `sea-orm` | 数据库操作（async） |
| 数据库驱动 | `sqlx` | 备用，编译期检查 SQL |
| HTTP 客户端 | `reqwest` | 下载 GitHub 文件 |
| RabbitMQ | `lapin` | 消费队列 |
| MeiliSearch | `meilisearch-sdk` | 更新索引 |
| MinIO (S3) | `aws-sdk-s3` | 上传文件 |
| Redis | `redis` (tokio-rs) | 分布式锁 |
| 配置 | `config` + `dotenvy` | 环境变量 |
| 日志 | `tracing` + `tracing-subscriber` | 结构化日志 |
| XML 解析 | `quick-xml` + `serde` | TTML 解析 |
| JSON | `serde` + `serde_json` | 序列化 |
| 拼音 | `pinyin` | 中文转拼音 |

### 6.2 代码结构

```
/ttml-worker-rust
├── Cargo.toml
├── src/
│   ├── main.rs
│   ├── config.rs              # 配置读取
│   ├── app.rs                 # 应用状态（连接池集合）
│   ├── worker/
│   │   ├── mod.rs
│   │   ├── consumer.rs        # RabbitMQ 消费者
│   │   ├── sync_task.rs       # 同步任务主逻辑
│   │   └── progress.rs        # 进度更新
│   ├── sync/
│   │   ├── mod.rs
│   │   ├── github.rs          # GitHub API 调用
│   │   ├── index_parser.rs    # raw-lyrics-index.jsonl 解析
│   │   ├── diff.rs            # 差异计算
│   │   ├── downloader.rs      # 文件下载（并发控制）
│   │   └── ttml_parser.rs     # TTML 解析 + 拼音提取
│   ├── db/
│   │   ├── mod.rs
│   │   ├── models.rs          # SeaORM 模型
│   │   └── repository.rs      # 数据访问层
│   ├── search/
│   │   └── meilisearch.rs     # MeiliSearch 索引更新
│   ├── storage/
│   │   ├── minio.rs           # MinIO 文件上传
│   │   └── redis.rs           # Redis 锁 + 缓存
│   └── infra/
│       ├── postgres.rs          # PG 连接池
│       ├── rabbitmq.rs          # RabbitMQ 连接
│       └── meilisearch_client.rs # MeiliSearch 连接
├── migrations/
│   └── (SeaORM 迁移文件，与 Go 端共用 SQL)
├── Dockerfile
└── README.md
```

### 6.3 关键模块实现

#### 并发下载（Tokio Semaphore）

```rust
use tokio::sync::Semaphore;
use std::sync::Arc;

async fn download_files(tasks: Vec<FileTask>, app: Arc<AppState>) -> Result<(), Error> {
    let semaphore = Arc::new(Semaphore::new(20)); // 20 并发
    let mut handles = Vec::with_capacity(tasks.len());

    for task in tasks {
        let permit = semaphore.clone().acquire_owned().await?;
        let app = app.clone();

        handles.push(tokio::spawn(async move {
            let result = download_and_upload(&app, &task).await;
            drop(permit);
            result
        }));
    }

    // 等待全部完成，收集错误
    let results = futures::future::join_all(handles).await;
    for res in results {
        res??; // 外层 JoinHandle，内层业务 Result
    }

    Ok(())
}
```

#### 数据库事务（SeaORM）

```rust
use sea_orm::TransactionTrait;

let txn = db.begin().await?;

// 插入歌曲
let song = song::ActiveModel {
    music_name: Set(json![["宜", "Yi"]]),
    album: Set(json![["宜"]]),
    raw_lyric_file: Set("1778433565542-115442729-B1QRSIWy.ttml".to_string()),
    minio_path: Set("raw-lyrics/1778433565542-115442729-B1QRSIWy.ttml".to_string()),
    lyric_text: Set(Some("提取的纯文本...".to_string())),
    word_count: Set(250),
    line_count: Set(40),
    ..Default::default()
};
let song_id = Song::insert(song).exec(&txn).await?.last_insert_id;

// 插入艺术家关联
SongArtist::insert_many(vec![
    song_artist::ActiveModel {
        song_id: Set(song_id),
        artist_id: Set(artist_id),
        ..Default::default()
    }
]).exec(&txn).await?;

// 插入平台映射
PlatformMapping::insert_many(vec![
    platform_mapping::ActiveModel {
        song_id: Set(song_id),
        platform: Set("ncm".to_string()),
        platform_id: Set("3370944459".to_string()),
        ..Default::default()
    }
]).exec(&txn).await?;

txn.commit().await?;
```

#### 拼音提取

```rust
use pinyin::{Pinyin, Style};

fn extract_pinyin(text: &str) -> Vec<String> {
    let pinyin = Pinyin::new();
    text.chars()
        .filter(|c| c.is_chinese())
        .map(|c| {
            pinyin.convert(c, Style::Normal)
                .unwrap_or_default()
                .to_string()
        })
        .collect()
}

// 提取 "普阿山" → ["pu", "a", "shan"]
// 提取 "宜" → ["yi"]
```

#### MeiliSearch 批量索引

```rust
use meilisearch_sdk::client::Client;

let client = Client::new(meili_url, Some(master_key));
let index = client.index("songs");

// 攒批
let mut batch: Vec<Document> = Vec::with_capacity(100);

for song in songs {
    batch.push(Document {
        id: song.id.to_string(),
        music_names: song.music_name,
        music_names_pinyin: extract_pinyin(&song.music_name.join("")),
        artists: song.artists,
        artists_pinyin: extract_pinyin(&song.artists.join("")),
        albums: song.albums,
        albums_pinyin: extract_pinyin(&song.albums.join("")),
        lyric_text: song.lyric_text,
        platform_ids_ncm: song.platform_ids.get("ncm").cloned(),
        platform_ids_qq: song.platform_ids.get("qq").cloned(),
        platform_ids_spotify: song.platform_ids.get("spotify").cloned(),
        platform_ids_apple: song.platform_ids.get("apple").cloned(),
        raw_lyric_file: song.raw_lyric_file,
        word_count: song.word_count,
        line_count: song.line_count,
    });

    if batch.len() >= 100 {
        index.add_documents(&batch, Some("id")).await?;
        batch.clear();
    }
}

// 剩余不足 100 条的
if !batch.is_empty() {
    index.add_documents(&batch, Some("id")).await?;
}
```

---

## 7. 同步机制详细设计

### 7.1 核心原则
- **无本地 Git 仓库**：不执行 `git clone`，通过 HTTP 直接下载所需文件
- **Commit Hash 对比**：以 GitHub 最新 commit hash 作为版本标识，一致则跳过全部下载
- **串行同步**：全局只有一个同步任务执行，通过 RabbitMQ 队列 + Redis 分布式锁实现
- **幂等消费**：同一任务重复执行不会产生脏数据
- **进度持久化**：同步进度实时写入 PostgreSQL，服务重启不丢失

### 7.2 触发方式
1. **API 手动触发**：Go 端 `POST /api/v1/sync` → 入队
2. **GitHub Action 自动触发**：仓库更新后调用 Go API → 入队
3. **定时 Cron 兜底**：Go 端每 5~15 分钟检查一次（防止消息丢失）

### 7.3 同步流程详细步骤

```
[触发同步]
    │
    ▼
[1. Go 服务获取远程 Commit Hash]
    主路径：GET https://api.github.com/repos/amll-dev/amll-ttml-db/commits/main
    备用路径：git ls-remote https://github.com/amll-dev/amll-ttml-db.git HEAD
    │
    ▼
[2. Go 服务对比本地 Commit Hash]
    查 sync_state 表 key='last_synced_commit'
    │
    ├─ 一致 ──→ 返回 {status: "up_to_date"}
    │
    └─ 不一致 ──→ 入队，返回 {status: "syncing" 或 "queued"}
    │
    ▼
[3. Rust Worker 消费队列]
    │
    ▼
[4. Rust Worker 获取 Redis 锁]
    SET sync_lock {request_id} NX EX 3600
    │
    ├─ 获取失败 ──→ 消息 NACK + requeue（延迟 5 秒）
    │
    └─ 获取成功 ──→ 继续
    │
    ▼
[5. Rust Worker 创建同步历史记录]
    插入 sync_history，status='running'
    插入 sync_progress，total=0, downloaded=0, failed=0
    │
    ▼
[6. Rust Worker 下载 raw-lyrics-index.jsonl]
    URL: https://raw.githubusercontent.com/amll-dev/amll-ttml-db/main/raw-lyrics-index.jsonl
    │
    ▼
[7. Rust Worker 解析与对比]
    逐行解析 JSON Lines 格式
    以 rawLyricFile 为唯一标识，与本地 songs 表对比：
    - 远程有，本地无 → 新增
    - 远程有，本地有但 metadata 不同 → 更新
    - 远程无，本地有 → 删除（预留逻辑，当前 CC0 仓库不主动删除）
    │
    ▼
[8. Rust Worker 更新 sync_progress.total]
    │
    ▼
[9. Rust Worker 并发下载文件到 MinIO]
    Tokio Semaphore 控制 20 并发
    每完成一个文件，更新 sync_progress：
      downloaded += 1
      current_file = 当前文件名
    │
    ▼
[10. Rust Worker 解析 TTML 提取纯文本 + 拼音]
    quick-xml 解析 TTML
    提取 <span>, <p> 标签纯文本
    pinyin crate 提取中文拼音
    统计字数、行数
    │
    ▼
[11. Rust Worker 写入 PostgreSQL（事务）]
    SeaORM 事务：songs + artists + song_artists + platform_mappings
    │
    ▼
[12. Rust Worker 更新 MeiliSearch 索引]
    批量写入（每 100 条），包含拼音字段
    │
    ▼
[13. Rust Worker 更新同步状态]
    更新 sync_state：last_synced_commit, last_synced_at
    更新 sync_history：status='success', completed_at=NOW()
    更新 sync_progress：最终状态
    │
    ▼
[14. Rust Worker 释放锁]
    DEL sync_lock
    │
    ▼
[15. Rust Worker 检查队列]
    若 sync_queue 还有消息，继续消费下一条
    下一条消息会再次获取最新 commit hash，若已一致则秒级跳过
```

### 7.4 并发控制详细说明

**RabbitMQ 配置：**
- Exchange: `ttml.sync` (type: direct)
- Queue: `sync_queue` (durable: true)
- Routing Key: `sync.request`
- 消息持久化：delivery_mode = 2

**Redis 锁：**
- Key: `sync_lock`
- Value: 当前 request_id
- 过期时间：3600 秒（1 小时，防止死锁）
- 获取失败策略：消息 NACK + requeue，延迟 5 秒后重新投递

**队列堆积处理：**
- 假设队列中有 10 条消息，当前执行到第 1 条时 GitHub 从 A→C
- 第 1 条执行同步到 C
- 第 2~10 条获取锁后，发现本地 commit 已经是 C，与远程一致，直接返回 up_to_date，秒级消费

### 7.5 错误处理与死信队列

**RabbitMQ DLQ 配置：**
- Queue: `sync_queue.dlq` (durable: true)
- 触发条件：
  - 文件下载后大小为 0
  - 网络超时且重试 3 次失败
  - JSON 解析失败（单条记录跳过，不影响整体）
  - XML 解析失败（单条记录跳过，不影响整体）

**消息体（含错误信息）：**
```json
{
  "request_id": "uuid",
  "failed_at": "2026-06-29T01:00:00Z",
  "error_type": "download_failed",
  "error_message": "ncm-lyrics/3370944459.ttml 下载超时",
  "raw_lyric_file": "1778433565542-115442729-B1QRSIWy.ttml"
}
```

---

## 8. MinIO 文件存储设计

### 8.1 Bucket 结构

```
ttml-db/
├── raw-lyrics/
│   └── {timestamp}-{github_id}-{random}.ttml
├── ncm-lyrics/
│   └── {ncmMusicId}.ttml
├── qq-lyrics/
│   └── {qqMusicId}.ttml
├── spotify-lyrics/
│   └── {spotifyId}.ttml
└── am-lyrics/
    └── {appleMusicId}.ttml
```

### 8.2 存储规则
- 原始 TTML 文件**不做任何修改**，原样上传（Rust Worker）
- 不压缩
- Content-Type 设置为 `application/xml; charset=utf-8`

### 8.3 歌词获取 API 的数据流（Go 端）

```
用户请求：GET /api/v1/ncm-lyrics/3370944459.ttml
    │
    ▼
Gin 解析路径参数：folder=ncm-lyrics, filename=3370944459.ttml
    │
    ▼
解析 Range 请求头（可选）：
  Range: bytes=0-1024 → 只返回前 1025 字节
    │
    ▼
查 Redis 缓存：platform_id → minio_path（可选，缓存未命中则查 PG）
    │
    ▼
查 PostgreSQL：platform_mappings 表获取 song_id，再查 songs 表获取 minio_path
    │
    ▼
MinIO GetObject：
  无 Range：直接获取完整对象流
  有 Range：获取指定范围的对象流
    │
    ▼
io.Copy(responseWriter, minioObject)
    │
    ▼
设置响应头：
  Content-Type: application/xml; charset=utf-8
  Cache-Control: public, max-age=31536000, immutable
  有 Range 时：Content-Range: bytes 0-1024/2847
               HTTP 206 Partial Content
  无 Range 时：HTTP 200 OK
```

**关键要求：不做任何 JSON 包装，响应体就是纯文件内容。**

---

## 9. API 接口规范（Go 端）

### 9.1 通用规范

**请求追踪：**
- 每个请求必须生成 `X-Request-ID`（UUID v4），贯穿所有下游调用
- 日志中必须包含该字段

**响应格式：**
- 歌词获取 API (`/api/v1/{folder}/{filename}`)：**直接返回文件内容**，非 JSON
- 其他 API：统一 JSON 格式

```json
{
  "code": 200,
  "message": "success",
  "data": {}
}
```

### 9.2 同步触发 API

```
POST /api/v1/sync
```

**响应（已为最新）：**
```json
{
  "code": 200,
  "status": "up_to_date",
  "message": "当前已为最新版本",
  "last_synced_commit": "abc123...",
  "last_synced_at": "2026-06-29T01:00:00Z"
}
```

**响应（开始同步）：**
```json
{
  "code": 200,
  "status": "syncing",
  "message": "同步任务已开始",
  "request_id": "uuid-1",
  "previous_commit": "abc123...",
  "target_commit": "def456...",
  "started_at": "2026-06-29T01:05:00Z"
}
```

**响应（排队中）：**
```json
{
  "code": 200,
  "status": "queued",
  "message": "当前有同步任务进行中，已加入队列等待",
  "request_id": "uuid-2",
  "queue_position": 1,
  "current_sync_request_id": "uuid-1",
  "current_sync_started_at": "2026-06-29T01:05:00Z"
}
```

### 9.3 同步状态查询 API

```
GET /api/v1/sync/status
```

**响应（同步中）：**
```json
{
  "code": 200,
  "syncing": true,
  "started_at": "2026-06-29T01:20:00Z",
  "progress": {
    "total": 1500,
    "downloaded": 300,
    "failed": 2,
    "current_file": "ncm-lyrics/3370944459.ttml"
  }
}
```

**响应（空闲）：**
```json
{
  "code": 200,
  "syncing": false,
  "last_synced_at": "2026-06-29T01:00:00Z",
  "last_synced_commit": "abc123..."
}
```

**前端展示：** 当 `syncing=true` 时，页面显示"🔄 词库同步中"标签，不影响搜索和歌词获取功能。

### 9.4 歌词获取 API

```
GET /api/v1/{folder}/{filename}
```

**路径参数：**
- `folder`：必填，取值 `raw-lyrics`, `ncm-lyrics`, `qq-lyrics`, `spotify-lyrics`, `am-lyrics`
- `filename`：必填，对应文件夹下的文件名

**请求头（可选）：**
```
Range: bytes={start}-{end}
```

**响应：**
- 成功（无 Range）：HTTP 200，直接返回 TTML 文件原始字节流
  - Content-Type: `application/xml; charset=utf-8`
  - Cache-Control: `public, max-age=31536000, immutable`
- 成功（有 Range）：HTTP 206，返回指定范围的字节流
  - Content-Range: `bytes {start}-{end}/{total}`
- 失败：HTTP 404，空响应体或默认 404 页面

**示例：**
```
GET /api/v1/ncm-lyrics/3370944459.ttml
GET /api/v1/ncm-lyrics/3370944459.ttml
Range: bytes=0-1024
GET /api/v1/raw-lyrics/1778433565542-115442729-B1QRSIWy.ttml
```

### 9.5 搜索 API

```
GET /api/v1/search?q={keyword}&field={all|song|artist|album|id|lyric}&limit=20&offset=0
```

**参数说明：**
- `q`：搜索关键词，必填
- `field`：搜索范围，选填，默认 `all`
  - `all`：搜索所有字段（musicNames, artists, albums, platformIds, lyricText, 及拼音字段）
  - `song`：仅搜索歌曲名（musicNames + musicNamesPinyin）
  - `artist`：仅搜索艺术家（artists + artistsPinyin）
  - `album`：仅搜索专辑名（albums + albumsPinyin）
  - `id`：仅搜索音乐平台 ID（精确匹配）
  - `lyric`：仅搜索歌词内容（lyricText）
- `limit`：返回数量，默认 20，最大 100
- `offset`：分页偏移，默认 0

**搜索实现策略：**
- `all`, `song`, `artist`, `album`, `lyric`：使用 MeiliSearch `attributesToSearchOn` 限定搜索字段
  - `song`/`artist`/`album` 同时搜索原文和拼音字段
- `id`：使用 MeiliSearch `filter` 精确匹配所有平台 ID 字段，不使用全文搜索

**响应：**
```json
{
  "code": 200,
  "data": {
    "hits": [
      {
        "id": "song_123",
        "musicNames": ["宜", "Yi"],
        "artists": ["普阿山", "HeartStrings"],
        "albums": ["宜"],
        "platformIds": {
          "ncm": "3370944459",
          "qq": "657428009",
          "spotify": "5z3b2aIcHmgcN0Ppx1U7oy",
          "apple": "6763001274"
        },
        "rawLyricFile": "1778433565542-115442729-B1QRSIWy.ttml",
        "versions": [
          {
            "rawLyricFile": "1778433565542-115442729-B1QRSIWy.ttml",
            "timestamp": 1778433565542
          }
        ]
      }
    ],
    "totalHits": 1,
    "limit": 20,
    "offset": 0,
    "processingTimeMs": 12
  }
}
```

**重复版本处理：**
- 同一平台 ID 可能对应多个 `rawLyricFile` 版本
- 默认返回最新版本（按 `rawLyricFile` 中的时间戳降序）
- `versions` 字段列出所有可用版本，供前端选择

### 9.6 批量查询 API

```
POST /api/v1/batch
Content-Type: application/json
```

**请求体：**
```json
{
  "platform": "ncm",
  "ids": ["3370944459", "657428009", "5z3b2aIcHmgcN0Ppx1U7oy"]
}
```

**响应：**
```json
{
  "code": 200,
  "data": [
    {
      "id": "song_123",
      "musicNames": ["宜", "Yi"],
      "artists": ["普阿山", "HeartStrings"],
      "albums": ["宜"],
      "platformIds": {
        "ncm": "3370944459",
        "qq": "657428009",
        "spotify": "5z3b2aIcHmgcN0Ppx1U7oy",
        "apple": "6763001274"
      },
      "rawLyricFile": "1778433565542-115442729-B1QRSIWy.ttml",
      "minioPath": "raw-lyrics/1778433565542-115442729-B1QRSIWy.ttml"
    }
  ]
}
```

### 9.7 词库统计 API

```
GET /api/v1/stats
```

**响应：**
```json
{
  "code": 200,
  "data": {
    "totalSongs": 15000,
    "totalArtists": 3200,
    "totalAlbums": 2800,
    "totalWords": 4500000,
    "totalLines": 180000,
    "platformDistribution": {
      "ncm": 12000,
      "qq": 11000,
      "spotify": 9000,
      "apple": 8500
    },
    "lastSyncAt": "2026-06-29T01:00:00Z"
  }
}
```

---

## 10. MeiliSearch 索引设计

### 10.1 文档结构

```json
{
  "id": "song_123",
  "musicNames": ["宜", "Yi"],
  "musicNamesPinyin": ["yi"],
  "artists": ["普阿山", "HeartStrings"],
  "artistsPinyin": ["pu a shan"],
  "albums": ["宜"],
  "albumsPinyin": ["yi"],
  "lyricText": "提取出的纯歌词文本，用于全文搜索...",
  "platformIds_ncm": "3370944459",
  "platformIds_qq": "657428009",
  "platformIds_spotify": "5z3b2aIcHmgcN0Ppx1U7oy",
  "platformIds_apple": "6763001274",
  "rawLyricFile": "1778433565542-115442729-B1QRSIWy.ttml",
  "ttmlAuthorGithub": "115442729",
  "wordCount": 250,
  "lineCount": 40
}
```

### 10.2 索引设置

```json
{
  "searchableAttributes": [
    "musicNames",
    "musicNamesPinyin",
    "artists",
    "artistsPinyin",
    "albums",
    "albumsPinyin",
    "lyricText",
    "platformIds_ncm",
    "platformIds_qq",
    "platformIds_spotify",
    "platformIds_apple"
  ],
  "filterableAttributes": [
    "platformIds_ncm",
    "platformIds_qq",
    "platformIds_spotify",
    "platformIds_apple",
    "artists",
    "albums",
    "ttmlAuthorGithub"
  ],
  "rankingRules": [
    "words",
    "typo",
    "proximity",
    "attribute",
    "sort",
    "exactness"
  ],
  "typoTolerance": {
    "enabled": true,
    "minWordSizeForTypos": {
      "oneTypo": 5,
      "twoTypos": 9
    }
  }
}
```

### 10.3 拼音搜索说明

- 拼音字段只在 `searchableAttributes` 中，不在 `filterableAttributes` 中
- 用户搜 "yishan" 时，MeiliSearch 会在 `musicNamesPinyin` 和 `artistsPinyin` 中匹配
- 拼音使用全小写、无音调格式，空格分词（如 "pu a shan"）
- 拼音字段的权重低于原文字段，确保搜 "宜" 时原文匹配排在拼音匹配前面

### 10.4 批量索引策略
- Rust Worker 同步时内存中攒批，每 100 条或每 5 秒调用一次 `addDocumentsInBatches(100)`
- 删除操作也批量处理

---

## 11. 工程规范与优化要求

### 11.1 数据库迁移
- 使用 **golang-migrate** 管理所有 Schema 变更（Go 端执行）
- 迁移文件命名：`{序号}_{描述}.up.sql` / `{序号}_{描述}.down.sql`
- **生产环境禁止使用 GORM AutoMigrate**
- Rust 端 SeaORM 模型与 Go 端 GORM 模型需手动同步，以 `migrations/` 目录的 SQL 为准

### 11.2 全链路请求追踪
- 每个 HTTP 请求生成 `X-Request-ID`（UUID v4）
- Go Gin 中间件注入到 Context
- RabbitMQ 消息头携带该 ID（Go 发送 → Rust 消费）
- Go GORM 日志、MinIO 操作、MeiliSearch 请求均记录该 ID
- Rust tracing 日志也记录该 ID
- 日志格式统一为 JSON，包含 `request_id`, `timestamp`, `level`, `message`, `error`

### 11.3 优雅关闭

**Go 服务：**
- 监听 `SIGTERM` 和 `SIGINT`
- 收到信号后：
  1. 停止接收新的 HTTP 请求（`http.Server.Shutdown`）
  2. 等待最长 30 秒后退出

**Rust Worker：**
- 监听 `SIGTERM` 和 `SIGINT`
- 收到信号后：
  1. 停止消费 RabbitMQ 新消息
  2. 把当前正在处理的消息做完（包括文件下载、数据库写入、索引更新）
  3. 释放 Redis 锁（如果持有）
  4. 等待最长 30 秒后退出

### 11.4 缓存预热
- Rust Worker 同步完成后，将高频查询结果预加载到 Redis：
  - 平台 ID → MinIO 路径映射（TTL 1 小时）
  - 搜索首页结果（TTL 10 分钟）
- Go 端使用 Redis Pipeline 批量读取

### 11.5 代码结构建议

**Go 服务 (`/ttml-api-go`)：**
```
/ttml-api-go
├── cmd/
│   └── api/
│       └── main.go
├── internal/
│   ├── config/
│   ├── handler/
│   │   ├── sync.go
│   │   ├── lyrics.go
│   │   ├── search.go
│   │   ├── batch.go
│   │   └── stats.go
│   ├── service/
│   ├── repository/
│   ├── model/
│   ├── infrastructure/
│   │   ├── postgres.go
│   │   ├── redis.go
│   │   ├── minio.go
│   │   ├── rabbitmq.go
│   │   └── meilisearch.go
│   ├── middleware/
│   │   ├── request_id.go
│   │   ├── logger.go
│   │   ├── recovery.go
│   │   └── range.go
│   └── pkg/
│       ├── validator.go
│       └── response.go
├── migrations/
├── docker-compose.yml
├── Dockerfile
└── README.md
```

**Rust Worker (`/ttml-worker-rust`)：**
```
/ttml-worker-rust
├── Cargo.toml
├── src/
│   ├── main.rs
│   ├── config.rs
│   ├── app.rs
│   ├── worker/
│   │   ├── mod.rs
│   │   ├── consumer.rs
│   │   ├── sync_task.rs
│   │   └── progress.rs
│   ├── sync/
│   │   ├── mod.rs
│   │   ├── github.rs
│   │   ├── index_parser.rs
│   │   ├── diff.rs
│   │   ├── downloader.rs
│   │   └── ttml_parser.rs
│   ├── db/
│   │   ├── mod.rs
│   │   ├── models.rs
│   │   └── repository.rs
│   ├── search/
│   │   └── meilisearch.rs
│   ├── storage/
│   │   ├── minio.rs
│   │   └── redis.rs
│   └── infra/
│       ├── postgres.rs
│       ├── rabbitmq.rs
│       └── meilisearch_client.rs
├── migrations/
├── Dockerfile
└── README.md
```

---

## 12. 部署说明

### 12.1 依赖服务启动顺序
1. PostgreSQL
2. Redis
3. MinIO
4. RabbitMQ
5. MeiliSearch
6. Rust Worker（先启动，准备消费队列）
7. Go API（后启动，开始接收 HTTP 请求）

### 12.2 环境变量（共用）

```env
# HTTP
PORT=8080

# PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_USER=ttml
DB_PASSWORD=xxx
DB_NAME=ttml_db
DB_SSLMODE=disable

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# MinIO
MINIO_ENDPOINT=localhost:9000
MINIO_ACCESS_KEY=xxx
MINIO_SECRET_KEY=xxx
MINIO_BUCKET=ttml-db
MINIO_USE_SSL=false

# RabbitMQ
RABBITMQ_URL=amqp://guest:guest@localhost:5672/
RABBITMQ_QUEUE=sync_queue

# MeiliSearch
MEILISEARCH_HOST=http://localhost:7700
MEILISEARCH_API_KEY=xxx
MEILISEARCH_INDEX=songs

# GitHub
GITHUB_TOKEN=                    # 可选，用于提高 API 限流额度
```

### 12.3 首次部署初始化
1. 启动所有依赖服务（Docker Compose）
2. 执行数据库迁移：`make migrate-up`（Go 端）
3. 初始化 MeiliSearch 索引设置（含拼音字段）
4. 启动 Rust Worker（等待队列消息）
5. 启动 Go API 服务
6. 触发首次同步：`POST /api/v1/sync`
7. 首次同步为全量同步，下载所有文件并建立索引

---

## 13. 暂不实现项（预留说明）

| 功能 | 状态 | 说明 |
|------|------|------|
| Casdoor 认证 | **暂不实现** | 架构层预留 JWT Middleware 接口，第一阶段所有 API 公开访问。后续接入时只需实现中间件并保护敏感接口（如手动触发同步） |
| 用户系统 | **暂不实现** | 无用户表、无收藏、无歌单功能 |
| 细粒度限流 | **暂不实现** | 全局基础限流即可 |
| 歌词文件压缩 | **暂不实现** | 按项目要求，MinIO 原样存储 |
| 软删除 | **暂不实现** | 仓库 CC0 协议，不主动删除。但数据库表已预留 `is_deleted` 字段 |
| 302 重定向 | **暂不实现** | 按项目要求，直接流式返回文件内容 |
| Swagger 文档 | **暂不实现** | 后期按需补充 |
| 健康检查端点 | **暂不实现** | 后期按需补充 |
| Prometheus 监控 | **暂不实现** | 后期流量大了再补 |
| MinIO 版本控制 | **暂不实现** | 当前仓库更新频率不高 |
| 冷热数据分离 | **暂不实现** | 当前数据量不需要 |
| API 审计表 | **暂不实现** | 前期用结构化日志替代 |
| 多环境配置 | **暂不实现** | 后期需要时再拆分 dev/staging/prod |
| 首次冷启动优化 | **暂不实现** | 全量同步即可 |
| XML 头校验 | **暂不实现** | GitHub Action 已做审核 |

---

## 14. 关键设计决策回顾

| 决策 | 选择 | 原因 |
|------|------|------|
| 本地 Git 仓库 | 无 | 只需要歌词文件，无需完整 Git 历史 |
| 歌词文件存储 | MinIO | 对象存储适合静态文件，PG 不存原始 XML |
| 同步并发 | 串行 | RabbitMQ 队列 + Redis 锁，避免并发冲突和重复下载 |
| 歌词获取返回 | 原始文件 | 用户要求直接返回 TTML，不做任何 JSON 包装 |
| 搜索 ID 模式 | Filter 精确匹配 | 平台 ID 是标识符，不应模糊搜索 |
| 重复版本 | 时间戳排序 | 简单可靠，无需信誉度系统 |
| 认证 | 暂不实现 | 第一阶段公开访问，预留接口 |
| 同步进度 | PostgreSQL 持久化 | 服务重启不丢失，前端直接查询 |
| Range 请求 | 支持 | 支持断点续传和歌词预览 |
| 拼音搜索 | MeiliSearch 索引 | 中文搜索体验优化 |
| **Go 负责 API** | **HTTP 服务** | 开发效率高，HTTP 生态成熟 |
| **Rust 负责 Worker** | **同步任务** | 内存安全、长时间运行稳定、高并发 IO 性能极致 |
| **语言间通信** | **RabbitMQ + 共享存储** | 天然解耦，无需 gRPC/HTTP 内部调用 |

---

*本指令为项目技术基线，开发过程中如有设计变更需同步更新。*

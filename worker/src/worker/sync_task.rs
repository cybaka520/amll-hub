use std::sync::Arc;

use anyhow::Result;
use tracing::{error, info, warn};

use crate::app::AppState;
use crate::db::repository::{Repository, SongUpsert};
use crate::db::types::SyncSummary;
use crate::search::meilisearch::{self, MeiliDocument};
use crate::storage::redis::SyncLock;
use crate::sync::{
    diff::{self, Diff},
    downloader, github, index_parser, ttml_parser,
};

/// 同步任务主流程
pub struct SyncTaskRunner {
    app: Arc<AppState>,
}

impl SyncTaskRunner {
    pub fn new(app: Arc<AppState>) -> Self {
        Self { app }
    }

    /// 执行同步
    /// 返回 true 表示跳过（本地已最新），false 表示执行了同步
    pub async fn run(
        &self,
        request_id: &str,
        triggered_by: &str,
        _payload: &serde_json::Value,
    ) -> Result<bool> {
        let repo = Repository::new(self.app.db.clone());

        // 1. 获取远程 commit
        let http = reqwest::Client::new();
        let remote_commit = github::fetch_latest_commit(&http, &self.app.cfg.github).await?;
        let local_state = repo.get_sync_state_all().await?;

        // 2. 与本地对比
        if !local_state.last_synced_commit.is_empty()
            && local_state.last_synced_commit == remote_commit
        {
            return Ok(true);
        }

        // 3. 获取 Redis 锁
        let mut lock = SyncLock::new(
            self.app.redis.clone(),
            "sync_lock",
            request_id,
            self.app.cfg.worker.sync_lock_ttl,
        );
        if !lock.try_acquire().await? {
            warn!(request_id, "lock busy, will be retried by rabbitmq requeue");
            anyhow::bail!("sync lock busy");
        }

        // 4. 创建同步历史
        let history_id = repo
            .create_sync_history(
                &remote_commit,
                if local_state.last_synced_commit.is_empty() {
                    None
                } else {
                    Some(&local_state.last_synced_commit)
                },
                triggered_by,
            )
            .await?;

        // 执行同步主流程
        let result = self
            .execute_sync(&http, &repo, history_id, &remote_commit)
            .await;

        match result {
            Ok(summary) => {
                // 更新 sync_state
                repo.set_sync_state("last_synced_commit", &remote_commit)
                    .await?;
                let now = chrono::Utc::now().to_rfc3339();
                repo.set_sync_state("last_synced_at", &now).await?;
                repo.finish_sync_history(history_id, &summary, None).await?;
                info!(request_id, history_id, "sync finished");
                let _ = lock.release().await;
                Ok(false)
            }
            Err(e) => {
                error!(request_id, history_id, error = %e, "sync failed");
                let msg = e.to_string();
                repo.finish_sync_history(history_id, &SyncSummary::default(), Some(&msg))
                    .await?;
                let _ = lock.release().await;
                Err(e)
            }
        }
    }

    /// 实际同步流程：下载索引 -> diff -> 下载文件 -> 入库 -> 索引
    async fn execute_sync(
        &self,
        http: &reqwest::Client,
        repo: &Repository,
        history_id: i64,
        _target_commit: &str,
    ) -> Result<SyncSummary> {
        // 1. 下载 raw-lyrics-index.jsonl
        let index_url = self.app.cfg.github.raw_url("raw-lyrics-index.jsonl");
        let text = github::download_raw_text(http, &index_url).await?;
        let entries = index_parser::parse_index(&text)?;

        // 2. 本地已有 raw 列表
        let local = repo.list_raw_lyric_files().await?;

        // 3. 计算差异
        let Diff {
            to_add,
            to_update,
            to_delete,
        } = diff::compute_diff(entries, local);

        let total = to_add.len() + to_update.len();
        let progress_id = repo.create_sync_progress(history_id, total as i32).await?;

        // 4. 合并待处理列表（新增 + 更新），先处理新增
        let mut all: Vec<_> = to_add.into_iter().chain(to_update).collect();
        let added_count_hint = all.len() - to_delete.len().min(all.len());

        // 进度状态
        let progress_state = Arc::new(tokio::sync::Mutex::new(ProgressState {
            downloaded: 0,
            failed: 0,
        }));
        let repo_arc = repo_db_arc(self);
        let progress_id_inner = progress_id;

        // 5. 并发下载与上传
        let downloaded = downloader::download_and_upload_all(
            self.app.clone(),
            std::mem::take(&mut all),
            {
                let ps = progress_state.clone();
                let repo_arc = repo_arc.clone();
                move |cur, total, file| {
                    let repo = repo_arc.clone();
                    let ps = ps.clone();
                    let pid = progress_id_inner;
                    let file = file.to_string();
                    tokio::spawn(async move {
                        let mut p = ps.lock().await;
                        p.downloaded += 1;
                        let _ = repo
                            .update_sync_progress(pid, p.downloaded, p.failed, Some(&file))
                            .await;
                        tracing::debug!(cur, total, "file processed");
                    });
                }
            },
            {
                let ps = progress_state.clone();
                let repo_arc = repo_arc.clone();
                move |entry, err| {
                    let ps = ps.clone();
                    let pid = progress_id_inner;
                    let repo = repo_arc.clone();
                    let file = entry.raw_file().unwrap_or("").to_string();
                    tokio::spawn(async move {
                        let mut p = ps.lock().await;
                        p.failed += 1;
                        let _ = repo
                            .update_sync_progress(pid, p.downloaded, p.failed, Some(&file))
                            .await;
                        warn!(file, error = %err, "download failed");
                    });
                }
            },
        )
        .await;

        // 6. 逐条解析 + 入库 + 累积 MeiliSearch 文档
        let mut meili_docs: Vec<MeiliDocument> = Vec::with_capacity(downloaded.len());
        let mut summary = SyncSummary {
            added: 0,
            updated: 0,
            deleted: 0,
        };

        for d in &downloaded {
            match self.process_one(repo, d, &mut meili_docs).await {
                Ok(true) => summary.added += 1,
                Ok(false) => summary.updated += 1,
                Err(e) => {
                    warn!(file = %d.raw_lyric_file, error = %e, "process failed");
                }
            }
        }

        // 7. 删除：本地有但远程无（CC0 仓库暂不主动删除）
        // summary.deleted = to_delete.len();
        // 预留：未来若开启删除，需要同时删除 MinIO 对象与 MeiliSearch 文档

        // 8. 写入 MeiliSearch
        if !meili_docs.is_empty() {
            meilisearch::add_documents_in_batches(
                &self.app.meili,
                &self.app.cfg.meilisearch.index,
                meili_docs,
                self.app.cfg.worker.batch_size,
            )
            .await?;
        }

        // 9. 缓存预热：平台 ID -> MinioPath
        self.warmup_cache(repo).await;

        let _ = added_count_hint;
        Ok(summary)
    }

    /// 处理单个文件：解析 TTML -> 入库 -> 准备 MeiliSearch 文档
    /// 返回 Ok(true) 表示新增，Ok(false) 表示更新
    async fn process_one(
        &self,
        repo: &Repository,
        d: &downloader::DownloadResult,
        meili_docs: &mut Vec<MeiliDocument>,
    ) -> Result<bool> {
        let parsed = ttml_parser::parse_ttml(&d.bytes)?;
        let entry = &d.entry;

        let music_names = entry.music_names();
        let albums = entry.albums();
        let artists_names: Vec<String> = music_names.to_vec();
        // artists 信息在 index.jsonl 中并不直接提供，简化处理：
        // 若 music_name 中第二项通常是英文名，将其作为 artists 暂存
        let artists: Vec<String> = if music_names.len() > 1 {
            vec![music_names[1].clone()]
        } else {
            Vec::new()
        };

        let music_pinyin = ttml_parser::extract_pinyin_list(&music_names.join(""));
        let artists_pinyin = ttml_parser::extract_pinyin_list(&artists.join(""));
        let albums_pinyin = ttml_parser::extract_pinyin_list(&albums.join(""));

        let platform_mappings = entry.platform_mappings();
        let isrc = entry.isrc.clone();
        let ttml_author_github = entry.ttml_author_github.clone();
        let ttml_author_github_login = entry.ttml_author_github_login.clone();

        // 是否已存在
        let existed = repo.find_song_id_by_raw(&d.raw_lyric_file).await?.is_some();

        let song_id = repo
            .upsert_song(SongUpsert {
                raw_lyric_file: d.raw_lyric_file.clone(),
                minio_path: format!("raw-lyrics/{}", d.raw_lyric_file),
                music_name: music_names.clone(),
                album: albums.clone(),
                isrc,
                lyric_text: Some(parsed.lyric_text.clone()),
                ttml_author_github,
                ttml_author_github_login,
                word_count: parsed.word_count,
                line_count: parsed.line_count,
                artists,
                platform_mappings,
            })
            .await?;

        let pm = &d.entry.platform_mappings();
        meili_docs.push(MeiliDocument {
            id: format!("song_{}", song_id),
            music_names: music_names.clone(),
            music_names_pinyin: music_pinyin,
            artists: artists_names.clone(),
            artists_pinyin,
            albums,
            albums_pinyin,
            lyric_text: parsed.lyric_text,
            platform_ids_ncm: pm.iter().find(|(p, _)| p == "ncm").map(|(_, v)| v.clone()),
            platform_ids_qq: pm.iter().find(|(p, _)| p == "qq").map(|(_, v)| v.clone()),
            platform_ids_spotify: pm
                .iter()
                .find(|(p, _)| p == "spotify")
                .map(|(_, v)| v.clone()),
            platform_ids_apple: pm
                .iter()
                .find(|(p, _)| p == "apple")
                .map(|(_, v)| v.clone()),
            raw_lyric_file: d.raw_lyric_file.clone(),
            ttml_author_github: entry.ttml_author_github.clone(),
            word_count: parsed.word_count as i64,
            line_count: parsed.line_count as i64,
        });

        Ok(!existed)
    }

    /// 缓存预热：扫描 platform_mappings，写入 Redis
    async fn warmup_cache(&self, _repo: &Repository) {
        // 简化实现：当前不做全量预热，避免启动时大量 DB 查询
        // 实际预热可在 Go 端首次访问时 lazy 加载
    }
}

struct ProgressState {
    downloaded: i32,
    failed: i32,
}

/// 在闭包中共享 Repository（线程安全）
fn repo_db_arc(s: &SyncTaskRunner) -> Arc<Repository> {
    Arc::new(Repository::new(s.app.db.clone()))
}

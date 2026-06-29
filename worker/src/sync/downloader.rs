use std::collections::HashMap;
use std::io::Read;
use std::sync::Arc;

use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;
use tokio::sync::Semaphore;

use crate::app::AppState;
use crate::sync::index_parser::IndexEntry;

/// 单个下载任务结果
#[derive(Debug)]
pub struct DownloadResult {
    pub raw_lyric_file: String,
    pub bytes: Vec<u8>,
    pub entry: IndexEntry,
}

/// 并发下载所有文件（逐文件下载），并上传到 MinIO
///
/// 返回成功下载的列表，失败项通过 on_failure 回调上报
pub async fn download_and_upload_all(
    app: Arc<AppState>,
    entries: Vec<IndexEntry>,
    on_progress: impl Fn(usize, usize, &str) + Send + Sync + 'static,
    on_failure: impl Fn(&IndexEntry, anyhow::Error) + Send + Sync + 'static,
) -> Vec<DownloadResult> {
    let total = entries.len();
    let semaphore = Arc::new(Semaphore::new(app.cfg.worker.concurrency));
    let on_progress = Arc::new(on_progress);
    let on_failure = Arc::new(on_failure);
    let http = reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(60))
        .build()
        .expect("build reqwest client");

    let mut handles = Vec::with_capacity(entries.len());
    for (idx, entry) in entries.into_iter().enumerate() {
        let cur = idx + 1;
        let permit = semaphore.clone().acquire_owned().await.unwrap();
        let app = app.clone();
        let http = http.clone();
        let on_progress = on_progress.clone();
        let on_failure = on_failure.clone();
        handles.push(tokio::spawn(async move {
            let _permit = permit;
            let raw = entry.raw_file().unwrap_or("").to_string();
            let url = app.cfg.github.raw_url(&format!("raw-lyrics/{}", raw));

            let result =
                download_one(&http, &app.s3, &app.cfg.minio.bucket, &url, &raw, &entry, &app.cfg.github).await;
            match result {
                Ok(b) => {
                    on_progress(cur, total, &raw);
                    Some(DownloadResult {
                        raw_lyric_file: raw,
                        bytes: b,
                        entry,
                    })
                }
                Err(e) => {
                    on_failure(&entry, e);
                    on_progress(cur, total, &raw);
                    None
                }
            }
        }));
    }

    let mut out = Vec::new();
    for h in handles {
        if let Ok(Some(r)) = h.await {
            out.push(r);
        }
    }
    out
}

/// 首次同步路径：从已下载的 raw-lyrics.zip 字节中按文件名匹配 entries，并上传到 MinIO
///
/// - zip 内部条目按 basename 匹配 IndexEntry.raw_file()
/// - 仅上传 entries 中列出的文件，不会上传 zip 中的额外文件
/// - zip 解压失败直接返回 Err；单个文件上传失败通过 on_failure 上报
pub async fn download_and_upload_from_zip(
    app: Arc<AppState>,
    entries: Vec<IndexEntry>,
    zip_bytes: Vec<u8>,
    on_progress: impl Fn(usize, usize, &str) + Send + Sync + 'static,
    on_failure: impl Fn(&IndexEntry, anyhow::Error) + Send + Sync + 'static,
) -> Result<Vec<DownloadResult>> {
    let total = entries.len();

    // 在阻塞线程中解压并构建 HashMap<basename, bytes>
    let files_map = tokio::task::spawn_blocking(move || -> Result<HashMap<String, Vec<u8>>> {
        let cursor = std::io::Cursor::new(zip_bytes);
        let mut archive = zip::ZipArchive::new(cursor).context("open zip archive")?;
        let mut map = HashMap::new();
        for i in 0..archive.len() {
            let mut file = archive
                .by_index(i)
                .with_context(|| format!("zip entry {}", i))?;
            if file.is_dir() {
                continue;
            }
            let name = file.name().to_string();
            // 取 basename 兼容 zip 内部不同目录结构
            let basename = std::path::Path::new(&name)
                .file_name()
                .and_then(|s| s.to_str())
                .unwrap_or("")
                .to_string();
            if basename.is_empty() {
                continue;
            }
            let mut buf = Vec::with_capacity(file.size() as usize);
            file.read_to_end(&mut buf)
                .with_context(|| format!("read zip entry {}", name))?;
            map.insert(basename, buf);
        }
        Ok(map)
    })
    .await
    .context("zip extract task")??;

    let files_map = Arc::new(files_map);
    let semaphore = Arc::new(Semaphore::new(app.cfg.worker.concurrency));
    let on_progress = Arc::new(on_progress);
    let on_failure = Arc::new(on_failure);

    let mut handles = Vec::with_capacity(entries.len());
    for (idx, entry) in entries.into_iter().enumerate() {
        let cur = idx + 1;
        let permit = semaphore.clone().acquire_owned().await.unwrap();
        let app = app.clone();
        let on_progress = on_progress.clone();
        let on_failure = on_failure.clone();
        let files_map = files_map.clone();
        handles.push(tokio::spawn(async move {
            let _permit = permit;
            let raw = entry.raw_file().unwrap_or("").to_string();

            let bytes = match files_map.get(&raw) {
                Some(b) => b.clone(),
                None => {
                    let err = anyhow::anyhow!("file not found in zip: {}", raw);
                    on_failure(&entry, err);
                    on_progress(cur, total, &raw);
                    return None;
                }
            };

            if bytes.is_empty() {
                let err = anyhow::anyhow!("empty file in zip: {}", raw);
                on_failure(&entry, err);
                on_progress(cur, total, &raw);
                return None;
            }

            match upload_to_minio(&app.s3, &app.cfg.minio.bucket, &raw, &bytes).await {
                Ok(_) => {
                    on_progress(cur, total, &raw);
                    Some(DownloadResult {
                        raw_lyric_file: raw,
                        bytes,
                        entry,
                    })
                }
                Err(e) => {
                    on_failure(&entry, e);
                    on_progress(cur, total, &raw);
                    None
                }
            }
        }));
    }

    let mut out = Vec::new();
    for h in handles {
        if let Ok(Some(r)) = h.await {
            out.push(r);
        }
    }
    Ok(out)
}

async fn download_one(
    http: &reqwest::Client,
    s3: &S3Client,
    bucket: &str,
    url: &str,
    raw: &str,
    _entry: &IndexEntry,
    github_cfg: &crate::config::GitHubConfig,
) -> Result<Vec<u8>> {
    let bytes = crate::sync::github::download_raw_bytes(http, url, github_cfg).await?;

    if bytes.is_empty() {
        anyhow::bail!("downloaded file is empty: {}", raw);
    }

    upload_to_minio(s3, bucket, raw, &bytes).await?;
    Ok(bytes)
}

async fn upload_to_minio(s3: &S3Client, bucket: &str, raw: &str, bytes: &[u8]) -> Result<()> {
    let key = format!("raw-lyrics/{}", raw);
    s3.put_object()
        .bucket(bucket)
        .key(&key)
        .body(bytes::Bytes::from(bytes.to_vec()).into())
        .content_type("application/xml; charset=utf-8")
        .send()
        .await
        .with_context(|| format!("upload to s3: {}", key))?;
    Ok(())
}

use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;
use std::sync::Arc;
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

/// 并发下载所有文件，并上传到 MinIO
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
                download_one(&http, &app.s3, &app.cfg.minio.bucket, &url, &raw, &entry).await;
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

async fn download_one(
    http: &reqwest::Client,
    s3: &S3Client,
    bucket: &str,
    url: &str,
    raw: &str,
    _entry: &IndexEntry,
) -> Result<Vec<u8>> {
    let bytes = crate::sync::github::download_raw_bytes(http, url).await?;

    if bytes.is_empty() {
        anyhow::bail!("downloaded file is empty: {}", raw);
    }

    // 上传到 MinIO，对象 key 为 raw-lyrics/{raw}
    let key = format!("raw-lyrics/{}", raw);
    s3.put_object()
        .bucket(bucket)
        .key(&key)
        .body(bytes::Bytes::from(bytes.clone()).into())
        .content_type("application/xml; charset=utf-8")
        .send()
        .await
        .with_context(|| format!("upload to s3: {}", key))?;

    Ok(bytes)
}

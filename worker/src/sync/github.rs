use anyhow::{Context, Result};
use reqwest::Client;
use serde::Deserialize;

use crate::config::GitHubConfig;

/// GitHub commits API 响应（仅关心的字段）
#[derive(Debug, Deserialize)]
pub struct CommitResponse {
    pub sha: String,
}

/// 获取远程最新 commit hash
pub async fn fetch_latest_commit(client: &Client, cfg: &GitHubConfig) -> Result<String> {
    let mut req = client
        .get(cfg.api_commits_url())
        .header("Accept", "application/vnd.github+json")
        .header("User-Agent", "amll-ttml-worker");
    if !cfg.token.is_empty() {
        req = req.bearer_auth(&cfg.token);
    }
    let resp = req.send().await.context("github commits request")?;
    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("github api status {}: {}", status, body);
    }
    let parsed: CommitResponse = resp.json().await.context("parse github commits response")?;
    if parsed.sha.is_empty() {
        anyhow::bail!("empty commit sha from github");
    }
    Ok(parsed.sha)
}

/// 下载 raw 文件（如 raw-lyrics-index.jsonl）
pub async fn download_raw_text(client: &Client, url: &str) -> Result<String> {
    let resp = client
        .get(url)
        .header("User-Agent", "amll-ttml-worker")
        .send()
        .await
        .with_context(|| format!("download raw: {}", url))?;
    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("download {} status {}: {}", url, status, body);
    }
    let text = resp.text().await.context("read raw text")?;
    Ok(text)
}

/// 下载 TTML 文件原始字节
pub async fn download_raw_bytes(client: &Client, url: &str) -> Result<Vec<u8>> {
    let resp = client
        .get(url)
        .header("User-Agent", "amll-ttml-worker")
        .send()
        .await
        .with_context(|| format!("download bytes: {}", url))?;
    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("download {} status {}: {}", url, status, body);
    }
    let bytes = resp.bytes().await.context("read raw bytes")?;
    Ok(bytes.to_vec())
}

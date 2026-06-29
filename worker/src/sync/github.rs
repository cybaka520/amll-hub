use anyhow::{Context, Result};
use reqwest::Client;
use serde::Deserialize;

use crate::config::GitHubConfig;

/// GitHub commits API 响应（仅关心的字段）
#[derive(Debug, Deserialize)]
pub struct CommitResponse {
    pub sha: String,
}

/// 给请求加上 token（若配置了）
fn with_auth(mut req: reqwest::RequestBuilder, cfg: &GitHubConfig) -> reqwest::RequestBuilder {
    if !cfg.token.is_empty() {
        req = req.bearer_auth(&cfg.token);
    }
    req
}

/// 获取远程最新 commit hash
pub async fn fetch_latest_commit(client: &Client, cfg: &GitHubConfig) -> Result<String> {
    let req = client
        .get(cfg.api_commits_url())
        .header("Accept", "application/vnd.github+json")
        .header("User-Agent", "amll-ttml-worker");
    let req = with_auth(req, cfg);
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

/// 下载 raw 文本（如 raw-lyrics-index.jsonl），带 token
pub async fn download_raw_text(
    client: &Client,
    url: &str,
    cfg: &GitHubConfig,
) -> Result<String> {
    let req = client
        .get(url)
        .header("User-Agent", "amll-ttml-worker");
    let req = with_auth(req, cfg);
    let resp = req
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

/// 下载 TTML 文件原始字节，带 token
pub async fn download_raw_bytes(
    client: &Client,
    url: &str,
    cfg: &GitHubConfig,
) -> Result<Vec<u8>> {
    let req = client
        .get(url)
        .header("User-Agent", "amll-ttml-worker");
    let req = with_auth(req, cfg);
    let resp = req
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

/// 下载首次同步用的整包 zip（raw-lyrics.zip），带 token
pub async fn download_zip(client: &Client, cfg: &GitHubConfig) -> Result<Vec<u8>> {
    let url = cfg.raw_lyrics_zip_url();
    let req = client
        .get(&url)
        .header("User-Agent", "amll-ttml-worker");
    let req = with_auth(req, cfg);
    let resp = req.send().await.context("download zip request")?;
    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().await.unwrap_or_default();
        anyhow::bail!("download zip {} status {}: {}", url, status, body);
    }
    let bytes = resp.bytes().await.context("read zip bytes")?;
    Ok(bytes.to_vec())
}

use anyhow::{Context, Result};
use serde::Deserialize;

/// raw-lyrics-index.jsonl 单行结构
///
/// 实际仓库格式（按 amll-ttml-db README 与样例推断）：
/// {"ncmMusicId":"...","qqMusicId":"...","spotifyId":"...","appleMusicId":"...","rawLyricFile":"...-GithubId-xxx.ttml"}
///
/// 不同条目可能有不同字段，所有字段都设为可选
#[derive(Debug, Clone, Deserialize, Default)]
pub struct IndexEntry {
    #[serde(default)]
    pub raw_lyric_file: Option<String>,
    #[serde(default, alias = "ncmMusicId")]
    pub ncm_music_id: Option<String>,
    #[serde(default, alias = "qqMusicId")]
    pub qq_music_id: Option<String>,
    #[serde(default, alias = "spotifyId")]
    pub spotify_id: Option<String>,
    #[serde(default, alias = "appleMusicId")]
    pub apple_music_id: Option<String>,
    #[serde(default, alias = "musicName")]
    pub music_name: Option<serde_json::Value>,
    #[serde(default, alias = "album")]
    pub album: Option<serde_json::Value>,
    #[serde(default, alias = "isrc")]
    pub isrc: Option<String>,
    #[serde(default, alias = "ttmlAuthorGithub")]
    pub ttml_author_github: Option<String>,
    #[serde(default, alias = "ttmlAuthorGithubLogin")]
    pub ttml_author_github_login: Option<String>,
}

impl IndexEntry {
    /// 提取 rawLyricFile（必须存在）
    pub fn raw_file(&self) -> Option<&str> {
        self.raw_lyric_file.as_deref()
    }

    /// 从 rawLyricFile 文件名中提取 GitHub ID 与时间戳
    /// 文件名格式：{timestamp}-{githubId}-{random}.ttml
    pub fn parse_file_meta(&self) -> Option<(u64, Option<String>)> {
        let raw = self.raw_file()?;
        let stem = raw.strip_suffix(".ttml").unwrap_or(raw);
        let mut parts = stem.splitn(3, '-');
        let ts: u64 = parts.next()?.parse().ok()?;
        let github_id = parts.next().map(|s| s.to_string());
        Some((ts, github_id))
    }

    /// 收集所有非空平台映射 (platform, platform_id)
    pub fn platform_mappings(&self) -> Vec<(String, String)> {
        let mut out = Vec::new();
        if let Some(id) = &self.ncm_music_id {
            if !id.is_empty() {
                out.push(("ncm".to_string(), id.clone()));
            }
        }
        if let Some(id) = &self.qq_music_id {
            if !id.is_empty() {
                out.push(("qq".to_string(), id.clone()));
            }
        }
        if let Some(id) = &self.spotify_id {
            if !id.is_empty() {
                out.push(("spotify".to_string(), id.clone()));
            }
        }
        if let Some(id) = &self.apple_music_id {
            if !id.is_empty() {
                out.push(("apple".to_string(), id.clone()));
            }
        }
        out
    }

    /// 将 music_name 字段规范化为 Vec<String>
    pub fn music_names(&self) -> Vec<String> {
        json_to_string_array(&self.music_name)
    }

    /// 将 album 字段规范化为 Vec<String>
    pub fn albums(&self) -> Vec<String> {
        json_to_string_array(&self.album)
    }
}

fn json_to_string_array(v: &Option<serde_json::Value>) -> Vec<String> {
    match v {
        Some(serde_json::Value::Array(arr)) => arr
            .iter()
            .filter_map(|x| x.as_str().map(|s| s.to_string()))
            .collect(),
        Some(serde_json::Value::String(s)) => vec![s.clone()],
        _ => Vec::new(),
    }
}

/// 解析整份 raw-lyrics-index.jsonl
pub fn parse_index(text: &str) -> Result<Vec<IndexEntry>> {
    let mut entries = Vec::new();
    for (lineno, line) in text.lines().enumerate() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        let entry: IndexEntry = serde_json::from_str(line)
            .with_context(|| format!("parse line {}", lineno + 1))?;
        if entry.raw_file().is_some() {
            entries.push(entry);
        }
    }
    Ok(entries)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_simple_line() {
        let jsonl = r#"{"ncmMusicId":"3370944459","rawLyricFile":"1778433565542-115442729-B1QRSIWy.ttml"}"#;
        let parsed = parse_index(jsonl).unwrap();
        assert_eq!(parsed.len(), 1);
        assert_eq!(
            parsed[0].raw_file(),
            Some("1778433565542-115442729-B1QRSIWy.ttml")
        );
        assert_eq!(parsed[0].ncm_music_id.as_deref(), Some("3370944459"));
        let (ts, gid) = parsed[0].parse_file_meta().unwrap();
        assert_eq!(ts, 1778433565542);
        assert_eq!(gid.as_deref(), Some("115442729"));
        let maps = parsed[0].platform_mappings();
        assert_eq!(maps, vec![("ncm".to_string(), "3370944459".to_string())]);
    }
}

use anyhow::{Context, Result};
use meilisearch_sdk::client::Client;
use meilisearch_sdk::indexes::Index;
use serde_json::Value;

/// 索引文档（与规范中 MeiliSearch 文档结构一致）
pub struct MeiliDocument {
    pub id: String,
    pub music_names: Vec<String>,
    pub music_names_pinyin: Vec<String>,
    pub artists: Vec<String>,
    pub artists_pinyin: Vec<String>,
    pub albums: Vec<String>,
    pub albums_pinyin: Vec<String>,
    pub lyric_text: String,
    pub platform_ids_ncm: Option<String>,
    pub platform_ids_qq: Option<String>,
    pub platform_ids_spotify: Option<String>,
    pub platform_ids_apple: Option<String>,
    pub raw_lyric_file: String,
    pub ttml_author_github: Option<String>,
    pub word_count: i64,
    pub line_count: i64,
}

impl MeiliDocument {
    pub fn to_json(&self) -> Value {
        serde_json::json!({
            "id": self.id,
            "musicNames": self.music_names,
            "musicNamesPinyin": self.music_names_pinyin,
            "artists": self.artists,
            "artistsPinyin": self.artists_pinyin,
            "albums": self.albums,
            "albumsPinyin": self.albums_pinyin,
            "lyricText": self.lyric_text,
            "platformIds_ncm": self.platform_ids_ncm,
            "platformIds_qq": self.platform_ids_qq,
            "platformIds_spotify": self.platform_ids_spotify,
            "platformIds_apple": self.platform_ids_apple,
            "rawLyricFile": self.raw_lyric_file,
            "ttmlAuthorGithub": self.ttml_author_github,
            "wordCount": self.word_count,
            "lineCount": self.line_count,
        })
    }
}

/// 批量更新 MeiliSearch 索引
pub async fn add_documents_in_batches(
    client: &Client,
    index_name: &str,
    documents: Vec<MeiliDocument>,
    batch_size: usize,
) -> Result<()> {
    let index: Index = client.index(index_name);
    let batch_size = if batch_size == 0 { 100 } else { batch_size };

    for chunk in documents.chunks(batch_size) {
        let docs: Vec<Value> = chunk.iter().map(|d| d.to_json()).collect();
        index
            .add_documents(&docs, Some("id"))
            .await
            .context("add_documents")?
            .wait_for_completion(client, None, None)
            .await
            .context("wait add_documents")?;
    }

    Ok(())
}

/// 删除索引文档（按 raw_lyric_file -> id 关联，id 形如 "song_{song_id}"）
pub async fn delete_documents(client: &Client, index_name: &str, ids: &[String]) -> Result<()> {
    if ids.is_empty() {
        return Ok(());
    }
    let index: Index = client.index(index_name);
    let refs: Vec<&str> = ids.iter().map(|s| s.as_str()).collect();
    index
        .delete_documents(&refs)
        .await
        .context("delete_documents")?
        .wait_for_completion(client, None, None)
        .await
        .context("wait delete_documents")?;
    Ok(())
}

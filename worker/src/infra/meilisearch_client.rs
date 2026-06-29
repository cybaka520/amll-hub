use anyhow::{Context, Result};
use meilisearch_sdk::client::Client;
use meilisearch_sdk::indexes::Index;
use meilisearch_sdk::settings::Settings;

use crate::config::Config;

/// 初始化 MeiliSearch 客户端并确保索引存在
pub async fn init_meilisearch(cfg: &Config) -> Result<Client> {
    let client = Client::new(&cfg.meilisearch.host, Some(&cfg.meilisearch.api_key))?;

    // 探活
    client.health().await.context("meilisearch health check")?;

    ensure_index(&client, &cfg.meilisearch.index).await?;

    Ok(client)
}

async fn ensure_index(client: &Client, name: &str) -> Result<()> {
    let index: Index = match client.get_index(name).await {
        Ok(idx) => idx,
        Err(_) => {
            let task = client
                .create_index(name, Some("id"))
                .await
                .context("create meilisearch index")?;
            let completed = task
                .wait_for_completion(client, None, None)
                .await
                .context("wait create index")?;
            completed
                .try_make_index(client)
                .map_err(|t| anyhow::anyhow!("make index failed: {:?}", t))?
        }
    };

    let searchable: Vec<&str> = vec![
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
        "platformIds_apple",
    ];
    let filterable: Vec<&str> = vec![
        "platformIds_ncm",
        "platformIds_qq",
        "platformIds_spotify",
        "platformIds_apple",
        "artists",
        "albums",
        "ttmlAuthorGithub",
    ];

    let settings = Settings::new()
        .with_searchable_attributes(searchable)
        .with_filterable_attributes(filterable);

    let task = index
        .set_settings(&settings)
        .await
        .context("set meilisearch settings")?;
    task.wait_for_completion(client, None, None)
        .await
        .context("wait set settings")?;

    Ok(())
}

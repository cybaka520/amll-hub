use std::sync::Arc;

use aws_sdk_s3::Client as S3Client;
use meilisearch_sdk::client::Client as MeiliClient;
use redis::aio::ConnectionManager;
use sea_orm::DatabaseConnection;

use crate::config::Config;

/// 应用共享状态：所有连接池
#[derive(Clone)]
pub struct AppState {
    pub db: DatabaseConnection,
    pub redis: ConnectionManager,
    pub s3: S3Client,
    pub meili: MeiliClient,
    pub cfg: Arc<Config>,
}

impl AppState {
    pub fn new(
        db: DatabaseConnection,
        redis: ConnectionManager,
        s3: S3Client,
        meili: MeiliClient,
        cfg: Arc<Config>,
    ) -> Self {
        Self { db, redis, s3, meili, cfg }
    }
}

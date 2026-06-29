use anyhow::{Context, Result};
use sea_orm::{ConnectOptions, Database};

use crate::config::Config;

/// 初始化 PostgreSQL 连接池
pub async fn init_postgres(cfg: &Config) -> Result<sea_orm::DatabaseConnection> {
    let mut opt = ConnectOptions::new(cfg.database.dsn());
    opt.max_connections(cfg.database.max_open_conns);
    opt.min_connections(cfg.database.max_idle_conns);
    opt.sqlx_logging(false);
    let db = Database::connect(opt).await.context("connect postgres")?;
    Ok(db)
}

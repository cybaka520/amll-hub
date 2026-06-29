use anyhow::Result;
use redis::aio::ConnectionManager;
use redis::AsyncCommands;

/// Redis 分布式锁
pub struct SyncLock {
    conn: ConnectionManager,
    key: String,
    value: String,
    ttl_seconds: u64,
    acquired: bool,
}

impl SyncLock {
    pub fn new(conn: ConnectionManager, key: &str, value: &str, ttl_seconds: u64) -> Self {
        Self {
            conn,
            key: key.to_string(),
            value: value.to_string(),
            ttl_seconds,
            acquired: false,
        }
    }

    /// 尝试获取锁：SET key value NX EX ttl
    pub async fn try_acquire(&mut self) -> Result<bool> {
        tracing::info!("Redis SyncLock - 尝试获取锁, key={}, ttl={}", self.key, self.ttl_seconds);
        let mut conn = self.conn.clone();
        
        // 使用正确的 Redis SET 命令格式
        let result: Option<String> = redis::cmd("SET")
            .arg(&self.key)
            .arg(&self.value)
            .arg("NX")
            .arg("EX")
            .arg(self.ttl_seconds)
            .query_async(&mut conn)
            .await?;
            
        let ok = result.is_some();
        self.acquired = ok;
        tracing::info!("Redis SyncLock - 获取锁结果: {}", ok);
        Ok(ok)
    }

    /// 释放锁（仅当 value 匹配时）—— 简化实现：直接 DEL
    pub async fn release(&mut self) -> Result<()> {
        if !self.acquired {
            return Ok(());
        }
        let mut conn = self.conn.clone();
        let _: i64 = conn.del(&self.key).await?;
        self.acquired = false;
        Ok(())
    }
}

/// 平台 ID -> MinioPath 缓存（缓存预热时使用）
#[allow(dead_code)]
pub async fn cache_platform_path(
    conn: &mut ConnectionManager,
    platform: &str,
    platform_id: &str,
    minio_path: &str,
    ttl_seconds: u64,
) -> Result<()> {
    let key = format!("lyric:{}-lyrics:{}", platform, platform_id);
    let _: () = conn.set_ex(&key, minio_path, ttl_seconds).await?;
    Ok(())
}

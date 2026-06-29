/// 同步状态聚合
#[derive(Debug, Clone, Default)]
pub struct SyncState {
    pub last_synced_commit: String,
    #[allow(dead_code)]
    pub last_synced_at: String,
}

/// 同步摘要
#[derive(Debug, Clone, Default)]
pub struct SyncSummary {
    pub added: usize,
    pub updated: usize,
    pub deleted: usize,
}

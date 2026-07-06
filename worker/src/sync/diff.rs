use std::collections::HashMap;

use crate::sync::index_parser::IndexEntry;

/// 差异计算结果
#[derive(Debug, Default)]
pub struct Diff {
    pub to_add: Vec<IndexEntry>,
    pub to_update: Vec<IndexEntry>,
    pub to_delete: Vec<String>,
}

/// 计算远程索引与本地已有 raw_lyric_file 的差异
///
/// - 远程有，本地无 -> to_add
/// - 远程有，本地有，但 commit_timestamp 不同 -> to_update（只有时间戳不同才需要更新）
/// - 远程有，本地有，且 commit_timestamp 相同 -> 跳过（文件内容和 metadata 未变化）
/// - 远程无，本地有 -> to_delete（当前 CC0 仓库不主动删除，留空实现预留）
pub fn compute_diff(remote: Vec<IndexEntry>, local: HashMap<String, Option<i64>>) -> Diff {
    let mut to_add = Vec::new();
    let mut to_update = Vec::new();
    let mut remote_seen: std::collections::HashSet<String> = std::collections::HashSet::new();

    for entry in remote {
        if let Some(raw) = entry.raw_file() {
            let raw = raw.to_string();
            remote_seen.insert(raw.clone());

            // 从文件名中提取 commit_timestamp，转换为 i64 以便比较
            let remote_ts = entry.parse_file_meta().map(|(ts, _)| ts as i64);

            match local.get(&raw) {
                Some(local_ts) => {
                    // 本地有此文件，比较 commit_timestamp
                    if *local_ts != remote_ts {
                        // 时间戳不同，需要更新
                        to_update.push(entry);
                    }
                    // 时间戳相同，跳过（不需要更新）
                }
                None => {
                    // 本地没有此文件，需要新增
                    to_add.push(entry);
                }
            }
        }
    }

    let to_delete: Vec<String> = local
        .keys()
        .filter(|k| !remote_seen.contains(*k))
        .cloned()
        .collect();

    Diff {
        to_add,
        to_update,
        to_delete,
    }
}

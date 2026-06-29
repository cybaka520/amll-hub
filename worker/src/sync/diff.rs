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
/// - 远程有，本地有 -> to_update（简化处理：全部重新下载与更新；CC0 仓库以文件名唯一标识，文件名相同即视为同一首歌）
/// - 远程无，本地有 -> to_delete（当前 CC0 仓库不主动删除，留空实现预留）
pub fn compute_diff(remote: Vec<IndexEntry>, local: HashMap<String, i64>) -> Diff {
    let mut to_add = Vec::new();
    let mut to_update = Vec::new();
    let mut remote_seen: std::collections::HashSet<String> = std::collections::HashSet::new();

    for entry in remote {
        if let Some(raw) = entry.raw_file() {
            let raw = raw.to_string();
            remote_seen.insert(raw.clone());
            if local.contains_key(&raw) {
                to_update.push(entry);
            } else {
                to_add.push(entry);
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

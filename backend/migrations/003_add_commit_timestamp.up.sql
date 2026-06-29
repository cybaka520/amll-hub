-- 为 songs 表添加提交时间戳与人类可读时间
-- 文件名格式: [提交 UNIX 毫秒时间戳]-[提交者 Github ID]-[8 位随机 ID].ttml
-- commit_timestamp: 毫秒级 UNIX 时间戳（从文件名解析）
-- commit_time:      转换后的 TIMESTAMPTZ，便于人类阅读与跨时区比较

ALTER TABLE songs
    ADD COLUMN commit_timestamp BIGINT,
    ADD COLUMN commit_time TIMESTAMPTZ;

-- 用于按提交时间排序（新旧版本判定）
CREATE INDEX idx_songs_commit_timestamp ON songs(commit_timestamp DESC);
CREATE INDEX idx_songs_commit_time ON songs(commit_time DESC);

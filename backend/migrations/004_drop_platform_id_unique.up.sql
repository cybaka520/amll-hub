-- 移除 platform_mappings 表的 UNIQUE(platform, platform_id) 约束
-- 目的：允许同一个 platform_id 关联到多个 song（即同一首歌的多个版本文件共存）
-- 按 id 查询时由后端 ORDER BY commit_timestamp DESC 取最新版本
-- 按文件名查询时仍可访问任意历史版本

-- 1. 删除 UNIQUE(platform, platform_id) 约束及其索引
ALTER TABLE platform_mappings
    DROP CONSTRAINT IF EXISTS platform_mappings_platform_platform_id_key;

DROP INDEX IF EXISTS idx_pm_platform_id;
DROP INDEX IF EXISTS platform_mappings_platform_platform_id_key;
DROP INDEX IF EXISTS idx_platform_mappings_platform;

-- 2. 保留 UNIQUE(song_id, platform)：同一首歌同一平台只能有一条映射
--    （此约束不变，由 idx_pm_song_platform 或 platform_mappings_song_id_platform_key 保障）

-- 3. 新建非唯一索引，保障按 (platform, platform_id) 查询性能
CREATE INDEX IF NOT EXISTS idx_pm_platform_id ON platform_mappings(platform, platform_id);

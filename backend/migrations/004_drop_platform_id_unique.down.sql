-- 回滚：恢复 UNIQUE(platform, platform_id) 约束
-- 注意：如果有同一 platform_id 关联多个 song 的数据，恢复约束会失败

DROP INDEX IF EXISTS idx_pm_platform_id;

ALTER TABLE platform_mappings
    ADD CONSTRAINT platform_mappings_platform_platform_id_key
    UNIQUE (platform, platform_id);

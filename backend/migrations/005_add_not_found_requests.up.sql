-- 无歌词记录表（存储歌词不存在的请求，每周清空 category='not_found'）
CREATE TABLE not_found_requests (
    id               BIGSERIAL PRIMARY KEY,
    platform         VARCHAR(20) NOT NULL,
    platform_id      VARCHAR(100) NOT NULL,
    song_name        VARCHAR(255),
    request_count    INT NOT NULL DEFAULT 1,
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    daily_requests   JSONB NOT NULL DEFAULT '{}'::jsonb,
    first_request_ip VARCHAR(50),
    category         VARCHAR(20) NOT NULL DEFAULT 'unknown',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(platform, platform_id)
);

COMMENT ON TABLE not_found_requests IS '无歌词记录表，每周一清空 category=not_found 的记录';
COMMENT ON COLUMN not_found_requests.category IS 'pure_music/cloud_music 白名单保留，not_found 每周清空';

CREATE INDEX idx_not_found_platform_id ON not_found_requests(platform, platform_id);
CREATE INDEX idx_not_found_count ON not_found_requests(request_count DESC);
CREATE INDEX idx_not_found_category ON not_found_requests(category);
CREATE INDEX idx_not_found_last_seen ON not_found_requests(last_seen_at DESC);

-- 纯音乐白名单表（永久保留）
CREATE TABLE pure_music_whitelist (
    id           BIGSERIAL PRIMARY KEY,
    platform     VARCHAR(20) NOT NULL,
    platform_id  VARCHAR(100) NOT NULL,
    song_name    VARCHAR(255),
    reason       VARCHAR(255),
    detected_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    detected_by  VARCHAR(50),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(platform, platform_id)
);

CREATE INDEX idx_pure_music_platform_id ON pure_music_whitelist(platform, platform_id);

-- 云盘音乐白名单表（永久保留）
CREATE TABLE cloud_music_whitelist (
    id           BIGSERIAL PRIMARY KEY,
    platform     VARCHAR(20) NOT NULL,
    platform_id  VARCHAR(100) NOT NULL,
    song_name    VARCHAR(255),
    reason       VARCHAR(255),
    detected_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    detected_by  VARCHAR(50),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(platform, platform_id)
);

CREATE INDEX idx_cloud_music_platform_id ON cloud_music_whitelist(platform, platform_id);

-- updated_at 触发器
CREATE TRIGGER trg_not_found_updated_at
    BEFORE UPDATE ON not_found_requests
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 歌曲主表
CREATE TABLE songs (
    id              BIGSERIAL PRIMARY KEY,
    music_name      JSONB NOT NULL DEFAULT '[]',
    album           JSONB NOT NULL DEFAULT '[]',
    isrc            VARCHAR(20),
    raw_lyric_file  VARCHAR(255) NOT NULL UNIQUE,
    minio_path      VARCHAR(500) NOT NULL,
    lyric_text      TEXT,
    ttml_author_github        VARCHAR(50),
    ttml_author_github_login  VARCHAR(100),
    word_count      INT NOT NULL DEFAULT 0,
    line_count      INT NOT NULL DEFAULT 0,
    is_deleted      BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_songs_music_name ON songs USING GIN(music_name);
CREATE INDEX idx_songs_album ON songs USING GIN(album);
CREATE INDEX idx_songs_raw_lyric_file ON songs(raw_lyric_file);

-- 艺术家表
CREATE TABLE artists (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_artists_name ON artists(name);

-- 歌曲-艺术家关联表
CREATE TABLE song_artists (
    song_id     BIGINT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    artist_id   BIGINT NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
    PRIMARY KEY (song_id, artist_id)
);

-- 平台 ID 映射表
CREATE TABLE platform_mappings (
    id          BIGSERIAL PRIMARY KEY,
    song_id     BIGINT NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    platform    VARCHAR(20) NOT NULL,
    platform_id VARCHAR(100) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(song_id, platform),
    UNIQUE(platform, platform_id)
);

CREATE INDEX idx_platform_mappings_platform ON platform_mappings(platform, platform_id);
CREATE INDEX idx_platform_mappings_song ON platform_mappings(song_id);

-- 同步状态表
CREATE TABLE sync_state (
    key   VARCHAR(50) PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO sync_state (key, value) VALUES ('last_synced_commit', '');
INSERT INTO sync_state (key, value) VALUES ('last_synced_at', '');

-- 同步历史表
CREATE TABLE sync_history (
    id              BIGSERIAL PRIMARY KEY,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    previous_commit VARCHAR(40),
    target_commit   VARCHAR(40) NOT NULL,
    status          VARCHAR(20) NOT NULL,
    added_count     INT NOT NULL DEFAULT 0,
    updated_count   INT NOT NULL DEFAULT 0,
    deleted_count   INT NOT NULL DEFAULT 0,
    error_message   TEXT,
    triggered_by    VARCHAR(20) NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sync_history_time ON sync_history(started_at DESC);

-- updated_at 触发器
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_songs_updated_at
    BEFORE UPDATE ON songs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

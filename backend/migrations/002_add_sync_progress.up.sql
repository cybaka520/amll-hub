-- 同步进度表
CREATE TABLE sync_progress (
    id              BIGSERIAL PRIMARY KEY,
    sync_history_id BIGINT NOT NULL REFERENCES sync_history(id) ON DELETE CASCADE,
    total           INT NOT NULL DEFAULT 0,
    downloaded      INT NOT NULL DEFAULT 0,
    failed          INT NOT NULL DEFAULT 0,
    current_file    VARCHAR(255),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sync_progress_history ON sync_progress(sync_history_id);

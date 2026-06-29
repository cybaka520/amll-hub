DROP INDEX IF EXISTS idx_songs_commit_time;
DROP INDEX IF EXISTS idx_songs_commit_timestamp;

ALTER TABLE songs
    DROP COLUMN IF EXISTS commit_time,
    DROP COLUMN IF EXISTS commit_timestamp;

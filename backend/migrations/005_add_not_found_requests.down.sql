DROP TABLE IF EXISTS cloud_music_whitelist;
DROP TABLE IF EXISTS pure_music_whitelist;
DROP TRIGGER IF EXISTS trg_not_found_updated_at ON not_found_requests;
DROP TABLE IF EXISTS not_found_requests;

-- Migration: Add status_summary fields to endpoints table (PostgreSQL)

ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS status_summary jsonb DEFAULT NULL;
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS last_status_summary_at timestamptz(3) DEFAULT NULL;

-- Rollback:
-- ALTER TABLE endpoints DROP COLUMN IF EXISTS status_summary;
-- ALTER TABLE endpoints DROP COLUMN IF EXISTS last_status_summary_at;

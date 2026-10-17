-- Migration: Add pending-related fields to workers table (PostgreSQL)
-- Requirements: 6.1 - Worker model shall include pending_phase, pending_phase_since, pending_reason, pending_message

ALTER TABLE workers ADD COLUMN IF NOT EXISTS pending_phase varchar(32) DEFAULT NULL;
ALTER TABLE workers ADD COLUMN IF NOT EXISTS pending_phase_since timestamptz(3) DEFAULT NULL;
ALTER TABLE workers ADD COLUMN IF NOT EXISTS pending_reason varchar(255) DEFAULT NULL;
ALTER TABLE workers ADD COLUMN IF NOT EXISTS pending_message text DEFAULT NULL;

CREATE INDEX IF NOT EXISTS idx_workers_pending_phase ON workers (pending_phase);

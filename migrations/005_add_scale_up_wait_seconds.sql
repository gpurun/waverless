-- P0 wait-age: per-endpoint oldest-pending wait threshold (seconds). 0 = use global autoscaler.scale_up_wait_seconds / disabled.

ALTER TABLE autoscaler_configs
  ADD COLUMN IF NOT EXISTS scale_up_wait_seconds int NOT NULL DEFAULT 0;

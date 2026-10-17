-- M2a: multi active slices per endpoint, endpoint_mode, external id, slice deploy keys.

ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS endpoint_mode varchar(32) NOT NULL DEFAULT 'http_service';
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS external_endpoint_id varchar(255) NOT NULL DEFAULT '';

DROP INDEX IF EXISTS uq_endpoint_provider_slices_one_active;

CREATE UNIQUE INDEX IF NOT EXISTS uq_endpoint_provider_slices_active_region
  ON endpoint_provider_slices (endpoint, provider_id, physical_region, provider_sku_id)
  WHERE status = 'active';

ALTER TABLE endpoint_provider_slices ADD COLUMN IF NOT EXISTS deploy_key varchar(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_endpoint_provider_slices_deploy_key
  ON endpoint_provider_slices (deploy_key)
  WHERE deploy_key <> '';

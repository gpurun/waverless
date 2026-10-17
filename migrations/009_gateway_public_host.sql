-- Gateway / http_service: public host and worker backend URLs.

ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS public_host varchar(255) NOT NULL DEFAULT '';
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS gateway_route_version bigint NOT NULL DEFAULT 0;
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS gateway_status varchar(32) NOT NULL DEFAULT '';

ALTER TABLE workers ADD COLUMN IF NOT EXISTS backend_url varchar(512) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_endpoints_public_host
  ON endpoints (public_host)
  WHERE public_host <> '';

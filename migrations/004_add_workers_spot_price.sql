-- Migration: Add spot price tracking fields to workers table (PostgreSQL)

ALTER TABLE workers ADD COLUMN IF NOT EXISTS spot_price numeric(10,6) DEFAULT NULL;
ALTER TABLE workers ADD COLUMN IF NOT EXISTS spot_instance_type varchar(64) DEFAULT NULL;

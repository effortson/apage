-- Daily usage rollups for the trend chart (spec §29 usage timeseries / UI §7.7).
-- The worker upserts today's row as it flushes the Redis usage buffer.
CREATE TABLE usage_daily (
    tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    day           DATE NOT NULL DEFAULT CURRENT_DATE,
    storage_bytes BIGINT NOT NULL DEFAULT 0,
    tunnel_egress BIGINT NOT NULL DEFAULT 0,
    cloud_egress  BIGINT NOT NULL DEFAULT 0,
    conversions   BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (tenant_id, day)
);

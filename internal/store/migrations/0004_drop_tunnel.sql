-- Cloud-only migration: remove tunnel/hybrid mode and the agent tunnel surface.
-- Links are now created only by agents (cloud backing); tunnel file refs, agent
-- connection state, and tunnel egress accounting are dropped.

-- Drop tunnel-backed links (file_id IS NULL) before tightening the constraint.
DELETE FROM preview_links WHERE file_id IS NULL;

-- Tunnel file metadata table.
DROP TABLE IF EXISTS file_refs;

-- preview_links: drop the tunnel backing column. Dropping file_ref also drops
-- the unnamed XOR CHECK that referenced it, so we re-add a cloud-only backing
-- constraint and lock the mode to 'cloud'.
ALTER TABLE preview_links DROP COLUMN file_ref;
ALTER TABLE preview_links ADD CONSTRAINT preview_links_cloud_backing CHECK (file_id IS NOT NULL);
UPDATE preview_links SET mode = 'cloud';
ALTER TABLE preview_links ADD CONSTRAINT preview_links_mode_cloud CHECK (mode = 'cloud');

-- agent_instances: remove the agent tunnel auth + live-connection columns.
ALTER TABLE agent_instances
  DROP COLUMN agent_token_hash,
  DROP COLUMN status,
  DROP COLUMN agent_version,
  DROP COLUMN last_seen_at;
UPDATE agent_instances SET mode = 'cloud';
ALTER TABLE agent_instances ALTER COLUMN mode SET DEFAULT 'cloud';
ALTER TABLE agent_instances ADD CONSTRAINT agent_instances_mode_cloud CHECK (mode = 'cloud');

-- Tunnel egress accounting is gone; cloud egress + storage remain.
ALTER TABLE quotas DROP COLUMN tunnel_egress_limit, DROP COLUMN tunnel_egress_used;
ALTER TABLE usage_daily DROP COLUMN tunnel_egress;

-- APAGE initial schema (spec §32). All tenant-scoped tables carry tenant_id.
-- Indexes follow spec §19.7.

-- Tenants (spec §2 Tenant)
CREATE TABLE tenants (
    tenant_id   TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    plan        TEXT NOT NULL DEFAULT 'lite',        -- lite|starter|pro|team
    trust_level TEXT NOT NULL DEFAULT 'new',         -- new|basic|trusted
    status      TEXT NOT NULL DEFAULT 'active',      -- active|suspended
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users (spec §2 User)
CREATE TABLE users (
    user_id           TEXT PRIMARY KEY,
    email             TEXT NOT NULL UNIQUE,
    email_verified_at TIMESTAMPTZ,
    auth_provider     TEXT NOT NULL DEFAULT 'password', -- password|oauth
    password_hash     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Memberships (spec §2 Membership, RBAC)
CREATE TABLE memberships (
    membership_id TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    tenant_id     TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    role          TEXT NOT NULL,                      -- owner|admin|member|viewer
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);
CREATE INDEX idx_memberships_tenant ON memberships(tenant_id);

-- Quotas / Usage (spec §2 Quota)
CREATE TABLE quotas (
    tenant_id           TEXT PRIMARY KEY REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    plan                TEXT NOT NULL,
    instance_limit      INT  NOT NULL DEFAULT 1,
    storage_bytes_limit BIGINT NOT NULL DEFAULT 104857600,   -- 100MB lite
    storage_bytes_used  BIGINT NOT NULL DEFAULT 0,
    tunnel_egress_limit BIGINT NOT NULL DEFAULT 1073741824,  -- 1GB lite
    tunnel_egress_used  BIGINT NOT NULL DEFAULT 0,
    cloud_egress_limit  BIGINT NOT NULL DEFAULT 0,
    cloud_egress_used   BIGINT NOT NULL DEFAULT 0,
    conversion_limit    INT  NOT NULL DEFAULT 0,
    conversion_used     INT  NOT NULL DEFAULT 0,
    custom_domain_limit INT  NOT NULL DEFAULT 0,
    custom_domain_used  INT  NOT NULL DEFAULT 0,
    period_start        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Agent instances (spec §2 Agent Instance, §26)
CREATE TABLE agent_instances (
    instance_id           TEXT PRIMARY KEY,
    tenant_id             TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    agent_type            TEXT NOT NULL,              -- openclaw|hermes|custom
    agent_name            TEXT NOT NULL,
    subdomain             TEXT NOT NULL UNIQUE,
    mode                  TEXT NOT NULL DEFAULT 'tunnel', -- tunnel|cloud|hybrid
    status                TEXT NOT NULL DEFAULT 'offline', -- online|offline
    agent_version         TEXT,
    agent_token_hash      TEXT NOT NULL,
    instance_api_key_hash TEXT NOT NULL,
    token_grace_hash      TEXT,                       -- previous token during rotation grace
    last_seen_at          TIMESTAMPTZ,
    frozen_at             TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_instances_tenant ON agent_instances(tenant_id);

-- File refs (tunnel mode, spec §2 File Ref). Metadata-only; no raw path.
CREATE TABLE file_refs (
    file_ref       TEXT PRIMARY KEY,
    instance_id    TEXT NOT NULL REFERENCES agent_instances(instance_id) ON DELETE CASCADE,
    local_path_hash TEXT,
    display_name   TEXT NOT NULL,
    size           BIGINT NOT NULL DEFAULT 0,
    mime_type      TEXT,
    modified_at    TIMESTAMPTZ,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_file_refs_instance ON file_refs(instance_id);

-- Cloud files (spec §11)
CREATE TABLE files (
    file_id        TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    instance_id    TEXT REFERENCES agent_instances(instance_id) ON DELETE SET NULL,
    status         TEXT NOT NULL DEFAULT 'created',   -- created|uploading|uploaded|scanning|rejected|converting|ready|failed|expired|deleted
    preview_status TEXT NOT NULL DEFAULT 'pending',   -- pending|ready|failed
    display_name   TEXT NOT NULL,
    size           BIGINT NOT NULL DEFAULT 0,
    mime_type      TEXT,
    storage_key    TEXT,
    visibility     TEXT NOT NULL DEFAULT 'private',   -- private|link
    reject_reason  TEXT,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_files_tenant_expires ON files(tenant_id, expires_at);
CREATE INDEX idx_files_tenant_created ON files(tenant_id, created_at);

-- Preview links (spec §2 Preview Link, §8, §14)
CREATE TABLE preview_links (
    link_id          TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    instance_id      TEXT REFERENCES agent_instances(instance_id) ON DELETE SET NULL,
    file_ref         TEXT,                            -- mode=tunnel
    file_id          TEXT,                            -- mode=cloud
    mode             TEXT NOT NULL,                   -- tunnel|cloud
    display_name     TEXT,
    secret_hash      TEXT NOT NULL,
    access_policy    JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at       TIMESTAMPTZ,
    revoked_at       TIMESTAMPTZ,
    frozen_at        TIMESTAMPTZ,
    frozen_reason    TEXT,
    last_accessed_at TIMESTAMPTZ,
    view_count       BIGINT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((file_ref IS NOT NULL) <> (file_id IS NOT NULL))  -- exactly one backing (spec §2)
);
CREATE INDEX idx_links_tenant_created ON preview_links(tenant_id, created_at);
CREATE INDEX idx_links_instance ON preview_links(instance_id);

-- Custom domains (spec §5, §28)
CREATE TABLE custom_domains (
    domain_id      TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    domain         TEXT NOT NULL UNIQUE,
    status         TEXT NOT NULL DEFAULT 'pending',   -- pending|verified|failed
    txt_value      TEXT NOT NULL,
    cert_status    TEXT NOT NULL DEFAULT 'none',      -- none|issued|renewing|failed
    last_checked_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_domains_tenant ON custom_domains(tenant_id);

-- Audit logs (spec §15). Partitioned by created_at range in production;
-- a single table is used for MVP single-box.
CREATE TABLE audit_logs (
    event_id      TEXT PRIMARY KEY,
    tenant_id     TEXT,
    instance_id   TEXT,
    event         TEXT NOT NULL,
    actor_type    TEXT NOT NULL,
    actor_id      TEXT,
    resource_type TEXT,
    resource_id   TEXT,
    ip            TEXT,
    user_agent    TEXT,
    reason        TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_tenant_created ON audit_logs(tenant_id, created_at);
CREATE INDEX idx_audit_event ON audit_logs(event);

-- Abuse reports (spec §15.5, §30)
CREATE TABLE abuse_reports (
    report_id   TEXT PRIMARY KEY,
    link_id     TEXT,
    tenant_id   TEXT,
    category    TEXT NOT NULL,                        -- phishing|malware|illegal|other
    detail      TEXT,
    source_ip   TEXT,
    status      TEXT NOT NULL DEFAULT 'open',         -- open|actioned|dismissed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_abuse_status ON abuse_reports(status);

-- Email verification / password reset / member invite tokens
CREATE TABLE auth_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    TEXT,
    tenant_id  TEXT,
    purpose    TEXT NOT NULL,                         -- verify_email|reset_password|invite
    email      TEXT,
    role       TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sessions (spec §25)
CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

-- Platform administrators (spec §8). A separate identity plane from tenant
-- users, with password + mandatory TOTP MFA. Admin sessions live in Redis.
CREATE TABLE platform_admins (
    admin_id      TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret   TEXT,
    mfa_enrolled  BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

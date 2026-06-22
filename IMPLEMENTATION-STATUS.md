# APAGE Implementation Status

Maps the [implementation checklist](specs/apage-implementation-checklist.md) to
what is built. ✅ implemented & verified · 🟡 implemented (stub/integration
point) · ⬜ deferred production-hardening (spec §21 defers these) · ❌ removed.

> **Cloud-only (tunnel removed).** APAGE no longer supports the reverse-tunnel /
> hybrid modes. The `apage-gateway` service and the old `apage-agent` tunnel CLI
> are gone; object storage (S3/MinIO) is now **required**. The customer-side
> `apage-cli` runs an HTTP **MCP server** that an agent calls to upload files and
> create/manage cloud preview links (`create_preview_link`, `list_links`,
> `revoke_link`, `modify_link`). Links can only be created via an instance API
> key — not from a console session. Items below marked ❌ reflect that removal.

## Verified end-to-end (live, against Postgres/Redis/MinIO)

- ✅ Register → session cookie → tenant/owner/quota created atomically
- ✅ Instance provisioning → `instanceApiKey` (shown once, hashed at rest)
- ❌ Agent tunnel connect / protocol handshake / heartbeat / registry — removed (cloud-only)
- ✅ Path validation (traversal/symlink/hidden/exec blocked) — reused by the `apage-cli` MCP upload allowlist
- ❌ Tunnel preview streamed agent → gateway → API → client — removed (cloud-only)
- ✅ Cloud upload (instance API key) → scan → `ready` (initial status never `ready` — P0-1) → cloud preview from MinIO
- ✅ Wrong secret → 404 (constant-time); revoke → 410; expiry (three-layer) → 410
- ✅ `maxViews=2` under 4 concurrent requests → exactly 2×200 / 2×410 (atomic, P0-2)
- ✅ Password gate: page on no-cookie, wrong → 403, correct → unlock cookie → content; **hash redacted in list output**
- ✅ GDPR/CCPA data deletion → purges files/refs/links, queues object cleanup, audit confirmation
- ✅ Frontend: landing/auth/console all build (18 routes); register+session via web proxy

## P0 — Foundation
- ✅ Go monorepo, config (env), id/secret (≥128-bit CSPRNG, prefixes), argon2id + constant-time hash
- ✅ httpx: error envelope, cursor pagination, idempotency, rate-limit headers, secret-scrubbing logger
- ✅ Redis: atomic view consume (Lua), rate limit, idempotency, queue, link cache invalidation (agent registry removed)
- ✅ DB migrations (all §32 tables + indexes; tunnel columns/file_refs dropped by `0004_drop_tunnel.sql`, cloud-only CHECKs)
- ✅ docker-compose (api/worker/postgres/redis/minio/caddy) + Dockerfile + Caddyfile (gateway removed)
- ✅ Frontend design tokens (light/dark), component library, API client

## P1 — MVP-0 (cloud-only; was DNS + Tunnel)
- ✅ Auth (§25): register/login/logout/session/verify-email/reset/resend; argon2id; anti-enumeration; rate limit
- 🟡 OAuth start/callback — routes/flow described; provider wiring is `TODO(prod)`
- ✅ Instances (§26): create/list/get/delete/rotate (revoke-token + allowlist-change-request removed with tunnel)
- ✅ apage-cli (§6): `init`/`mcp` CLI + allowlist + 7-step path check (tunnel client / local register / `apage-agent` removed)
- 🟡 CLI install integrity (checksum/minisign), auto-update — documented; release pipeline is `TODO(prod)`
- ❌ Tunnel transport (§7): WS handshake / multiplexed streams / backpressure / heartbeat — removed (cloud-only)
- ✅ Preview links (§8): create (cloud, instance-API-key only), list, revoke, update/`modify_link`; three-layer expiry clamp
- ✅ Access policy (§14): public/password/account/ip_allowlist/single_use/maxViews; strong vs final consistency split
- ✅ Runtime (§30): /p admission order, /unlock, account/ip gates, per-type CSP (§15)
- ✅ Audit query (§14), health/readyz/metrics (§31)
- ✅ Agent integration: `apage-cli` MCP server (HTTP) — `create_preview_link`/`list_links`/`revoke_link`/`modify_link`
- ✅ Console (§17): overview, instances, links (view+revoke only), files, audit, usage, members, domains, settings; visitor password page

## P2 — MVP-1 (Cloud)
- ✅ Cloud storage (§11) S3/MinIO, key scheme, file state machine, three-layer expiry cascade
- ✅ Upload (§12): direct (≤8MiB / 413), presign, complete; ready-only cloud links
- 🟡 Scanner (§10): MIME-allowlist verdict implemented; ClamAV/phishing/hash signatures are `TODO(prod)`
- ✅ Expiry sweep (≥hourly), object delete with re-queue retry, metering + quota enforcement, usage API
- ✅ Plan presets + Lite 24h-retention default + quota checks

## P3 — V1
- ✅ Custom domains (§28): CRUD + TXT verify (live DNS lookup); 🟡 ACME issuance is `TODO(prod)`
- ✅ Abuse report endpoint (§30/§15.5) + audit + queue; 🟡 admin processing/freeze workflow partial
- ✅ Compliance data deletion (§15.6); region residency is config-level
- ❌ Office conversion (§13): **out of scope** — APAGE is view-only (no in-browser editing); office documents are not converted or accepted (rejected at the MIME allowlist)
- ✅ Admin console (§8): platform-admin backend — password + mandatory TOTP MFA + IP allowlist, tenant list/detail/trust/suspend/restore (with link-freeze teeth), abuse queue + actioning, cross-tenant audit, overview; admin login + overview/tenant UI wired. 🟡 enterprise SSO (SAML/OIDC) is the deferred IdP integration

## Deferred (spec §21 "暂缓" / external services)
- ⬜ Admin SSO + MFA platform auth, network isolation
- ⬜ Real ClamAV, ACME automation, Safe Browsing/URLhaus (LibreOffice converter removed — view-only product)
- ⬜ Multi-region active-active, CloudFront, ECS/Fargate migration (Phase 2/3, §22.4)

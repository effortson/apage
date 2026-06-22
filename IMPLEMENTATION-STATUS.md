# APAGE Implementation Status

Maps the [implementation checklist](specs/apage-implementation-checklist.md) to
what is built. ✅ implemented & verified · 🟡 implemented (stub/integration
point) · ⬜ deferred production-hardening (spec §21 defers these).

## Verified end-to-end (live, against Postgres/Redis/MinIO)

- ✅ Register → session cookie → tenant/owner/quota created atomically
- ✅ Instance provisioning → `agentToken` + `instanceApiKey` (shown once, hashed at rest)
- ✅ Agent tunnel connect + protocol handshake + heartbeat + registry
- ✅ Local file register (loopback only) + path validation (traversal/symlink/hidden/exec blocked)
- ✅ Tunnel preview streamed agent → gateway → API → client, per-type CSP + security headers
- ✅ Cloud upload → scan → `ready` (initial status never `ready` — P0-1) → cloud preview from MinIO
- ✅ Wrong secret → 404 (constant-time); revoke → 410; expiry (three-layer) → 410
- ✅ `maxViews=2` under 4 concurrent requests → exactly 2×200 / 2×410 (atomic, P0-2)
- ✅ Password gate: page on no-cookie, wrong → 403, correct → unlock cookie → content; **hash redacted in list output**
- ✅ GDPR/CCPA data deletion → purges files/refs/links, queues object cleanup, audit confirmation
- ✅ Frontend: landing/auth/console all build (18 routes); register+session via web proxy

## P0 — Foundation
- ✅ Go monorepo, config (env), id/secret (≥128-bit CSPRNG, prefixes), argon2id + constant-time hash
- ✅ httpx: error envelope, cursor pagination, idempotency, rate-limit headers, secret-scrubbing logger
- ✅ Redis: atomic view consume (Lua), rate limit, agent registry, idempotency, queue, link cache invalidation
- ✅ DB migrations (all §32 tables + indexes + file_ref/file_id mutual-exclusion check)
- ✅ docker-compose (api/gateway/worker/postgres/redis/minio/caddy) + Dockerfile + Caddyfile
- ✅ Frontend design tokens (light/dark), component library, API client

## P1 — MVP-0 (DNS + Tunnel)
- ✅ Auth (§25): register/login/logout/session/verify-email/reset/resend; argon2id; anti-enumeration; rate limit
- 🟡 OAuth start/callback — routes/flow described; provider wiring is `TODO(prod)`
- ✅ Instances (§26): create/list/get/delete/rotate/revoke-token/allowlist-change-request
- ✅ Agent (§6): init/start/share CLI, allowlist + 7-step path check, local register, tunnel client, reconnect backoff
- 🟡 Agent install integrity (checksum/minisign), auto-update — documented; release pipeline is `TODO(prod)`
- ✅ Tunnel (§7): WS handshake, version floor, session params, heartbeat, multiplexed streams, backpressure, cancel, error codes
- ✅ Preview links (§8): create (tunnel upsert from metadata / cloud), list, revoke; three-layer expiry clamp
- ✅ Access policy (§14): public/password/account/ip_allowlist/single_use/maxViews; strong vs final consistency split
- ✅ Runtime (§30): /p admission order, /unlock, account/ip gates, per-type CSP (§15)
- ✅ Audit query (§14), health/readyz/metrics (§31)
- ✅ Agent integration: CLI helper + local HTTP API (MCP tool/SDK adapters are thin wrappers, `TODO(prod)`)
- ✅ Console (§17): overview, instances, links, files, audit, usage, members, domains, settings; visitor password page

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
- 🟡 Admin console (§8): frontend shell + documented surfaces; ⬜ platform SSO+MFA auth, live wiring (spec §21: admin post-MVP)

## Deferred (spec §21 "暂缓" / external services)
- ⬜ Admin SSO + MFA platform auth, network isolation
- ⬜ Real ClamAV, ACME automation, Safe Browsing/URLhaus (LibreOffice converter removed — view-only product)
- ⬜ Multi-region active-active, CloudFront, ECS/Fargate migration (Phase 2/3, §22.4)

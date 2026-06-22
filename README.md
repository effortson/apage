# APAGE

Preview & Share Provider for agents — cloud-hosted file preview, temporary
sharing, and subdomain access. Built per [`specs/apage-spec.md`](specs/apage-spec.md)
and [`specs/apage-ui-spec.md`](specs/apage-ui-spec.md); implementation plan in
[`specs/apage-implementation-checklist.md`](specs/apage-implementation-checklist.md).

## Architecture (MVP single-box, spec §22)

```
apage-api      REST control plane + data plane + visitor runtime (/p, /f)
apage-worker   async scan / expiry-sweep / object-delete / usage-flush / domain-recheck
apage-cli      customer-side: runs an MCP server an agent calls to upload files
               and create/manage cloud preview links
web            Next.js frontend: marketing, auth, console, admin shell
```

Datastores: PostgreSQL (metadata), Redis (sessions / rate limit / atomic view
counter / queue), S3-compatible object storage (cloud files).

## Run locally

**One command** — starts infra, builds, and runs api + worker + web in this
terminal with colored per-service output; `Ctrl+C` stops everything (apage-cli is
built too, but you run it manually — see "Try the cloud + MCP flow"):

```bash
make dev          # or: ./scripts/dev.sh
./scripts/dev.sh --no-web        # backend only
STOP_INFRA=1 ./scripts/dev.sh    # also stop docker infra on exit
```

Then open `http://localhost:3000`. Output looks like:

```
dev    │ starting infra (postgres :5433, redis :6379, minio :9100)…
API    │ {"level":"INFO","msg":"apage-api listening","addr":":8080"}
WORKER │ {"level":"INFO","msg":"apage-worker started"}
WEB    │  ✓ Ready in 1148ms
```

<details><summary>Manual steps (if you prefer)</summary>

```bash
docker compose up -d postgres redis minio   # Postgres :5433, Redis :6379, MinIO :9100
cp .env.example .env
make build
set -a; source .env; set +a
./bin/apage-api & ./bin/apage-worker &
cd web && npm install && npm run dev          # :3000 (proxies /api/v1 -> :8080)
```
</details>

Full stack via Docker: `make up` (adds Caddy edge with wildcard + TLS).

## Try the cloud + MCP flow

Links are created by the agent through `apage-cli`'s MCP server — not by hand in
the console. Register + create an instance in the console (or via API) to get an
`instanceApiKey`, then:

```bash
# Configure the CLI with the instance API key and the workspace it may upload from.
apage-cli init --instance alice --workspace ~/outputs \
               --api http://localhost:8080 --api-key <instanceApiKey>

# Start the MCP server (foreground; Ctrl+C to stop). Point your agent at it.
apage-cli mcp
```

Your agent then calls the `create_preview_link` MCP tool (e.g.
`{path: "report.pdf"}`). `apage-cli` uploads the file, waits for the worker scan
to mark it `ready`, creates a cloud preview link, and returns its URL. In dev the
preview opens at `http://localhost:8080/p/<linkId>/<secret>`. Other MCP tools:
`list_links`, `revoke_link`, `modify_link`.

## Local debugging

### Run services in the foreground (see logs live)

Logs are structured JSON to stdout (`apage-cli` uses text). Run one service at a
time in the foreground and pretty-print with `jq`:

```bash
set -a; source .env; set +a            # export env into the shell
go run ./cmd/api      2>&1 | jq .       # or ./bin/apage-api
go run ./cmd/worker   2>&1 | jq .
```

If you run them detached (`./bin/apage-api > /tmp/api.log 2>&1 &`), tail with
`tail -f /tmp/api.log | jq .`. Every request carries an `X-Request-Id`; grep a
failing response's `requestId` across the logs to trace it. Secret path segments
are scrubbed from logs (`/p/<id>/***`) by design.

### Step through with a debugger (Delve)

```bash
go install github.com/go-delve/delve/cmd/dlv@latest
set -a; source .env; set +a
dlv debug ./cmd/api -- # then: break api.(*Server).handlePreview / continue
# or attach to a running binary:
dlv attach "$(pgrep -f bin/apage-api)"
```

VS Code `.vscode/launch.json`:

```json
{ "version": "0.2.0", "configurations": [{
  "name": "apage-api", "type": "go", "request": "launch",
  "program": "${workspaceFolder}/cmd/api",
  "envFile": "${workspaceFolder}/.env"
}]}
```

### Inspect datastore state

```bash
# Postgres
PGPASSWORD=apage psql -h 127.0.0.1 -p 5433 -U apage -d apage
#   \dt                                   list tables
#   SELECT instance_id,subdomain FROM agent_instances;
#   SELECT link_id,mode,revoked_at,frozen_at,view_count FROM preview_links ORDER BY created_at DESC LIMIT 10;
#   SELECT event,actor_type,resource_id,created_at FROM audit_logs ORDER BY created_at DESC LIMIT 20;

# Redis (key prefixes: view: / rl: / idem: / link: / queue:)
redis-cli
#   GET  view:plink_xxx    atomic view counter for maxViews/single_use
#   LLEN queue:scan        pending scan jobs

# MinIO — console UI at http://localhost:9101 (minioadmin / minioadmin)
#   or: mc alias set local http://localhost:9100 minioadmin minioadmin && mc ls --recursive local/apage
```

### Probe the services

```bash
curl -s localhost:8080/readyz | jq .      # {"deps":{"db":"ok","redis":"ok"}} — shows which dep is down
```

### Frontend

`npm run dev` gives hot reload. The browser stays same-origin: `/api/v1/*` is
proxied to `APAGE_API_URL` (default `http://localhost:8080`) by `next.config.mjs`
— point it elsewhere with `APAGE_API_URL=… npm run dev`. Use the browser
Network tab to see the proxied calls and the `X-Request-Id` on each response.

### Reset state

```bash
docker compose down -v        # wipe Postgres/Redis/MinIO volumes
docker compose up -d postgres redis minio
# the api re-runs migrations on next start
```

### Common gotchas (hit during bring-up)

- **Port already allocated** (`5432`/`9000` taken by another stack): this repo
  remaps host ports to **Postgres :5433** and **MinIO :9100/:9101**. Find a
  conflict with `lsof -nP -iTCP:<port> -sTCP:LISTEN`.
- **Postgres "password authentication failed"** with the right password: a stale
  `apage_pgdata` volume kept old credentials (`POSTGRES_PASSWORD` only applies on
  first init). Fix: `docker compose rm -sf postgres && docker volume rm apage_pgdata && docker compose up -d postgres`.
- **apage-cli "unauthorized"**: the `--api-key` must be the `instanceApiKey`
  returned when the instance was created (shown once). Lost it? Rotate via
  `POST /api/v1/instances/{id}/rotate-credentials`.
- **`readyz` returns 503**: a dependency is unreachable — the JSON `deps` field
  names which (`db`/`redis`); cloud upload also needs MinIO.
- **Can't find a link's secret**: secrets are returned only once at creation and
  stored hashed — there is no endpoint to retrieve them; create a new link.

### Race detector & focused tests

```bash
go test -race ./...                       # catches data races (view counters, queues)
go test -run TestResolvePath ./internal/agent -v
```

## What's implemented vs production-hardening

See [`IMPLEMENTATION-STATUS.md`](IMPLEMENTATION-STATUS.md) for the full mapping.
MVP-0, MVP-1, and most V1 control-plane surfaces are implemented and verified
end-to-end. Items requiring external services (real ClamAV, ACME automation,
Safe Browsing, admin SSO/MFA) are stubbed with clear `TODO(prod)` markers and
documented integration points. Office conversion is intentionally out of scope:
APAGE is view-only (no in-browser editing), so office documents are not
converted or accepted.

## Tests

```bash
go test ./...        # path traversal, argon2, access policy, expiry, redaction
cd web && npm run build
```

### End-to-end (multi-surface) tests

[`internal/e2e/`](internal/e2e/) wires the **real** servers — `apage-api` and the
worker — in-process against the live infra and drives the full operator + agent +
visitor journey, asserting on every surface. They are tagged `e2e` so a plain
`go test ./...` (no infra) stays green, and they `t.Skip` (not fail) when the
datastores are unreachable.

```bash
make test-e2e        # brings infra up, then: go test -tags e2e ./internal/e2e/...
```

Covered flows: health; register/login/logout + CSRF enforcement; instance
lifecycle incl. cross-tenant subdomain conflict (409) and lite quota; the cloud
preview path (instance-API-key upload → worker scan → ready → preview → delete)
with Range, wrong-secret 404, and revoke 410; access policies (password gate,
atomic `maxViews` under concurrency, single-use, IP allowlist); abuse
freeze→410→unfreeze; cross-surface audit trail; MIME-allowlist rejection; and
that links can only be created via an instance API key (console session → 403).

The browser/web surface (register → console → instance creation, with the quota
error path) is exercised via the running stack at `http://localhost:3000`.

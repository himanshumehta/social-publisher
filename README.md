# social-publisher

A Go service that takes posts and videos from people or pipelines, puts each one through automated checks and a human approval, and publishes it to Instagram, Facebook, LinkedIn, Threads, X and YouTube.

[![CI](https://github.com/himanshumehta/social-publisher/actions/workflows/ci.yml/badge.svg)](https://github.com/himanshumehta/social-publisher/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8.svg)](go.mod)

## Demo

Worker log from the [Quickstart](#quickstart) in a fresh clone, with no credentials and `PUBLISH_DRY_RUN=true` (timestamps and repeated fields trimmed). One post fans out to five networks, then a second post arrives through the API-key endpoint that pipelines use.

```text
{"level":"INFO","msg":"pub worker: publishing","post_id":"396abae6-…","platforms":["instagram","facebook","linkedin","threads","twitter"],"retry_count":0}
{"level":"INFO","msg":"would publish instagram 396abae6-… Hello from social-publisher on Instagram","dry_run":true}
{"level":"INFO","msg":"would publish facebook 396abae6-… Hello from social-publisher on Facebook","dry_run":true}
{"level":"INFO","msg":"would publish linkedin 396abae6-… Hello from social-publisher on LinkedIn","dry_run":true}
{"level":"INFO","msg":"would publish threads 396abae6-… Hello from social-publisher on Threads","dry_run":true}
{"level":"INFO","msg":"would publish twitter 396abae6-… Hello from social-publisher on X","dry_run":true}
{"level":"INFO","msg":"pub worker: publish complete","post_id":"396abae6-…","published":5,"failed":0,"retry_count":0}
{"level":"INFO","msg":"would publish linkedin 70cea5cc-… Rendered by data-to-video, reviewed by a...","dry_run":true}
{"level":"INFO","msg":"pub worker: publish complete","post_id":"70cea5cc-…","published":1,"failed":0,"retry_count":0}
```

Each platform gets a `post_publish_links` row with a synthetic `https://dry-run.invalid/...` URL and the post ends `published`. With `PUBLISH_DRY_RUN=false` and connected channels, the same flow calls the real APIs.

## Why it exists

Every network has its own OAuth flavour, upload protocol (resumable chunks, container-then-poll, URL fetch), rate limits and failure modes. A content pipeline should not know any of that, and it should not be able to post without a person signing off. This service is the publishing half of a data-to-video agent pipeline: the generator renders and describes a video, and this service owns channel credentials, review, scheduling, retries and the per-platform upload work. The constraints it is built around: tokens for real brand accounts must be safe at rest, a crash must never double-post, and nothing goes out without an admin's approval.

## Architecture

```mermaid
flowchart LR
    client["data-to-video / curl / UI"] -->|"JWT or API key"| api["server (cmd/server)"]
    api --> db[("PostgreSQL: posts, upload_queue, publish_records, channels with AES-GCM tokens")]
    api -->|"presigned URLs"| s3[("S3 / MinIO: videos, thumbnails")]
    db -->|"FOR UPDATE SKIP LOCKED"| worker["worker (cmd/worker)"]
    worker --> pipe["post pipeline: scan, generate, review, publish"]
    worker --> sched["schedule release (60s)"]
    worker --> upq["upload queue (10s) + processing poller (30s)"]
    pipe -->|"optional"| llm["Claude API: platform copy"]
    pipe --> adapters
    upq --> adapters
    subgraph adapters["PlatformAdapter: rate limiter + circuit breaker"]
        direction TB
        real["Instagram, Facebook, LinkedIn, Threads, X, YouTube"]
        dry["DryRunAdapter when PUBLISH_DRY_RUN=true"]
    end
    worker -.-> redis[("Redis: quota counters, scan and generation cache, OAuth state")]
```

A text/image post moves through these states. Every path to `approved` goes through an admin; scheduling only holds an approved post until its time.

```mermaid
stateDiagram-v2
    [*] --> draft: created with source_url
    [*] --> creator_editing: created with platform_posts
    draft --> scanned: worker fetches the page through netguard
    scanned --> generated: Claude writes per-platform copy
    generated --> pending_review: automated review
    generated --> flagged: automated review blocks
    creator_editing --> pending_review: submit (automated review)
    pending_review --> approved: admin approves
    flagged --> approved: admin overrides
    pending_review --> scheduled: admin approves with scheduled_at
    approved --> scheduled: author schedules
    scheduled --> approved: due
    approved --> publishing: worker claims
    publishing --> published: at least one platform succeeded
    publishing --> publish_failed: every platform failed
    publish_failed --> publishing: retry, up to 3
    pending_review --> rejected: admin rejects
```

| Component | Responsibility | Key file |
|---|---|---|
| Router | Declarative route table with an access level per route; global middleware chain | `internal/router/router.go` |
| Post pipeline | Scan, generate and publish workers over Postgres-backed stages | `internal/worker/post_pipeline.go` |
| Post publisher | Fans one post out to each platform concurrently; rate limiter and circuit breaker per platform; dry-run swap | `internal/service/post_publisher.go`, `publish_adapter.go` |
| Upload orchestrator | Video queue: claim, upload via the platform service, poll asynchronous processing | `internal/service/upload_orchestrator.go`, `platform_*.go` |
| Generator and reviewer | Claude call with retries and cache; output validation; rule-based review (length, hashtags, blocked terms and domains) | `internal/service/post_generator.go`, `post_reviewer.go` |
| Channel OAuth | Per-provider connect flows (state, PKCE for X), token refresh, expiry warnings | `internal/service/channel_oauth*.go` |
| Token crypto | AES-256-GCM sealing of OAuth tokens with key rotation | `internal/service/token_crypto.go` |
| netguard | HTTP client for user-supplied URLs that refuses private destinations | `internal/netguard/netguard.go` |
| Operator CLI | Creates accounts and API keys (there is no public signup) | `cmd/admin/main.go` |
| Migrations | Ordered SQL files applied in transactions under an advisory lock | `internal/migrate/migrate.go`, `migrations/` |

Five binaries share one module: `server`, `worker`, `migrate`, `admin`, and `thumbgen` (backfills video thumbnails with ffmpeg).

## Design decisions and trade-offs

**Postgres is the queue.**
*Why:* every stage claims one row with `UPDATE … WHERE id = (SELECT … FOR UPDATE SKIP LOCKED)`, so any number of workers can run without double-claiming, and work survives restarts with no extra broker.
*Trade-off:* polling (10 to 60 s per loop) instead of push, and throughput bounded by one database. Redis queues were rejected because the state already lives in Postgres and a second source of truth would need reconciling.

**One adapter interface, and dry run is just another adapter.**
*Why:* every network sits behind `PlatformAdapter`, so `PUBLISH_DRY_RUN=true` (the default) swaps all of them for `DryRunAdapter` in one place. The whole system, workers included, runs end to end with zero credentials.
*Trade-off:* the video path (`UploadOrchestrator`) and the text path (`PostPublisher`) still have separate per-platform code because their protocols differ (resumable uploads and container polling versus single calls).

**Human approval is the only way to publish.**
*Why:* AI-written and pipeline-supplied copy is untrusted. The automated reviewer flags it; only an admin moves a post to `approved`. Scheduling was originally a side door (authors could schedule unreviewed drafts and the schedule worker approved them); it now accepts only approved posts, and admins approve straight into the schedule in one statement.
*Trade-off:* a person is on the critical path, so nothing publishes unattended.

**Idempotency at the platform level, not the request level.**
*Why:* a worker can die after posting to two of five networks. Each success is written to `post_publish_links (UNIQUE post_id, platform)` as it happens; a post stuck in `publishing` for 5 minutes is reclaimed, and platforms already recorded are skipped.
*Trade-off:* there is no `Idempotency-Key` on the HTTP API, so a client that retries a create request makes a second post (see Limitations).

**Tokens are encrypted in the application, with rotation.**
*Why:* AES-256-GCM in-process means a database dump alone does not expose brand-account tokens. `TOKEN_ENCRYPTION_KEY_PREVIOUS` lets a key roll over without downtime; tokens move to the new key on their next refresh.
*Trade-off:* the key is an environment variable, not a KMS. That is simpler to run and weaker if the host is compromised.

**Fail closed by default.**
*Why:* development is opt-in (`APP_ENV=development`). Any other value, including unset, gets strict validation at start-up: no built-in JWT secret, an encryption key present and well-formed, TLS to Postgres, no wildcard CORS, no dev admin.
*Trade-off:* a bare `go run ./cmd/server` without `.env` refuses to start, which is deliberate.

## AI in the loop

- **Where the model sits.** Only in the generate stage, and only for posts created from a `source_url`. The scanner fetches the page (through `netguard`), extracts title, description and body text, and `PostGenerator` asks Claude (`claude-sonnet-4-5-20250929`, 4,096 max tokens) for JSON with one entry per requested platform. Posts that arrive with `platform_posts`, which is what the Quickstart and pipelines do, never call a model.
- **Guardrails on output.** The response must parse as JSON; only requested platforms are kept, each must have non-empty text, and character counts are recomputed from the text rather than trusted from the model (`validateGenerated`). The rule-based `PostReviewer` then checks length limits, hashtag counts, blocked terms and blocked domains and marks the post `pending_review` or `flagged`. Hand-written and API-supplied copy goes through the same reviewer on submit.
- **Human gate.** An admin approves, rejects or overrides every post. There is no auto-publish path.
- **Fallbacks and cost.** Three attempts with exponential backoff, 60 s timeout per call, results cached in Redis for an hour by content hash, and no network call at all when `CLAUDE_API_KEY` is unset (the post fails generation immediately with a clear error).
- **Tests.** Output validation, the fail-fast path and the review gate have unit and DB-backed tests. No test calls a real model or platform.

## Reliability and operations

- **Retries.** Text posts that fail on every platform retry up to 3 times. Partial success marks the post `published` and records per-platform errors in `publish_results`; failed platforms are not retried automatically. Video upload queue items are attempted once and marked `failed` with a mapped, redacted error.
- **Back-pressure.** Per-platform token buckets (`golang.org/x/time/rate`) and a circuit breaker that opens after 5 consecutive failures for 5 minutes. YouTube quota (1,600 units per upload) is counted in Redis and shown at `GET /api/v1/admin/quota`; it is not enforced before an upload.
- **Stuck work.** Rows left in a transient state for 5 minutes are reclaimed by the next worker.
- **Observability.** JSON logs (`log/slog`) with request IDs, every value passed through a redactor that masks tokens, secrets and OAuth codes. `GET /health` for liveness, `GET /health/ready` checks Postgres (and reports Redis as degraded when down). There are no metrics endpoints yet.
- **Housekeeping.** The worker removes abandoned upload sessions and expired refresh tokens every 6 hours and warns about channel tokens within 7 days of expiry.

## Security

The threat model assumes the API is reachable from the internet, callers may be malicious or compromised, and pasted URLs may point anywhere. Controls, all covered by tests:

- **Authorization:** routes are a table with an access level each; a test walks every route and pins the public surface. Posts are scoped to their author, the video path is admin-only, and the review gate has DB-backed tests.
- **Authentication:** HS256-only JWTs with mandatory expiry, rotating refresh tokens with reuse detection, bcrypt cost 12, uniform login failures, stricter rate limits on login and refresh, scoped API keys stored as SHA-256. No public signup; accounts come from `cmd/admin`.
- **Secrets:** OAuth tokens sealed with AES-256-GCM, start-up fails on a bad key, and the redactor keeps tokens out of logs, stored errors and API responses.
- **SSRF:** user-supplied URLs are fetched only through `netguard`, which checks the resolved address on every hop and caps response size.
- **HTTP:** 1 MiB bodies, single-origin CORS, security headers with a `default-src 'none'` CSP, header and read timeouts, sanitised upload file names.
- **Supply chain:** govulncheck, gosec, staticcheck and gitleaks in CI; Dependabot for Go modules, Actions and the Dockerfile; non-root container.

Reporting, scope and known gaps: [SECURITY.md](SECURITY.md).

## Quickstart

Requirements: Go 1.26+ (the `toolchain go1.27.3` line in `go.mod` downloads the exact toolchain), PostgreSQL (CI runs 16; the clean-clone check below used 18), Redis (checked with 7.2), `curl` and `jq`. No accounts or API keys.

```bash
git clone https://github.com/himanshumehta/social-publisher && cd social-publisher
cp .env.example .env    # point DB_* and REDIS_ADDR at your Postgres and Redis; APP_PORT is the API port
make migrate            # 39 migrations
make dev                # terminal 1: API
make worker             # terminal 2: background loops
```

`.env` expects a database and user named `socialpub` (password `socialpub`) by default. In development the server creates the admin from `DEV_ADMIN_EMAIL` / `DEV_ADMIN_PASSWORD`. Log in, create a post with ready-made copy for each network, submit it (automated review), then approve it:

```bash
API=http://localhost:8081
TOKEN=$(curl -s -X POST $API/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"changeme-dev-only"}' | jq -r .access_token)

POST_ID=$(curl -s -X POST $API/api/v1/posts \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{
    "title": "Hello from social-publisher",
    "target_platforms": ["instagram","facebook","linkedin","threads","twitter"],
    "platform_posts": {
      "instagram": {"text": "Hello from social-publisher on Instagram"},
      "facebook":  {"text": "Hello from social-publisher on Facebook"},
      "linkedin":  {"text": "Hello from social-publisher on LinkedIn"},
      "threads":   {"text": "Hello from social-publisher on Threads"},
      "twitter":   {"text": "Hello from social-publisher on X"}
    }
  }' | jq -r .id)

curl -s -X POST $API/api/v1/posts/$POST_ID/submit        -H "Authorization: Bearer $TOKEN"
curl -s -X POST $API/api/v1/admin/posts/$POST_ID/approve -H "Authorization: Bearer $TOKEN"
```

The publish worker polls every 15 seconds. The worker terminal shows five `would publish` lines, and:

```bash
sleep 20
curl -s $API/api/v1/admin/posts/$POST_ID/publish-status -H "Authorization: Bearer $TOKEN" | jq .status   # "published"
```

**As a pipeline (API key).** Mint a key for the dev admin, create a post with it (it lands in review automatically), and approve it as above:

```bash
KEY=$(make -s api-key | tail -1)     # printed once; only its SHA-256 is stored
EXT_ID=$(curl -s -X POST $API/api/v1/external/posts \
  -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
  -d '{"title":"From a pipeline","target_platforms":["linkedin"],"platform_posts":{"linkedin":{"text":"Rendered by data-to-video, reviewed by a human"}}}' | jq -r .id)
curl -s -X POST $API/api/v1/admin/posts/$EXT_ID/approve -H "Authorization: Bearer $TOKEN"
sleep 20; curl -s $API/api/v1/external/posts/$EXT_ID -H "Authorization: Bearer $KEY" | jq -r .status   # "published"
```

**Other flows.** To schedule, approve with a time: `POST /api/v1/admin/posts/{id}/approve` with `{"scheduled_at":"<RFC3339>"}` (authors can also schedule their own approved posts with `PATCH /api/v1/posts/{id}/schedule`). Videos go through `/api/v1/uploads/*` (presigned multipart to S3) and `POST /api/v1/videos/{id}/publish-to-platform`; that path needs S3 (MinIO works) and a connected channel row.

**Docker.** `docker compose up -d --build` starts Postgres, Redis, migrations, API and worker from the same `.env` (`--profile storage` adds MinIO). The compose file and Dockerfile were not exercised in this revision; the native path above is the verified one.

### Configuration

All variables are in [`.env.example`](.env.example). The ones that matter:

| Variable | Default | Purpose |
|---|---|---|
| `APP_ENV` | `production` | `development` relaxes start-up checks; anything else is validated strictly. `.env.example` sets `development`. |
| `PUBLISH_DRY_RUN` | `true` | Route every platform adapter to the dry-run mock. `false` posts for real. |
| `DEV_ADMIN_EMAIL`, `DEV_ADMIN_PASSWORD` | unset | Development-only admin bootstrap. Rejected outside development. |
| `APP_HOST`, `APP_PORT` | `localhost`, `8081` | Listener. |
| `APP_FRONTEND_URL` | `http://localhost:5173` | The single CORS origin, and where OAuth callbacks redirect the browser. |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` | `localhost`, `5432`, `socialpub` ×3, `disable` | PostgreSQL. `disable` is rejected outside development. |
| `REDIS_ADDR`, `REDIS_PASSWORD`, `REDIS_DB`, `REDIS_TLS` | `localhost:6379`, empty, `1`, `false` | Redis (quota, caches, OAuth state). The API keeps working if it is down. |
| `JWT_SECRET`, `JWT_ACCESS_TOKEN_TTL`, `JWT_REFRESH_TOKEN_TTL` | dev value, `15m`, `720h` | Auth. The secret must be 32+ characters; the dev value is rejected outside development. |
| `TOKEN_ENCRYPTION_KEY` | empty | 64 hex chars (`openssl rand -hex 32`). Required outside development. |
| `TOKEN_ENCRYPTION_KEY_PREVIOUS` | empty | Comma-separated retired keys accepted for decryption during rotation. |
| `TRUST_PROXY_HEADERS` | `false` | Key rate limits on `CF-Connecting-IP` / `X-Real-IP`. Only behind a proxy that sets them. |
| `OUTBOUND_ALLOW_PRIVATE_NETWORKS` | `false` | Let user-supplied URLs reach private addresses (local MinIO testing only). |
| `S3_*` | MinIO on `localhost:9000` | Video and thumbnail storage. |
| `GOOGLE_*`, `FACEBOOK_*`, `INSTAGRAM_*`, `THREADS_*`, `TWITTER_*`, `LINKEDIN_*` | placeholders | OAuth app credentials and redirect URLs for connecting real channels. |
| `CLAUDE_API_KEY` | empty | Optional. Generates copy for posts created from a `source_url`. |

Platform rate limits and daily quotas are constants in `internal/service/post_publisher.go` and `quota_tracker.go`.

### Testing

```bash
make test                                    # unit tests; DB-backed tests skip themselves
TEST_DATABASE_URL=postgres://socialpub:socialpub@localhost:5432/socialpub_test?sslmode=disable \
  go test ./...                              # after `make migrate` against that database
```

There are 260 `Test` functions (`grep '^func Test'` over `*_test.go`). Against a freshly migrated database, `go test -json ./...` reported 444 passing tests and subtests, 0 failing and 4 skipped (those four need pre-seeded upload fixtures). Without a database: 398 pass, 50 skip, 0 fail. Coverage includes the route access table, JWT and rate-limit hardening, config validation, token encryption and rotation, SSRF and redaction, upload validation, model-output validation, the review gate, account provisioning, and the dry-run adapters. CI runs the DB-backed suite against a Postgres service container.

## Project layout

```text
cmd/
  server/    HTTP API               worker/    background loops
  migrate/   SQL migrations         admin/     accounts and API keys
  thumbgen/  thumbnail backfill
internal/
  router/    route table + middleware chain     handler/   HTTP handlers
  service/   posts, channels, OAuth, uploads, platform clients, crypto
  worker/    post pipeline                      middleware/ auth, rate limit, CORS, headers
  netguard/  SSRF-safe HTTP client              redact/     credential masking
  config/    env config + validation            app/        shared start-up
migrations/  39 ordered SQL migrations
```

## Limitations and roadmap

- **data-to-video contract.** The sibling pipeline's `scripts/publish.py` (dry run by default) is written against a multipart `POST /v1/posts` that carries the video file and an `Idempotency-Key`. This service exposes JSON `POST /api/v1/external/posts` with a scoped API key and does not read `Idempotency-Key`, and videos enter through the upload API rather than in the post request. A video-ingest endpoint for API keys that honours the idempotency key is the next piece of work.
- **Real posting** needs your own developer app on each network, connected channels and usually app review (Meta for Instagram, Facebook and Threads publishing; LinkedIn product access; an X app with write scopes). Platform quotas apply.
- **No UI.** The API is the interface; OAuth callbacks redirect to `APP_FRONTEND_URL`, which you provide.
- **Single-instance assumptions.** Rate limiters, circuit breakers and the OAuth-state fallback are per process.
- **Retries** cover whole-post failure only; partially failed posts and failed video uploads need a manual re-queue.
- **Schema history.** This code was extracted from a larger product. The migrations still create tables for modules that were removed (marketplace, payouts, newsletters and others); no code reads or writes them, and migration `000019` is a no-op seed guard on a fresh database.
- **Docker path** not exercised in this revision (see Quickstart).

## Licence

[MIT](LICENSE) © 2026 Himanshu Mehta. Go dependencies are under their own licences (MIT, BSD and Apache-2.0; see `go.mod`). The container installs ffmpeg from Alpine packages under ffmpeg's licence. No models or datasets are bundled; generation calls the Claude API only when you configure a key.

---

Built by **Himanshu Mehta** — staff engineer, 10+ years building large-scale distributed systems at Outreach, Builder.ai and Twilio; now building agentic AI systems. [LinkedIn](https://www.linkedin.com/in/himanshu-mehta-29920525) · [GitHub](https://github.com/himanshumehta)

Related projects: [data-to-video](https://github.com/himanshumehta/data-to-video) · [social-publisher](https://github.com/himanshumehta/social-publisher) · [vehicle-image-studio](https://github.com/himanshumehta/vehicle-image-studio) · [multi-brand-design-system](https://github.com/himanshumehta/multi-brand-design-system)

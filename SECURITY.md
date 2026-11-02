# Security policy

## Reporting a vulnerability

Email **mehta.august@gmail.com** with a description, the affected commit or
version, and steps to reproduce. Please do not open a public issue for an
unfixed vulnerability. You should get an acknowledgement within a few days;
fixes land on `main`.

## Supported versions

Only `main` is supported. There are no release branches.

## Scope

In scope: the Go code in this repository (`cmd/`, `internal/`), the SQL
migrations, the Dockerfile and `docker-compose.yml`, and the CI workflow.

Out of scope: the social platforms' own APIs and OAuth implementations,
third-party dependencies (report those upstream; Dependabot and govulncheck
track them here), and deployments that run with `APP_ENV=development`.

## What the service protects

| Asset | Control |
|---|---|
| Platform OAuth tokens | AES-256-GCM at rest with a random nonce per value (`internal/service/token_crypto.go`). The key comes from `TOKEN_ENCRYPTION_KEY`, is required outside development and is validated at start-up; a bad key stops the process. `TOKEN_ENCRYPTION_KEY_PREVIOUS` supports rotation. `channels.token_encrypted` always reflects what is stored. |
| Tokens in logs and errors | Every log line, persisted error message and API error passes through `internal/redact`, which masks token, secret, code and signature parameters, JSON token fields and bearer values. |
| API access | HS256-only JWTs with mandatory `exp` and a secret of at least 32 characters; refresh tokens are random, stored as SHA-256, rotated on use, and reuse revokes the whole family. Machine clients use scoped API keys (stored as SHA-256, optional expiry, disabled with their owner). |
| Authorization | The router is a table of `(route, access level)`; `internal/router/router_test.go` checks every entry and pins the public surface. Posts are scoped to their author; the review gate (only admins approve, and scheduling cannot skip review) has DB-backed tests. The whole video path is admin-only. |
| Accounts | No public signup. Accounts and API keys are created with `cmd/admin`. Passwords use bcrypt cost 12; every failed login returns the same message in roughly the same time. The development admin bootstrap is refused outside `APP_ENV=development`. |
| Server-side requests | User-supplied URLs (source pages, post images, thumbnails) go through `internal/netguard`: http(s) only, no credentials, destination checked after DNS resolution and on every redirect against loopback, private, link-local, metadata and reserved ranges, with size caps. |
| HTTP surface | 1 MiB request bodies, per-IP rate limits (stricter on login and refresh), single-origin CORS, security headers and a `default-src 'none'` CSP, header and read timeouts. |
| Configuration | Fails closed: anything other than `APP_ENV=development` gets strict validation (no default JWT secret, encryption key required, TLS to Postgres, no wildcard CORS, no dev admin). |
| Publishing | `PUBLISH_DRY_RUN` defaults to `true`; real platform calls need an explicit `false`. |
| Container | Non-root user (uid 10001), multi-stage build, no build tools in the runtime image. |

## What it does not protect against

- **A compromised host or database.** The token key lives in the process
  environment; anyone who can read both the environment and the database can
  decrypt tokens. Use a secrets manager or KMS-wrapped key in production.
- **Malicious admins.** Admins can connect channels, approve posts and publish.
- **Multi-instance rate limiting.** Limits and the OAuth-state fallback (when
  Redis is unreachable) are per process.
- **Uploaded media content.** Videos and images are passed to the platforms
  as-is; there is no malware or content scanning.
- **Tables from removed modules.** The migrations still create tables for
  features that were removed from this codebase; no code reads or writes them.

## Verification

CI runs `go vet`, `staticcheck`, the test suite against Postgres,
`govulncheck`, `gosec` and `gitleaks` on every push and pull request.

# traces-observer-service — agent guide

Small Go service that serves the console's trace/observability API. It is **not** an OpenSearch client — it is an adapter that calls an upstream **Observer service** over HTTP, then enriches and reshapes the responses. Built on the **stdlib `net/http`** (no Gin), structured logging with **`slog`**, and only one third-party dep: `github.com/golang-jwt/jwt/v5`.

## Request flow

```
HTTP → middleware (Logger → CORS → JWTAuth) → handlers/ → controllers/ → observer/ (HTTP client) → Observer service
                                                                        ↘ opensearch/ (span parsing / enrichment)
```

- **`handlers/`** — parse/validate the request, extract path params, call the controller, write the response. Client-facing errors are generic (`"Failed to retrieve …"`); real detail is logged server-side.
- **`controllers/`** — orchestration + enrichment. Fetches trace overviews, then fans out to fetch span details and aggregates input/output/token usage.
- **`observer/`** — the typed HTTP client to the upstream Observer (`QueryTraces`, `QueryTraceSpans`, `GetSpanDetails`), plus auth token management and response→`opensearch.Span` conversion.
- **`opensearch/`** — pure span-parsing logic. Extracts input/output from spans; branches by vendor (CrewAI via `crewai.*` attributes vs LangChain/Traceloop via `traceloop.entity.*`).
- **`config/`** — env-var config loading + startup validation.

## File map

| Need | Location |
|---|---|
| Entry point / server + route table | `main.go` |
| HTTP handlers, path parsing | `handlers/handlers.go` |
| Orchestration + enrichment | `controllers/controller.go` |
| Observer client (interface + impl) | `observer/client.go`, `observer/types.go` |
| Observer auth (token cache + retry) | `observer/auth.go` |
| Response conversion | `observer/convert.go` |
| Span parsing / vendor branches | `opensearch/process.go`, `opensearch/crewai_process.go` |
| JWT / CORS / request logging | `middleware/` |
| Config + validation | `config/config.go` |

## Routing

Plain `http.ServeMux` in `main.go`. Dynamic segments (`/api/v1/traces/{traceId}/spans/{spanId}`) are parsed **by hand** with `strings.CutPrefix`/`Index` in the handlers — `pathSegment()` rejects any segment containing `/` (path-traversal guard). When you add a nested route, extend that manual dispatch; there is no path-param router.

Middleware wraps in this order (outer→inner): `RequestLogger → CORS → JWTAuth → handler`. Keep it — CORS must see the request before auth rejects it, and the logger must wrap both.

## Commands

```bash
make run          # hot-reload via air (go install github.com/air-verse/air@latest)
make build        # go build -o amp-traces-observer .
make test         # bash scripts/run_tests.sh
make test-cover   # coverage + HTML report
make fmt          # gofmt -w .
make lint         # golangci-lint run ./...
make mock-traces  # generate OTLP traces via telemetrygen (for local testing)
```

`.air.toml` watches `.go` and `.env` files, so editing `.env` reloads the app.

## Config

`config.Load()` reads env vars with defaults and validates at startup (`config/config.go`):

| Var | Purpose |
|---|---|
| `TRACES_OBSERVER_PORT` | listen port (default 9098) |
| `OBSERVER_BASE_URL`, `IDP_TOKEN_URL`, `IDP_CLIENT_ID`, `IDP_CLIENT_SECRET` | upstream Observer + its OAuth2 creds (all required together once `OBSERVER_BASE_URL` is set) |
| `KEY_MANAGER_JWKS_URL`, `KEY_MANAGER_ISSUER`, `KEY_MANAGER_AUDIENCE` | JWT validation (required unless local dev) |
| `IS_LOCAL_DEV_ENV` | `true` skips JWT signature validation (checks expiry only) |
| `LOG_LEVEL` | DEBUG/INFO/WARN/ERROR |

Co-dependent values are checked together — a partially-set Observer or Auth config fails at startup, not first request. `getEnvAsList` parses comma-separated issuer/audience lists.

## Auth

`middleware.JWTAuth` validates `Authorization: Bearer` against JWKS (RSA), caching keys in-memory with a 1-hour TTL and force-refreshing on `kid` mismatch. It checks issuer, audience, and a publisher-audience regex (`amp-publisher-[a-zA-Z0-9][a-zA-Z0-9._-]*`). In `IS_LOCAL_DEV_ENV=true` mode signature checks are skipped.

The **observer client** holds its own OAuth2 client-credentials token (cached with a 30s expiry buffer). On a `401` it invalidates the token and retries once, falling back from Basic-auth to POST-form auth for Keycloak compatibility.

## Engineering rules (as practiced here)

- **Errors** — wrap with a `pkg.Func:` prefix and `%w`: `fmt.Errorf("observer.QueryTraces: %w", err)`. Never return a nil context.
- **Context** — every call takes `ctx` first and propagates it; handlers pass `r.Context()` down. Passing `nil` panics.
- **Logging** — `slog` (JSON). Request-scoped logger via `logger.WithLogger`/`GetLogger(ctx)`; log with fields (`method`, `path`, `status`, `duration`). Upstream partial failures are logged as warnings, not fatal.
- **Concurrency in enrichment** — the controller uses a **two-tier semaphore** (outer: max 10 concurrent traces; inner: max 50 concurrent span fetches). Do not collapse to one pool — it prevents deadlock. Enrichment short-circuits once fields are filled, skips leaf aggregation above 100 spans, and caps leaf fetches at 50 (`TokenUsage.Partial=true` when truncated). The export path uses `context.WithCancel` + `atomic.Bool` for fail-fast.

## Gotchas

- Service **cannot run in isolation** — it needs a reachable Observer (`OBSERVER_BASE_URL` + IDP creds). For local work set `IS_LOCAL_DEV_ENV=true`.
- The old README mentions OpenSearch directly; the code does **not** talk to OpenSearch — the `opensearch/` package is only span-parsing types/logic.
- Tests set/unset env vars themselves (`config_test.go` pattern: set → run → `defer` unset).

## Done checklist

- [ ] `make test` passes.
- [ ] `make lint` (`golangci-lint run ./...`) clean.
- [ ] `make fmt` applied.
- [ ] New I/O paths take and propagate `context.Context`; errors wrapped with `%w`.

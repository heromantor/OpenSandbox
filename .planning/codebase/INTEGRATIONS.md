# External Integrations

**Analysis Date:** 2026-04-04

## APIs & External Services

**OpenSandbox Lifecycle API:**
- Service - Primary API for sandbox lifecycle management (create, delete, list)
  - SDK/Client (Go): `LifecycleClient` at `sdks/sandbox/go/opensandbox/lifecycle.go`
  - SDK/Client (Python): Adapters at `sdks/sandbox/python/src/opensandbox/adapters/sandboxes_adapter.py`
  - SDK/Client (JavaScript): `LifecycleClient` at `sdks/sandbox/javascript/src/openapi/lifecycleClient.ts`
  - Auth: Header `OPEN-SANDBOX-API-KEY` (environment var `OPEN_SANDBOX_API_KEY`)

**OpenSandbox Execd API:**
- Service - Code execution API within sandboxes
  - SDK/Client (Go): `ExecdClient` at `sdks/sandbox/go/opensandbox/execd.go`
  - SDK/Client (Python): `ExecdClient` adapter at `sdks/sandbox/python/src/opensandbox/adapters/`
  - SDK/Client (JavaScript): `ExecdClient` at `sdks/sandbox/javascript/src/openapi/execdClient.ts`
  - Port: 44772 (default, configurable per sandbox)
  - Auth: Token-based (obtained from sandbox creation response)

**OpenSandbox Egress API:**
- Service - Outbound network requests from sandboxes
  - SDK/Client (Go): `EgressClient` at `sdks/sandbox/go/opensandbox/egress.go`
  - SDK/Client (Python): `EgressAdapter` at `sdks/sandbox/python/src/opensandbox/adapters/egress_adapter.py`
  - SDK/Client (JavaScript): `EgressClient` at `sdks/sandbox/javascript/src/openapi/egressClient.ts`
  - Port: 18080 (default, sidecar in sandbox)
  - Auth: Header `OPENSANDBOX-EGRESS-AUTH` (token-based)

## Data Storage

**Databases:**
- Not applicable - SDKs are client libraries only

**File Storage:**
- Local filesystem only - SDKs manage file I/O to sandbox container directories
  - Python: `FileSystemAdapter` at `sdks/sandbox/python/src/opensandbox/adapters/filesystem.py`
  - Go: File operations at `sdks/sandbox/go/opensandbox/sandbox_files.go`
  - JavaScript: File operations at `sdks/sandbox/javascript/src/models/filesystem.ts`

**Caching:**
- None - SDKs make direct API requests without caching layer

## Authentication & Identity

**Auth Provider:**
- Custom API Key authentication
  - Implementation: Bearer token in HTTP headers
  - Environment variable: `OPEN_SANDBOX_API_KEY`
  - Header name: `OPEN-SANDBOX-API-KEY` (customizable via `AuthHeader` field in `ConnectionConfig`)

**Token Management:**
- Sandbox-scoped tokens obtained from lifecycle API responses
- Used for subsequent execd/egress requests to specific sandboxes
- Tokens are generated server-side, consumed by SDKs

## Monitoring & Observability

**Error Tracking:**
- None built-in - SDKs return error types/exceptions to caller
  - Go: `APIError` type at `sdks/sandbox/go/opensandbox/errors.go`
  - Python: `SandboxException` hierarchy at `sdks/sandbox/python/src/opensandbox/exceptions/`
  - JavaScript: Exception handling in adapters

**Logs:**
- Debug logging support via `debug` flag in `ConnectionConfig`:
  - Go: Custom logging via headers/context
  - Python: Python logging framework (stdout via `debug=True`)
  - JavaScript: Console logging when `debug: true` in config
- Request/response logging available when debug mode enabled

## CI/CD & Deployment

**Hosting:**
- Not applicable - SDKs are client libraries deployed as packages in target environments

**CI Pipeline:**
- Go: Local `make test`, `make test-integration`, `make test-staging`
- Python: `uv` based CI pipeline via Makefile targets (`make ci`)
- JavaScript: `pnpm` based pipeline with build, lint, typecheck steps

## Environment Configuration

**Required env vars:**
- `OPEN_SANDBOX_API_KEY` - API authentication token (required for API access)
- `OPEN_SANDBOX_DOMAIN` - Server address (optional, defaults to `localhost:8080`)
- `OPEN_SANDBOX_PROTOCOL` - Protocol http/https (optional, defaults to `http`)

**Optional env vars:**
- Python SDK creation: `UV_*` env vars for `uv` package manager
- Test environment: Tags like `integration`, `staging` control test suite execution

**Secrets location:**
- Environment variables only - never hardcoded in SDKs
- `.env` files supported by SDKs (Python/JavaScript can read via `os.getenv()` or `process.env`)
- Note: Do NOT commit `.env` files containing `OPEN_SANDBOX_API_KEY`

## Webhooks & Callbacks

**Incoming:**
- None - SDKs are request-response only

**Outgoing:**
- None - SDKs do not initiate outbound webhooks
- Egress API used for controlled sandbox outbound network access only

## HTTP Configuration

**Connection Management:**
- Go SDK (`sdks/sandbox/go/opensandbox/`):
  - Default transport with connection pooling: `DefaultTransport()` at `transport.go`
  - Configurable pool: `TransportConfig` struct allows tuning max idle connections, keep-alive, timeouts
  - TLS 1.2+ enforcement
  - Keep-alive timeout: 30 seconds (default)
  - Max idle connections: 100 (default)
  - Max idle connections per host: 10 (default)

- Python SDK:
  - Uses `httpx.AsyncHTTPTransport` with connection pooling
  - Default limits: max_connections=100, max_keepalive_connections=20, keepalive_expiry=30.0
  - Configurable via `transport` parameter in `ConnectionConfig`

- JavaScript SDK:
  - Node.js: Uses `undici` HTTP Agent with keep-alive
  - Browser: Uses native fetch API
  - Keep-alive timeout: 30 seconds (default)
  - Request timeout: 30 seconds (configurable)
  - Supports timeout abort signals

**Retry & Resilience:**
- Go SDK: `RetryConfig` at `sdks/sandbox/go/opensandbox/retry.go`
  - Retries on: 429 (rate limit), 502, 503, 504 errors
  - Exponential backoff strategy
  - Optional retry configuration: `Retry *RetryConfig` in `ConnectionConfig`

- Python SDK: No built-in retry logic (can be added via transport)

- JavaScript SDK: No built-in retry logic (can be added via fetch wrapper)

**Request/Response Formats:**
- All SDKs: JSON request/response bodies
- Content-Type: `application/json` for regular requests, `text/event-stream` for SSE
- Custom headers: All SDKs support additional headers via `Headers` map in `ConnectionConfig`

## Endpoint Resolution

**Server Proxy Mode:**
- Go: `UseServerProxy` flag routes execd/egress requests through sandbox server instead of direct connection
  - Useful when client cannot reach sandbox endpoint directly
  - Header rewriting: `EndpointHostRewrite` map for Docker host resolution (`host.docker.internal` → `localhost`)
  - See `sdks/sandbox/go/opensandbox/config.go` for implementation

- Python: `use_server_proxy` boolean flag in `ConnectionConfig`
  - Server proxy routes are applied transparently

- JavaScript: `useServerProxy` flag in `ConnectionConfigOptions`
  - Transparent proxy routing for endpoint requests

---

*Integration audit: 2026-04-04*

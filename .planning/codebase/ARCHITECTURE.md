# Architecture

**Analysis Date:** 2026-04-04

## Pattern Overview

**Overall:** Three-layer HTTP client architecture with composition-based delegation

**Key Characteristics:**
- Modular API clients (Lifecycle, Execd, Egress) built on a shared HTTP transport foundation
- High-level `Sandbox` wrapper that lazily resolves and delegates to specialized clients
- Configuration-driven client initialization with environment variable fallbacks
- Streaming-first design for long-running code/command execution (Server-Sent Events + NDJSON)
- Retry logic and connection pooling abstracted into reusable transport layer

## Layers

**HTTP Transport Layer:**
- Purpose: Provides low-level HTTP request/response handling, retry logic, and connection pooling
- Location: `sdks/sandbox/go/opensandbox/http.go`, `transport.go`, `retry.go`
- Contains: `Client` base class, `Option` functional configuration, request serialization, error handling, retry policies
- Depends on: Standard library (`net/http`, `encoding/json`)
- Used by: LifecycleClient, ExecdClient, EgressClient

**API Client Layer:**
- Purpose: Implement domain-specific API operations (lifecycle management, code execution, file operations, egress control)
- Location: 
  - `sdks/sandbox/go/opensandbox/lifecycle.go` - Sandbox creation, status, endpoints
  - `sdks/sandbox/go/opensandbox/execd.go` - Code/command execution, file I/O, metrics
  - `sdks/sandbox/go/opensandbox/egress.go` - Network policy management
- Contains: Individual client types wrapping the base `Client`, method signatures for each API endpoint
- Depends on: HTTP Transport Layer
- Used by: Sandbox wrapper, CodeInterpreter, SandboxManager

**High-Level SDK Layer:**
- Purpose: Provide ergonomic, user-facing APIs that hide endpoint resolution and client coordination
- Location: 
  - `sdks/sandbox/go/opensandbox/sandbox.go` - Core Sandbox type with lifecycle methods
  - `sdks/sandbox/go/opensandbox/sandbox_exec.go` - Command/code execution on Sandbox
  - `sdks/sandbox/go/opensandbox/sandbox_files.go` - File operations on Sandbox
  - `sdks/sandbox/go/opensandbox/sandbox_egress.go` - Egress policy on Sandbox
  - `sdks/sandbox/go/opensandbox/manager.go` - Administrative operations
  - `sdks/sandbox/go/opensandbox/code_interpreter.go` - Specialized wrapper for code execution
- Contains: `Sandbox`, `CodeInterpreter`, `SandboxManager` types, convenience methods
- Depends on: API Client Layer, Config Layer
- Used by: User applications

**Configuration & Types Layer:**
- Purpose: Configuration management, type definitions, streaming event handling
- Location: 
  - `sdks/sandbox/go/opensandbox/config.go` - Connection configuration with env var resolution
  - `sdks/sandbox/go/opensandbox/types.go` - Data model types (SandboxInfo, Execution types, Endpoint, etc.)
  - `sdks/sandbox/go/opensandbox/execution.go` - Execution result accumulator and SSE event processing
  - `sdks/sandbox/go/opensandbox/streaming.go` - Server-Sent Event parser
  - `sdks/sandbox/go/opensandbox/constants.go` - Default values and resource limits
  - `sdks/sandbox/go/opensandbox/errors.go` - Custom error types
- Contains: Type definitions, config builders, constants, error types
- Depends on: Standard library
- Used by: All other layers

## Data Flow

**Sandbox Creation Flow:**

1. User calls `CreateSandbox(ctx, config, options)` in `sandbox.go`
2. Config builds LifecycleClient (via `config.lifecycleClient()`)
3. LifecycleClient.CreateSandbox(req) sends HTTP POST to `/sandboxes`
4. Sandbox polls `/sandboxes/{id}` until Running (via `waitForRunning`)
5. Sandbox resolves execd endpoint via `resolveExecd()` using `GetEndpoint` API
6. Sandbox waits until execd responds to `/ping` via `WaitUntilReady`
7. Sandbox returned ready for use

**Code Execution Flow:**

1. User calls `sandbox.ExecuteCode(ctx, req, handlers)` in `sandbox_exec.go`
2. Sandbox ensures ExecdClient is initialized (lazy init in `resolveExecd`)
3. ExecdClient.ExecuteCode opens SSE stream via `doStreamRequest`
4. streamSSE in `streaming.go` parses incoming events (handles both standard SSE and NDJSON)
5. Each event calls `processStreamEvent` in `execution.go` which:
   - Parses JSON event payload (init, stdout, stderr, result, error, complete)
   - Accumulates into Execution struct
   - Invokes user-provided handlers if set
6. Stream completes, Execution returned with full structured result

**Configuration Resolution Flow:**

1. User creates ConnectionConfig with partial fields
2. Getter methods (GetDomain, GetProtocol, GetAPIKey) resolve fallback chain:
   - Explicit config field → environment variable → default constant
3. Factory methods (lifecycleClient, execdClient, egressClient) apply Options to create typed clients
4. Options pattern allows composition: retry config, custom headers, HTTP client, timeout

## Key Abstractions

**Sandbox:**
- Purpose: Represents a single running sandbox with all three API clients (lifecycle, execd, egress)
- Examples: `sdks/sandbox/go/opensandbox/sandbox.go`, `sandbox_exec.go`, `sandbox_files.go`, `sandbox_egress.go`
- Pattern: Lazy initialization of execd/egress clients; mutex-protected endpoint resolution; delegation to specialized clients

**Client (Base HTTP Client):**
- Purpose: Shared HTTP transport, request/response marshalling, retry orchestration, header management
- Examples: `sdks/sandbox/go/opensandbox/http.go`
- Pattern: Generic handler methods (doRequest, doStreamRequest) with Option-based configuration; retry wrapper via withRetry

**ExecutionHandlers & Execution:**
- Purpose: Streaming event processing pipeline for code/command execution
- Examples: `sdks/sandbox/go/opensandbox/execution.go`, `streaming.go`
- Pattern: Handler functions invoked on each event; Execution accumulator collects all events into typed results

**ConnectionConfig:**
- Purpose: Environment-aware configuration with sensible fallbacks
- Examples: `sdks/sandbox/go/opensandbox/config.go`
- Pattern: Getter methods resolve config → env → default; factory methods create typed clients with Options

## Entry Points

**CreateSandbox:**
- Location: `sdks/sandbox/go/opensandbox/sandbox.go:78`
- Triggers: Application creates new isolated execution environment
- Responsibilities: Validates image, calls lifecycle API, polls until Running, resolves endpoints, waits for readiness

**ConnectSandbox:**
- Location: `sdks/sandbox/go/opensandbox/sandbox.go:156`
- Triggers: Application reconnects to existing sandbox by ID
- Responsibilities: Resolves execd endpoint, optionally waits for readiness

**CreateCodeInterpreter:**
- Location: `sdks/sandbox/go/opensandbox/code_interpreter.go:52`
- Triggers: Application needs multi-language code execution with state persistence
- Responsibilities: Wraps CreateSandbox with code-interpreter image/entrypoint defaults

**NewSandboxManager:**
- Location: `sdks/sandbox/go/opensandbox/manager.go:15`
- Triggers: Application performs administrative operations (list, pause, resume, renew)
- Responsibilities: Provides access to lifecycle API without connecting to a specific sandbox

## Error Handling

**Strategy:** Three-level error classification: API errors (status codes), transient vs permanent, custom SDK errors

**Patterns:**

- **APIError:** Wraps HTTP error responses with status code, request ID, and Retry-After header
  - Classified by `IsTransient()`: true for 429, 502, 503, 504; false for 400, 401, 403, 404, etc.
  - Network errors classified as transient via `net.Error` interface
  - `sdks/sandbox/go/opensandbox/types.go:184`, `retry.go:79`

- **Custom SDK Errors:** Type-specific errors for SDK invariants
  - `SandboxReadyTimeoutError`: WaitUntilReady timeout (unwrap chain preserved via `Unwrap()`)
  - `SandboxUnhealthyError`: Sandbox health check failures
  - `InvalidArgumentError`: Missing required parameters
  - `sdks/sandbox/go/opensandbox/errors.go`

- **Retry Logic:** Transient errors automatically retried with exponential backoff + jitter
  - Controlled by `RetryConfig`: max retries, initial/max backoff, multiplier, jitter fraction
  - `DefaultRetryConfig()` provides sensible defaults: 3 retries, 500ms initial, 2x multiplier, 30s cap
  - `sdks/sandbox/go/opensandbox/retry.go:36`

## Cross-Cutting Concerns

**Logging:** Not present; relies on context propagation and error return values

**Validation:** Input validation in sandbox creation (required Image field), path escaping in URL construction, enum validation via Go's type system

**Authentication:** 
- Lifecycle API: `OPEN-SANDBOX-API-KEY` header (or custom via `AuthHeader`) with API key from config
- Execd API: `X-EXECD-ACCESS-TOKEN` header with token from endpoint resolution
- Egress API: `OPENSANDBOX-EGRESS-AUTH` header with token from endpoint resolution
- Server proxy routing: When `UseServerProxy=true`, all requests routed through server with server's API key
- `sdks/sandbox/go/opensandbox/config.go:167`, `sandbox.go:353`

**Concurrency:**
- Endpoint resolution protected by mutex to avoid duplicate resolutions: `sdks/sandbox/go/opensandbox/sandbox.go:323`
- HTTP client pooling via `TransportConfig` with configurable idle connection limits
- Streaming event handlers called sequentially per stream (no concurrent handler invocation)

**Streaming:**
- SSE + NDJSON hybrid support in `streamSSE()`: handles standard "data: ..." lines and raw JSON blobs
- Large buffer support (4MiB) for execd output events
- Context cancellation checked on each iteration (responsive to ctx.Done())
- `sdks/sandbox/go/opensandbox/streaming.go:31`

---

*Architecture analysis: 2026-04-04*

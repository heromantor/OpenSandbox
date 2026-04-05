# Codebase Structure

**Analysis Date:** 2026-04-04

## Directory Layout

```
OpenSandbox/
├── .git/                           # Git repository
├── .planning/
│   └── codebase/                   # Analysis documents (ARCHITECTURE.md, STRUCTURE.md, etc.)
├── sdks/
│   └── sandbox/                    # SDK implementations for multiple languages
│       ├── csharp/                 # C# SDK
│       ├── go/                     # Go SDK (primary focus)
│       │   ├── opensandbox/        # Main package with all client code
│       │   ├── examples/           # Example applications
│       │   ├── go.mod              # Go module definition
│       │   └── go.sum              # Dependency lock file
│       ├── javascript/             # Node.js/TypeScript SDK
│       └── python/                 # Python SDK
├── RFC-GO-SDK-TEST-GAPS.md         # RFC documenting test coverage gaps
├── TEST-REPORT.md                  # Test execution report and results
```

## Directory Purposes

**sdks/sandbox/go/opensandbox/:**
- Purpose: Core SDK implementation - all public API types, clients, and business logic
- Contains: 23 .go source files organized by concern
- Key files: `sandbox.go`, `config.go`, `types.go`, `http.go`, `execution.go`

**sdks/sandbox/go/opensandbox/api/:**
- Purpose: Generated API stubs from OpenAPI specifications (generated, not hand-written)
- Contains: Three subdirectories for each API domain
- Generated via: `oapi-codegen` from YAML specs; run via `make generate` or `go generate ./...`

**sdks/sandbox/go/opensandbox/api/lifecycle/:**
- Purpose: Generated types and stubs for Lifecycle API (sandbox CRUD)
- Contains: `gen.go` (generated code), `generate.go` (directive)
- Used by: `LifecycleClient` wraps the generated operations

**sdks/sandbox/go/opensandbox/api/execd/:**
- Purpose: Generated types and stubs for Execd API (execution, files, metrics)
- Contains: `gen.go` (generated code), `generate.go` (directive)
- Used by: `ExecdClient` wraps the generated operations

**sdks/sandbox/go/opensandbox/api/egress/:**
- Purpose: Generated types and stubs for Egress API (network policy)
- Contains: `gen.go` (generated code), `generate.go` (directive)
- Used by: `EgressClient` wraps the generated operations

**sdks/sandbox/go/examples/:**
- Purpose: Reference applications demonstrating SDK usage patterns
- Contains: Two example applications with main.go
- Structure:
  - `agent_loop/` - Agent loop pattern with streaming execution
  - `code_interpreter_agent/` - Code interpreter example

## Key File Locations

**Entry Points:**
- `sdks/sandbox/go/opensandbox/sandbox.go`: CreateSandbox, ConnectSandbox, ResumeSandbox (lines 78-190)
- `sdks/sandbox/go/opensandbox/code_interpreter.go`: CreateCodeInterpreter (line 52)
- `sdks/sandbox/go/opensandbox/manager.go`: NewSandboxManager (line 15)

**Configuration:**
- `sdks/sandbox/go/opensandbox/config.go`: ConnectionConfig type, getters, client factories
- `sdks/sandbox/go/opensandbox/constants.go`: Defaults for timeouts, ports, resource limits, domain

**Core Logic:**
- `sdks/sandbox/go/opensandbox/sandbox.go`: Sandbox lifecycle and coordination
- `sdks/sandbox/go/opensandbox/sandbox_exec.go`: Command/code execution methods on Sandbox
- `sdks/sandbox/go/opensandbox/sandbox_files.go`: File operation methods on Sandbox
- `sdks/sandbox/go/opensandbox/sandbox_egress.go`: Egress policy methods on Sandbox
- `sdks/sandbox/go/opensandbox/lifecycle.go`: LifecycleClient implementation
- `sdks/sandbox/go/opensandbox/execd.go`: ExecdClient implementation (14KB file, see code for all methods)
- `sdks/sandbox/go/opensandbox/egress.go`: EgressClient implementation
- `sdks/sandbox/go/opensandbox/manager.go`: SandboxManager for admin operations

**HTTP & Transport:**
- `sdks/sandbox/go/opensandbox/http.go`: Base Client, doRequest, doStreamRequest, error handling
- `sdks/sandbox/go/opensandbox/transport.go`: TransportConfig for connection pooling
- `sdks/sandbox/go/opensandbox/retry.go`: RetryConfig, retry logic, exponential backoff

**Streaming & Execution:**
- `sdks/sandbox/go/opensandbox/streaming.go`: streamSSE parser (SSE + NDJSON hybrid support)
- `sdks/sandbox/go/opensandbox/execution.go`: Execution type, ExecutionHandlers, event processing
- `sdks/sandbox/go/opensandbox/code_interpreter.go`: CodeInterpreter wrapper

**Data Models:**
- `sdks/sandbox/go/opensandbox/types.go`: SandboxInfo, SandboxStatus, ImageSpec, Volume types, error types
- `sdks/sandbox/go/opensandbox/errors.go`: Custom error types (SandboxReadyTimeoutError, etc.)

**Testing:**
- `sdks/sandbox/go/opensandbox/opensandbox_test.go`: Unit tests for core Sandbox functionality (2251 lines)
- `sdks/sandbox/go/opensandbox/integration_test.go`: Integration tests against live server (1644 lines)
- `sdks/sandbox/go/opensandbox/staging_test.go`: Tests against staging environment (638 lines)
- `sdks/sandbox/go/opensandbox/retry_test.go`: Retry logic tests (557 lines)

## Naming Conventions

**Files:**
- `{feature}.go`: Core implementation (e.g., `sandbox.go`, `config.go`)
- `sandbox_{subsystem}.go`: Sandbox methods grouped by subsystem (e.g., `sandbox_exec.go` for execution)
- `{type}_test.go`: Test file for corresponding implementation file (e.g., `opensandbox_test.go` for integration)
- `generate.go`: Marker file for `go generate` directives (in api/*/generate.go)

**Directories:**
- `opensandbox/`: Lowercase package name following Go conventions
- `api/{domain}/`: Lowercase domain name (lifecycle, execd, egress)
- `examples/{pattern}/`: Lowercase pattern name (agent_loop, code_interpreter_agent)

**Types:**
- Exported (public): PascalCase (e.g., `Sandbox`, `CreateSandboxRequest`, `SandboxInfo`)
- Private: camelCase (e.g., `resolveExecd`, `sseEvent`)
- Constants: UPPER_SNAKE_CASE (e.g., `DefaultExecdPort`, `DefaultTimeoutSeconds`)

**Methods:**
- Exported: PascalCase (e.g., `CreateSandbox`, `ExecuteCode`)
- Private: camelCase (e.g., `waitForRunning`, `doRequest`)
- Factory methods: `New{Type}` (e.g., `NewSandbox`, `NewLifecycleClient`)

## Where to Add New Code

**New Sandbox Lifecycle Feature (pause, resume, renew):**
- Primary code: `sdks/sandbox/go/opensandbox/sandbox.go`
- Delegate to: Add method to `LifecycleClient` in `lifecycle.go` if API call needed
- Test: Add test to `opensandbox_test.go`
- Pattern: Public method on Sandbox calls client method on s.lifecycle, returns typed result

**New Execution Feature (new command type or streaming handler):**
- Primary code: Create `sandbox_{feature}.go` or add to existing `sandbox_exec.go`
- Delegate to: Add method to `ExecdClient` in `execd.go`
- Streaming handler: Extend `sseEvent` struct in `execution.go` and `processStreamEvent` function
- Test: Add test case to `opensandbox_test.go` and/or `integration_test.go`
- Pattern: Public Sandbox method → ExecdClient method → stream processing via handlers

**New File Operation:**
- Primary code: `sdks/sandbox/go/opensandbox/sandbox_files.go`
- Delegate to: Add method to `ExecdClient` in `execd.go` (likely multipart upload/download handling)
- Test: Add to `opensandbox_test.go` with file I/O assertions
- Pattern: Check s.execd != nil, call client method, return result

**New Egress Feature (network policy rules, custom headers):**
- Primary code: `sdks/sandbox/go/opensandbox/sandbox_egress.go` for Sandbox methods
- Delegate to: Extend `EgressClient` in `egress.go`
- Model types: Add to `NetworkRule`, `NetworkPolicy` in `types.go`
- Test: Add test to `opensandbox_test.go` with policy assertions
- Pattern: Lazy resolve egress endpoint (resolveEgress), call egress.Method

**New Configuration Option:**
- Primary code: Add field to `ConnectionConfig` in `config.go`
- Getter: Add `Get{FieldName}()` method with env var fallback
- Propagation: Update `clientOpts()` method to pass to Option if needed
- Test: Add env var override test to `opensandbox_test.go`
- Pattern: Explicit field → getter with env fallback → applied via Options

**Utilities (retry backoff, error classification, etc.):**
- Shared: `sdks/sandbox/go/opensandbox/` root package
- Network utilities: `transport.go`
- Retry utilities: `retry.go`
- Streaming utilities: `streaming.go`
- Pattern: Package-level exported functions, no new types unless needed

## Special Directories

**sdks/sandbox/go/opensandbox/api/:**
- Purpose: Generated API bindings from OpenAPI specifications
- Generated: Yes (via oapi-codegen)
- Committed: Yes (committed to maintain reproducible builds)
- Regenerate: Run `make generate` or `go generate ./...` after updating YAML specs

**sdks/sandbox/go/examples/:**
- Purpose: Reference code demonstrating SDK patterns
- Committed: Yes (part of SDK documentation)
- Usage: Copy/adapt for new patterns; don't use as tests (see *_test.go files instead)

**.planning/codebase/:**
- Purpose: Living analysis documents for this codebase
- Committed: Yes (supports GSD command orchestration)
- Updated: Regularly via `/gsd-map-codebase` with focus areas (arch, tech, quality, concerns)

---

*Structure analysis: 2026-04-04*

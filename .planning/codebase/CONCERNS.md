# Codebase Concerns

**Analysis Date:** 2026-04-04

## Test Coverage Gaps

**SandboxManager E2E Testing:**
- What's not tested: `SandboxManager` has complete implementation in `sdks/sandbox/go/opensandbox/manager.go` but 0% test coverage
- Files: `sdks/sandbox/go/opensandbox/manager.go`, `sdks/sandbox/go/opensandbox/opensandbox_test.go`
- Risk: Manager operations (list with filters, kill, pause, resume) are untested — could mask breaking changes during code reviews or refactors
- Priority: **High** — every other SDK (Python, JS, C#, Java/Kotlin) tests manager e2e. Go SDK gap is visible to reviewers.
- Fix approach: Add unit tests for `ListSandboxInfos()` with filters, `KillSandbox()`, `PauseSandbox()`, `ResumeSandbox()` and integration test exercising full manager workflow (filter, paginate, kill)

**File Operations Coverage:**
- What's not tested: 8/8 file operation endpoints have no e2e coverage — only unit tests exist for `GetFileInfo` and `UploadFile`
- Files: `sdks/sandbox/go/opensandbox/sandbox_files.go`, `sdks/sandbox/go/opensandbox/integration_test.go`
- Endpoints untested: `CreateDirectory`, `DeleteDirectory`, `DeleteFiles`, `MoveFiles`, `SearchFiles`, `SetPermissions`, `ReplaceInFiles`, `DownloadFile`
- Risk: File write/delete/move operations may have silent failures or incorrect serialization. Integration tests would catch server incompatibilities.
- Priority: **High** — core SDK use case, Python/JS/C#/Java all test these
- Fix approach: Add unit tests (mock server) for each write operation, then integration test creating dir → uploading → moving → searching → replacing → downloading → deleting

**CodeInterpreter E2E Testing:**
- What's not tested: `CodeInterpreter` fully implemented in `sdks/sandbox/go/opensandbox/code_interpreter.go` but 0% coverage
- Files: `sdks/sandbox/go/opensandbox/code_interpreter.go`, `sdks/sandbox/go/opensandbox/execution.go`
- Risk: Code execution streaming, context persistence, error handling during execution untested — execution failures could occur silently or crash the SDK
- Priority: **Medium** — feature exists and is API-complete, but untested. Python/JS/C#/Java all test this.
- Fix approach: Unit test SSE parsing for code execution, integration test creating interpreter → executing Python → verifying stdout/stderr/results

**Sessions E2E Testing:**
- What's not tested: `CreateSession`, `RunInSession`, `DeleteSession` implemented but untested
- Files: `sdks/sandbox/go/opensandbox/sandbox_exec.go` (lines 65-91), `sdks/sandbox/go/opensandbox/integration_test.go`
- Risk: Stateful session handling (env persistence across commands) not verified. Silent failures possible.
- Priority: **Low** — no other SDK tests sessions in e2e, but Go SDK implements the API
- Fix approach: Integration test creating session → exporting env var → reading env var in next command → deleting session

**Command Management E2E Testing:**
- What's not tested: Background command operations (`GetCommandStatus`, `GetCommandLogs`, `InterruptCommand`)
- Files: `sdks/sandbox/go/opensandbox/execd.go` (generated API)
- Risk: Command interruption and log retrieval flows untested
- Priority: **Low** — niche use case, no other SDK tests this in e2e
- Fix approach: Integration test spawning long command → polling status → retrieving logs → interrupting

**Metrics Watch SSE:**
- What's not tested: Real-time metrics streaming via SSE
- Files: `sdks/sandbox/go/opensandbox/execd.go`, `sdks/sandbox/go/opensandbox/streaming.go`
- Risk: SSE streaming for metrics may have edge cases in buffer handling or event parsing
- Priority: **Low** — specialized use case, no other SDK tests this
- Fix approach: Unit test with mock SSE server, integration test collecting metric events

---

## Fragile Areas

**Sandbox Endpoint Resolution (`sandbox.go:321-395`):**
- Files: `sdks/sandbox/go/opensandbox/sandbox.go` lines 321-395 (`resolveExecd`, `resolveEgress`)
- Why fragile: Both methods use mutex-guarded initialization pattern, but:
  - If `GetEndpoint` fails, subsequent calls see `s.execd == nil` but no logged indication of why
  - Token extraction from `endpoint.Headers` is silent if token is missing (falls back to API key)
  - String prefix checking for `http`/`https` is brittle (lines 337-338, 377-378) — doesn't handle `localhost:port` properly
  - Header filtering (lines 347-350, 385-388) duplicates logic for execd and egress
- Safe modification: Extract `resolveClient` helper for both execd and egress; add logging for missing tokens; validate endpoint URLs more robustly
- Test coverage: `resolveExecd` partially covered in integration tests; `resolveEgress` only via egress method calls

**Execution Stream Parsing (`execution.go:125-236`):**
- Files: `sdks/sandbox/go/opensandbox/execution.go`
- Why fragile: Complex JSON unmarshaling with backward compatibility paths:
  - Line 132: Silent JSON parse failure → treats as raw stdout (may hide protocol mismatches)
  - Lines 182-190: Prefers nested error object but silently falls back to flat fields; no validation that both aren't set
  - Line 199: `strconv.Atoi(evalue)` parses exit code from error value — side effect, no validation
  - Line 213: Assumes exit code 0 if no error and no explicit code — implicit assumption
  - Lines 221-232: Unknown event types treated as stdout — may swallow server-sent debug/meta events
- Safe modification: Log JSON parse failures with event data; validate error object consistency; make exit code inference explicit; define event type whitelist
- Test coverage: Unit tests cover main paths; edge cases (malformed JSON, conflicting nested/flat fields) untested

**HTTP Client Configuration (`http.go:69-91`):**
- Files: `sdks/sandbox/go/opensandbox/http.go`
- Why fragile: Timeout handling has ordering dependencies:
  - Line 84-89: `timeout` is applied after all options — if `WithHTTPClient` is called after `WithTimeout`, custom client's timeout is overwritten
  - Line 16: `defaultTimeout = 0` (no global timeout) designed to allow long-lived SSE streams, but caller must use context deadlines — easy to forget
  - No validation that custom `*http.Client` is provided to `WithHTTPClient`
- Safe modification: Document timeout vs context deadline tradeoff prominently; apply timeout as first step; warn if `WithTimeout` is used (SSE streams will hang)
- Test coverage: Unit tests cover standard paths; ordering edge cases (WithHTTPClient then WithTimeout) may not be tested

**Retry Configuration Defaults (`retry.go:36-45`):**
- Files: `sdks/sandbox/go/opensandbox/retry.go`
- Why fragile: `DefaultRetryConfig` returns values without validation:
  - No bounds checking: Multiplier could be 0 or negative (line 93)
  - No bounds checking: Jitter could be > 1.0 (line 98)
  - Backoff could exceed MaxBackoff in edge cases (line 94 caps to float, then converts to Duration)
- Safe modification: Validate RetryConfig in `WithRetry` or constructor; return error for invalid multiplier/jitter
- Test coverage: DefaultRetryConfig tested; custom bad configs not validated

---

## Performance Bottlenecks

**Polling in `waitForRunning` (`sandbox.go:301-319`):**
- Problem: Fixed 2-second sleep regardless of timeout or remaining time
- Files: `sdks/sandbox/go/opensandbox/sandbox.go` line 317
- Cause: No adaptive backoff — sleeps 2s even after 58s of a 60s timeout
- Improvement path: Use exponential backoff (e.g., 100ms → 200ms → 500ms capped at 1s) to reduce latency in fast paths while respecting slow paths

**Streaming Buffer Size (`streaming.go:35-36`):**
- Problem: SSE scanner buffer increased to 4 MiB but allocated on every stream start
- Files: `sdks/sandbox/go/opensandbox/streaming.go` line 36
- Cause: `scanner.Buffer(make(...), ...)` creates heap allocation per stream
- Improvement path: Consider reusing buffer across streams or providing configurable buffer size; profile actual event sizes to validate 4 MiB cap

**Concurrent Endpoint Resolution (`sandbox.go:323-359`):**
- Problem: `resolveExecd` and `resolveEgress` use mutex but don't retry on transient failures
- Files: `sdks/sandbox/go/opensandbox/sandbox.go` lines 331-334, 371-374 (`GetEndpoint` calls)
- Cause: If `GetEndpoint` returns transient error, subsequent calls fail with same error (no exponential backoff)
- Improvement path: Integrate retry loop inside `resolveExecd`/`resolveEgress` or expose retry config to caller

---

## Tech Debt

**Duplicate Error Handling in `sandbox_exec.go` and `sandbox_egress.go`:**
- Issue: Every method repeats `if s.execd == nil` / `if s.egress == nil` checks
- Files: `sdks/sandbox/go/opensandbox/sandbox_exec.go` lines 15-16, 31-32, 43-44, 51-52, 59-60, 67-68, 75-76, 86-87, 95-96; `sdks/sandbox/go/opensandbox/sandbox_egress.go` lines 7-8, 15-16
- Impact: 16 identical checks — code duplication, harder to maintain consistent error messages
- Fix approach: Extract `s.requireExecd()` and `s.requireEgress()` helpers that return `(*ExecdClient, error)` and `(*EgressClient, error)`; use throughout

**String Prefix Checking for Protocol (`sandbox.go:337-338, 377-378`):**
- Issue: `!strings.HasPrefix(execdURL, "http")` assumes URL format — fails for `localhost:port` or `127.0.0.1:port`
- Files: `sdks/sandbox/go/opensandbox/sandbox.go`
- Impact: May construct invalid URLs like `https://localhost:8091` when `localhost:8091` intended
- Fix approach: Use `url.Parse()` to validate and reconstruct URLs safely; check for scheme presence explicitly

**Endpoint Header Filtering Duplication (`sandbox.go:347-350, 385-388`):**
- Issue: Same logic to extract auth token and preserve other headers written twice
- Files: `sdks/sandbox/go/opensandbox/sandbox.go`
- Impact: Inconsistent handling if one path is updated; hard-coded header names `X-EXECD-ACCESS-TOKEN` and `OPENSANDBOX-EGRESS-AUTH` appear twice
- Fix approach: Extract `extractTokenAndHeaders(endpoint, tokenKey)` helper function

**Implicit Execution Exit Code Assumption (`execution.go:213-215`):**
- Issue: Sets exit code to 0 if no error and no explicit exit code — implicit assumption not documented
- Files: `sdks/sandbox/go/opensandbox/execution.go`
- Impact: Caller may assume exit code 0 means success, but it could mean "no error event received"
- Fix approach: Document behavior clearly; consider returning nil for ExitCode if not provided by server

**HTTP Client Default Timeout Confusion (`http.go:13-16, 40-45`):**
- Issue: `defaultTimeout = 0` allows SSE streams but is confusing; `WithTimeout` applies after options, leading to ordering fragility
- Files: `sdks/sandbox/go/opensandbox/http.go`
- Impact: Users may set HTTP timeout expecting it to work, but SSE streams hang; ordering of `WithTimeout` vs `WithHTTPClient` matters
- Fix approach: Separate timeouts for HTTP vs streaming; document clearly; consider deprecating `WithTimeout` in favor of context deadlines

---

## Security Considerations

**Missing Request ID Validation (`http.go:211`):**
- Risk: `X-Request-Id` header extracted and stored but never validated or rate-limited by value
- Files: `sdks/sandbox/go/opensandbox/http.go` line 211
- Current mitigation: Header used only for error reporting
- Recommendations: None needed unless used for server affinity/retry routing — then add validation

**Endpoint Token Extraction Silent Fallback (`sandbox.go:353-355`):**
- Risk: If server provides no auth token in endpoint headers, SDK silently uses API key instead
- Files: `sdks/sandbox/go/opensandbox/sandbox.go` lines 353-355, 383-385
- Current mitigation: API key is expected to be correct; execd endpoint should provide token
- Recommendations: Log warning if token missing but API key present; consider making token-less fallback configurable

**SSE Scanner Buffer Overflow Risk (`streaming.go:35-36`):**
- Risk: 4 MiB buffer could be exploited by malicious server sending single huge event
- Files: `sdks/sandbox/go/opensandbox/streaming.go` line 36
- Current mitigation: Buffer is 4 MiB (fixed), not unlimited
- Recommendations: Make buffer size configurable; add rate limiting per event size; document DoS risk in large file uploads

---

## Scaling Limits

**Sandbox List Pagination:**
- Current capacity: `ListSandboxes` supports `ListOptions` with limit/offset (untested)
- Limit: No query param validation; could request limit=1000000
- Scaling path: Add client-side validation (e.g., cap limit to 1000); implement cursor-based pagination if server supports

**HTTP Connection Pooling:**
- Current capacity: `TransportConfig` defaults to 100 idle connections, 10 per host
- Limit: No auto-scaling; fixed pool size
- Scaling path: Make pool size configurable; consider adaptive pooling based on concurrent requests

**Streaming Buffer Memory:**
- Current capacity: 4 MiB scanner buffer per stream
- Limit: Long-running application with many concurrent streams could consume memory linearly
- Scaling path: Implement buffer pool or ring buffer for SSE parsing; measure actual event sizes in production

---

## Dependencies at Risk

**math/rand/v2 Deprecation Risk:**
- Risk: Go 1.22+ introduced `math/rand/v2`; older Go versions won't compile
- Files: `sdks/sandbox/go/opensandbox/retry.go` line 7 (`math/rand/v2`)
- Impact: If supporting Go < 1.22, build will fail
- Migration plan: Pin Go version in `go.mod`; document minimum version; consider fallback to `math/rand` with `Seed()` if older Go needed

**Indirect HTTP/2 Dependency:**
- Risk: `http.Client.Transport` defaults to HTTP/2 if `golang.org/x/net` is available; no explicit control
- Files: `sdks/sandbox/go/opensandbox/http.go`, `sdks/sandbox/go/opensandbox/transport.go`
- Impact: Behavior depends on transitive dependencies; may differ across deployments
- Migration plan: Explicitly set `HTTP/2` or HTTP/1.1 in `http2.ConfigureTransport()` or disable HTTP/2 if needed

---

## Missing Critical Features

**No Request/Response Logging:**
- Problem: No built-in request/response logging for debugging; users must intercept HTTP client
- Blocks: Debugging protocol mismatches, reverse-engineering unexpected errors
- Impact: Hard to diagnose integration issues without proxying all traffic through Wireshark or similar

**No Metrics/Tracing Integration:**
- Problem: No OpenTelemetry or Prometheus metrics exported
- Blocks: Monitoring SDK latency, error rates, streaming performance in production
- Impact: No visibility into SDK behavior under load

**No Connection Pooling Statistics:**
- Problem: `TransportConfig` creates pool but provides no stats (active connections, pool utilization)
- Blocks: Diagnosing connection exhaustion or slow requests
- Impact: Hard to tune pool size for workload

**No SSE Event Type Registry:**
- Problem: Unknown event types silently treated as stdout (line 225 in `execution.go`)
- Blocks: Forward-compatibility; new server-sent event types get swallowed
- Impact: Server-side events (e.g., debugging events, metrics) could be misinterpreted

---

## Test Coverage Gaps (Summary)

| Component | Coverage | Priority | Tests Needed |
|---|---|---|---|
| SandboxManager | 0% | High | List/filter, kill, pause, resume |
| File write ops (8 endpoints) | 0% | High | Create/delete dir, move, search, delete, replace, download |
| CodeInterpreter | 0% | Medium | Execute, execute-in-context, context lifecycle |
| Sessions | 0% | Low | Create, run, delete |
| Command management | 0% | Low | Status, logs, interrupt |
| Metrics watch | 0% | Low | SSE streaming collection |
| Endpoint resolution | Partial | Medium | Error paths, missing tokens, bad URLs |
| Execution stream edge cases | Partial | Medium | Malformed JSON, conflicting fields, unknown types |

---

*Concerns audit: 2026-04-04*

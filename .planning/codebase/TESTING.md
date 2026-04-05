# Testing Patterns

**Analysis Date:** 2026-04-04

## Test Framework

**Runner:**
- Go's built-in `testing` package (no external test framework)
- Test discovery: standard Go convention (`*_test.go` files)
- Build tags for test categorization: `//go:build integration`, `//go:build staging`

**Assertion Library:**
- Standard library only; no external assertion libraries like testify/assert
- Manual comparison with `if` statements and `t.Errorf()` / `t.Fatalf()`
- Pattern: test table-driven with struct slice:
  ```go
  tests := []struct {
    status    int
    transient bool
  }{
    {http.StatusTooManyRequests, true},
    {http.StatusBadRequest, false},
  }
  for _, tt := range tests {
    if got := someFunc(tt.status); got != tt.transient {
      t.Errorf("status %d: got %v, want %v", tt.status, got, tt.transient)
    }
  }
  ```

**Run Commands:**
```bash
make test                # Run all unit tests
make test-integration    # Run integration tests (build tag: integration)
make test-staging        # Run staging tests (build tag: staging)
go test ./opensandbox/ -v  # Verbose unit tests
go test -tags=integration ./opensandbox/ -v -timeout 3m  # Integration with timeout
go test -tags=staging ./opensandbox/ -v -timeout 3m  # Staging tests
go test ./opensandbox/ -cover  # Coverage report
```

## Test File Organization

**Location:**
- Co-located with source code in same package
- `opensandbox_test.go`: main unit test file
- `retry_test.go`: focused tests for retry logic
- `integration_test.go`: integration tests (tagged `//go:build integration`)
- `staging_test.go`: staging server tests (tagged `//go:build staging`)

**Naming:**
- Test functions: `func Test{Feature}(t *testing.T)` (e.g., `TestCreateSandbox`, `TestRetry_TransientThenSuccess`)
- Subtests not commonly used; each test is separate function
- Table-driven test naming: simple prefix like `TestIsTransient`

**Structure:**
```
sdks/sandbox/go/opensandbox/
├── opensandbox_test.go           # Main unit tests
├── retry_test.go                 # Retry-specific tests
├── integration_test.go           # Integration tests (//go:build integration)
└── staging_test.go               # Staging tests (//go:build staging)
```

## Test Structure

**Suite Organization:**
Each test file has minimal setup. Helper functions defined at top:
```go
// Example from opensandbox_test.go
func newLifecycleServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *LifecycleClient) {
  t.Helper()
  srv := httptest.NewServer(handler)
  t.Cleanup(srv.Close)
  client := NewLifecycleClient(srv.URL, "test-api-key")
  return srv, client
}
```

**Patterns:**
- Setup: `t.Helper()` marks helper functions; `t.Cleanup()` registers deferred cleanup
- Test creation: `httptest.NewServer(handler)` for mock servers
- Context: `context.WithTimeout(context.Background(), duration)` for deadline testing
- Assertion: explicit `if got != want` with descriptive `t.Errorf()` messages

Example test structure (`opensandbox_test.go` line 58-102):
```go
func TestCreateSandbox(t *testing.T) {
  // 1. Setup (define expected value)
  want := SandboxInfo{
    ID: "sbx-123",
    Status: SandboxStatus{State: StatePending},
    CreatedAt: time.Now().UTC().Truncate(time.Second),
  }

  // 2. Create mock server
  _, client := newLifecycleServer(t, func(w http.ResponseWriter, r *http.Request) {
    // 3. Assert request shape
    if r.Method != http.MethodPost {
      t.Errorf("expected POST, got %s", r.Method)
    }
    
    // 4. Decode request body
    var req CreateSandboxRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // 5. Assert request content
    if req.Image.URI != "python:3.12" {
      t.Errorf("expected image python:3.12, got %s", req.Image.URI)
    }
    
    // 6. Return mock response
    jsonResponse(w, http.StatusCreated, want)
  })

  // 7. Execute
  got, err := client.CreateSandbox(context.Background(), CreateSandboxRequest{
    Image: ImageSpec{URI: "python:3.12"},
    Entrypoint: []string{"/bin/sh"},
    ResourceLimits: ResourceLimits{"cpu": "500m", "memory": "512Mi"},
  })

  // 8. Verify
  if err != nil {
    t.Fatalf("CreateSandbox: %v", err)
  }
  if got.ID != want.ID {
    t.Errorf("ID = %q, want %q", got.ID, want.ID)
  }
}
```

## Mocking

**Framework:** Standard library only
- `httptest.NewServer()` for mock HTTP endpoints
- `httptest.NewRequest()` / `httptest.NewRecorder()` not used (direct handler testing)
- Custom mock handlers inline as `http.HandlerFunc` closures

**Patterns:**
Mock server creation wrapper in `opensandbox_test.go`:
```go
func newLifecycleServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *LifecycleClient) {
  t.Helper()
  srv := httptest.NewServer(handler)
  t.Cleanup(srv.Close)
  client := NewLifecycleClient(srv.URL, "test-api-key")
  return srv, client
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
  w.Header().Set("Content-Type", "application/json")
  w.WriteHeader(status)
  json.NewEncoder(w).Encode(v)
}
```

**What to Mock:**
- HTTP servers and responses (via `httptest`)
- Request/response handling (inline handler checks)
- Error conditions (intentional 4xx/5xx responses)
- Timing (mock time progression in retry tests via atomic counters)
- State transitions (mock lifecycle polling with controlled state values)

**What NOT to Mock:**
- Standard library types (`http.Client`, `context`)
- Real business logic (always execute actual code, not mocks)
- Timeout behavior (use real context deadlines)
- Retry calculations (test actual exponential backoff math)
- Streaming/SSE protocol (test actual streaming logic)

**Example mock from `retry_test.go` (TestRetry_TransientThenSuccess):**
```go
var attempts atomic.Int32

srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  n := attempts.Add(1)
  if n <= 2 {
    w.WriteHeader(http.StatusServiceUnavailable)
    w.Write([]byte(`{"code":"UNAVAILABLE","message":"try again"}`))
    return
  }
  jsonResponse(w, http.StatusOK, SandboxInfo{ID: "sbx-ok", CreatedAt: time.Now()})
}))
```

## Fixtures and Factories

**Test Data:**
No separate fixture files. Inline test data creation:
```go
want := SandboxInfo{
  ID: "sbx-123",
  Status: SandboxStatus{State: StatePending},
  CreatedAt: time.Now().UTC().Truncate(time.Second),
}
```

Factory pattern for mock servers:
```go
func newLifecycleServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *LifecycleClient)
func newEgressServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *EgressClient)
func newExecdServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *ExecdClient)
```

**Location:**
- In test files themselves (e.g., top of `opensandbox_test.go`)
- No separate `testdata/` directory for JSON fixtures
- Test data inlined as struct literals or JSON strings within test functions

## Coverage

**Requirements:** No enforced coverage targets
- Coverage can be viewed: `go test ./opensandbox/ -cover`
- Recent commits show 100% test coverage achieved: commit `2b7c624 test(sdk): close all Go SDK test coverage gaps (43% → 100%)`
- Integration and staging tests validate E2E scenarios not captured by unit tests

**View Coverage:**
```bash
go test ./opensandbox/ -cover
go test ./opensandbox/ -coverprofile=coverage.out && go tool cover -html=coverage.out
```

## Test Types

**Unit Tests:**
- Scope: individual functions and types in isolation
- Location: `opensandbox_test.go`, `retry_test.go`
- Approach: HTTP mocking via `httptest`
- Examples:
  - `TestCreateSandbox`: lifecycle client request/response
  - `TestRetry_TransientThenSuccess`: retry logic with transient errors
  - `TestIsTransient`: error classification
  - `TestAPIError`: error type behavior
  - `TestStreamSSE`: streaming protocol parsing

**Integration Tests:**
- Scope: full client stack against a live or containerized OpenSandbox server
- Location: `integration_test.go` (build tag: `//go:build integration`)
- Approach: real HTTP connections; requires `OPENSANDBOX_URL` env var (defaults to `http://localhost:8090`)
- Run: `make test-integration` or `go test -tags=integration ./opensandbox/ -v -timeout 3m`
- Examples:
  - `TestIntegration_FullLifecycle`: sandbox creation, polling, execution
  - `TestIntegration_FileOperations`: file upload/download/move
  - `TestIntegration_NetworkPolicy`: egress control validation
  - `TestIntegration_VolumeMounts`: PVC and host volume mounts
  - `TestIntegration_MultiLanguageCodeExecution`: code interpreter across languages
  - Negative tests: `TestIntegration_Negative_GetNonexistentSandbox`

**E2E/Staging Tests:**
- Scope: compatibility with staging/production server deployments
- Location: `staging_test.go` (build tag: `//go:build staging`)
- Approach: real HTTP against staging server; requires `STAGING_URL` and `STAGING_API_KEY` env vars
- Run: `make test-staging` or `STAGING_URL=https://... STAGING_API_KEY=... go test -tags=staging ./opensandbox/ -v -timeout 3m`
- Purpose: validate SDK works with different server configurations (auth headers, URL prefixes, proxy endpoints)
- Examples:
  - `TestStaging_FullLifecycle`: validates X-API-Key header instead of OPEN-SANDBOX-API-KEY
  - `TestStaging_VolumeMounts`: server-side volume handling
  - `TestStaging_NetworkPolicy`: egress rules on staging infrastructure

## Common Patterns

**Async Testing:**
Tests use context timeouts to bound execution:
```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
defer cancel()

// Polling loop with context deadline
for i := 0; i < 30; i++ {
  running, err := client.GetSandbox(ctx, sb.ID)
  if running.Status.State == StateRunning {
    break
  }
  time.Sleep(2 * time.Second)
}
```

**Error Testing:**
Classification and transient error handling:
```go
// From retry_test.go: TestRetry_TransientThenSuccess
func TestRetry_TransientThenSuccess(t *testing.T) {
  var attempts atomic.Int32
  
  srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    n := attempts.Add(1)
    if n <= 2 {
      w.WriteHeader(http.StatusServiceUnavailable)  // Transient
      w.Write([]byte(`{"code":"UNAVAILABLE","message":"try again"}`))
      return
    }
    jsonResponse(w, http.StatusOK, SandboxInfo{...})  // Success
  }))
  defer srv.Close()
  
  client := NewLifecycleClient(srv.URL, "key", WithRetry(RetryConfig{
    MaxRetries:     3,
    InitialBackoff: 10 * time.Millisecond,
    MaxBackoff:     100 * time.Millisecond,
    Multiplier:     2.0,
  }))
  
  got, err := client.GetSandbox(context.Background(), "sbx-ok")
  if err != nil {
    t.Fatalf("expected success after retries, got: %v", err)
  }
  if attempts.Load() != 3 {  // Verify attempt count
    t.Errorf("attempts = %d, want 3", attempts.Load())
  }
}
```

**Permanent Error Testing:**
```go
// From retry_test.go: TestRetry_PermanentError
func TestRetry_PermanentError(t *testing.T) {
  var attempts atomic.Int32
  
  srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    attempts.Add(1)
    jsonResponse(w, http.StatusNotFound, ErrorResponse{  // Permanent error
      Code:    "NOT_FOUND",
      Message: "sandbox not found",
    })
  }))
  
  client := NewLifecycleClient(srv.URL, "key", WithRetry(DefaultRetryConfig()))
  _, err := client.GetSandbox(context.Background(), "sbx-missing")
  
  if err == nil {
    t.Fatal("expected error, got nil")
  }
  if attempts.Load() != 1 {  // No retries for permanent errors
    t.Errorf("attempts = %d, want 1", attempts.Load())
  }
}
```

**Streaming/SSE Testing:**
```go
// From opensandbox_test.go: TestRunCommand_SSE
func TestRunCommand_SSE(t *testing.T) {
  _, client := newExecdServer(t, func(w http.ResponseWriter, r *http.Request) {
    // ... validate request ...
    
    // Return SSE stream
    w.Header().Set("Content-Type", "text/event-stream")
    fmt.Fprintf(w, "event: init\ndata: %s\n\n", `{"text":"cmd-123"}`)
    fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", `{"text":"output","timestamp":1000}`)
    fmt.Fprintf(w, "event: complete\ndata: %s\n\n", `{"execution_time":50}`)
  })
  
  var exec *Execution
  handlers := &ExecutionHandlers{
    OnInit: func(e ExecutionInit) error {
      exec = &Execution{ID: e.ID}
      return nil
    },
    OnStdout: func(m OutputMessage) error {
      exec.Stdout = append(exec.Stdout, m)
      return nil
    },
    // ...
  }
  
  err := client.RunCommand(context.Background(), "sbx-id", RunCommandRequest{...}, handlers)
  if err != nil {
    t.Fatalf("RunCommand: %v", err)
  }
}
```

---

*Testing analysis: 2026-04-04*

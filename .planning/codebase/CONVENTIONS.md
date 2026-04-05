# Coding Conventions

**Analysis Date:** 2026-04-04

## Naming Patterns

**Files:**
- Package files use lowercase with underscores: `retry_test.go`, `sandbox_egress.go`, `code_interpreter.go`
- API client implementations: `{api_domain}.go` (e.g., `lifecycle.go`, `execd.go`)
- Test files follow standard Go convention: `{name}_test.go` (e.g., `opensandbox_test.go`, `retry_test.go`)
- Integration/staging tests use build tags: `//go:build integration` and `//go:build staging` at file top

**Functions:**
- Receiver methods use single-letter receiver names for types under 10 methods, otherwise short abbreviations (e.g., `(c *Client)`, `(s *Sandbox)`, `(e *APIError)`)
- Public methods use PascalCase: `CreateSandbox()`, `GetSandbox()`, `WaitUntilReady()`
- Private helper methods use camelCase: `waitForRunning()`, `resolveExecd()`, `doRequest()`
- Constructor functions: `New{TypeName}()` (e.g., `NewLifecycleClient()`, `NewSandbox()`)
- Retry/backoff logic: `isTransient()`, `IsTransient()` (method), `retryDelay()`

**Variables:**
- Package-level constants in UPPER_CASE: `DefaultTimeoutSeconds`, `APIVersion`, `DefaultRequestTimeout`
- Package-level defaults: PascalCase: `DefaultEntrypoint`, `DefaultResourceLimits`, `DefaultRetryConfig()`
- Struct fields use PascalCase for exported, camelCase for private: `ID`, `MaxRetries` vs `httpClient`, `authHeader`
- Error types wrap contextual data with descriptive field names: `SandboxID`, `Elapsed`, `LastErr`
- Configuration fields are exported for flexibility: `Domain`, `Protocol`, `APIKey`, `UseServerProxy`

**Types:**
- Struct types: PascalCase without suffix (e.g., `Client`, `Sandbox`, `ConnectionConfig`)
- Error types: PascalCase with `Error` suffix (e.g., `SandboxReadyTimeoutError`, `InvalidArgumentError`, `APIError`)
- Request/response types: PascalCase with semantic suffix (e.g., `CreateSandboxRequest`, `SandboxInfo`, `ListSandboxesResponse`)
- Enum-like types (state strings): `SandboxState` with const values like `StatePending`, `StateRunning`

## Code Style

**Formatting:**
- Uses standard `gofmt` formatting
- Import statements grouped: stdlib imports, then blank line, then third-party imports
- Imports organized alphabetically within groups

**Linting:**
- Tool: `staticcheck` (optional, available via `make lint`)
- Lint run: `go vet ./...` (required)
- Build check: `go build ./...` (required)

## Import Organization

**Order:**
1. Standard library packages (`fmt`, `net/http`, `context`, `time`, `encoding/json`)
2. Blank line
3. Third-party imports from external modules
4. No local relative imports (single package `opensandbox`)

**Examples from codebase:**
- `opensandbox_test.go`: stdlib first, then package imports
- `sandbox.go`: stdlib imports (context, fmt, strings, sync, time), then internal package imports
- `http.go`: stdlib imports only (Client is self-contained)

**Path Aliases:**
- No aliases used; single package within `opensandbox` directory
- Generated API code kept in subdirectories: `opensandbox/api/lifecycle/`, `opensandbox/api/execd/`, `opensandbox/api/egress/`

## Error Handling

**Patterns:**
- Custom error types implement `error` interface with `Error()` string method
- Error wrapping uses `fmt.Errorf()` with `%w` for error chaining: `fmt.Errorf("opensandbox: create sandbox: %w", err)`
- Sentinel errors use pointer receivers for equality checks: `var apiErr *APIError; errors.As(err, &apiErr)`
- Error types store context for debugging:
  ```go
  type SandboxReadyTimeoutError struct {
    SandboxID string
    Elapsed   string
    LastErr   error
  }
  ```
- `Unwrap()` method provides error chain introspection: `func (e *SandboxReadyTimeoutError) Unwrap() error { return e.LastErr }`
- Transient vs permanent errors classified in `IsTransient()` method on `APIError`
- Network errors detected via `errors.As(err, &netErr)` pattern for `net.Error` interface

## Logging

**Framework:** Standard Go logging via `testing.T` for tests; production code uses structured output

**Patterns:**
- Test logging only: `t.Logf()`, `t.Fatalf()`, `t.Errorf()`
- No global loggers in SDK code
- SDK code returns errors instead of logging
- Detailed diagnostics included in error messages via `fmt.Errorf()` wrapping

## Comments

**When to Comment:**
- Function documentation: all exported functions have doc comments explaining purpose, parameters, and behavior
- Interface documentation: describe contracts (e.g., `EventHandler` callback behavior)
- Complex logic: internal algorithms get explanation (e.g., `backoff()` exponential calculation)
- Build tags and special behavior: documented with comment blocks (e.g., `//go:build integration`)
- Default behavior: documented in constant/variable comments (e.g., `DefaultEntrypoint keeps the sandbox alive...`)

**JSDoc/TSDoc:**
- Not used; this is Go, not TypeScript
- Uses standard Go doc comment style: `// Comment starts with function name`
- Example from `http.go`:
  ```go
  // NewClient creates a new base Client. The authHeader parameter specifies
  // which HTTP header carries the API key (e.g. "OPEN-SANDBOX-API-KEY" for
  // lifecycle, "OPENSANDBOX-EGRESS-AUTH" for egress).
  func NewClient(...) *Client
  ```

## Function Design

**Size:** Generally 20-60 lines for core logic; larger functions break concerns:
- `CreateSandbox()` in `sandbox.go`: ~85 lines including setup, validation, and cleanup
- `doRequestOnce()` in `http.go`: ~48 lines for request building, execution, response handling
- Retry logic `withRetry()`: ~24 lines for attempt loop and backoff calculation

**Parameters:**
- Prefer struct parameters for 3+ arguments: `ReadyOptions`, `ConnectionConfig`, `RetryConfig`
- Constructor functions accept variadic options: `NewLifecycleClient(baseURL, apiKey, opts ...Option)`
- Context always first parameter in async functions: `func (c *Client) GetSandbox(ctx context.Context, id string)`
- Options pattern for flexible configuration:
  ```go
  WithRetry(cfg RetryConfig) Option
  WithHeaders(headers map[string]string) Option
  WithAuthHeader(header string) Option
  ```

**Return Values:**
- Single value (common for getters): `func (s *Sandbox) ID() string`
- Error-last pattern: `func (c *Client) CreateSandbox(...) (*SandboxInfo, error)`
- Pointer receivers for mutations: `func (s *Sandbox) WaitUntilReady(ctx context.Context, opts ReadyOptions) error`
- Multiple returns for structured data: `(info *SandboxInfo, err error)` or `(items []Item, pagination PaginationInfo, err error)`

## Module Design

**Exports:**
- All public API types exported at package level: `Sandbox`, `Client`, `SandboxInfo`
- Configuration structs exported for external instantiation: `ConnectionConfig`, `RetryConfig`, `TransportConfig`
- Constructors and factories return public types: `NewLifecycleClient()` returns `*LifecycleClient`
- Private internal types not exported: internal `Client` shared by lifecycle/execd/egress

**Barrel Files:**
- Not used; single-file imports via `package opensandbox`
- Related types grouped in single files: `types.go` for all API models
- Generated API code in subdirectories maintains separation: `api/lifecycle/`, `api/execd/`, `api/egress/`

## Struct Design

**Configuration Structs:**
- All fields exported to allow builder pattern: `ConnectionConfig.Domain`, `.Protocol`, `.APIKey`
- Getter methods for defaulting: `GetDomain()`, `GetProtocol()`, `GetAPIKey()` check field, then env var, then constant
- Fluent initialization enabled via exported fields:
  ```go
  config := ConnectionConfig{
    Domain: "example.com",
    APIKey: os.Getenv("KEY"),
  }
  ```

**Option Pattern:**
- Used for flexible client configuration: `type Option func(*Client)`
- Applied in constructor: `func NewClient(..., opts ...Option)`
- Examples: `WithRetry()`, `WithHeaders()`, `WithHTTPClient()`, `WithAuthHeader()`

**Request/Response Types:**
- Inline JSON tags for serialization: `` `json:"fieldName,omitempty"` ``
- Omitempty used for optional fields
- Pointers for optional nested structures: `Auth *ImageAuth`, `Volumes []Volume`
- Map types for flexible key-value: `Env map[string]string`, `Metadata map[string]string`

---

*Convention analysis: 2026-04-04*

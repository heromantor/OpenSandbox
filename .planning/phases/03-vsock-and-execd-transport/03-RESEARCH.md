# Phase 3: vsock and Execd Transport - Research

**Researched:** 2026-04-04
**Domain:** Firecracker vsock host-guest IPC, HTTP-over-vsock transport, CID allocation
**Confidence:** HIGH

## Summary

Phase 3 adds vsock-based communication between the host and guest, enabling the execd agent inside the Firecracker VM to be reachable from the host without requiring TAP networking. Firecracker's vsock implementation uses a virtio-vsock device that maps to a Unix Domain Socket (UDS) on the host side. The host connects to this UDS, sends a `CONNECT <port>\n` handshake message, receives `OK <port>\n`, and then has a raw `net.Conn` to the guest listener. The firecracker-go-sdk already provides a complete `vsock.DialContext()` implementation that handles the CONNECT handshake with retry logic.

The core work is: (1) add vsock device configuration to VMConfig and wire it through to the SDK's `Config.VsockDevices`, (2) implement a CID allocator that guarantees host-wide uniqueness, (3) design a vsock UDS path scheme that is unique per VM instance and compatible with snapshot restore (where the path changes but the CID can stay the same), (4) build a vsock-backed HTTP transport so the host can speak HTTP to execd over the vsock connection, and (5) implement a health-check that verifies execd is listening on vsock port 44772 after boot.

**Primary recommendation:** Use the SDK's built-in `vsock.DialContext()` for the CONNECT handshake, wrap it in a `net/http.Transport` with custom `DialContext` for HTTP-over-vsock, implement an atomic CID counter starting at 3 (the minimum valid guest CID), and derive vsock UDS paths from the VM ID using the same temp directory pattern as the existing `socketPath()` helper.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- discuss phase was skipped per user setting.

### Claude's Discretion
All implementation choices are at Claude's discretion -- discuss phase was skipped per user setting. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None -- discuss phase skipped.
</user_constraints>

## Project Constraints (from CLAUDE.md)

- Go 1.24.0 runtime (actual: 1.24.5 installed)
- `go vet ./...` required; `go build ./...` required
- Standard `gofmt` formatting
- Import grouping: stdlib, blank line, third-party
- Single-letter receivers for types under 10 methods
- Exported functions have doc comments
- Error wrapping with `fmt.Errorf()` and `%w`
- Constructor pattern: `New{TypeName}()`
- Context always first parameter in async functions
- Test files: `{name}_test.go`; integration tests use build tags
- No global loggers in SDK code; return errors instead of logging
- Dependency: `github.com/firecracker-microvm/firecracker-go-sdk` v1.0.0

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VSOCK-01 | Host-guest communication uses vsock instead of TCP | VMConfig gets VsockCID + VsockUDSPath fields; toFirecrackerConfig() wires VsockDevices into SDK Config; host connects via UDS with CONNECT handshake |
| VSOCK-02 | Each VM gets a unique CID (no collisions on same host) | Atomic CID allocator (sync/atomic counter starting at 3, minimum valid guest CID); CID tracked in VM struct and released on destroy |
| VSOCK-03 | Execd agent inside guest listens on vsock port 44772 | Health-check dials vsock UDS port 44772 using SDK's vsock.DialContext(), sends HTTP GET /health, validates 200 response |
| VSOCK-04 | Host-side connects via Unix domain socket with CONNECT handshake protocol | SDK's vsock.DialContext() already implements full CONNECT handshake with retry; wrap in http.Transport for HTTP-over-vsock |
| VSOCK-05 | vsock UDS paths are unique per VM instance (prevents collision on snapshot restore) | Path scheme: `{tempdir}/firecracker-vsock-{vm-id}.sock` for non-jailed; `vsock.sock` inside chroot for jailed; compatible with snapshot restore (new VM ID = new path) |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/firecracker-microvm/firecracker-go-sdk` | v1.0.0 | VsockDevice config, addVsock handler, CONNECT handshake via vsock subpackage | Already pinned in go.mod; provides complete vsock.DialContext() with retry [VERIFIED: go.mod] |
| `github.com/firecracker-microvm/firecracker-go-sdk/vsock` | v1.0.0 | Host-side CONNECT handshake dial (vsock.DialContext, vsock.Dial) | Subpackage of the SDK; host-side uses net.DialTimeout("unix") -- no AF_VSOCK kernel support needed on host [VERIFIED: SDK source code] |
| `net/http` | stdlib | HTTP client with custom transport for execd health-check | Standard library; Transport.DialContext accepts custom dialer -- plug in vsock dial [VERIFIED: Go stdlib] |
| `sync/atomic` | stdlib | Atomic CID counter for unique allocation | Lock-free, correct for concurrent VM creation [VERIFIED: Go stdlib] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/sirupsen/logrus` | v1.9.4 | Logger passed to vsock.DialContext options | Already a direct dependency; SDK's vsock package accepts logrus.FieldLogger [VERIFIED: go.mod, SDK source] |
| `github.com/google/uuid` | v1.6.0 | VM ID generation (existing) | Already used for VM IDs; CIDs are independent (counter-based, not UUID-derived) [VERIFIED: go.mod] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Atomic counter for CIDs | UUID hash to CID | Counter is simpler and guarantees uniqueness; UUID hash has theoretical collision risk in 32-bit space; counter restarts at process restart but CID recycling is safe since old VMs are gone |
| SDK vsock.DialContext | Raw net.Dial("unix") + manual CONNECT | SDK handles retry, timeout, error classification; manual approach is fragile |
| net/http.Transport custom DialContext | httputil.ReverseProxy | Transport is lower-level and appropriate here; we are a client, not a proxy |

## Architecture Patterns

### Recommended Project Structure
```
runtime/firecracker/
  vsock.go              # VsockConfig, CID allocator, vsock UDS path helpers
  vsock_linux.go        # toFirecrackerConfig vsock wiring (build-tagged)
  vsock_transport.go    # HTTP-over-vsock transport (VsockHTTPClient)
  vsock_health.go       # ExecdHealthCheck over vsock
  vsock_test.go         # Unit tests (CID allocation, path generation, config)
  vsock_linux_test.go   # Linux-specific tests (SDK config wiring)
  vsock_transport_test.go  # HTTP transport tests (mock UDS server)
  vsock_health_test.go  # Health check tests
```

### Pattern 1: VMConfig Extension for vsock
**What:** Add VsockCID and VsockUDSPath fields to VMConfig; wire them through toFirecrackerConfig() into sdk.Config.VsockDevices
**When to use:** Every VM creation path
**Example:**
```go
// Source: firecracker-go-sdk machine.go VsockDevice struct
// In VMConfig (vm.go):
type VMConfig struct {
    // ... existing fields ...
    // VsockCID is the 32-bit Context Identifier for the vsock device.
    // Must be >= 3 (CID 0=hypervisor, 1=reserved, 2=host). Auto-assigned if 0.
    VsockCID uint32
}

// In vm.go, new helper:
func (c *VMConfig) vsockUDSPath() string {
    if c.JailerEnabled {
        return "vsock.sock"  // relative inside chroot
    }
    return filepath.Join(os.TempDir(), fmt.Sprintf("firecracker-vsock-%s.sock", c.ID))
}

// In vm_linux.go toFirecrackerConfig():
if c.VsockCID >= 3 {
    cfg.VsockDevices = []sdk.VsockDevice{{
        ID:   "vsock0",
        Path: c.vsockUDSPath(),
        CID:  c.VsockCID,
    }}
}
```

### Pattern 2: Atomic CID Allocator
**What:** Process-wide atomic counter starting at 3 (minimum valid guest CID), incremented per VM creation
**When to use:** Every VM creation in Manager.Create()
**Example:**
```go
// Source: Firecracker API spec -- guest_cid minimum is 3
// CID 0 = hypervisor, CID 1 = reserved, CID 2 = host

// CIDAllocator assigns unique guest CIDs for vsock devices.
type CIDAllocator struct {
    next atomic.Uint32
}

// NewCIDAllocator creates a CIDAllocator starting at firstCID.
func NewCIDAllocator(firstCID uint32) *CIDAllocator {
    a := &CIDAllocator{}
    a.next.Store(firstCID)
    return a
}

// Allocate returns the next unique CID. Thread-safe.
func (a *CIDAllocator) Allocate() uint32 {
    return a.next.Add(1) - 1
}

const MinGuestCID uint32 = 3
```

### Pattern 3: HTTP-over-vsock Transport
**What:** A custom `http.Transport` that dials the vsock UDS and performs the CONNECT handshake, returning a `net.Conn` that speaks raw TCP to execd
**When to use:** Any HTTP request to execd inside the guest (health check, command execution, file ops)
**Example:**
```go
// Source: Go stdlib net/http Transport.DialContext + firecracker-go-sdk vsock.DialContext

// NewVsockHTTPClient creates an HTTP client that routes all requests
// through a Firecracker vsock UDS with CONNECT handshake to the specified port.
func NewVsockHTTPClient(udsPath string, guestPort uint32) *http.Client {
    return &http.Client{
        Transport: &http.Transport{
            DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
                return fcvsock.DialContext(ctx, udsPath, guestPort)
            },
        },
    }
}

// Usage: health check
func healthCheck(ctx context.Context, udsPath string) error {
    client := NewVsockHTTPClient(udsPath, ExecdPort)
    req, _ := http.NewRequestWithContext(ctx, "GET", "http://execd/health", nil)
    resp, err := client.Do(req)
    if err != nil {
        return fmt.Errorf("execd health check failed: %w", err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("execd health check: unexpected status %d", resp.StatusCode)
    }
    return nil
}
```

### Pattern 4: VM Struct Extension
**What:** Track VsockCID and VsockUDSPath in the VM struct so they are available for health-check, snapshot restore, and cleanup
**When to use:** VM creation, health check, cleanup
**Example:**
```go
type VM struct {
    // ... existing fields ...
    // VsockCID is the assigned guest Context Identifier.
    VsockCID uint32
    // VsockUDSPath is the host-side Unix domain socket path for vsock.
    VsockUDSPath string
}
```

### Anti-Patterns to Avoid
- **Using CID 2 for the guest:** CID 2 is reserved for the host; guest CID must be >= 3. The SDK model validates `minimum: 3`. [VERIFIED: SDK models/vsock.go line 70]
- **Sharing CIDs across concurrent VMs:** Even though multiple VMs CAN share CIDs if they have different UDS paths (with jailer), assigning unique CIDs is simpler and avoids confusion during debugging.
- **Using the same UDS path for vsock and Firecracker API socket:** These are different sockets. The API socket (`firecracker-{id}.socket`) is for the REST API; the vsock UDS (`firecracker-vsock-{id}.sock`) is for guest communication.
- **Hard-coding vsock UDS paths without VM-ID:** Path must be unique per VM to support concurrent VMs and snapshot restore. Never use a static path like `./v.sock`.
- **Polling for execd readiness without timeout:** Always use context with timeout for health-check dial. The SDK's `vsock.DialContext` has a default `RetryTimeout` of 20 seconds, but the health-check should have its own shorter timeout (e.g., 30 seconds).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| vsock CONNECT handshake | Manual UDS dial + write "CONNECT" + read "OK" | `firecracker-go-sdk/vsock.DialContext()` | Handles retry on temporary errors, configurable timeouts, proper error classification (ackError vs connectMsgError) [VERIFIED: SDK vsock/dial.go] |
| VsockDevice model | Custom struct for PUT /vsock API | `sdk.VsockDevice{ID, Path, CID}` wired via `Config.VsockDevices` | SDK's handler chain calls addVsock() automatically during Machine.Start() [VERIFIED: SDK handlers.go line 314] |
| HTTP-over-UDS transport | Raw socket I/O with HTTP parsing | `http.Transport{DialContext: ...}` with vsock.DialContext | Go's HTTP client handles keep-alive, chunked encoding, timeouts correctly |
| Unix socket path length validation | String length check | Reuse existing `maxUnixSocketPathLen` constant (108) from jailer.go | Already defined and used for Firecracker API socket validation [VERIFIED: jailer.go line 49] |

**Key insight:** The firecracker-go-sdk's `vsock` subpackage is purpose-built for this exact use case. The host-side dial implementation (`vsock/dial.go`) uses standard Unix socket operations (no kernel AF_VSOCK support needed on the host) and implements the full Firecracker CONNECT protocol. The SDK's handler chain (`loadSnapshotHandlerList` in `handlers.go`) already calls `AddVsocksHandler` AFTER `LoadSnapshotHandler`, meaning vsock reconfiguration during snapshot restore is a supported flow.

## Common Pitfalls

### Pitfall 1: vsock UDS Path Collision on Snapshot Restore
**What goes wrong:** Two VMs restored from the same snapshot use the same vsock UDS path. The second VM fails to bind the socket. Connection attempts hit a stale socket or "address already in use" error.
**Why it happens:** The snapshot encodes the original vsock UDS path. Without overriding it on restore, restored VMs inherit the same path.
**How to avoid:** Always assign a new, unique vsock UDS path at VM creation or restore. The path scheme must include the VM instance ID (which is unique per restore). The Jailer automatically scopes paths per chroot, but non-jailed mode requires explicit uniqueness. For snapshot restore (Phase 6), use the `PUT /vsock` API to reconfigure the UDS path after loading the snapshot -- the SDK's handler chain supports this pattern.
**Warning signs:** `bind: address already in use` in Firecracker logs; execd unreachable after restore.

### Pitfall 2: CID Minimum Violation
**What goes wrong:** Assigning CID 0, 1, or 2 to a guest VM. The Firecracker API rejects CID < 3 with a validation error.
**Why it happens:** Using a counter starting at 0, or deriving CID from a hash that can produce values < 3.
**How to avoid:** Start the atomic counter at 3. The allocator must never return a CID < 3. The SDK model validates `minimum: 3` at the API level, providing a safety net. [VERIFIED: SDK client/models/vsock.go line 70]
**Warning signs:** PUT /vsock returns 400 Bad Request with validation error mentioning guest_cid.

### Pitfall 3: Unix Socket Path Length Exceeded
**What goes wrong:** The vsock UDS path exceeds 108 characters (Linux `sun_path` limit). Firecracker fails to create the socket.
**Why it happens:** Deep chroot paths combined with long VM IDs. The existing jailer validation already checks API socket paths against 108 chars, but vsock paths need the same validation.
**How to avoid:** Validate vsock UDS path length during VMConfig.Validate(). Reuse the `maxUnixSocketPathLen` constant from jailer.go. For jailed VMs, the vsock socket lives inside the chroot, so the full path is `<chroot>/vsock.sock`.
**Warning signs:** Socket creation fails with "name too long" or similar OS error.

### Pitfall 4: Stale vsock UDS File After VM Crash
**What goes wrong:** A VM crashes without cleanup. The vsock UDS file remains on disk. A new VM with the same ID cannot bind to it.
**Why it happens:** Abnormal termination bypasses the cleanup path. Unix domain socket files persist on the filesystem.
**How to avoid:** Add VsockUDSPath to VMResources for cleanup tracking. On VM creation, check for and remove stale socket files before starting. The existing `VMResources.Cleanup()` already handles socket cleanup -- extend it to include vsock UDS paths.
**Warning signs:** VM creation fails intermittently with "address already in use" after previous crashes.

### Pitfall 5: Health-Check Succeeds Before Execd Is Actually Ready
**What goes wrong:** The vsock CONNECT handshake succeeds (Firecracker's vsock device is up) but execd inside the guest has not started listening yet. The HTTP request to /health fails.
**Why it happens:** The vsock device is available as soon as Firecracker starts. The guest kernel boots, but execd may take a few hundred milliseconds to start its HTTP listener.
**How to avoid:** The health-check must retry the full sequence (vsock dial + HTTP GET /health) with backoff, not just the vsock connection. The SDK's `vsock.DialContext()` retries the CONNECT handshake (default 20s timeout, 100ms interval), but the HTTP request needs its own retry loop on top.
**Warning signs:** Intermittent "connection refused" from execd immediately after VM start, succeeding on retry.

## Code Examples

### VMConfig vsock Wiring (vm_linux.go)
```go
// Source: firecracker-go-sdk machine.go VsockDevice, handlers.go addVsocks

// In toFirecrackerConfig(), after existing CPU template and jailer config:
if c.VsockCID >= MinGuestCID {
    udsPath := c.vsockUDSPath()
    cfg.VsockDevices = []sdk.VsockDevice{{
        ID:   "vsock0",
        Path: udsPath,
        CID:  c.VsockCID,
    }}
}
```

### CID Allocator Integration in Manager
```go
// Source: Pattern derived from CID constraints in Firecracker API spec

type Manager struct {
    config    ManagerConfig
    vms       map[string]*VM
    machines  map[string]*sdk.Machine
    cidAlloc  *CIDAllocator  // new field
    mu        sync.RWMutex
}

func NewManager(cfg ManagerConfig) *Manager {
    cfg = cfg.withDefaults()
    return &Manager{
        config:   cfg,
        vms:      make(map[string]*VM),
        machines: make(map[string]*sdk.Machine),
        cidAlloc: NewCIDAllocator(MinGuestCID),
    }
}

// In Create(), after cfg.withDefaults():
if cfg.VsockCID == 0 {
    cfg.VsockCID = m.cidAlloc.Allocate()
}
```

### Execd Health Check with Retry
```go
// Source: firecracker-go-sdk vsock.DialContext + Go stdlib http.Client

const ExecdPort uint32 = 44772

// WaitForExecd blocks until execd responds to a health check over vsock,
// or the context is canceled.
func WaitForExecd(ctx context.Context, vsockUDSPath string) error {
    client := NewVsockHTTPClient(vsockUDSPath, ExecdPort)
    
    ticker := time.NewTicker(200 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return fmt.Errorf("execd not ready: %w", ctx.Err())
        case <-ticker.C:
            if err := pingExecd(ctx, client); err == nil {
                return nil
            }
        }
    }
}

func pingExecd(ctx context.Context, client *http.Client) error {
    reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    
    req, err := http.NewRequestWithContext(reqCtx, "GET", "http://execd/health", nil)
    if err != nil {
        return err
    }
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("unexpected status: %d", resp.StatusCode)
    }
    return nil
}
```

### VMResources Extension for vsock Cleanup
```go
// In VMResources (cleanup.go):
type VMResources struct {
    // ... existing fields ...
    // VsockUDSPath is the path to the vsock Unix domain socket.
    VsockUDSPath string
}

// In Cleanup(), add after socket cleanup:
if r.VsockUDSPath != "" {
    if err := os.Remove(r.VsockUDSPath); err != nil && !os.IsNotExist(err) {
        result = multierror.Append(result,
            fmt.Errorf("remove vsock uds %s: %w", r.VsockUDSPath, err))
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| vsock CID change at snapshot restore (not supported) | UDS path override via PUT /vsock after loadSnapshot | Firecracker v1.9+ / Go SDK handler chain | Multiple VMs can share CID 3 if they have unique UDS paths; but unique CIDs are recommended for clarity [CITED: github.com/firecracker-microvm/firecracker/issues/3344] |
| VsockID field required | VsockID deprecated since v1.0.0 | firecracker-go-sdk v1.0.0 | Still accepted but not required; pass empty string or "vsock0" [VERIFIED: SDK client/models/vsock.go line 42-43] |
| Manual CONNECT handshake in user code | SDK vsock.DialContext with retry and error classification | firecracker-go-sdk v1.0.0 | No need to implement CONNECT protocol manually [VERIFIED: SDK vsock/dial.go] |

**Deprecated/outdated:**
- VsockID field in the Vsock model: Deprecated since Firecracker v1.0.0. Still accepted for backward compatibility but not required. Use "vsock0" as a conventional value. [VERIFIED: SDK client/models/vsock.go]

## Assumptions Log

> List all claims tagged [ASSUMED] in this research.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Execd health-check endpoint is GET /health returning 200 | Pattern 3 / Code Examples | If endpoint differs (e.g., GET /ping or different path), health-check code needs adjustment. Low risk -- can be changed trivially |
| A2 | Execd listens on AF_VSOCK inside the guest (not just TCP) | Phase Requirements VSOCK-03 | If execd only supports TCP, a vsock-to-TCP proxy inside the guest is needed. This is a fundamental assumption from PROJECT.md that execd will be modified to listen on vsock |
| A3 | Atomic counter CIDs do not need to be released/recycled | Architecture Pattern 2 | If the counter wraps around 2^32, CIDs collide. In practice, a single process will never create 4 billion VMs. Zero risk for realistic workloads |
| A4 | The vsock UDS path for jailed VMs should be `vsock.sock` relative to chroot | Architecture Pattern 1 | If the jailer expects a specific path or the SDK derives it differently, the path needs adjustment. The jailer does not dictate vsock path -- it only manages the API socket [VERIFIED: SDK jailer.go, no vsock path constraints] |

## Open Questions

1. **Execd vsock listening -- is it already implemented or needs guest-side work?**
   - What we know: PROJECT.md says "execd needs to listen on vsock instead of (or in addition to) a TCP socket." The existing execd code in `components/execd/` is not checked out in this sparse checkout.
   - What's unclear: Whether execd already has AF_VSOCK support or if guest-side changes are needed.
   - Recommendation: Phase 3 scope covers the HOST-side vsock transport and wiring. Guest-side execd changes (if needed) would be a separate concern. For testing, we can verify the vsock CONNECT handshake succeeds even without a guest listener -- the handshake itself proves the device is configured correctly. Full end-to-end testing (VSOCK-03) requires a rootfs image with execd listening on vsock.

2. **SDK version and vsock_override for snapshot restore**
   - What we know: The SDK v1.0.0 does NOT include `vsock_override` in SnapshotLoadParams. However, the SDK's snapshot handler chain calls `AddVsocksHandler` AFTER `LoadSnapshotHandler`, which means vsock devices are reconfigured with the new Config.VsockDevices after loading the snapshot state. [VERIFIED: SDK handlers.go line 318-325]
   - What's unclear: Whether the Firecracker binary v1.10+ natively supports the vsock_override field in the /snapshot/load API, and whether the current SDK version wraps it.
   - Recommendation: For Phase 3, the vsock path scheme just needs to be unique per VM ID. Phase 6 (restore) will handle the override mechanics. The SDK handler chain already supports reconfiguring vsock after snapshot load, which is the correct approach regardless of the API-level vsock_override field.

3. **Integration testing without KVM**
   - What we know: Development is on macOS (darwin/arm64). Integration tests require Linux with KVM.
   - What's unclear: Whether there is a CI environment with KVM for integration tests.
   - Recommendation: Write unit tests that validate config wiring, CID allocation, path generation, and HTTP transport (using mock UDS servers). Mark integration tests with `//go:build integration` tag. The unit tests provide good coverage for the logic; integration tests verify end-to-end on Linux.

## Environment Availability

> Step 2.6: SKIPPED (phase is code/config-only changes within the existing Go module. No new external dependencies beyond what is already in go.mod. The vsock host-side transport uses standard Unix sockets -- no AF_VSOCK kernel module needed on the development host.)

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go built-in `testing` package |
| Config file | None (standard `go test`) |
| Quick run command | `go test ./... -v -short` |
| Full suite command | `go test ./... -v -timeout 3m` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VSOCK-01 | VMConfig vsock fields wire to SDK VsockDevices | unit | `go test -run TestToFirecrackerConfig_VsockDevice -v` | No -- Wave 0 |
| VSOCK-02 | CID allocator returns unique CIDs >= 3 | unit | `go test -run TestCIDAllocator -v` | No -- Wave 0 |
| VSOCK-03 | Health-check succeeds when mock execd responds on vsock | unit | `go test -run TestExecdHealthCheck -v` | No -- Wave 0 |
| VSOCK-04 | VsockHTTPClient dials UDS with CONNECT handshake | unit | `go test -run TestVsockHTTPClient -v` | No -- Wave 0 |
| VSOCK-05 | vsock UDS paths are unique per VM ID and fit 108 chars | unit | `go test -run TestVsockUDSPath -v` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./... -v -short` (< 5 seconds)
- **Per wave merge:** `go test ./... -v -timeout 3m`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `vsock_test.go` -- covers VSOCK-01, VSOCK-02, VSOCK-05 (CID allocation, path generation, config validation)
- [ ] `vsock_linux_test.go` -- covers VSOCK-01 (SDK VsockDevices wiring, build-tagged)
- [ ] `vsock_transport_test.go` -- covers VSOCK-04 (HTTP-over-vsock with mock UDS server)
- [ ] `vsock_health_test.go` -- covers VSOCK-03 (health check with mock server)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | N/A (vsock is a VM-internal transport, not user-facing) |
| V3 Session Management | no | N/A |
| V4 Access Control | yes | CID isolation -- each VM gets unique CID; UDS paths are per-VM; no cross-VM vsock access possible by Firecracker design |
| V5 Input Validation | yes | CID >= 3 validation; UDS path length <= 108; VMConfig.Validate() |
| V6 Cryptography | no | N/A (vsock is a local VM transport, not network-facing) |

### Known Threat Patterns for Firecracker vsock

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| CID collision allowing cross-VM communication | Spoofing | Unique CID per VM via atomic allocator; Firecracker enforces CID isolation at the hypervisor level |
| Stale UDS file hijack | Tampering | Remove stale socket files before binding; use unique paths per VM instance |
| UDS path traversal | Tampering | Paths are constructed programmatically from validated VM IDs; VM ID regex `[a-zA-Z0-9-]+` prevents path traversal |

## Sources

### Primary (HIGH confidence)
- firecracker-go-sdk v1.0.0 source code (local module cache) -- VsockDevice struct, vsock/dial.go, vsock/listener.go, handlers.go, machine.go
- firecracker-go-sdk client/models/vsock.go -- GuestCid minimum: 3, UdsPath required, VsockID deprecated
- Existing codebase: runtime/firecracker/ -- VMConfig, toFirecrackerConfig(), Manager, VMResources, jailer.go

### Secondary (MEDIUM confidence)
- [Firecracker vsock.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md) -- CONNECT protocol, host-initiated connections, guest-initiated connections
- [Firecracker snapshot-support.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md) -- vsock device reset on snapshot, listen sockets survive restore
- [Firecracker issue #3344](https://github.com/firecracker-microvm/firecracker/issues/3344) -- vsock UDS path override on snapshot restore (implemented, merged)

### Tertiary (LOW confidence)
- [vm0 DeepWiki](https://deepwiki.com/vm0-ai/vm0/3.6.2-firecracker-vm-management) -- CID allocation patterns, inotify-based socket readiness

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries verified in go.mod and SDK source code
- Architecture: HIGH -- patterns directly derived from SDK handler chain and existing codebase conventions
- Pitfalls: HIGH -- verified against SDK source code and Firecracker documentation

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable -- SDK v1.0.0 is a stable release, Firecracker vsock API is mature)

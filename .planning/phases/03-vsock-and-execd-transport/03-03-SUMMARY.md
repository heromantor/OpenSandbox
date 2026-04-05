---
phase: 03-vsock-and-execd-transport
plan: 03
subsystem: runtime
tags: [vsock, http-transport, health-check, firecracker, execd]

# Dependency graph
requires:
  - phase: 01-vm-lifecycle-and-jailer
    provides: "Go module with firecracker-go-sdk dependency in go.mod"
provides:
  - "NewVsockHTTPClient: HTTP client routed through vsock UDS with CONNECT handshake"
  - "WaitForExecd: Ticker-based health check polling execd /health over vsock"
  - "ExecdPort constant (44772) for guest-side execd port"
affects: [03-01-vsock-lifecycle, 04-networking, snapshot-restore]

# Tech tracking
tech-stack:
  added: [firecracker-go-sdk/vsock]
  patterns: [vsock-over-http transport, ticker-based health polling, mock-UDS testing]

key-files:
  created:
    - runtime/firecracker/vsock_transport.go
    - runtime/firecracker/vsock_health.go
    - runtime/firecracker/vsock_transport_test.go
    - runtime/firecracker/vsock_health_test.go
  modified:
    - runtime/firecracker/go.mod
    - runtime/firecracker/go.sum

key-decisions:
  - "Defined ExecdPort locally in vsock_health.go since vsock.go from Plan 01 not yet merged"
  - "Used connResponseWriter adapter for mock HTTP serving over raw net.Conn in tests"

patterns-established:
  - "Mock vsock UDS: simulate CONNECT handshake + HTTP serving for unit testing vsock transport"
  - "Ticker-based health polling with context timeout for guest readiness detection"

requirements-completed: [VSOCK-03, VSOCK-04]

# Metrics
duration: 5min
completed: 2026-04-05
---

# Phase 03 Plan 03: Vsock Transport and Execd Health Check Summary

**HTTP-over-vsock transport wrapping fcvsock.DialContext and ticker-based WaitForExecd health check on port 44772**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-05T18:22:41Z
- **Completed:** 2026-04-05T18:27:36Z
- **Tasks:** 2
- **Files created:** 4
- **Files modified:** 2 (go.mod, go.sum)

## Accomplishments
- NewVsockHTTPClient wraps fcvsock.DialContext in http.Transport for transparent HTTP-over-vsock
- WaitForExecd polls GET /health every 200ms with 2s per-request timeout until 200 OK or context done
- Mock UDS tests prove full CONNECT handshake + HTTP request flow end-to-end
- 9 tests total: 4 transport + 5 health check, all passing within 30s

## Task Commits

Each task was committed atomically:

1. **Task 1: Create vsock_transport.go with NewVsockHTTPClient and tests** - `84cab66` (test: RED), `c5442f1` (feat: GREEN)
2. **Task 2: Create vsock_health.go with WaitForExecd and tests** - `81dc088` (test: RED), `81c4cb2` (feat: GREEN)

_TDD tasks have two commits each (failing test then passing implementation)_

## Files Created/Modified
- `runtime/firecracker/vsock_transport.go` - NewVsockHTTPClient wrapping fcvsock.DialContext in http.Transport
- `runtime/firecracker/vsock_transport_test.go` - 4 tests: non-nil, transport type, mock UDS success, connect refused
- `runtime/firecracker/vsock_health.go` - WaitForExecd and pingExecd with ExecdPort constant
- `runtime/firecracker/vsock_health_test.go` - 5 tests: success, retry, cancel, timeout, non-200 status
- `runtime/firecracker/go.mod` - Added firecracker-go-sdk/vsock transitive deps
- `runtime/firecracker/go.sum` - Updated checksums

## Decisions Made
- Defined ExecdPort (44772) locally in vsock_health.go because vsock.go from Plan 01 does not yet exist in this branch. When Plan 01 merges, the duplicate constant should be deduplicated (one definition in vsock.go).
- Used a custom connResponseWriter adapter in health check tests to serve HTTP responses over raw net.Conn after the CONNECT handshake, avoiding complex http.Server plumbing for single-connection mocks.
- Used os.CreateTemp in /tmp for test UDS paths to stay under the 108-character Unix domain socket path limit on macOS.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Missing go.sum entries for vsock transitive dependencies**
- **Found during:** Task 1 (GREEN phase, first test run)
- **Issue:** firecracker-go-sdk/vsock imports github.com/mdlayher/vsock which was not in go.sum
- **Fix:** Ran `go get github.com/firecracker-microvm/firecracker-go-sdk/vsock@v1.0.0` and `go mod tidy`
- **Files modified:** runtime/firecracker/go.mod, runtime/firecracker/go.sum
- **Verification:** `go build ./...` and all tests pass
- **Committed in:** c5442f1 (Task 1 GREEN commit)

**2. [Rule 1 - Bug] Unix domain socket path too long on macOS**
- **Found during:** Task 1 (GREEN phase, mock UDS test)
- **Issue:** t.TempDir() produces paths exceeding 108-char Unix socket limit, causing "bind: invalid argument"
- **Fix:** Changed startMockVsockUDS to use os.CreateTemp in /tmp for shorter paths with cleanup
- **Files modified:** runtime/firecracker/vsock_transport_test.go
- **Verification:** TestVsockHTTPClient_MockUDS_Success passes
- **Committed in:** c5442f1 (Task 1 GREEN commit)

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both auto-fixes necessary for correct builds and test execution. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations above.

## Threat Model Compliance

- **T-03-09 (DoS - WaitForExecd retry loop):** Mitigated. Context with timeout required by caller; pingExecd uses 2s per-request timeout; ticker-based polling (200ms) prevents tight loops.
- **T-03-08, T-03-10, T-03-11:** Accepted as per threat model -- no additional mitigation required.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- vsock transport and health check are ready for integration with VM lifecycle (Plan 01)
- ExecdPort constant needs deduplication when vsock.go from Plan 01 is merged
- WaitForExecd can be called from VM.Start() to block until guest execd is reachable

---
*Phase: 03-vsock-and-execd-transport*
*Completed: 2026-04-05*

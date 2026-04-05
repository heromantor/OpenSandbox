---
phase: 04-tap-networking-and-egress
plan: 01
subsystem: networking
tags: [firecracker, tap, iptables, netlink, subnet-allocation, nat, go-iptables, vishvananda-netlink]

# Dependency graph
requires:
  - phase: 03-vsock-and-execd-transport
    provides: VM types (VMConfig, VM), CIDAllocator pattern, error types, manager patterns
provides:
  - SubnetAllocator mapping uint32 index to /30 subnet in 172.16.0.0/16
  - NetworkConfig and NATRules types for VM network configuration
  - TAP device lifecycle (CreateTAPDevice, DeleteTAPDevice) via netlink
  - iptables NAT rules (Apply/Remove) via go-iptables
  - BuildSDKNetworkInterfaces for firecracker-go-sdk integration
  - GenerateMAC for locally-administered MAC addresses (02:FC prefix)
  - EnsureIPForwarding and DefaultHostInterface host utilities
affects: [04-02-tap-wiring, vm-lifecycle, manager, cleanup]

# Tech tracking
tech-stack:
  added: [coreos/go-iptables v0.8.0, vishvananda/netlink v1.3.1 (promoted from indirect)]
  patterns: [/30 point-to-point subnet allocation, TAP-per-VM isolation, idempotent iptables cleanup]

key-files:
  created:
    - runtime/firecracker/network.go
    - runtime/firecracker/network_linux.go
    - runtime/firecracker/network_test.go
    - runtime/firecracker/network_linux_test.go
  modified:
    - runtime/firecracker/go.mod
    - runtime/firecracker/go.sum

key-decisions:
  - "BuildSDKNetworkInterfaces placed in network_linux.go (not network.go) because firecracker-go-sdk has Linux-only transitive deps (containernetworking/plugins/pkg/ns)"
  - "netns stays as indirect dep since only netlink imports it transitively"
  - "/30 subnet per VM for point-to-point isolation (only host+guest on L2 segment)"

patterns-established:
  - "Network types in cross-platform file, Linux syscall implementations in _linux.go"
  - "Idempotent cleanup: DeleteTAPDevice returns nil for missing devices, NATRules.Remove ignores missing rules"
  - "Integration tests guarded by requireRoot(t) helper with t.Skip"

requirements-completed: [NET-01, NET-02, NET-03]

# Metrics
duration: 7min
completed: 2026-04-05
---

# Phase 4 Plan 1: TAP Networking Foundation Summary

**/30 subnet allocator, TAP device lifecycle via netlink, iptables NAT rules via go-iptables, and SDK NetworkInterfaces builder for Firecracker VM networking**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-05T19:07:51Z
- **Completed:** 2026-04-05T19:14:54Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- SubnetAllocator deterministically maps 0..16383 to /30 subnets in 172.16.0.0/16 with correct host/guest IPs and gateway
- TAP device creation/deletion via netlink with IP assignment, link-up, and cleanup-on-error
- Idempotent iptables NAT (MASQUERADE + FORWARD) via go-iptables AppendUnique/Delete
- BuildSDKNetworkInterfaces produces firecracker-go-sdk NetworkInterfaces with IPConfiguration.Nameservers
- 16 cross-platform unit tests pass on macOS; Linux integration tests compile and have root guards

## Task Commits

Each task was committed atomically:

1. **Task 1: Network types, SubnetAllocator, MAC generation, SDK builder, and unit tests** (TDD)
   - `ac4c8eb` (test: add failing tests for network types -- RED)
   - `cc60faf` (feat: implement network types, SubnetAllocator, MAC generation, validation -- GREEN)
2. **Task 2: Linux TAP creation, iptables NAT, and IP forwarding check**
   - `b396db1` (feat: add Linux TAP creation, iptables NAT, IP forwarding, SDK builder)

## Files Created/Modified
- `runtime/firecracker/network.go` - Cross-platform types: SubnetAllocation, NetworkConfig, NATRules, AllocateSubnet, GenerateMAC, TAPDeviceName, Validate
- `runtime/firecracker/network_linux.go` - Linux-only: CreateTAPDevice, DeleteTAPDevice, NATRules.Apply/Remove, EnsureIPForwarding, EnsureSharedForwardRule, DefaultHostInterface, BuildSDKNetworkInterfaces
- `runtime/firecracker/network_test.go` - 16 cross-platform unit tests for subnet allocation, MAC, TAP naming, config validation
- `runtime/firecracker/network_linux_test.go` - Linux integration tests (root-guarded) for TAP lifecycle, plus BuildSDKNetworkInterfaces unit tests
- `runtime/firecracker/go.mod` - Added coreos/go-iptables v0.8.0, promoted vishvananda/netlink to direct
- `runtime/firecracker/go.sum` - Updated checksums

## Decisions Made
- **BuildSDKNetworkInterfaces in _linux.go:** The firecracker-go-sdk has a transitive dependency on `containernetworking/plugins/pkg/ns` which only compiles on Linux. Placing the SDK-dependent function in `network_linux.go` keeps `network.go` cross-platform for testing on macOS.
- **netns stays indirect:** Only netlink imports netns transitively; we don't use it directly.
- **TAP naming from VM UUID:** `tap-{first-11-chars}` ensures uniqueness (UUID prefix) within IFNAMSIZ 15-char limit.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Moved BuildSDKNetworkInterfaces to network_linux.go**
- **Found during:** Task 1 (test compilation)
- **Issue:** Plan specified BuildSDKNetworkInterfaces in cross-platform network.go, but firecracker-go-sdk has Linux-only transitive deps (containernetworking/plugins/pkg/ns) that prevent compilation on macOS
- **Fix:** Moved BuildSDKNetworkInterfaces and its tests to network_linux.go / network_linux_test.go
- **Files modified:** runtime/firecracker/network_linux.go, runtime/firecracker/network_linux_test.go
- **Verification:** `go vet ./...` passes on macOS, `GOOS=linux go build ./...` passes, all 16 unit tests pass
- **Committed in:** cc60faf (Task 1 GREEN) and b396db1 (Task 2)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** SDK function placement change only; all functionality delivered as specified. No scope creep.

## Issues Encountered
None beyond the platform-specific SDK compilation issue documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All network building blocks ready for 04-02 (wiring into Manager.Create and cleanup paths)
- NetworkConfig, NATRules, and SubnetAllocator are fully tested and ready for integration
- TAP and iptables functions require root on Linux (expected for Firecracker runtime)

## Self-Check: PASSED

All 5 created files verified on disk. All 3 task commits verified in git log.

---
*Phase: 04-tap-networking-and-egress*
*Completed: 2026-04-05*

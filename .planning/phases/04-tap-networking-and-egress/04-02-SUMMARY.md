---
phase: 04-tap-networking-and-egress
plan: 02
subsystem: infra
tags: [firecracker, networking, tap, iptables, nat, egress-proxy, dns]

# Dependency graph
requires:
  - phase: 04-01
    provides: "NetworkConfig, SubnetAllocator, NATRules, TAP/iptables helpers, BuildSDKNetworkInterfaces"
provides:
  - "VMConfig.NetworkConfig field wired into validation and toFirecrackerConfig"
  - "Manager.Create provisions TAP device and iptables NAT rules before Machine creation"
  - "VMResources tracks TAPDeviceName and NATRules for cleanup"
  - "cleanup_linux.go / cleanup_other.go platform-specific network cleanup"
  - "SubnetIndexAllocator for monotonic unique subnet indices"
  - "ManagerConfig.EgressProxyAddr for DNS-based egress policy enforcement"
  - "ManagerConfig.HostInterface for explicit or auto-detected NAT interface"
affects: [04-03-egress-proxy, 05-snapshot-and-restore]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Platform-specific cleanup via build tags (cleanup_linux.go / cleanup_other.go)"
    - "Manager-level resource allocator pattern (SubnetIndexAllocator mirrors CIDAllocator)"
    - "Rollback-on-failure for TAP/NAT provisioning in Manager.Create"

key-files:
  created:
    - runtime/firecracker/cleanup_linux.go
    - runtime/firecracker/cleanup_other.go
  modified:
    - runtime/firecracker/vm.go
    - runtime/firecracker/vm_linux.go
    - runtime/firecracker/manager.go
    - runtime/firecracker/manager_linux.go
    - runtime/firecracker/cleanup.go
    - runtime/firecracker/network.go
    - runtime/firecracker/vm_test.go
    - runtime/firecracker/vm_linux_test.go
    - runtime/firecracker/cleanup_test.go

key-decisions:
  - "SubnetIndexAllocator placed in network.go (cross-platform) rather than manager_linux.go since it has no Linux deps and tests run on macOS"
  - "cleanupNetwork split into cleanup_linux.go and cleanup_other.go via build tags to keep cleanup.go cross-platform"
  - "EgressProxyAddr prepended as first DNS nameserver with max 2 limit to match SDK/kernel constraint"

patterns-established:
  - "Platform-split cleanup: cleanup.go (cross-platform orchestrator) delegates to cleanupNetwork() defined per-platform"
  - "Manager.Create rollback: TAP and NAT cleaned up if sdk.NewMachine or toFirecrackerConfig fails after provisioning"

requirements-completed: [NET-04, NET-05]

# Metrics
duration: 5min
completed: 2026-04-05
---

# Phase 04 Plan 02: Network Config Wiring Summary

**TAP device provisioning, iptables NAT rules, and egress proxy DNS injection wired into VM lifecycle with rollback-on-failure and platform-specific cleanup**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-05T19:19:02Z
- **Completed:** 2026-04-05T19:24:45Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- VMConfig.NetworkConfig field integrated into validation and toFirecrackerConfig (nil = no network, backward compatible)
- Manager.Create provisions TAP device and iptables NAT rules BEFORE sdk.NewMachine with full rollback on failure
- VMResources cleanup extended with TAPDeviceName and NATRules, using platform-specific cleanup files
- ManagerConfig.EgressProxyAddr prepended to guest DNS nameservers for FQDN-based egress policy enforcement
- SubnetIndexAllocator provides monotonic unique subnet indices (mirrors CIDAllocator pattern)

## Task Commits

Each task was committed atomically (TDD: RED then GREEN):

1. **Task 1: Extend VMConfig, ManagerConfig, and toFirecrackerConfig** - `0d6e1b0` (test), `0cf3434` (feat)
2. **Task 2: VMResources cleanup, Manager.Create TAP provisioning, egress proxy** - `38d1335` (test), `20266a8` (feat)

_TDD tasks had separate RED (test) and GREEN (feat) commits._

## Files Created/Modified
- `runtime/firecracker/vm.go` - Added NetworkConfig field to VMConfig and VM structs; validation delegation
- `runtime/firecracker/vm_linux.go` - toFirecrackerConfig populates cfg.NetworkInterfaces via BuildSDKNetworkInterfaces
- `runtime/firecracker/manager.go` - ManagerConfig extended with HostInterface and EgressProxyAddr
- `runtime/firecracker/manager_linux.go` - Manager.Create: subnet allocation, TAP provisioning, NAT rules, egress proxy DNS, rollback
- `runtime/firecracker/cleanup.go` - VMResources: TAPDeviceName, NATRules fields; IsEmpty update; cleanupNetwork call
- `runtime/firecracker/cleanup_linux.go` - Platform-specific TAP deletion and NAT rule removal
- `runtime/firecracker/cleanup_other.go` - No-op cleanupNetwork for non-Linux
- `runtime/firecracker/network.go` - SubnetIndexAllocator type (atomic, monotonic)
- `runtime/firecracker/vm_test.go` - Tests for nil/invalid NetworkConfig validation, ManagerConfig defaults
- `runtime/firecracker/vm_linux_test.go` - Tests for toFirecrackerConfig with/without NetworkConfig, custom nameservers
- `runtime/firecracker/cleanup_test.go` - Tests for IsEmpty with TAP/NAT, cleanup backward compat, SubnetIndexAllocator

## Decisions Made
- **SubnetIndexAllocator in network.go:** Placed in cross-platform file since it uses only sync/atomic (no Linux deps). This allows tests to run on macOS CI.
- **Platform-split cleanup pattern:** cleanup.go remains cross-platform; cleanupNetwork() dispatched to cleanup_linux.go (real TAP/iptables) or cleanup_other.go (no-op).
- **DNS nameserver limit:** EgressProxyAddr prepended then truncated to max 2 nameservers, matching Firecracker SDK and Linux kernel limits.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Network config fully wired into VM lifecycle; VMs created with NetworkConfig get TAP devices, NAT rules, and DNS
- Ready for Plan 04-03 (egress proxy integration) which will use EgressProxyAddr to route guest DNS through the proxy
- All existing Phase 1-3 tests continue to pass (backward compatible nil NetworkConfig)

---
*Phase: 04-tap-networking-and-egress*
*Completed: 2026-04-05*

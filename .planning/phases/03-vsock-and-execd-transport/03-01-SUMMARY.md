---
phase: "03"
plan: "01"
subsystem: "runtime/firecracker"
tags: [vsock, cid-allocator, uds-path, vm-config, cleanup]
dependency_graph:
  requires: ["02-01", "02-02", "02-03"]
  provides: ["CIDAllocator", "MinGuestCID", "ExecdPort", "vsockUDSPath", "VMConfig.VsockCID", "VMResources.VsockUDSPath"]
  affects: ["runtime/firecracker/vm.go", "runtime/firecracker/cleanup.go"]
tech_stack:
  added: ["sync/atomic"]
  patterns: ["atomic counter for CID allocation", "UDS path generation per jailed/non-jailed mode"]
key_files:
  created:
    - runtime/firecracker/vsock.go
    - runtime/firecracker/vsock_test.go
  modified:
    - runtime/firecracker/vm.go
    - runtime/firecracker/cleanup.go
    - runtime/firecracker/cleanup_test.go
decisions:
  - "Used atomic.Uint32.Add(1)-1 pattern for lock-free CID allocation (no mutex needed)"
  - "Jailed vsock UDS path is just 'vsock.sock' (relative in chroot), non-jailed uses temp dir with VM ID"
  - "VsockCID validation only rejects 1 and 2; 0 means auto-assign, >=3 is explicit"
  - "UDS path length validation skipped for jailed VMs (chroot path always short enough)"
metrics:
  duration: "214s"
  completed: "2026-04-05T18:25:08Z"
  tasks_completed: 2
  tasks_total: 2
  files_created: 2
  files_modified: 3
  test_count: 14
---

# Phase 03 Plan 01: Vsock Foundation Types Summary

Atomic CID allocator with sync/atomic, vsock UDS path helpers for jailed/non-jailed VMs, VMConfig/VM/VMResources extensions with validation rejecting reserved CIDs 1-2 and UDS paths exceeding 108 chars.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | CIDAllocator, constants, UDS path helpers (TDD) | `dd87bca` (RED), `ae8024b` (GREEN) | `vsock.go`, `vsock_test.go` |
| 2 | Extend VMConfig, VM, VMResources for vsock | `3ff3e72` | `vm.go`, `cleanup.go`, `cleanup_test.go`, `vsock_test.go` |

## Implementation Details

### vsock.go (49 lines)
- `MinGuestCID uint32 = 3` -- minimum valid guest CID (0=hypervisor, 1=reserved, 2=host)
- `ExecdPort uint32 = 44772` -- execd agent listen port inside guest
- `CIDAllocator` struct with `atomic.Uint32` -- `NewCIDAllocator(firstCID)` / `Allocate() uint32`
- `vsockUDSPath(id, jailerEnabled)` -- returns `"vsock.sock"` (jailed) or `<tmpdir>/firecracker-vsock-<id>.sock`

### vm.go changes
- Added `VsockCID uint32` to `VMConfig` (after `TrackDirtyPages`)
- Added `VsockCID uint32` and `VsockUDSPath string` to `VM` struct
- Extended `Validate()`: rejects CID 1/2, validates UDS path length for non-jailed VMs

### cleanup.go changes
- Added `VsockUDSPath string` to `VMResources`
- `Cleanup()` removes vsock UDS file (between socket and chroot removal)
- `IsEmpty()` includes `VsockUDSPath == ""` check

### Test coverage (14 new tests, 160 lines)
- CIDAllocator: start value, monotonic increment, 1000-goroutine concurrent uniqueness, custom start
- Constants: MinGuestCID=3, ExecdPort=44772
- vsockUDSPath: non-jailed path format, jailed returns "vsock.sock", unique per ID, length within 108
- VMConfig validation: rejects CID 1/2, accepts 0/3/1000
- VMResources: vsock UDS cleanup removes file, IsEmpty detects VsockUDSPath

## Decisions Made

1. **Lock-free CID allocation:** Used `atomic.Uint32.Add(1)-1` instead of mutex-protected counter. The Add returns the new value, so subtracting 1 gives the pre-increment value. This is simpler and faster than mutex for a monotonic counter.

2. **Jailed UDS path is relative:** Jailed VMs get `"vsock.sock"` (Firecracker creates it inside the chroot). Non-jailed VMs get an absolute temp path. This mirrors the existing `socketPath()` pattern in vm.go.

3. **UDS path length validation scope:** Only validate non-jailed paths against the 108-char limit. Jailed paths are always short (`<chroot>/vsock.sock`) and the chroot base is already validated in `validateJailerOpts`.

4. **VsockCID=0 means auto-assign:** Zero value signals the Manager should allocate from its CIDAllocator. This follows Go convention of zero-value meaning "use default".

## Deviations from Plan

None -- plan executed exactly as written.

## Threat Surface Scan

No new threat surface introduced beyond what the plan's threat model covers. The CID allocator uses atomic operations (T-03-01 mitigated). UDS paths are constructed from validated VM IDs with regex `^[a-zA-Z0-9-]+$` preventing path traversal (T-03-02 mitigated). Path length validated against 108-char limit (T-03-03 mitigated).

## Self-Check: PASSED

All 6 files verified present. All 3 commits verified in git log.

---
phase: 01-vm-lifecycle-and-jailer
plan: 02
subsystem: runtime/firecracker
tags: [vm-lifecycle, firecracker-sdk, jailer, manager, makefile]
dependency_graph:
  requires: [01-01]
  provides: [vm-lifecycle-implementation, makefile]
  affects: [runtime/firecracker/manager.go, runtime/firecracker/vm.go, runtime/firecracker/jailer.go]
tech_stack:
  added: []
  patterns: [linux-build-tags, sdk-config-translation, two-phase-shutdown]
key_files:
  created:
    - runtime/firecracker/vm_linux.go
    - runtime/firecracker/jailer_linux.go
    - runtime/firecracker/manager_linux.go
    - runtime/firecracker/Makefile
  modified:
    - runtime/firecracker/vm.go
    - runtime/firecracker/jailer.go
    - runtime/firecracker/manager.go
  deleted:
    - runtime/firecracker/tools.go
decisions:
  - "Used Linux build tags to separate SDK-dependent code from cross-platform types, enabling macOS compilation"
  - "Set DisableValidation=true on SDK Config since we validate at VMConfig level and kernel/rootfs may not exist on dev host"
  - "Manager uses two-phase shutdown: Ctrl+Alt+Del first, SIGTERM fallback, then Wait with process-exited error tolerance"
metrics:
  duration: 4m
  completed: 2026-04-05
  tasks_completed: 2
  tasks_total: 2
  files_created: 4
  files_modified: 3
  files_deleted: 1
---

# Phase 01 Plan 02: VM Lifecycle Manager Implementation Summary

Full VMManager implementation with firecracker-go-sdk Config translation, Jailer integration, two-phase shutdown, and Makefile for the runtime module.

## What Was Done

### Task 1: VMConfig-to-SDK translation and Manager lifecycle methods (646bccc)

Implemented the complete VM lifecycle through five Manager methods backed by the firecracker-go-sdk:

**VMConfig translation (vm_linux.go):**
- `toFirecrackerConfig()` translates VMConfig to `sdk.Config` including drives, machine configuration, CPU template, and optional Jailer configuration
- `socketPath()` resolves API socket path with jailer-awareness (relative in chroot vs absolute temp path)
- CPU template applied via `models.CPUTemplate` cast when `Static != TemplateNone`
- `DisableValidation: true` set on SDK Config since our VMConfig.Validate() handles validation and the kernel/rootfs paths may not exist on macOS dev machines

**Jailer SDK conversion (jailer_linux.go):**
- `toSDKConfig()` on `resolvedJailerConfig` converts to `*sdk.JailerConfig` with proper pointer semantics for UID, GID, and optional NumaNode

**Manager implementation (manager_linux.go):**
- `Create`: validates config (VMConfig, CPUTemplate, kernel image), checks for duplicate IDs, calls `toFirecrackerConfig()`, creates SDK Machine via `sdk.NewMachine`, builds VMResources for cleanup tracking, stores VM and Machine references
- `Start`: transitions Created -> Starting -> Running, calls `machine.Start(ctx)`, records PID via `machine.PID()`
- `Stop`: transitions Running -> Stopping -> Stopped, two-phase shutdown (Ctrl+Alt+Del via `machine.Shutdown`, then SIGTERM via `machine.StopVMM`), waits for exit via `machine.Wait`, tolerates "process already exited" errors
- `Destroy`: stops running VMs first, calls `vm.Resources.Cleanup()` for socket/chroot/cgroup removal, removes from registry, wraps cleanup errors in `CleanupError`
- `Get`: returns shallow copy of VM to prevent external mutation of internal state

**Architecture decision -- Linux build tags:**
The firecracker-go-sdk imports `containernetworking/plugins/pkg/ns` which only builds on Linux. To enable macOS compilation (for development and CI), all SDK-dependent code is in `_linux.go` files with `//go:build linux` tags. Cross-platform types (VMConfig, VM, VMManager, ManagerConfig, errors, etc.) remain in non-tagged files.

### Task 2: Makefile and build verification (7f3c866)

Created `runtime/firecracker/Makefile` following the pattern from `sdks/sandbox/go/Makefile`:
- Targets: `build`, `vet`, `test`, `test-integration`, `lint`, `clean`, `fmt`, `check`
- `make build` and `make vet` verified passing on macOS

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] SDK cannot compile on macOS due to Linux-only transitive dependency**
- **Found during:** Task 1
- **Issue:** `firecracker-go-sdk` imports `containernetworking/plugins/pkg/ns` which only has `ns_linux.go` -- no cross-platform files. This causes `go build` to fail on macOS.
- **Fix:** Split all SDK-importing code into `_linux.go` files with `//go:build linux` build tags. Cross-platform types remain in untagged files. Both `GOOS=darwin` and `GOOS=linux` builds succeed.
- **Files created:** `vm_linux.go`, `jailer_linux.go`, `manager_linux.go`
- **Commit:** 646bccc

**2. [Rule 3 - Blocking] tools.go no longer needed after direct SDK imports**
- **Found during:** Task 1
- **Issue:** `tools.go` (with `//go:build tools` tag) existed to pin the SDK dependency in go.mod before it was actively used. With SDK now directly imported in `_linux.go` files, tools.go became redundant and would cause unused import warnings.
- **Fix:** Deleted `tools.go`.
- **Files deleted:** `tools.go`
- **Commit:** 646bccc

## Decisions Made

1. **Linux build tags for SDK isolation**: Used `//go:build linux` on all files importing the firecracker-go-sdk. This allows the module to compile on macOS for development while the full implementation only compiles on Linux where Firecracker actually runs.

2. **DisableValidation on SDK Config**: Set `DisableValidation: true` to bypass the SDK's built-in file existence checks for kernel image and rootfs. Our own `VMConfig.Validate()` and `ValidateKernelImage()` handle validation, and the SDK's checks would fail on macOS where these files don't exist.

3. **Two-phase shutdown with error tolerance**: Stop() tries graceful Ctrl+Alt+Del first, falls back to SIGTERM, then waits. Process-exited errors during Wait are tolerated since they indicate successful termination.

## Threat Model Compliance

- **T-02-01 (Elevation of Privilege)**: JailerEnabled is configurable per VMConfig. When enabled, full jailer chroot/cgroup/seccomp isolation is applied via SDK JailerConfig. Production deployment guide will mandate JailerEnabled=true (documentation deferred).
- **T-02-03 (DoS - Stop hangs)**: Stop uses context-aware Shutdown, then StopVMM (SIGTERM), then Wait with context deadline. If context expires, Wait returns context error.
- **T-02-02, T-02-04, T-02-05**: Accepted per plan -- path validation, orphan cleanup, and access control addressed in future phases.

## Self-Check: PASSED

All files verified present, all commits verified in git log, tools.go confirmed deleted.

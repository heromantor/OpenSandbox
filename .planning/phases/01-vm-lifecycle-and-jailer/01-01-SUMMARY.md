---
phase: 01-vm-lifecycle-and-jailer
plan: 01
subsystem: runtime
tags: [firecracker, go, microvm, jailer, cgroup, cpu-template, kernel]

# Dependency graph
requires: []
provides:
  - VMConfig, VM, VMState types for Firecracker VM configuration and lifecycle
  - VMManager interface defining Create/Start/Stop/Destroy/Get contract
  - JailerOpts with cgroup v2 auto-detection and socket path validation
  - KernelManifest with version pinning at 5.10
  - CPUTemplateConfig supporting static (T2/T2S/C3) and custom JSON templates
  - VMResources cleanup with multierror aggregation
  - Manager struct implementing VMManager (stubs for Plan 02)
  - Error types for runtime-specific failures
affects: [01-02, 01-03, 02-snapshot-lifecycle]

# Tech tracking
tech-stack:
  added:
    - github.com/firecracker-microvm/firecracker-go-sdk v1.0.0
    - github.com/google/uuid v1.6.0
    - github.com/hashicorp/go-multierror v1.1.1
    - github.com/sirupsen/logrus v1.9.4
  patterns:
    - VMManager interface for testable VM lifecycle abstraction
    - Config struct with withDefaults() and Validate() methods
    - Compile-time interface check via var _ VMManager = (*Manager)(nil)
    - Error types following "firecracker: domain: message" prefix pattern
    - Cgroup version auto-detection via /sys/fs/cgroup/cgroup.controllers
    - resolvedJailerConfig unexported struct as intermediate before SDK translation

key-files:
  created:
    - runtime/firecracker/go.mod
    - runtime/firecracker/vm.go
    - runtime/firecracker/errors.go
    - runtime/firecracker/manager.go
    - runtime/firecracker/jailer.go
    - runtime/firecracker/kernel.go
    - runtime/firecracker/cpu_template.go
    - runtime/firecracker/cleanup.go
    - runtime/firecracker/tools.go
  modified: []

key-decisions:
  - "Module path github.com/alibaba/OpenSandbox/runtime/firecracker matches existing SDK module org prefix"
  - "Used tools.go with //go:build tools to pin firecracker-go-sdk in go.mod without direct imports yet"
  - "Jailer socket path validation uses 36-char UUID placeholder to estimate max path length against 108-char limit"
  - "Manager VMManager stubs return errors.New('not implemented') for Plan 02 to fill in"

patterns-established:
  - "Config-with-defaults: VMConfig.withDefaults() fills zero-values, Validate() checks invariants"
  - "Error prefix convention: 'firecracker: {domain}: {message}' matching existing SDK pattern"
  - "Interface-first: VMManager defined before implementation for testability"
  - "Resource tracking: VMResources struct collects all paths needing cleanup"

requirements-completed: [VMLC-01, VMLC-04, VMLC-05, VMLC-06]

# Metrics
duration: 6min
completed: 2026-04-05
---

# Phase 01 Plan 01: Foundation Types Summary

**Firecracker runtime Go module with VMManager interface, jailer config with cgroup v2 detection, kernel manifest pinned at 5.10, and CPU template supporting T2/T2S/C3 and custom JSON**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-05T03:19:16Z
- **Completed:** 2026-04-05T03:25:16Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Created runtime/firecracker Go module with all foundation types that Plan 02 builds against
- Defined VMManager interface (Create/Start/Stop/Destroy/Get) with compile-time check on Manager
- Implemented VMConfig validation covering vCPU range, memory minimum, path requirements, and ID format
- Built JailerOpts with cgroup v2 auto-detection and Unix socket path length validation (108-char limit)
- Defined CPUTemplateConfig supporting both deprecated static templates and forward-compatible custom JSON
- Implemented VMResources.Cleanup() with go-multierror aggregation for deterministic resource cleanup

## Task Commits

Each task was committed atomically:

1. **Task 1: Initialize Go module and define core types** - `316d81b` (feat)
2. **Task 2: Define Jailer, kernel, CPU template, and cleanup types** - `5c7b706` (feat)
3. **Task 2b: Pin firecracker-go-sdk and logrus dependencies** - `9d7eee3` (chore)

## Files Created/Modified
- `runtime/firecracker/go.mod` - Go module declaration with firecracker-go-sdk, uuid, go-multierror, logrus
- `runtime/firecracker/go.sum` - Dependency checksums
- `runtime/firecracker/vm.go` - VMConfig, VMState, VM types and VMManager interface with validation
- `runtime/firecracker/errors.go` - VMNotFoundError, VMAlreadyExistsError, InvalidVMConfigError, VMStartError, VMStopError, CleanupError
- `runtime/firecracker/manager.go` - Manager struct, ManagerConfig, NewManager constructor, VMManager stub methods
- `runtime/firecracker/jailer.go` - JailerOpts, detectCgroupVersion, resolveJailerConfig, validateJailerOpts, path helpers
- `runtime/firecracker/kernel.go` - KernelManifest, ValidateKernelImage, ResolveKernelPath, version constants
- `runtime/firecracker/cpu_template.go` - StaticTemplate (T2/T2S/C3), CPUTemplateConfig with Validate/IsSet
- `runtime/firecracker/cleanup.go` - VMResources with Cleanup() using go-multierror and IsEmpty()
- `runtime/firecracker/tools.go` - Build-tagged imports pinning firecracker-go-sdk and logrus

## Decisions Made
- Used `github.com/alibaba/OpenSandbox/runtime/firecracker` module path to match existing SDK org prefix from `sdks/sandbox/go/go.mod`
- Pinned firecracker-go-sdk via tools.go build tag since no code imports it yet (Plan 02 will use it directly)
- Added sparse-checkout pattern for `runtime/firecracker/` to enable git operations in worktree
- Used unexported `resolvedJailerConfig` struct as intermediate representation before SDK translation in Plan 02

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added sparse-checkout pattern for runtime/firecracker/**
- **Found during:** Task 1 (commit stage)
- **Issue:** Git rejected staging files outside sparse-checkout definition
- **Fix:** Added `runtime/firecracker/` to sparse-checkout patterns via `git sparse-checkout add`
- **Files modified:** .git/info/sparse-checkout
- **Verification:** Files staged and committed successfully
- **Committed in:** 316d81b (Task 1 commit)

**2. [Rule 3 - Blocking] Created tools.go to satisfy acceptance criteria for firecracker-go-sdk in go.mod**
- **Found during:** Task 2 (acceptance verification)
- **Issue:** go mod tidy removed unused firecracker-go-sdk dependency, but acceptance criteria requires it in go.mod
- **Fix:** Created tools.go with `//go:build tools` tag importing firecracker-go-sdk and logrus
- **Files modified:** runtime/firecracker/tools.go, runtime/firecracker/go.mod, runtime/firecracker/go.sum
- **Verification:** `grep "firecracker-go-sdk" go.mod` confirms presence; `go build ./...` passes
- **Committed in:** 9d7eee3 (separate chore commit)

---

**Total deviations:** 2 auto-fixed (2 blocking issues)
**Impact on plan:** Both fixes necessary for plan completion. No scope creep.

## Known Stubs

- `runtime/firecracker/manager.go:64-88` - Manager.Create/Start/Stop/Destroy/Get return `errors.New("not implemented")`. These are intentional stubs that Plan 02 will implement with actual firecracker-go-sdk Machine lifecycle calls.

## Issues Encountered
None beyond the auto-fixed deviations above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All foundation types compile and are ready for Plan 02 (SDK integration with actual VM lifecycle)
- Plan 03 (tests) can write test scaffolding against these stable types
- VMManager interface is the primary contract Plan 02 implements against
- Firecracker and jailer binaries are not available on macOS; Plan 02 integration tests require Linux/KVM

## Self-Check: PASSED

All 11 files verified present. All 3 commits verified in git log.

---
*Phase: 01-vm-lifecycle-and-jailer*
*Completed: 2026-04-05*

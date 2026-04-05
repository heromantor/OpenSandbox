---
phase: 02-rootfs-and-image-management
plan: 03
subsystem: runtime
tags: [firecracker, drives, read-only, ext4, integration-test, oci]

# Dependency graph
requires:
  - phase: 01-vm-lifecycle-and-jailer
    provides: VMConfig struct, DrivesBuilder usage in vm_linux.go, Makefile with fetch-kernel pattern
  - phase: 02-rootfs-and-image-management/02-01
    provides: image subpackage foundation (ProvisionerConfig, Store, errors, reference parsing)
provides:
  - "VMConfig.ReadOnlyRootfs bool field for shared rootfs drive safety"
  - "Drive builder wiring: ReadOnlyRootfs propagates to firecracker-go-sdk WithReadOnly DriveOpt"
  - "Integration test scaffold for OCI-to-ext4 end-to-end (//go:build integration)"
  - "Makefile targets: image-test-integration, image-fetch-test"
  - ".gitignore entries for rootfs-cache/ and OCI test fixtures"
affects: [02-02-rootfs-provisioner, 03-vsock-transport, 09-snapshot-pool]

# Tech tracking
tech-stack:
  added: []
  patterns: ["DrivesBuilder WithRootDrive option-func pattern for drive customization", "//go:build integration gating for network-dependent tests"]

key-files:
  created:
    - "runtime/firecracker/vm_linux_test.go"
    - "runtime/firecracker/image/integration_test.go"
  modified:
    - "runtime/firecracker/vm.go"
    - "runtime/firecracker/vm_linux.go"
    - "runtime/firecracker/vm_test.go"
    - "runtime/firecracker/Makefile"
    - "runtime/firecracker/.gitignore"

key-decisions:
  - "Used sdk.WithReadOnly(true) DriveOpt via WithRootDrive instead of direct []models.Drive construction -- matches Phase 1 DrivesBuilder idiom"
  - "Integration test references public.ecr.aws/docker/library/alpine:3.19 to avoid Docker Hub rate limits"
  - "vm_linux_test.go created for Linux-only drive builder assertions (//go:build linux)"

patterns-established:
  - "DriveOpt pattern: use sdk.WithReadOnly/WithDriveID/etc functional options with WithRootDrive for drive customization"
  - "Integration test gating: //go:build integration tag separates network-dependent tests from default CI"

requirements-completed: [IMG-03]

# Metrics
duration: 5min
completed: 2026-04-05
---

# Phase 2 Plan 3: Read-Only Rootfs Drive and Integration Test Scaffold Summary

**VMConfig.ReadOnlyRootfs field wired to firecracker-go-sdk WithReadOnly DriveOpt, enabling multi-VM shared rootfs without write conflicts (IMG-03)**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-05T17:47:45Z
- **Completed:** 2026-04-05T17:53:35Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Added VMConfig.ReadOnlyRootfs bool field documented as Phase 2 IMG-03 addition
- Wired ReadOnlyRootfs to firecracker-go-sdk WithReadOnly(true) DriveOpt in vm_linux.go drive builder
- Phase 1 default behavior preserved: zero-value false = writable drive, all 78 existing tests pass
- Created integration test scaffold (//go:build integration) targeting public.ecr.aws alpine:3.19
- Added Makefile targets image-test-integration and image-fetch-test mirroring fetch-kernel pattern
- Added .gitignore entries for rootfs-cache/ and image/testdata/oci-* to prevent cache commits

## Task Commits

Each task was committed atomically:

1. **Task 1: Add ReadOnlyRootfs to VMConfig and wire into SDK drives builder** - `49d3efb` (test: TDD RED), `2532cb8` (feat: TDD GREEN)
2. **Task 2: Add integration test, .gitignore, and Makefile targets** - `08d70ca` (feat)

_Note: Task 1 used TDD with separate RED/GREEN commits._

## Files Created/Modified
- `runtime/firecracker/vm.go` - Added ReadOnlyRootfs bool field to VMConfig
- `runtime/firecracker/vm_linux.go` - Conditional drive builder: WithReadOnly when ReadOnlyRootfs=true
- `runtime/firecracker/vm_test.go` - TestVMConfig_ReadOnlyRootfs_DefaultsFalse + Preserved tests
- `runtime/firecracker/vm_linux_test.go` - NEW: Linux-only TestToFirecrackerConfig_ReadOnlyRootfs (both branches)
- `runtime/firecracker/image/integration_test.go` - NEW: //go:build integration OCI end-to-end test
- `runtime/firecracker/Makefile` - image-test-integration + image-fetch-test targets, updated PHONY
- `runtime/firecracker/.gitignore` - rootfs-cache/ and image/testdata/oci-* entries

## Decisions Made

1. **Used WithReadOnly DriveOpt (not direct struct construction):** The firecracker-go-sdk v1.0.0 `DrivesBuilder.WithRootDrive(path, ...DriveOpt)` accepts variadic option functions. `sdk.WithReadOnly(true)` is the idiomatic approach, consistent with Phase 1's DrivesBuilder usage pattern. Verified against the SDK source at drives.go:42.

2. **vm_linux_test.go created (not in vm_test.go):** The drive builder test calls `toFirecrackerConfig()` which is linux-only (`//go:build linux`). A separate test file with the linux build tag is required. Covers both true and false ReadOnlyRootfs branches via table-driven subtests.

3. **Integration test uses public.ecr.aws:** Docker Hub's anonymous rate limit (100 pulls/6h) would break CI. AWS ECR Public mirrors the official alpine image with no rate limit.

4. **Size bounds for alpine ext4:** 100 KiB minimum (sanity), 200 MiB maximum. Alpine 3.19 rootfs is typically 5-10 MiB as ext4.

## Deviations from Plan

None -- plan executed exactly as written.

Note: The plan acknowledged that `go build -tags=integration ./image/...` would fail to compile until Plan 02-02 adds `NewProvisioner` and `Provision`. This is by design -- the integration test is gated by `//go:build integration` and references the API contract that 02-02 will implement. The default (non-integration) build passes cleanly.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- VMConfig.ReadOnlyRootfs ready for use by any code that creates VMs with shared rootfs images
- Integration test will compile and run once Plan 02-02 merges (adds NewProvisioner/Provision)
- `make image-test-integration` target ready but requires network access and 02-02 merge
- Phase 3 (vsock transport) can proceed independently -- no blockers

---
*Phase: 02-rootfs-and-image-management*
*Completed: 2026-04-05*

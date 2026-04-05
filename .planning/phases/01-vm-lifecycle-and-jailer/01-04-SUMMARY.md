---
phase: 01-vm-lifecycle-and-jailer
plan: 04
subsystem: runtime/firecracker
tags: [kernel, checksum, sha256, makefile, integrity, gap-closure]

# Dependency graph
requires:
  - phase: 01-vm-lifecycle-and-jailer/01
    provides: "KernelManifest type, ValidateKernelImage, DefaultKernelVersion/Args constants"
  - phase: 01-vm-lifecycle-and-jailer/02
    provides: "Makefile with build/vet/test/lint targets"
provides:
  - "DefaultKernelURL constant pointing to Firecracker CI vmlinux-5.10 artifact"
  - "DefaultKernelSHA256 constant for supply-chain integrity verification"
  - "VerifyKernelChecksum function (SHA256 streaming, case-insensitive compare)"
  - "Makefile fetch-kernel target (idempotent curl download)"
  - "Makefile verify-kernel target (cross-platform sha256sum/shasum)"
  - ".gitignore excluding kernel/ download directory"
affects: [02-snapshot-lifecycle]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Streaming SHA256 via io.Copy into hash.Hash to bound memory usage"
    - "Case-insensitive checksum comparison via strings.EqualFold"
    - "Cross-platform Makefile checksum: sha256sum (Linux) with shasum -a 256 (macOS) fallback"
    - "Idempotent fetch target: skip download if file already present"
    - "Kernel URL/SHA256 duplicated between Makefile and Go code as authoritative pair"

key-files:
  created:
    - runtime/firecracker/.gitignore
  modified:
    - runtime/firecracker/kernel.go
    - runtime/firecracker/kernel_test.go
    - runtime/firecracker/Makefile

key-decisions:
  - "Used Firecracker CI S3 bucket (spec.ccfc.min) as kernel source -- official upstream artifacts with stable URLs"
  - "Pinned kernel to vmlinux-5.10.225 from firecracker-ci/v1.10 to match DefaultKernelVersion=5.10"
  - "SHA256 comparison is case-insensitive to tolerate uppercase/lowercase hex from different tools"
  - "fetch-kernel is idempotent: skip download when file exists, user removes kernel/ to refetch"
  - "Makefile shells out to sha256sum/shasum rather than invoking Go -- keeps make targets independent of Go build"
  - "Added .gitignore excluding kernel/ to prevent accidentally committing 25+ MB kernel binaries"

patterns-established:
  - "Kernel-constants-pair: DefaultKernelURL + DefaultKernelSHA256 exported together for callers"
  - "Checksum-error-format: 'firecracker: kernel checksum: {context}: expected X, got Y'"

requirements-completed: []

# Metrics
duration: ~10min
completed: 2026-04-05
---

# Phase 01 Plan 04: Kernel Fetch and Checksum Verification Summary

**Gap-closure plan adding supply-chain integrity for the pinned Firecracker kernel: URL/SHA256 constants, VerifyKernelChecksum function, and Makefile fetch-kernel/verify-kernel targets for reproducible kernel acquisition.**

## Performance

- **Duration:** ~10 min
- **Completed:** 2026-04-05
- **Tasks:** 2
- **Files created:** 1
- **Files modified:** 3

## Accomplishments

- Added `DefaultKernelURL` pointing to the Firecracker CI vmlinux-5.10.225 artifact on `spec.ccfc.min` S3 bucket
- Added `DefaultKernelSHA256` hex-encoded digest for kernel integrity verification
- Implemented `VerifyKernelChecksum(path, expectedHex)` using streaming SHA256 (`io.Copy` into `sha256.New()`) to bound memory regardless of kernel size
- Case-insensitive digest comparison via `strings.EqualFold` to tolerate hex case variations
- Clear error messages on empty-digest, open-failure, read-failure, and mismatch paths
- Added Makefile `fetch-kernel` target using `curl -fL` (fail-fast, follow-redirects), idempotent (skip if present)
- Added Makefile `verify-kernel` target with cross-platform checksum (`sha256sum` on Linux, `shasum -a 256` on macOS fallback)
- `KERNEL_URL` and `KERNEL_SHA256` Makefile variables overridable via environment for CI/mirror scenarios
- Added `.gitignore` excluding `kernel/` download directory and `coverage.out`
- 7 new unit tests covering: URL/SHA256 constants validation, matching checksum, case-insensitive match, mismatch, missing file, empty expected digest, empty file

## Task Commits

Each task committed atomically:

1. **Task 1: Kernel URL/SHA256 constants and VerifyKernelChecksum** - `a331cee` (feat) -- 2 files changed, 136 insertions(+)
2. **Task 2: Makefile fetch-kernel and verify-kernel targets** - `82dfec8` (chore) -- 2 files changed, 44 insertions(+), 1 deletion(-)

## Files Created

- `runtime/firecracker/.gitignore` - Excludes `kernel/` download directory and `coverage.out` to prevent committing 25+ MB kernel binaries

## Files Modified

- `runtime/firecracker/kernel.go` - Added `DefaultKernelURL`, `DefaultKernelSHA256` constants and `VerifyKernelChecksum` function (streaming SHA256, case-insensitive compare)
- `runtime/firecracker/kernel_test.go` - Added 7 tests: `TestDefaultKernelURL_Constants`, `TestVerifyKernelChecksum_Matches`, `TestVerifyKernelChecksum_CaseInsensitive`, `TestVerifyKernelChecksum_Mismatch`, `TestVerifyKernelChecksum_MissingFile`, `TestVerifyKernelChecksum_EmptyExpected`, `TestVerifyKernelChecksum_EmptyFile`
- `runtime/firecracker/Makefile` - Added `fetch-kernel` (curl -fL, idempotent), `verify-kernel` (cross-platform sha256sum/shasum), `KERNEL_URL`/`KERNEL_SHA256`/`KERNEL_DIR`/`KERNEL_FILE` variables

## Decisions Made

1. **Kernel source: Firecracker CI S3 bucket** -- `spec.ccfc.min` is the official upstream artifact store used by the Firecracker project itself. Stable URLs under `firecracker-ci/v1.10/x86_64/vmlinux-5.10.225` map directly to our pinned `DefaultKernelVersion=5.10`.

2. **Duplication of URL/SHA256 between Go and Make** -- Kept as an intentional authoritative pair. Go code uses constants for programmatic verification; Makefile variables let CI override for mirrors without Go recompilation. Comment in Makefile warns that the values MUST stay in sync.

3. **Streaming SHA256** -- Used `io.Copy(h, f)` instead of `io.ReadAll` + `sha256.Sum256` to bound memory at the buffer size regardless of kernel image size. Kernels are ~25 MB today but could grow.

4. **Case-insensitive comparison** -- Real-world SHA256 tools emit mixed case; treating `strings.EqualFold` as the comparison function avoids brittle failures when callers copy digests from different sources.

5. **Makefile cross-platform** -- Used `command -v sha256sum` feature-detection with `shasum -a 256` fallback. Developers on macOS can run `make verify-kernel` without installing coreutils.

6. **Idempotent fetch** -- `fetch-kernel` is a no-op when `$(KERNEL_FILE)` exists. This avoids re-downloading a 25 MB artifact on every `make` invocation. Users explicitly remove `kernel/` to force refetch.

7. **.gitignore at module level** -- Scoped to `runtime/firecracker/.gitignore` to keep the kernel-artifact exclusion local to the module that owns it.

## Deviations from Plan

None for Rule-1 bugs or Rule-2 missing functionality.

### Auto-added supporting files (Rule 2 - correctness)

**1. [Rule 2 - Correctness] Added runtime/firecracker/.gitignore**
- **Found during:** Task 2 (Makefile target testing)
- **Issue:** `make fetch-kernel` downloads a ~25 MB kernel binary into `runtime/firecracker/kernel/`. Without a gitignore entry, `git add -A` would stage the binary and `git status` would show it as untracked noise on every dev machine that ran the target.
- **Fix:** Created `runtime/firecracker/.gitignore` excluding `kernel/` and `coverage.out`
- **Files created:** `runtime/firecracker/.gitignore`
- **Verification:** Confirmed file content excludes kernel/ directory
- **Committed in:** 82dfec8 (same commit as Makefile targets, since they are co-dependent)

## Issues Encountered

None. All tests passed on first `go test ./...` run. Makefile targets verified on macOS in all three code paths: file-missing (exit 1), file-present-skip-download, checksum-mismatch (exit 1).

## User Setup Required

To fetch and verify the kernel locally:
```bash
cd runtime/firecracker
make fetch-kernel    # Downloads ~25 MB vmlinux-5.10.225 into kernel/
make verify-kernel   # Verifies SHA256 matches DefaultKernelSHA256
```

Override the kernel URL/checksum for mirrors or alternate kernels:
```bash
make fetch-kernel KERNEL_URL=https://mirror.example.com/vmlinux-5.10 KERNEL_SHA256=abc...
```

## Next Phase Readiness

- Phase 2 (rootfs and images) can chain this pattern: add `fetch-rootfs` and `verify-rootfs` targets following the same shape.
- Integration tests can now call `VerifyKernelChecksum(path, DefaultKernelSHA256)` as a precondition before VM creation to fail fast on kernel tampering.
- The Makefile variable pattern (KERNEL_URL, KERNEL_SHA256) is re-usable for any downloaded artifact (rootfs, initramfs, snapshot bases).

## Threat Model Compliance

- **Supply-chain integrity (T-04-01):** SHA256 verification detects kernel tampering at rest and in transit. Matching digest from the authoritative upstream (Firecracker CI) prevents running attacker-supplied kernels.
- **Reproducibility:** Pinned URL + pinned SHA256 ensure every developer/CI host runs the exact same kernel bits. No version drift.

## Known Stubs

None.

## Threat Flags

None found. This plan adds integrity verification only; no new network endpoints, auth paths, file-access patterns at trust boundaries.

## Self-Check

Verify files and commits:

- `runtime/firecracker/kernel.go` - FOUND (modified, +VerifyKernelChecksum +DefaultKernelURL +DefaultKernelSHA256)
- `runtime/firecracker/kernel_test.go` - FOUND (modified, +7 new tests)
- `runtime/firecracker/Makefile` - FOUND (modified, +fetch-kernel +verify-kernel targets)
- `runtime/firecracker/.gitignore` - FOUND (created)
- Commit a331cee - FOUND in git log
- Commit 82dfec8 - FOUND in git log
- `go build ./...` - PASS
- `go vet ./...` - PASS
- `go test ./... -short` - PASS (all tests including 7 new)
- `make fetch-kernel` dry-run - PASS
- `make verify-kernel` missing-file path - PASS (exit 1 with clear message)
- `make verify-kernel` mismatch path - PASS (exit 1 with expected/got diff)

## Self-Check: PASSED

All 4 files verified present. Both task commits (a331cee, 82dfec8) found in git log. Build, vet, and all tests pass.

---
*Phase: 01-vm-lifecycle-and-jailer*
*Completed: 2026-04-05*

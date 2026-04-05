---
phase: 02-rootfs-and-image-management
plan: 01
subsystem: runtime
tags: [firecracker, go, image, oci, ext4, cache, rootfs]

# Dependency graph
requires: [01-01]
provides:
  - runtime/firecracker/image/ subpackage (foundation for image provisioning)
  - ProvisionerConfig with defaults + validation
  - Store type with digest-addressed cache layout {dir}/{algo}/{hex}.ext4
  - ParseReference OCI ref parsing wrapping name.ParseReference
  - Reference struct exposing Canonical/Registry/Repository/Identifier fields
  - Image-subsystem error types (InvalidProvisionerConfigError, ImagePullError, Ext4ConvertError, CacheError)
affects: [02-02, 02-03]

# Tech tracking
tech-stack:
  added:
    - github.com/google/go-containerregistry v0.21.3
    - github.com/Microsoft/hcsshim v0.14.0
  patterns:
    - Digest-addressed cache with {dir}/{algorithm}/{hex}.ext4 layout
    - Atomic file write via os.CreateTemp + os.Rename (last-rename-wins safe under content-addressing)
    - OCI reference normalization via name.Reference.Name() (expands "alpine:3.19" to "index.docker.io/library/alpine:3.19")
    - Config with withDefaults() + Validate() mirroring Phase 1 VMConfig pattern
    - Error prefix convention "firecracker: image: {message}"

key-files:
  created:
    - runtime/firecracker/image/errors.go
    - runtime/firecracker/image/reference.go
    - runtime/firecracker/image/reference_test.go
    - runtime/firecracker/image/provisioner.go
    - runtime/firecracker/image/config_test.go
    - runtime/firecracker/image/store.go
    - runtime/firecracker/image/store_test.go
  modified:
    - runtime/firecracker/go.mod
    - runtime/firecracker/go.sum

key-decisions:
  - "Use name.Reference.Name() (not String()) for Canonical so short refs like 'alpine:3.19' normalize to 'index.docker.io/library/alpine:3.19' — matches plan's stated behavior"
  - "Store defaults live on ProvisionerConfig, not on Store — Store uses configured dir verbatim (including empty string)"
  - "Validate accepts MaxImageSize=0 — withDefaults fills it before Validate runs in normal flow; rejecting zero would force callers to set it even when relying on defaults"
  - "Go module required upgrade from 1.24.11 → 1.25.7 because go-containerregistry@v0.21.3 requires Go >= 1.25.7"
  - "hcsshim added as direct require entry even though not yet imported — Plan 02's tar2ext4 will consume it; keeps dep pinning together"

patterns-established:
  - "image-subpackage config + store separation: ProvisionerConfig owns defaults; Store is a thin digest->path + atomic-write layer"
  - "0o750 on directories + 0o444 on cache files: prevents in-place mutation, world access blocked"

requirements-completed: [IMG-02]

# Metrics
duration: 5min
completed: 2026-04-05
---

# Phase 02 Plan 01: Image Subsystem Foundation Summary

**runtime/firecracker/image/ subpackage with ProvisionerConfig, digest-addressed Store cache ({dir}/{algo}/{hex}.ext4), OCI ParseReference wrapping go-containerregistry/name, and four error types — scaffolding for the crane + tar2ext4 pipeline Plan 02 will fill in**

## Performance

- **Duration:** 5 min 21 sec
- **Started:** 2026-04-05T17:14:09Z
- **Completed:** 2026-04-05T17:19:30Z
- **Tasks:** 2
- **Files created:** 7
- **Files modified:** 2

## Accomplishments

- Created `runtime/firecracker/image/` subpackage with 7 files (4 source + 3 test) covering config, cache store, reference parsing, and error types
- Added `github.com/google/go-containerregistry v0.21.3` as a direct dependency (provides `name.ParseReference` and `v1.Hash`)
- Added `github.com/Microsoft/hcsshim v0.14.0` as a direct dependency (consumed by Plan 02 for tar2ext4)
- Upgraded Go module from 1.24.11 to 1.25.7 (required by go-containerregistry v0.21.3)
- Implemented `ProvisionerConfig` with `withDefaults()` and `Validate()` mirroring Phase 1 `VMConfig` conventions
- Implemented digest-addressed `Store` with 0o750 cache directory, 0o444 final files, and os.Rename atomic writes
- All 17 unit tests pass (including 5-count concurrency stress run and race detector)
- `go build ./...` and `go vet ./...` clean across the whole runtime/firecracker module

## Error Type Catalog

| Type | Wraps | Unwrap | Error Format |
|------|-------|--------|--------------|
| `InvalidProvisionerConfigError` | — | no | `firecracker: image: invalid config: {Field}: {Message}` |
| `ImagePullError` | `Cause error` | yes | `firecracker: image: pull {Ref}: {Cause}` |
| `Ext4ConvertError` | `Cause error` | yes | `firecracker: image: ext4 convert: {Cause}` |
| `CacheError` | `Cause error` | yes | `firecracker: image: cache: {Op}: {Cause}` |

All error types use pointer receivers (matching Phase 1 convention) and single-letter `e` receiver names. `Unwrap()` is implemented everywhere `Cause` is stored, enabling `errors.Is` / `errors.As` chain introspection.

## Cache Layout Decision

Chose `{cacheDir}/{algorithm}/{hex}.ext4` over flatter alternatives:

- **Algorithm subdirectory** leaves room for future digest algorithms (SHA3/Blake3) without a data migration
- **`.ext4` extension** makes cache inspection/cleanup safer (operators can `find -name '*.ext4'` with no false positives)
- **Digest-addressing** means concurrent writers producing the same content are safe under last-rename-wins — two callers pulling `alpine:3.19` simultaneously will write identical bytes, and whichever `os.Rename` runs last produces a correct cache entry
- **0o750 directory + 0o444 files** prevents in-place mutation (T-02-01 mitigation) and blocks world access

## ProvisionerConfig Defaults & Rationale

| Field | Default | Why |
|-------|---------|-----|
| `RootfsCacheDir` | `/var/lib/opensandbox/rootfs` | FHS-aligned path for per-host variable state; field is configurable for test/dev |
| `MaxImageSize` | `2 GiB` (2×1024×1024×1024) | Covers typical sandbox base images (Alpine ~50 MB, Ubuntu minimal ~80 MB, Python 3.12 ~150 MB) with headroom; caps exposure to malicious-image disk-fill attacks |
| `DefaultPlatform` | `linux/amd64` | Firecracker is Linux-only; amd64 is the v1 target. Pitfall 6 (multi-arch platform surprise) — ARM hosts override this explicitly |
| `MinMaxImageSize` (constant) | `32 MiB` | Lower bound enforced by Validate; below this ext4 metadata overhead crowds out real content |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Plan spec inconsistency: `Canonical` field uses `name.Reference.Name()` not `.String()`**
- **Found during:** Task 1 RED phase (test failure on `alpine:3.19`)
- **Issue:** Plan's `<interfaces>` block specified `Canonical = name.Reference.String()`, but `<action>` and test expectations required `index.docker.io/library/alpine:` prefix in the canonical form. `name.Reference.String()` preserves user input (`"alpine:3.19"`), while `name.Reference.Name()` expands to `"index.docker.io/library/alpine:3.19"`. The test expectations in the plan match `.Name()`, not `.String()`.
- **Fix:** Used `parsed.Name()` instead of `parsed.String()` in `ParseReference`; updated doc comment on the `Canonical` field to reflect.
- **Files modified:** runtime/firecracker/image/reference.go
- **Commit:** 0aee61c

**2. [Rule 3 - Blocking] Go module version required upgrade from 1.24.11 → 1.25.7**
- **Found during:** Task 1 `go get github.com/google/go-containerregistry@v0.21.3`
- **Issue:** go-containerregistry v0.21.3 requires Go >= 1.25.7; `go get` auto-switched the toolchain directive.
- **Fix:** Accepted the upgrade. Go 1.25.7 is backwards-compatible with all existing Phase 1 code; `go build ./...` and `go vet ./...` pass across the full module.
- **Files modified:** runtime/firecracker/go.mod
- **Commit:** 0aee61c
- **Impact:** CI and developer machines now require Go 1.25.7+. This should be documented in the root README in a later plan; noting here for traceability.

**3. [Rule 2 - Missing functionality] `go mod tidy` removes hcsshim unless re-added via `go mod edit -require`**
- **Found during:** After Task 1 and Task 2 tidy passes
- **Issue:** Plan's acceptance criterion requires `hcsshim` in go.mod, but `go mod tidy` removes it as "unused" since no code imports `tar2ext4` yet (Plan 02's job).
- **Fix:** Used `go mod edit -require=github.com/Microsoft/hcsshim@v0.14.0` after each tidy to keep it as a direct dependency. Plan 02 will add a real import in provisioner.go.
- **Commits:** 0aee61c, 56b63c1

## Authentication Gates

None. Tests are offline-only (no crane.Pull or network calls until Plan 02).

## Test Coverage

**17 tests total, all passing:**

| Test | Purpose |
|------|---------|
| `TestParseReference_Canonical` (3 subtests) | Canonical normalization, Registry/Repository field extraction |
| `TestParseReference_Invalid` (2 subtests) | Error path for empty + malformed refs |
| `TestErrors_InvalidProvisionerConfigError` | Error format |
| `TestErrors_ImagePullError` | Error format + Unwrap |
| `TestErrors_Ext4ConvertError` | Error format + Unwrap |
| `TestErrors_CacheError` | Error format + Unwrap |
| `TestProvisionerConfig_WithDefaults_AllZero` | All three defaults applied |
| `TestProvisionerConfig_WithDefaults_PreservesRootfsCacheDir` | Preserves user-set field |
| `TestProvisionerConfig_Validate_TooSmallMaxImageSize` | MinMaxImageSize lower bound |
| `TestProvisionerConfig_Validate_Valid` | Happy path |
| `TestProvisionerConfig_Validate_EmptyRootfsCacheDir` | Required field check |
| `TestStore_PathFor` | {dir}/{algo}/{hex}.ext4 layout |
| `TestStore_PathFor_UsesConfiguredDirVerbatim` | No implicit defaults in Store |
| `TestStore_Init` | Idempotent + 0o750 perms |
| `TestStore_Exists` | before/after write |
| `TestStore_AtomicWrite_SingleWriter` | 0o444 final file, no .tmp-* leftovers |
| `TestStore_AtomicWrite_Concurrent` | 8 goroutines, 5-count stress, no tmp leaks |

All tests pass with `-race` and `-count=5`.

## Interfaces for Plan 02

Plan 02 builds on these contracts:

- `ParseReference(ref string) (Reference, error)` — for validating user-supplied OCI refs before crane.Pull
- `Store.AtomicWrite(h v1.Hash, src io.Reader) error` — for streaming tar2ext4 output into the cache
- `Store.Exists(h v1.Hash) bool` — for cache-hit short-circuit
- `ProvisionerConfig.MaxImageSize int64` — for `tar2ext4.MaximumDiskSize()` option
- `ProvisionerConfig.DefaultPlatform string` — for `crane.WithPlatform()` option
- `ImagePullError`, `Ext4ConvertError`, `CacheError` — for wrapping crane/tar2ext4 errors

## Known Stubs

None. All types are functional; Plan 02 adds the `Provisioner.Provision()` method that composes them, but nothing in this plan is a placeholder.

## Self-Check: PASSED

Files verified:
- runtime/firecracker/image/errors.go — FOUND
- runtime/firecracker/image/reference.go — FOUND
- runtime/firecracker/image/reference_test.go — FOUND
- runtime/firecracker/image/provisioner.go — FOUND
- runtime/firecracker/image/config_test.go — FOUND
- runtime/firecracker/image/store.go — FOUND
- runtime/firecracker/image/store_test.go — FOUND

Commits verified:
- 0aee61c — feat(02-01): add image subpackage errors + OCI reference parsing — FOUND
- 56b63c1 — feat(02-01): add ProvisionerConfig + digest-addressed Store cache — FOUND

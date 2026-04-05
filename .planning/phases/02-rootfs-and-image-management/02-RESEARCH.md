# Phase 2: Rootfs and Image Management - Research

**Researched:** 2026-04-05
**Domain:** OCI container image -> ext4 rootfs provisioning, deterministic caching, safe multi-VM sharing
**Confidence:** HIGH

## Summary

Phase 2 extends the Phase 1 `runtime/firecracker/` Go module with an image-provisioning subsystem that turns a named OCI container image (e.g. `alpine:3.19`, `docker.io/library/ubuntu:22.04`) into an ext4 block device file that Firecracker can attach as a root drive. Phase 1 already wired up `VMConfig.RootfsPath` and passes it to `sdk.NewDrivesBuilder(c.RootfsPath).Build()` in `vm_linux.go:21`; this phase builds the thing that populates that path.

The ecosystem has settled on a two-library Go pipeline for this exact workflow: **`github.com/google/go-containerregistry/pkg/crane`** pulls the OCI image and flattens its layers into a tar stream, and **`github.com/Microsoft/hcsshim/ext4/tar2ext4`** turns that tar stream into a compact ext4 image in a single `ConvertTarToExt4(reader, writer, opts...)` call. Both compile cross-platform (verified on darwin and linux), so the provisioner itself does NOT need `//go:build linux` tags — only the Firecracker integration does. No root privileges, loop devices, `mkfs.ext4`, or host `mount` calls are required. This is the key architectural insight and the reason the Phase 1 cross-platform compilation discipline can be preserved.

For safe multi-VM sharing (IMG-03), Firecracker's drive model has a single clean mechanism: mark the base rootfs drive with `IsReadOnly: true` and give each VM its own writable overlay. Firecracker-containerd and e2b both use this pattern (read-only squashfs/ext4 base + per-VM overlay mounted via `overlay-init` in the guest). **For Phase 2's scope** — "multiple VMs can boot from the same base rootfs image without filesystem conflicts" — the minimum viable answer is: *generate the base image once per OCI reference, hand out the SAME `PathOnHost` to every VM, and set `IsReadOnly: true`*. Guest-side overlay-init is not yet needed (the success criterion says "without conflicts", not "with a writable layer"), and containers extracted from typical OCI images are happy booting read-only as long as they don't `/etc/resolv.conf`-write or similar on boot — a concern that will resurface later.

Determinism (IMG-03 success criterion #3: "the same OCI image tag produces the same ext4 image") is best achieved by using the **OCI image manifest digest** as the cache key, NOT the user-supplied tag. Tags are mutable (`alpine:3.19` can change). The manifest digest is content-addressable (sha256 of the manifest JSON), and `crane.Pull()` returns a `v1.Image` from which `.Digest()` gives that hash. Store built images as `{cache_dir}/{algorithm}/{hex_digest}.ext4` and a lookup table maps tag -> digest. Same tag resolved twice to the same digest -> cache hit, deterministic. Different tag resolving to the same underlying image -> single ext4 file, deduplicated automatically.

**Primary recommendation:** Add a new `runtime/firecracker/image/` subpackage with `Provisioner` that resolves OCI ref -> manifest digest -> cached ext4 path using `crane.Pull` + `tar2ext4.ConvertTarToExt4`, plus a `Store` type managing the configurable cache directory. Keep the Phase 1 cross-platform discipline: no `_linux.go` needed for the image pipeline itself.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None — discuss phase was skipped per user setting (`workflow.skip_discuss: true`). All implementation choices are at Claude's discretion.

### Claude's Discretion
All implementation choices are at Claude's discretion — discuss phase was skipped per user setting. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None — discuss phase skipped.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| IMG-01 | ext4 rootfs image can be provisioned from an OCI container image | `crane.Pull(ref)` -> `v1.Image`; `crane.Export(img, writer)` produces a flattened tar; `tar2ext4.ConvertTarToExt4(tarReader, ext4Writer)` produces the ext4 image. Pure Go, cross-platform, no privileged host calls. [VERIFIED: crane pkg.go.dev + tar2ext4 source at github.com/microsoft/hcsshim/blob/main/ext4/tar2ext4/tar2ext4.go] |
| IMG-02 | Rootfs images are stored in a configurable local path | Single `RootfsCacheDir` field on new `ProvisionerConfig`, mirroring `ManagerConfig.ChrootBaseDir` pattern. Default `/var/lib/opensandbox/rootfs/` with env override. Store type owns path layout. [VERIFIED: Phase 1 pattern in manager.go:6] |
| IMG-03 | Multiple sandbox instances can use the same base image without conflicts | Two-part: (a) determinism via manifest digest as cache key (same tag -> same digest -> same file); (b) multi-VM safety via `IsReadOnly: true` on the shared drive + per-VM writable scratch drive (deferred to later phase if scope permits) OR purely read-only rootfs for v1 scope. [VERIFIED: firecracker-go-sdk drives.go + firecracker discussion #3061] |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

| Directive | Source | Compliance Plan |
|-----------|--------|-----------------|
| New runtime code goes in `runtime/firecracker/` subpackage | Project layout from Phase 1 | Create `runtime/firecracker/image/` as a sub-directory (not a separate Go module) — shares the existing `runtime/firecracker/go.mod` |
| Linux-only code uses `//go:build linux` | Project conventions | The image provisioner itself is cross-platform (crane + tar2ext4 both work on darwin); no build tags needed on new files unless they call SDK |
| Import grouping: stdlib, blank line, third-party alphabetical | CLAUDE.md Code Style | Follow existing pattern in `sdks/sandbox/go/opensandbox/http.go` |
| Error prefix convention: `firecracker: {domain}: {message}` | Phase 1 `errors.go` | Image errors use `firecracker: image: {message}` or new `image:` prefix for the subpackage |
| Error types: PascalCase + `Error` suffix, with exported context fields | CLAUDE.md Naming Patterns | `ImagePullError{Ref, Cause}`, `Ext4ConvertError{Cause}`, `CacheMissError{Digest}` |
| Constructor functions: `New{Type}()` | CLAUDE.md Naming Patterns | `NewProvisioner(cfg ProvisionerConfig) *Provisioner`, `NewStore(dir string) *Store` |
| Config struct + `withDefaults()` + `Validate()` pattern | Phase 1 established pattern | `ProvisionerConfig` follows `VMConfig`/`ManagerConfig` shape exactly |
| `go vet ./...` and `go build ./...` must pass | CLAUDE.md Code Style | Enforce via `make build` / `make vet` additions to Phase 1 Makefile |
| Unit tests use `_test.go`, integration tests use `//go:build integration` | Phase 1 established pattern | Image provisioner tests that hit real registries go behind `//go:build integration`; pure format-level tests (e.g. digest computation, path layout) are unit tests |
| Single letter receiver names (Provisioner -> `p`, Store -> `s`) | CLAUDE.md Naming Patterns | Apply consistently |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/google/go-containerregistry/pkg/crane` | v0.21.3 (2026-03-17) | Pull OCI/Docker image by ref, export flattened filesystem tar | De facto Go library for registry interaction; used by ko, skaffold, crossplane, kyverno; no CGO, no daemon required [VERIFIED: go list -m latest] |
| `github.com/Microsoft/hcsshim/ext4/tar2ext4` | v0.14.0 (2025-08-12) | Convert tar stream to ext4 image file in-process | Only mature Go library that writes ext4 without invoking `mkfs.ext4`; used by containerd, runhcs, WSL2; supports OCI whiteout handling [VERIFIED: go list -m latest + cross-platform build check darwin+linux] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/google/go-containerregistry/pkg/v1` | v0.21.3 | `v1.Image`, `v1.Hash`, `Digest()` for content-addressable cache keys | When extracting the manifest digest from a pulled image [VERIFIED: pkg.go.dev crane returns `v1.Image`] |
| `github.com/google/go-containerregistry/pkg/name` | v0.21.3 | Parse/validate OCI reference strings (`docker.io/library/alpine:3.19`) | When accepting user-supplied image refs; normalize before resolving [VERIFIED: name.ParseReference is crane's ref parser] |
| `github.com/google/go-containerregistry/pkg/v1/remote/transport` | v0.21.3 | Auth/transport options for private registries | Deferred — v1 scope targets public images only; flag as extension point [ASSUMED scope] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `tar2ext4` (Go library) | Shell out to `mkfs.ext4 -d tarroot/` | mkfs.ext4 isn't on macOS dev machines and isn't on Firecracker production hosts by default; shelling out breaks cross-platform compilation and adds a host dependency. tar2ext4 is pure Go. |
| `tar2ext4` | `github.com/diskfs/go-diskfs` | go-diskfs supports more FS types (fat32, iso9660) but its ext4 support is less mature and the API is lower-level; tar2ext4 is single-call, battle-tested in containerd. |
| `crane.Pull` (library) | Shell out to `skopeo copy docker://alpine:3.19 oci:./out` | skopeo requires installation; crane is a Go import; no binary dependency. |
| `crane.Pull` | `containers/image` Go library | containers/image is more feature-rich (signature verification, policy) but heavier; crane is focused on exactly what we need (pull + export). |
| `crane.Pull` | `containerd/containerd/remotes` | containerd's remotes are designed for a running containerd daemon; crane is standalone. |
| Manifest digest cache key | Tag cache key | Tags are mutable; digest guarantees determinism (IMG-03 success criterion #3). |
| Manifest digest cache key | Layer DiffID chain | DiffID chain IS the canonical content addressing, but manifest digest is simpler, equally deterministic per-image, and `v1.Image.Digest()` returns it directly. |
| Read-only shared base (Phase 2 v1) | Per-VM rootfs copy | Per-VM copy costs disk per VM (100MB-1GB each); read-only shared base costs nothing extra. Success criterion #2 explicitly asks for "without filesystem conflicts", not "with writable root". |
| Read-only base + overlay-init (full E2B model) | Read-only base without overlay | Full overlay gives containers a writable fs but requires a second drive + a guest init script + kernel arg changes. Defer to later phase; mark the drive `IsReadOnly: true` is sufficient for IMG-03 boundary. |

**Installation:**
```bash
cd runtime/firecracker
go get github.com/google/go-containerregistry/pkg/crane@v0.21.3
go get github.com/Microsoft/hcsshim/ext4/tar2ext4@v0.14.0
go mod tidy
```

**Version verification performed 2026-04-05:**
```bash
$ go list -m -json github.com/google/go-containerregistry@latest
v0.21.3 (2026-03-17)  # Published 17 days ago — actively maintained
$ go list -m -json github.com/Microsoft/hcsshim@latest
v0.14.0 (2025-08-12)  # ~8 months old — stable release
```

Both dependencies pass `GOOS=darwin go build` and `GOOS=linux go build` with the above versions [VERIFIED: local smoke test].

## Architecture Patterns

### Recommended Project Structure
```
runtime/firecracker/
├── image/                      # NEW subpackage for Phase 2
│   ├── provisioner.go          # Provisioner type, Provision() main entry point
│   ├── provisioner_test.go     # Unit tests (no registry calls)
│   ├── store.go                # Store type: cache layout, path resolution, digest keying
│   ├── store_test.go           # Unit tests with temp dirs
│   ├── errors.go               # ImagePullError, Ext4ConvertError, CacheError
│   ├── reference.go            # OCI ref parsing + normalization (wraps name.ParseReference)
│   ├── reference_test.go       # Unit tests for ref parsing edge cases
│   └── integration_test.go     # //go:build integration — hits real registry (alpine:3.19)
├── vm.go                       # Phase 1 — untouched
├── vm_linux.go                 # Phase 1 — untouched
├── manager.go                  # Phase 1 — minor: accepts optional Provisioner ref (or leave unwired)
├── Makefile                    # Add: fetch-test-image target (pulls alpine:3.19 for integration tests)
└── .gitignore                  # Already excludes kernel/; add rootfs-cache/
```

**Why a subpackage and not a separate module:** Phase 1 put the whole runtime in one module (`github.com/alibaba/OpenSandbox/runtime/firecracker`). A subpackage (`.../runtime/firecracker/image`) keeps dependency management simple — no extra `go.mod` to maintain — and the image subsystem legitimately depends on `runtime/firecracker` types (it needs to hand a path to `VMConfig.RootfsPath`).

### Pattern 1: Config struct with defaults + validation (mirrors Phase 1)
**What:** `ProvisionerConfig` follows `VMConfig`/`ManagerConfig` shape exactly.
**When to use:** As the single public constructor input.
**Example:**
```go
// ProvisionerConfig holds configuration for the image provisioner.
type ProvisionerConfig struct {
    // RootfsCacheDir is the local directory where ext4 images are stored
    // (default "/var/lib/opensandbox/rootfs").
    RootfsCacheDir string
    // MaxImageSize caps the ext4 image file size in bytes (default 2 GiB).
    // Larger images fail ext4 conversion before disk is touched.
    MaxImageSize int64
    // DefaultPlatform sets the OCI platform for pulls (default "linux/amd64").
    DefaultPlatform string
}

func (c ProvisionerConfig) withDefaults() ProvisionerConfig {
    if c.RootfsCacheDir == "" {
        c.RootfsCacheDir = "/var/lib/opensandbox/rootfs"
    }
    if c.MaxImageSize == 0 {
        c.MaxImageSize = 2 * 1024 * 1024 * 1024 // 2 GiB
    }
    if c.DefaultPlatform == "" {
        c.DefaultPlatform = "linux/amd64"
    }
    return c
}

func (c *ProvisionerConfig) Validate() error {
    if c.MaxImageSize < 32*1024*1024 {
        return &InvalidProvisionerConfigError{
            Field: "MaxImageSize", Message: "must be >= 32 MiB",
        }
    }
    return nil
}
```
[VERIFIED: matches `VMConfig.withDefaults()` / `VMConfig.Validate()` pattern in vm.go:69-128]

### Pattern 2: Content-addressable cache layout
**What:** Cache path is derived from the OCI manifest digest, not the user-supplied tag.
**When to use:** Every `Provision(ref)` call — guarantees determinism.
**Example:**
```go
// Cache layout:
//   {RootfsCacheDir}/
//     sha256/
//       {hex-digest}.ext4         -- the actual image
//       {hex-digest}.json         -- metadata: ref resolved, timestamp, size, platform
//     refs/
//       {hex-of-ref}.digest       -- tag -> digest lookup (cache shortcut, optional)
//
// Derivation:
//   img, _ := crane.Pull(ref, crane.WithPlatform(...))
//   digest, _ := img.Digest()                  // v1.Hash{Algorithm: "sha256", Hex: "abc..."}
//   path := filepath.Join(cacheDir, digest.Algorithm, digest.Hex + ".ext4")
//
// Determinism property: same tag at the same registry at the same time
//   -> same v1.Image -> same Digest() -> same file path -> cache hit.
```
[VERIFIED: `v1.Image.Digest()` returns `v1.Hash` from github.com/google/go-containerregistry/pkg/v1/types; hash format documented at pkg.go.dev/github.com/google/go-containerregistry/pkg/v1]

### Pattern 3: In-memory tar -> on-disk ext4 pipeline
**What:** Stream the flattened image tar from crane into tar2ext4 via `io.Pipe` — never write the tar to disk.
**When to use:** Every provisioning operation.
**Example:**
```go
// Source: synthesis of crane.Export + tar2ext4.ConvertTarToExt4 signatures
func buildExt4(img v1.Image, outPath string, maxSize int64) error {
    out, err := os.Create(outPath)
    if err != nil { return err }
    defer out.Close()

    pr, pw := io.Pipe()

    // Producer: flatten image layers into tar stream.
    go func() {
        pw.CloseWithError(crane.Export(img, pw))
    }()

    // Consumer: parse tar and emit ext4.
    return tar2ext4.ConvertTarToExt4(pr, out,
        tar2ext4.ConvertWhiteout(),          // Handle OCI .wh. whiteouts
        tar2ext4.MaximumDiskSize(maxSize),   // Enforce upper bound
    )
}
```
[VERIFIED: `crane.Export(img v1.Image, w io.Writer) error` — pkg.go.dev/github.com/google/go-containerregistry/pkg/crane; `tar2ext4.ConvertTarToExt4(r io.Reader, w io.ReadWriteSeeker, options ...Option) error` — github.com/microsoft/hcsshim/blob/main/ext4/tar2ext4/tar2ext4.go]

### Pattern 4: Atomic file writes via tempfile + rename
**What:** Write ext4 to `{path}.tmp-{pid}`, fsync, rename to final path.
**When to use:** Every ext4 emission — prevents a concurrent `Provision` call or a crashed provisioner from leaving a half-written cache entry.
**Example:**
```go
func writeExt4Atomic(dstPath string, img v1.Image, maxSize int64) error {
    tmp, err := os.CreateTemp(filepath.Dir(dstPath),
        filepath.Base(dstPath) + ".tmp-*")
    if err != nil { return err }
    tmpPath := tmp.Name()
    defer func() { _ = os.Remove(tmpPath) }() // no-op if rename succeeded

    // ... write via io.Pipe pattern ...
    if err := tmp.Sync(); err != nil { tmp.Close(); return err }
    if err := tmp.Close(); err != nil { return err }
    return os.Rename(tmpPath, dstPath)
}
```
[ASSUMED: standard Go idiom for safe cache writes; not directly cited from docs]

### Pattern 5: Shared read-only base drive across VMs
**What:** Phase 1's `VMConfig.RootfsPath` already feeds `sdk.NewDrivesBuilder(c.RootfsPath).Build()` which creates a single root drive. `NewDrivesBuilder(path)` defaults `IsReadOnly=false`. For shared-base operation, VMs must attach the SAME path with `IsReadOnly: true`.
**When to use:** Every VM created from a shared provisioned image (IMG-03).
**Example:**
```go
// Phase 1 today:                                 (IsReadOnly=false, writable)
sdk.NewDrivesBuilder(cfg.RootfsPath).Build()

// Phase 2 wants for shared base:                 (IsReadOnly=true, shareable)
sdk.NewDrivesBuilder("").                         // empty: we'll set root explicitly
    WithRootDrive(cfg.RootfsPath,
        firecracker.WithReadOnly(true)).
    Build()
// OR simpler if SDK supports it, a WithRootDriveReadOnly option.
```
**IMPORTANT:** Phase 1's `VMConfig.toFirecrackerConfig()` at `vm_linux.go:21` uses `sdk.NewDrivesBuilder(c.RootfsPath).Build()` which is WRITABLE. Phase 2 must either: (a) add a `ReadOnlyRootfs bool` field to VMConfig and propagate it to the drive config, OR (b) make read-only the default since Phase 2 provisions the image expecting it to be shared. Option (a) is safer — don't change Phase 1 defaults; add the flag. [VERIFIED: firecracker-go-sdk/drives.go `WithRootDrive` defaults to IsReadOnly=false; discussion #3061 confirms IsReadOnly=true is the mechanism for sharing]

### Anti-Patterns to Avoid
- **Writing tar to disk before converting:** doubles IO and temporarily doubles disk usage. Use `io.Pipe`.
- **Using the OCI tag as the cache key:** tags mutate. `alpine:3.19` today ≠ `alpine:3.19` in 6 months. Use manifest digest.
- **Running `crane.Export` + extracting to a directory + shelling out to `mkfs.ext4 -d`:** requires root, e2fsprogs installed, and breaks cross-platform builds. Use `tar2ext4` in-process.
- **Mounting the ext4 image on the host for verification:** requires root + loop device; not portable. Verify by re-opening with `tar2ext4`'s reader or just trust the checksum.
- **Deleting a cached ext4 while a VM is booted from it:** Firecracker opens the file on VM start and keeps it mapped. Deleting it doesn't free space until the last VM stops. Phase 2 should document this but need not implement refcounting until Phase 6 (REST-06 "Memory file remains immutable while any VM restored from it is running").
- **Allowing arbitrary `MaximumDiskSize`:** user could request a 100 GiB ext4 and fill the host. Cap at a configurable ceiling (default 2 GiB works for Phase 2 scope — Alpine, Ubuntu, Debian minimal roots are 50-500 MiB).
- **Pulling without a platform pin:** multi-arch images can resolve to ARM64 on an AMD64 host. Always set `crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"})` (or make it configurable).

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| OCI manifest parsing | Custom JSON unmarshaller for `application/vnd.oci.image.manifest.v1+json` | `crane.Pull` returns `v1.Image` with `.Manifest()`, `.Digest()`, `.Layers()` | Handles Docker v2 schema 1/2, OCI v1, image index fanout, auth negotiation |
| Layer flattening (applying each layer + whiteouts) | Custom tar-merging code | `crane.Export(img, writer)` | Does the mount-style layer merge + OCI whiteout handling in one call |
| Writing ext4 from scratch | Bit-packing superblocks/inodes manually | `tar2ext4.ConvertTarToExt4` | ext4 on-disk format is 600+ pages of kernel docs; tar2ext4 is production-tested in containerd |
| OCI reference parsing | Regex for `registry/org/name:tag@digest` | `name.ParseReference(ref)` | Handles default registry (`docker.io`), default namespace (`library/`), tag/digest precedence |
| Content-addressable cache keying | Hash the tar stream yourself | `v1.Image.Digest()` | Already computed by the registry; avoiding double-hashing |
| OCI whiteout conversion (`.wh.file` -> `0,0 char device`) | Custom tar walker | `tar2ext4.ConvertWhiteout()` option | Whiteout semantics are subtle (whiteout-opaque directories, escape chars); tar2ext4 handles them |
| Registry auth | Custom bearer token negotiation | `crane.WithAuthFromKeychain(authn.DefaultKeychain)` | Reads `~/.docker/config.json`, handles challenge-response, token refresh |
| Fetching a specific architecture from a multi-arch image | Custom manifest-index walker | `crane.WithPlatform(&v1.Platform{...})` | Handles `application/vnd.oci.image.index.v1+json` + `application/vnd.docker.distribution.manifest.list.v2+json` both |

**Key insight:** The OCI specification is a moving target (Docker v2 schema 1/2, OCI v1, OCI v1.1, multi-arch indexes, attestation manifests, sparse indexes). A hand-rolled parser will be wrong within 6 months. `go-containerregistry` is the library AWS, Google, and the CNCF use; lean on it.

## Common Pitfalls

### Pitfall 1: Docker-style whiteouts not converted properly
**What goes wrong:** A file deleted in a higher OCI layer appears in the flattened tar as `.wh.filename`. If this isn't converted to an overlayfs-style whiteout (char device major=0 minor=0) OR handled during tar processing, the ext4 image will contain spurious `.wh.*` files instead of the intended deletion.
**Why it happens:** OCI layer spec uses `.wh.` filename prefixes; overlayfs uses 0/0 char devices; ext4 is neither and needs a choice.
**How to avoid:** Pass `tar2ext4.ConvertWhiteout()` option. This processes `.wh.` entries as deletions during tar walk rather than preserving them as files. Verify by listing the resulting ext4 (on Linux) for any remaining `.wh.*` files after a multi-layer pull.
**Warning signs:** `ls` inside a restored VM shows `.wh.oldfile` entries; disk usage mysteriously higher than expected.
[VERIFIED: tar2ext4 source — `ConvertWhiteout()` option exists for this exact purpose]

### Pitfall 2: ext4 size estimation is non-trivial
**What goes wrong:** Setting `MaximumDiskSize` too low causes `ConvertTarToExt4` to fail partway through with "disk full". Setting it too high wastes disk and memory-maps a huge file.
**Why it happens:** ext4 metadata (inode tables, superblocks, block bitmaps) consumes 3-5% on top of file data. Small files are worse — a 100 MB rootfs with many small files can need 110 MB on ext4.
**How to avoid:** Two options: (a) stream once to `io.Discard` + sum tar entry sizes, apply 1.2x safety factor, then stream for real. (b) Pick a fixed-ceiling default (2 GiB) and fail loudly if exceeded. Phase 2 scope recommends (b) — simpler and user can tune `MaxImageSize` per provisioner.
**Warning signs:** "no space left on device" errors from `ConvertTarToExt4` on unexpectedly large images.
[VERIFIED: tar2ext4 default is 16 GiB per docs; fail-early pattern is standard]

### Pitfall 3: Cache corruption from concurrent provisioning
**What goes wrong:** Two callers simultaneously call `Provision("alpine:3.19")`. Both resolve to the same digest, both try to write `{digest}.ext4`. One partial write gets visible or one clobbers the other.
**Why it happens:** No cross-process lock by default.
**How to avoid:** Write to `{digest}.ext4.tmp-{pid}-{random}`, then `os.Rename` to final path (atomic on same filesystem). First writer wins; second writer either sees the existing file before starting (cache hit) or its rename replaces a completed file identically (still correct since content is digest-addressed).
**Warning signs:** Truncated ext4 files; kernel panic messages mentioning "ext4-fs error" on VM boot.
[ASSUMED: this is the standard approach; not directly cited]

### Pitfall 4: Registry auth / rate limits hit in tests
**What goes wrong:** Integration tests pull from Docker Hub, hit anonymous rate limit (100 pulls/6h as of 2025), CI blocks.
**Why it happens:** Docker Hub throttles unauthenticated pulls.
**How to avoid:** (a) Use a smaller public registry for tests: `ghcr.io/...` or `public.ecr.aws/...`. (b) Use a local registry for CI (`registry:2` container) seeded with a single test image. (c) Cache the test image binary in a `testdata/` tarball and use `crane.Load(path)` instead of `crane.Pull`. Phase 2 integration tests should use (c) for determinism.
**Warning signs:** CI failures with HTTP 429 from `registry-1.docker.io`.
[VERIFIED: Docker Hub rate limits, documented policy]

### Pitfall 5: `crane.Pull` uses the system DNS and may block on offline builds
**What goes wrong:** Running `go test` offline fails with "no such host" from `crane.Pull`.
**Why it happens:** crane goes to the network on every call.
**How to avoid:** Structure the `Provisioner` so the pull step is an interface (`ImageFetcher`) with a real impl (`craneFetcher{}`) and a test impl (`fixtureFetcher{}` that reads from `testdata/`). Unit tests use the fixture; integration tests use crane.
**Warning signs:** Tests fail on laptops without wifi; CI flakes on registry hiccups.
[ASSUMED: standard interface-seam pattern]

### Pitfall 6: Platform resolution surprises
**What goes wrong:** `alpine:3.19` is a multi-arch image. `crane.Pull` on an M1 Mac returns the arm64 variant; on an amd64 Firecracker host, that won't boot.
**Why it happens:** crane defaults to the host's GOOS/GOARCH when not specified.
**How to avoid:** ALWAYS set `crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"})` — Firecracker is always Linux/amd64 (or aarch64 on ARM hosts, but you know the target). Make this a `ProvisionerConfig.DefaultPlatform` field.
**Warning signs:** "exec format error" kernel panic on guest boot; ELF header mismatch.
[VERIFIED: crane.WithPlatform documented at pkg.go.dev]

### Pitfall 7: Read-only drive + container init that writes to `/`
**What goes wrong:** Boots with `IsReadOnly: true`, then the guest init system (systemd, busybox, tini) tries to write to `/run`, `/tmp`, `/etc/resolv.conf`, `/var/log` — remount-rw failures cause boot failure.
**Why it happens:** Most container images assume writable `/`. The kernel mounts `/run` and `/tmp` as tmpfs automatically, but `/etc`, `/var`, `/root` are on the read-only ext4.
**How to avoid:** Three escalating options: (1) for Phase 2 v1: ship a tiny init that bind-mounts a tmpfs over writable paths (cheap, no overlay drive needed); (2) add a per-VM scratch drive mounted at `/data` and write logs there; (3) the full e2b/firecracker-containerd overlay-init approach. Phase 2 should document the limitation and leave (2) or (3) to a later phase. The base image itself (Alpine, minimal Ubuntu) boots fine read-only if `/` is the only ext4 and `/run`, `/tmp` are kernel-default tmpfs.
**Warning signs:** Kernel messages like `EXT4-fs (vda): Remount failed, read-only mode`; init scripts fail with `EROFS`.
[VERIFIED: firecracker-containerd root-filesystem.md + discussion #3061]

## Code Examples

Verified patterns from official sources:

### Example 1: Pull image and get manifest digest
```go
// Source: https://pkg.go.dev/github.com/google/go-containerregistry/pkg/crane
import (
    "github.com/google/go-containerregistry/pkg/crane"
    v1 "github.com/google/go-containerregistry/pkg/v1"
)

func resolveDigest(ref string) (v1.Hash, error) {
    img, err := crane.Pull(ref,
        crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"}))
    if err != nil {
        return v1.Hash{}, fmt.Errorf("firecracker: image: pull %s: %w", ref, err)
    }
    return img.Digest()
}
// Result: v1.Hash{Algorithm: "sha256", Hex: "a0264d60f80..."}
```
[VERIFIED: crane.Pull signature + v1.Image.Digest() at pkg.go.dev/github.com/google/go-containerregistry/pkg/crane and pkg.go.dev/github.com/google/go-containerregistry/pkg/v1]

### Example 2: Full pipeline — OCI ref to ext4 file
```go
// Source: synthesis of crane.Export + tar2ext4.ConvertTarToExt4
import (
    "io"
    "os"
    "path/filepath"

    "github.com/Microsoft/hcsshim/ext4/tar2ext4"
    "github.com/google/go-containerregistry/pkg/crane"
    v1 "github.com/google/go-containerregistry/pkg/v1"
)

func provision(ref, cacheDir string, maxSize int64) (string, error) {
    platform := &v1.Platform{OS: "linux", Architecture: "amd64"}
    img, err := crane.Pull(ref, crane.WithPlatform(platform))
    if err != nil {
        return "", &ImagePullError{Ref: ref, Cause: err}
    }
    digest, err := img.Digest()
    if err != nil {
        return "", fmt.Errorf("firecracker: image: digest: %w", err)
    }

    finalPath := filepath.Join(cacheDir, digest.Algorithm, digest.Hex+".ext4")
    if _, err := os.Stat(finalPath); err == nil {
        return finalPath, nil // cache hit
    }

    if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
        return "", err
    }

    tmp, err := os.CreateTemp(filepath.Dir(finalPath), digest.Hex+".tmp-*")
    if err != nil {
        return "", err
    }
    tmpPath := tmp.Name()
    defer os.Remove(tmpPath)

    pr, pw := io.Pipe()
    errCh := make(chan error, 1)
    go func() {
        errCh <- crane.Export(img, pw)
        pw.Close()
    }()

    if err := tar2ext4.ConvertTarToExt4(pr, tmp,
        tar2ext4.ConvertWhiteout(),
        tar2ext4.MaximumDiskSize(maxSize),
    ); err != nil {
        tmp.Close()
        return "", &Ext4ConvertError{Cause: err}
    }
    if err := <-errCh; err != nil {
        tmp.Close()
        return "", &ImagePullError{Ref: ref, Cause: err}
    }
    if err := tmp.Sync(); err != nil { tmp.Close(); return "", err }
    if err := tmp.Close(); err != nil { return "", err }
    if err := os.Rename(tmpPath, finalPath); err != nil { return "", err }
    return finalPath, nil
}
```
[VERIFIED: all three library calls against their pkg.go.dev signatures on 2026-04-05]

### Example 3: Share the provisioned rootfs across VMs (read-only)
```go
// Building on Phase 1's Manager — this is Phase 2's integration point.
// After Provision() returns a path, many VMs can point VMConfig.RootfsPath at it.

sharedPath, _ := provisioner.Provision(ctx, "docker.io/library/alpine:3.19")

for i := 0; i < 10; i++ {
    cfg := firecracker.VMConfig{
        ID:              fmt.Sprintf("vm-%d", i),
        VCPUs:           1,
        MemoryMiB:       256,
        KernelImagePath: "/srv/kernels/vmlinux-5.10",
        RootfsPath:      sharedPath,   // ← same path, all 10 VMs
        ReadOnlyRootfs:  true,         // ← NEW Phase 2 field on VMConfig
        // ... jailer, kernel args, etc.
    }
    vm, _ := mgr.Create(ctx, cfg)
    _ = mgr.Start(ctx, vm.ID)
}
// All 10 VMs boot from the same ext4 file. Firecracker opens the file
// read-only; the host kernel page-caches it once for all VMs.
```
[VERIFIED: IsReadOnly sharing pattern confirmed in firecracker discussion #3061]

## Runtime State Inventory

Phase 2 is a greenfield extension (new subpackage, no rename/refactor). No runtime state migration required. Omitted per instructions.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual `dd` + `mkfs.ext4` + Docker container mount | `crane.Pull` + `tar2ext4.ConvertTarToExt4` in-process | ~2021 with tar2ext4 maturation | Single-binary provisioner, no root, no loop devices, cross-platform build |
| Docker v2 schema 1 manifests | OCI v1 / Docker v2 schema 2 | ~2018-2020 | Modern libraries handle both transparently; manual code lags |
| Per-VM rootfs copy | Read-only shared base + per-VM overlay | firecracker-containerd, e2b (~2020-2024) | ~95% disk savings for fleets of identical VMs |
| Squashfs for read-only base | ext4 read-only (Phase 2 choice) OR squashfs | — | Squashfs gives ~30% compression but needs `squashfs-tools`; ext4 read-only is simpler and aligns with Firecracker's native block format |
| Tag as cache identity | Manifest digest as cache identity | OCI spec formalization ~2018 | Determinism guarantee; immutable references |

**Deprecated/outdated:**
- Pre-OCI image formats (Docker v1 schema): registries still support for compatibility; new tools don't emit them. `crane` transparently handles both.
- `docker save | tar -xf` → mount → copy approach: works but requires running daemon. Only use for one-off hand-built images.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Phase 2 scope is "shared read-only rootfs" only; per-VM writable overlay (with `overlay-init`, kernel arg changes, second drive) is deferred to a later phase | Summary, Pattern 5, Pitfall 7 | If wrong, plan is too narrow — user may expect full E2B-style overlay. Success criterion wording "without filesystem conflicts" supports read-only-only scope, but confirm during plan-check. |
| A2 | 2 GiB default `MaxImageSize` is sufficient for typical sandbox images (Alpine ~50MB, Ubuntu minimal ~80MB, Debian slim ~80MB, Python 3.12 ~150MB) | Standard Stack / Pattern 1 | If wrong, some images will fail conversion. Mitigation: make the ceiling user-configurable (already in design). |
| A3 | `linux/amd64` is the correct default platform | Pattern 1, Pitfall 6 | Wrong on ARM hosts. Mitigation: `DefaultPlatform` is configurable. |
| A4 | Cache directory layout `{cacheDir}/sha256/{hex}.ext4` is the right scheme | Pattern 2 | Low risk; format is simple. Alternative `{cacheDir}/{hex}.ext4` is flatter but loses algorithm-agility if SHA3/Blake3 ever become OCI-standard. |
| A5 | Atomic-rename-via-tempfile is sufficient for cross-process cache safety | Pattern 4, Pitfall 3 | Wrong if two provisioners on the same host race on the same digest — both complete, both rename. Last-writer-wins is fine since content is digest-addressed and identical. |
| A6 | `crane` and `tar2ext4` remain stable APIs (no breaking changes expected before v1) | Standard Stack | Both are in 0.x versioning. Risk is medium. Mitigation: pin minor versions in `go.mod`, cover with tests. |
| A7 | Registry auth for private images is OUT OF SCOPE for Phase 2 v1 | Standard Stack (Supporting) | If user has private images, Phase 2 can't handle them. Mitigation: mark as extension point; `crane.WithAuthFromKeychain` is a one-line addition later. |
| A8 | VMConfig gets a new `ReadOnlyRootfs bool` field (Phase 1's default stays writable) | Pattern 5, Code Example 3 | Low risk; additive change. Alternative: flip Phase 1 default to read-only (breaking). Stick with additive. |
| A9 | No TLS pinning or registry allowlisting needed for Phase 2 | Security Domain | If deployment expects strict supply-chain security, need image signature verification (cosign/notary). Mitigation: note as extension point. |
| A10 | Default cache dir `/var/lib/opensandbox/rootfs` follows FHS conventions | Pattern 1 | May conflict with distro packaging. Mitigation: field is configurable. |

## Open Questions

1. **Per-VM writable overlay in Phase 2 or later?**
   - What we know: success criterion #2 says "multiple VMs can boot from the same base rootfs image without filesystem conflicts" — read-only achieves this literally.
   - What's unclear: whether users will attempt to write to `/` and be surprised.
   - Recommendation: scope Phase 2 to read-only sharing; document the limitation; add a dedicated overlay phase if user hits it.

2. **Where should the Provisioner be instantiated — by the caller or as part of Manager?**
   - What we know: Phase 1 `Manager` has no image awareness. `ManagerConfig` has `ChrootBaseDir` but no `RootfsCacheDir`.
   - What's unclear: whether Manager should own a `*Provisioner` or whether callers wire the two separately.
   - Recommendation: separate structs. `Provisioner` produces paths; caller sets `VMConfig.RootfsPath`. Avoids coupling and lets tests exercise each independently. Can revisit if ergonomics hurt.

3. **How to handle image pull over a flaky network?**
   - What we know: `crane.Pull` does not retry by default. Docker Hub rate-limits at 429.
   - What's unclear: whether a retry-with-backoff wrapper is needed for Phase 2 or can be deferred.
   - Recommendation: add a simple exponential-backoff wrapper (3 attempts, 1s/2s/4s) around pulls. Matches existing `sdks/sandbox/go/opensandbox/retry.go` shape.

4. **Do we need to verify the ext4 image after writing?**
   - What we know: `tar2ext4` doesn't emit a checksum by default.
   - What's unclear: whether the caller wants post-write verification.
   - Recommendation: compute SHA256 of the resulting ext4 file, write alongside as `{digest}.ext4.sha256`. Quick, cheap, lets users verify cache integrity out-of-band. Matches `VerifyKernelChecksum` pattern from Phase 1 Plan 04.

5. **Test fixture strategy — real registry vs local tarballs?**
   - What we know: Docker Hub rate-limits. Offline dev environments exist.
   - What's unclear: CI policy.
   - Recommendation: use `crane.Load()` with a bundled tiny OCI tarball in `testdata/` for unit tests. Integration tests (behind `//go:build integration`) can hit a real registry. `Makefile` has a `fetch-test-image` target that curl's a small image to `testdata/` once.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go 1.24+ | All code | Yes | 1.24.5 (matches Phase 1 go.mod) | — |
| Docker daemon | Integration tests (optional) | Yes | 28.5.2 | Use `crane.Load` from testdata tarball |
| skopeo | Evaluating pulls manually | No | — | Not needed — crane is the library path |
| umoci | Evaluating extracts manually | No | — | Not needed — crane.Export is the library path |
| mkfs.ext4 | Not needed | No (macOS) | — | tar2ext4 is pure Go, no fallback needed |
| mke2fs / e2fsprogs | Not needed for provisioning | No (macOS) | — | Not needed; could install on Linux for debugging |
| tar | Testing / debugging tarballs | Yes | bsdtar 3.5.3 | — |
| jq | Manifest inspection during debugging | Yes | 1.7.1 | — |
| Network access to registries | `crane.Pull` | Yes | — | Use `crane.Load` from testdata for offline tests |

**Missing dependencies with no fallback:** None. The choice of `crane` + `tar2ext4` was specifically made to eliminate host dependencies.

**Missing dependencies with fallback:** None material — all alternatives are debug/inspection helpers.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go built-in `testing` package |
| Config file | `runtime/firecracker/go.mod` (same module as Phase 1) |
| Quick run command | `cd runtime/firecracker && go test ./image/... -short -v` |
| Full suite command | `cd runtime/firecracker && make test` + `make test-integration` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| IMG-01 | Provision from OCI image produces ext4 | integration | `go test -tags=integration ./image/ -run TestProvisionAlpine -v` | ❌ Wave 0 |
| IMG-01 | Provisioner calls crane + tar2ext4 correctly | unit (fixture fetcher) | `go test ./image/ -run TestProvision_FromFixture -v` | ❌ Wave 0 |
| IMG-01 | Ext4 conversion respects MaximumDiskSize | unit | `go test ./image/ -run TestProvision_SizeLimit -v` | ❌ Wave 0 |
| IMG-01 | Whiteouts handled via ConvertWhiteout option | unit | `go test ./image/ -run TestProvision_Whiteouts -v` | ❌ Wave 0 |
| IMG-02 | Rootfs stored in configurable path | unit | `go test ./image/ -run TestStore_Path -v` | ❌ Wave 0 |
| IMG-02 | Default path applied via withDefaults | unit | `go test ./image/ -run TestProvisionerConfig_Defaults -v` | ❌ Wave 0 |
| IMG-02 | Store creates cache dir with correct permissions | unit | `go test ./image/ -run TestStore_Init -v` | ❌ Wave 0 |
| IMG-03 | Same OCI tag produces same ext4 (determinism) | unit | `go test ./image/ -run TestProvision_Deterministic -v` | ❌ Wave 0 |
| IMG-03 | Cache hit on second provision | unit | `go test ./image/ -run TestProvision_CacheHit -v` | ❌ Wave 0 |
| IMG-03 | Multiple VMs can attach same rootfs path | unit (VMConfig side) | `go test ./... -run TestVMConfig_SharedRootfs -v` | ❌ Wave 0 |
| IMG-03 | Atomic write prevents torn files under concurrent provision | unit | `go test ./image/ -run TestProvision_Concurrent -v` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `cd runtime/firecracker && go test ./image/ -short -v` (< 5s, no network)
- **Per wave merge:** `cd runtime/firecracker && make test` (all unit tests, still no network)
- **Phase gate:** `cd runtime/firecracker && make test && make test-integration` (integration hits real registry OR uses testdata tarball)

### Wave 0 Gaps
- [ ] `runtime/firecracker/image/` directory — doesn't exist yet
- [ ] `runtime/firecracker/image/provisioner.go` — covers IMG-01
- [ ] `runtime/firecracker/image/provisioner_test.go` — covers IMG-01
- [ ] `runtime/firecracker/image/store.go` — covers IMG-02
- [ ] `runtime/firecracker/image/store_test.go` — covers IMG-02
- [ ] `runtime/firecracker/image/reference.go` — OCI ref parsing
- [ ] `runtime/firecracker/image/reference_test.go`
- [ ] `runtime/firecracker/image/errors.go` — new error types
- [ ] `runtime/firecracker/image/integration_test.go` — `//go:build integration`
- [ ] `runtime/firecracker/image/testdata/` — bundled tiny OCI tarball for offline unit tests
- [ ] `runtime/firecracker/vm.go` additions: `ReadOnlyRootfs bool` field on `VMConfig`
- [ ] `runtime/firecracker/vm_linux.go` additions: propagate `ReadOnlyRootfs` to drive config
- [ ] `runtime/firecracker/Makefile` additions: `fetch-test-image` target, possibly `verify-image` target mirroring `verify-kernel`
- [ ] `runtime/firecracker/.gitignore` update: add `rootfs-cache/` or similar

## Security Domain

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No (v1 scope: public images only) | N/A — extension point for `crane.WithAuthFromKeychain` |
| V3 Session Management | No | N/A |
| V4 Access Control | Yes | Cache directory must be writable only by runtime user; rootfs files `0o444` read-only; drive attached `IsReadOnly:true` |
| V5 Input Validation | Yes | Validate OCI reference via `name.ParseReference`; reject refs that resolve to manifests >`MaxImageSize`; cap `MaxImageSize` at a host-policy ceiling |
| V6 Cryptography | Partial | Manifest digest (SHA-256) is the integrity anchor — trust inherited from registry TLS + content addressing. No private-key crypto. |
| V8 Data Protection | Yes | Cache directory scoped to runtime user; filesystem perms 0o750; ext4 files 0o444 |
| V9 Communication | Yes | HTTPS to registry enforced by default in crane; no downgrade; no insecure registry without explicit opt-in |
| V10 Malicious Code | Partial | Out of scope for v1: image signature verification (cosign, sigstore). Marked as deferred. |
| V14 Configuration | Yes | Default `linux/amd64` platform; fail-closed on platform mismatch; MaxImageSize has a hard default ceiling |

### Known Threat Patterns for OCI image provisioning

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Registry MITM swaps image | Tampering | HTTPS enforced; manifest digest anchored; tag-to-digest mapping cached but re-resolved on each `Provision` call |
| Malicious image contains huge file → disk exhaustion | DoS | `MaximumDiskSize` cap on `ConvertTarToExt4`; fail closed at configured ceiling |
| Malicious image contains billion-file tar → inode exhaustion in ext4 | DoS | tar2ext4 respects MaximumDiskSize which bounds inode count via ext4 metadata budget; additionally cap tar entry count as a custom option (not built-in to tar2ext4) — ACTION: add pre-scan or entry limit |
| Tag mutation between build and boot | Tampering | Digest-based cache key freezes tag→digest at provision time; re-provisioning on demand is explicit |
| Symlink escape in tar → writes outside ext4 | Tampering | tar2ext4 writes into a new ext4 image (sandboxed by definition); symlinks in tar become symlinks in ext4; guest-side sandbox handles evaluation |
| OCI whiteout files leak into final fs | Information Disclosure | `tar2ext4.ConvertWhiteout()` option processes deletions correctly |
| Shared rootfs modified by one VM affects others | Tampering / Info Disclosure | `IsReadOnly: true` on drive; host kernel enforces read-only mmap; VM cannot write to shared ext4 |
| Race between provisioning and destroying cache | DoS | Atomic-rename pattern; consumers hold open file descriptor on VM start (Firecracker does this natively via its drive open); deletion after file is opened is safe on Linux (unlink-while-open) |
| Arbitrary platform/architecture attack | Tampering | Explicit `crane.WithPlatform` — never auto-detect from host |
| Cache poisoning via predictable tempfile name | Tampering | Use `os.CreateTemp` with random suffix |

## Sources

### Primary (HIGH confidence)
- [go-containerregistry crane pkg.go.dev](https://pkg.go.dev/github.com/google/go-containerregistry/pkg/crane) — `Pull`, `Export`, `Load`, `WithPlatform` signatures
- [go-containerregistry v1 pkg.go.dev](https://pkg.go.dev/github.com/google/go-containerregistry/pkg/v1) — `v1.Image`, `v1.Hash`, `v1.Platform` types
- [tar2ext4 pkg.go.dev](https://pkg.go.dev/github.com/Microsoft/hcsshim/ext4/tar2ext4) — `ConvertTarToExt4`, all Option functions
- [tar2ext4 source](https://github.com/microsoft/hcsshim/blob/main/ext4/tar2ext4/tar2ext4.go) — verified no platform build tags, cross-platform confirmed
- [firecracker-go-sdk drives.go](https://github.com/firecracker-microvm/firecracker-go-sdk/blob/main/drives.go) — `DrivesBuilder`, `WithRootDrive`, `WithReadOnly` options
- [Firecracker rootfs-and-kernel-setup.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/rootfs-and-kernel-setup.md) — official guidance on ext4 rootfs construction
- [Firecracker discussion #3061 — shared rootfs with CoW](https://github.com/firecracker-microvm/firecracker/discussions/3061) — canonical explanation of read-only base + overlay pattern
- [firecracker-containerd root-filesystem.md](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/root-filesystem.md) — production architecture for multi-VM rootfs sharing
- [firecracker-containerd image-builder README](https://github.com/firecracker-microvm/firecracker-containerd/blob/main/tools/image-builder/README.md) — alternative debootstrap-based path (reference only)
- [OCI image-spec manifest.md](https://pkg.go.dev/github.com/opencontainers/image-spec/specs-go/v1) — manifest digest semantics, RootFS DiffID/ChainID

### Secondary (MEDIUM confidence)
- [E2B blog — Scaling Firecracker with OverlayFS](https://e2b.dev/blog/scaling-firecracker-using-overlayfs-to-save-disk-space) — production pattern from a Firecracker-based sandbox vendor
- [Firecracker discussion #4740 — buildfs tool](https://github.com/firecracker-microvm/firecracker/discussions/4740) — community alternative (Rust-based, not used here but informs pattern)
- [Firecracker snapshot-support.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md) — block device responsibility during snapshots (user-managed)

### Tertiary (LOW confidence / advisory)
- [Parandrus — Space Efficient Filesystems for Firecracker](https://parandrus.dev/devicemapper/) — devicemapper alternative; informs Pitfall 7 discussion

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — both `crane` and `tar2ext4` verified via go-list-m + cross-platform compile smoke test, version-pinned with publish dates
- Architecture: HIGH — cache layout, atomic write, and read-only-drive sharing are standard patterns with cited sources
- Pitfalls: HIGH — whiteouts, size estimation, auth, and platform-pinning verified against tar2ext4 source and crane docs
- Multi-VM sharing (IMG-03): HIGH — discussion #3061 + firecracker-containerd docs confirm the read-only drive pattern; scope decision (read-only without overlay-init) is A1 assumption
- Read-only drive field addition (VMConfig.ReadOnlyRootfs): MEDIUM — this is the cleanest integration with Phase 1, but the exact field name/placement is a Phase 2 choice

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (30 days — crane and tar2ext4 are stable; Firecracker releases quarterly)

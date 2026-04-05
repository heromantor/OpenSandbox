# Stack Research

**Domain:** Firecracker runtime backend with snapshot/restore in Go
**Researched:** 2026-04-04
**Confidence:** HIGH (core stack), MEDIUM (server integration layer)

## Context

The OpenSandbox server is a **Python/FastAPI application**. Adding a Firecracker runtime means implementing a new `FirecrackerSandboxService` that inherits from the `SandboxService` abstract base class — in Python. The Firecracker-specific orchestration logic (VM lifecycle, networking, snapshots) is written in Go as a separate daemon/controller that the Python service delegates to, or alternatively the entire backend can be implemented purely in Python calling the Firecracker Unix socket API directly.

This research focuses on the **Go-first path**: a Go daemon that owns Firecracker VM lifecycle and exposes an internal API to the Python service. This is the higher-performance option and aligns with the existing Go SDK work on this branch.

---

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `github.com/firecracker-microvm/firecracker-go-sdk` | main (post-v1.0.0) | Firecracker VM lifecycle management: create, start, pause, snapshot, load, stop | The only official Go SDK for the Firecracker API. Wraps the Firecracker Unix socket API, provides typed models for all operations, includes `PauseVM`, `ResumeVM`, `CreateSnapshot`, `LoadSnapshotHandler`, and `AddVsocksHandler`. Requires Go 1.24. Used by firecracker-containerd and production users (Fly.io ecosystem). |
| Firecracker binary | v1.15.0 | MicroVM hypervisor (runtime dependency, not a Go import) | Latest stable release (March 2025). Fixes a critical diff-snapshot memory corruption bug in v1.14.2. vsock UDS path override on restore (#5323) is essential for CID management across restored VMs. Same version must be used for snapshot and restore — never mix versions. |
| `github.com/mdlayher/vsock` | v1.2.1 | AF_VSOCK host-side listener and dialer for host↔guest communication | Pulled in by firecracker-go-sdk itself; surface it explicitly because the execd agent needs to accept vsock connections. MIT licensed, stable v1 API, actively maintained by the same maintainer who contributes to the firecracker-go-sdk. Provides `net.Conn`-compatible `Dial` and `Listen` — drops in where any net.Conn is expected. |
| `github.com/vishvananda/netlink` | v1.3.1 | TAP device creation, IP address assignment, routing setup in Go | The standard Go library for programmatic `ip link` / `ip addr` / `ip route` operations via netlink. Used by firecracker-go-sdk and firecracker-containerd for tap device setup. Avoids shell-out to `ip` commands. Needed for each VM's isolated tap device lifecycle. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/vishvananda/netns` | v0.0.5 | Linux network namespace operations | When each VM gets its own network namespace for full isolation. Not required for simple host-namespace tap setups, but needed for namespace-per-VM pattern. |
| `github.com/containernetworking/cni` + `plugins` | v1.3.0 / v1.9.0 | CNI plugin interface + standard plugins (ptp, host-local, firewall) | Use when integrating Firecracker networking with OpenSandbox's existing FQDN egress CNI chain. tc-redirect-tap from `github.com/awslabs/tc-redirect-tap` chains after any CNI plugin to hand off a tap device to Firecracker. Skip for simpler manual tap setups. |
| `github.com/sirupsen/logrus` | v1.9.3 | Structured logging | firecracker-go-sdk uses logrus natively — using it in the runtime daemon maintains log correlation with SDK-level events. If the project standardizes on `log/slog` (Go 1.21+), a logrus-to-slog bridge exists. |
| `github.com/google/uuid` | v1.6.0 | VM ID and vsock CID generation | Already in the existing Go SDK (same version). Generates UUIDs for VM IDs; derive deterministic CIDs from UUIDs to avoid conflicts across restored snapshots. |
| `github.com/hashicorp/go-multierror` | v1.1.1 | Collecting multiple cleanup errors during VM teardown | VM teardown involves multiple independent operations (stop VM, delete tap device, release CID, remove snapshot files). go-multierror collects all failures without short-circuiting. Already pulled by firecracker-go-sdk. |
| `golang.org/x/sys` | v0.39.0 | Low-level Linux syscalls (userfaultfd, mmap, epoll) | Needed if implementing a UFFD page-fault handler for lazy snapshot restore. For Phase 1/2 (File backend), the standard library suffices. Promote to direct dependency in Phase 3 if implementing UFFD. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `make` | Build orchestration | Follow existing SDK Makefile patterns. Add targets: `build-daemon`, `test-daemon`, `test-integration-firecracker`. |
| Firecracker v1.15.0 binary | Runtime test dependency | Pin version in `scripts/install-firecracker.sh`. Integration tests require KVM access (`/dev/kvm`) and root or CAP_NET_ADMIN. |
| Jailer binary (same version) | Production isolation | Ships alongside the Firecracker binary in the official release archive. Required for production; optional for development. Set `JailerCfg` in `firecracker.Config` when enabling. |
| `mkfs.ext4` (e2fsprogs) | Build rootfs images for guests | Standard Linux tool. Use to create ext4 block device images from OCI containers via `docker export`. Script-driven; not a Go import. |
| Linux kernel image (`vmlinux`) | Guest kernel binary | Download from official Firecracker S3 bucket (`s3://spec.ccfc.min/img/`). Pin the kernel version. Must match across all snapshot/restore operations. |

---

## Installation

```bash
# Go module — add to your go.mod
go get github.com/firecracker-microvm/firecracker-go-sdk@main
go get github.com/mdlayher/vsock@v1.2.1
go get github.com/vishvananda/netlink@v1.3.1
go get github.com/vishvananda/netns@v0.0.5

# System dependencies (Ubuntu/Debian host)
sudo apt-get install -y e2fsprogs  # mkfs.ext4

# Firecracker binary (pin exact version)
FIRECRACKER_VERSION="v1.15.0"
ARCH="$(uname -m)"
curl -L \
  "https://github.com/firecracker-microvm/firecracker/releases/download/${FIRECRACKER_VERSION}/firecracker-${FIRECRACKER_VERSION}-${ARCH}.tgz" \
  | tar xz
sudo install -o root -g root -m 0755 \
  "release-${FIRECRACKER_VERSION}-${ARCH}/firecracker-${FIRECRACKER_VERSION}-${ARCH}" \
  /usr/local/bin/firecracker
sudo install -o root -g root -m 0755 \
  "release-${FIRECRACKER_VERSION}-${ARCH}/jailer-${FIRECRACKER_VERSION}-${ARCH}" \
  /usr/local/bin/jailer
```

---

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `firecracker-go-sdk` (main) | Direct Firecracker HTTP/Unix socket client (`net/http` + `/run/firecracker.sock`) | Only if the Go SDK's OpenAPI-generated client surface becomes a maintenance problem. The SDK adds <5% overhead over raw socket calls and saves significant boilerplate. |
| `mdlayher/vsock` | `linuxkit/virtsock` | If targeting Hyper-V guests or macOS (HyperKit) in addition to KVM. For this project (Linux-only Firecracker), mdlayher/vsock is simpler and already a transitive dependency. |
| TAP + `vishvananda/netlink` (manual) | CNI + `tc-redirect-tap` | CNI is better when OpenSandbox's existing egress CNI chain needs to be composed with the Firecracker tap. Manual tap is simpler and sufficient for Phase 1 where egress goes through the host proxy, not CNI. Upgrade to CNI in Phase 2 when integrating FQDN egress directly. |
| Python `FirecrackerSandboxService` calling a Go daemon | Pure Python calling Firecracker's Unix socket API via `aiohttp` | Acceptable for simple lifecycle operations (create/delete). Not acceptable for snapshot management (file operations, memory backend logic) or vsock bridging — Go's concurrency model and binary performance matter there. |
| Full snapshot (File backend) | UFFD lazy restore | UFFD provides faster apparent restore (VM visible before memory fully loaded) but requires a user-space page-fault handler process, adds significant complexity, and needs kernel ≥ 6.1 for `/dev/userfaultfd`. Use File backend for Phase 2; consider UFFD only in Phase 3 if 5-10ms resume target requires it. |
| Diff snapshots (Phase 3 only) | Full snapshots for everything | Diff snapshots reduce storage and transfer size but are still "developer preview" in Firecracker 1.15. They require base+diff merge via `snapshot-editor` tool for standalone restore, and dirty page tracking conflicts with huge pages. Use full snapshots in Phases 1-2; re-evaluate diff support in Phase 3 when Firecracker stabilizes it. |

---

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `firecracker-containerd` | Full container runtime abstraction that wraps Firecracker's API — specifically hides the snapshot and vsock APIs that are the core value of this integration. You cannot call `CreateSnapshot` through containerd. | `firecracker-go-sdk` directly |
| Kata Containers | Confirmed out of scope: Kata wraps Firecracker but intentionally does not expose its snapshot API. This was the original motivation for a direct integration. | Direct Firecracker via `firecracker-go-sdk` |
| `768bit/firecracker-go-sdk` or `valyentdev/firecracker-go-sdk` | Community forks that diverge from the official SDK. The upstream `firecracker-microvm/firecracker-go-sdk` is maintained by AWS and tracks Firecracker releases. | `github.com/firecracker-microvm/firecracker-go-sdk` |
| Diff snapshots in production (Phase 1-2) | Still marked "developer preview" in Firecracker 1.15. v1.14.2 fixed a memory corruption bug in diff snapshots — the feature is not yet stable. | Full snapshots (`snapshot_type: "Full"`) |
| Cross-version snapshot restore | Firecracker's snapshot format is versioned independently since v1.6.0. Restoring a snapshot from a different Firecracker binary version is undefined behavior and was explicitly scoped out in PROJECT.md. | Pin Firecracker version; reject restores if version mismatches. |
| `logrus` if project migrates to `log/slog` | logrus is in maintenance mode (no new features). If OpenSandbox server standardizes on structured logging, use `log/slog` (Go 1.21+ stdlib) with a logrus sink adapter for firecracker-go-sdk compatibility. | `log/slog` with `slog.NewLogLogger` bridge for logrus callers |

---

## Stack Patterns by Variant

**If running without jailer (development/CI):**
- Skip `JailerCfg` in `firecracker.Config`
- Run the Go daemon as root or with `CAP_NET_ADMIN` + `/dev/kvm` access
- Because jailer requires chroot setup, binary copying, and UID/GID allocation — too heavy for fast iteration

**If running with jailer (production):**
- Set `JailerCfg` with `UID`, `GID`, `ID`, `NumaNode`, `ChrootBaseDir`, `ExecFile`
- Use `firecracker.NewJailerCommandBuilder()` provided by the SDK
- Because jailer provides the seccomp filter (24 syscalls), cgroup isolation, and chroot that are the actual production security boundary

**If vsock CID conflicts on multi-tenant host:**
- Derive CID from a persistent counter stored per-host, not from sandbox UUIDs directly
- Because Firecracker uses 32-bit CIDs, and the vsock driver rejects duplicate CIDs across running VMs — UUIDs are 128-bit and will collide when truncated
- The `mdlayher/vsock` `ContextID()` function reads the host's own CID; allocate guest CIDs from a range above 2 (Host=2 is reserved)

**If restoring snapshot on a different host (Phase 3 multi-node):**
- Require CPU template matching: set `cpu_template: "T2"` (Intel) or `"T2S"` (AMD) in the original VM config
- Because CPUID features differ across hosts; CPU templates normalize the visible feature set to the guest
- Without matching, restored guest may crash on instructions that don't exist on the restore host

---

## Version Compatibility

| Package | Compatible With | Notes |
|---------|-----------------|-------|
| `firecracker-go-sdk@main` | Firecracker v1.15.0, Go 1.24 | The SDK's go.mod now requires Go 1.24.11. The v1.0.0 tag (Sept 2022) is outdated — use main branch which tracks Firecracker API changes. |
| `mdlayher/vsock@v1.2.1` | Linux kernel ≥ 4.8 (AF_VSOCK), Go 1.23+ | Already pinned at v1.2.1 in `firecracker-go-sdk`'s go.mod. No need to upgrade. |
| Firecracker v1.15.0 snapshots | Only other v1.15.x instances | Snapshot format is versioned at v1.0.0 (independent of Firecracker version since FC v1.7.0). Snapshots from Firecracker ≤ v1.6.0 are incompatible. |
| `vishvananda/netlink@v1.3.1` | Linux kernel ≥ 3.0 (netlink), Go 1.21+ | Pinned at v1.3.1 in `firecracker-go-sdk`. Use same version to avoid transitive dependency conflicts. |
| Firecracker kernel images | Guest kernel must match between snapshot and restore | Pin the vmlinux version in a config constant. E.g., `vmlinux-5.10.217` from Firecracker's S3 bucket. |

---

## Sources

- `pkg.go.dev/github.com/firecracker-microvm/firecracker-go-sdk` — confirmed Go 1.24 requirement, snapshot/vsock APIs, and direct dependencies (mdlayher/vsock v1.2.1, vishvananda/netlink v1.3.1) — HIGH confidence
- `github.com/firecracker-microvm/firecracker/releases` — confirmed Firecracker v1.15.0 as latest stable (March 9, 2025) — HIGH confidence
- `github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md` — confirmed Full vs Diff types, PATCH /vm pause/resume flow, File vs UFFD backends, diff snapshot "developer preview" status — HIGH confidence
- `github.com/firecracker-microvm/firecracker/blob/main/CHANGELOG.md` — confirmed vsock UDS path override (#5323) and local port fix (#5688) in v1.15.x — HIGH confidence
- `pkg.go.dev/github.com/mdlayher/vsock` — confirmed v1.2.1, AF_VSOCK API surface, ContextID constant — HIGH confidence
- `deepwiki.com/alibaba/OpenSandbox` — confirmed Python/FastAPI server, `SandboxService` ABC pattern, Docker/K8s factory pattern — MEDIUM confidence (DeepWiki inference from source)
- WebSearch: Firecracker production Go implementations (Fly.io, E2B, CodeSandbox patterns) — MEDIUM confidence (not version-specific)
- WebSearch: UFFD lazy restore production readiness — MEDIUM confidence (confirms feature exists, kernel ≥ 6.1 for `/dev/userfaultfd`)

---
*Stack research for: Firecracker runtime backend with snapshot/restore (OpenSandbox)*
*Researched: 2026-04-04*

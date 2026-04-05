# Project Research Summary

**Project:** OpenSandbox Firecracker Runtime Backend
**Domain:** Firecracker-based microVM sandbox runtime with snapshot/restore
**Researched:** 2026-04-04
**Confidence:** HIGH

## Executive Summary

OpenSandbox is a Python/FastAPI sandbox orchestration server that currently supports Docker and Kubernetes runtimes. The goal of this work is to add a first-class Firecracker runtime backend that provides true VM-level isolation and, crucially, snapshot/restore semantics that neither Docker nor Kata Containers expose. The recommended approach is a Go daemon owning the Firecracker VM lifecycle, using `firecracker-go-sdk@main` against Firecracker v1.15.0, with the Python server delegating lifecycle operations to it via a `FirecrackerSandboxService` implementing the existing `SandboxService` abstract base class. Networking is handled via per-VM TAP devices and iptables NAT; host-to-guest communication uses vsock rather than TCP to survive snapshot/restore cycles cleanly.

The three-phase structure is well-supported by the research: Phase 1 establishes the VM runtime foundation (cold-boot lifecycle, TAP networking, vsock-based execd), Phase 2 adds snapshot/restore (the core value proposition: pause state, full snapshot create/load, TTL auto-pause, diff snapshots), and Phase 3 delivers production performance (template VM warm pool achieving ~5-10ms creation, S3/OSS multi-node snapshot backend, diff snapshot pipeline). The critical path is strict: snapshot/restore is impossible without a working VM lifecycle, and the warm pool is impossible without reliable snapshot/restore. Each phase is a non-trivial dependency on the previous one.

The primary risks are operational rather than technical: Firecracker's snapshot format is tightly version-coupled (binary version and host kernel), entropy state is shared across all clones of a snapshot (requiring mandatory reseeding), and memory files are held open via `MAP_PRIVATE` for the entire VM lifetime (requiring reference counting before any GC can run). These are all documented, well-understood problems with known mitigations — but they are "looks done but isn't" traps that break only in production under specific conditions. All three must be addressed in Phase 2, not retrofitted later.

---

## Key Findings

### Recommended Stack

The stack is anchored on `github.com/firecracker-microvm/firecracker-go-sdk` (main branch, requires Go 1.24) as the only official Go wrapper for Firecracker's Unix socket REST API. It provides typed models for all lifecycle and snapshot operations and is used in production by Fly.io-ecosystem projects and firecracker-containerd. Firecracker itself must be pinned at v1.15.0 — the latest stable release (March 2025), which includes the vsock UDS path override fix essential for multi-instance restore and a diff snapshot memory corruption fix from v1.14.2. The `mdlayher/vsock` library (v1.2.1, already a transitive dependency of the SDK) handles host-side AF_VSOCK operations; `vishvananda/netlink` (v1.3.1) handles TAP device provisioning without shelling out to `ip`.

The Go-first path is strongly preferred over a pure-Python implementation calling Firecracker's socket API directly. While Python can handle simple lifecycle calls, snapshot file management, vsock bridging, and the future UFFD page-fault handler all benefit from Go's concurrency model and binary performance. Diff snapshots and UFFD lazy restore are the right long-term path but both are deferred: diff snapshots remain "developer preview" in Firecracker 1.15, and UFFD requires significant complexity and Linux kernel >= 6.1. Use the File backend (full snapshots) in Phases 1-2.

**Core technologies:**
- `firecracker-go-sdk@main`: VM lifecycle, snapshot, vsock — only official Go SDK; tracks Firecracker API changes
- Firecracker v1.15.0 binary: microVM hypervisor — latest stable; includes critical diff-snapshot and vsock UDS fixes
- `mdlayher/vsock@v1.2.1`: host-side AF_VSOCK — stable v1 API, already pulled by SDK
- `vishvananda/netlink@v1.3.1`: TAP/routing via netlink — avoids shell-outs; used by firecracker-containerd
- Jailer binary (same version): production isolation — chroot + seccomp (24 syscalls) + cgroups

**Do not use:**
- `firecracker-containerd` or Kata Containers: both hide the snapshot API that is the entire point of this integration
- Diff snapshots in Phase 1-2: still developer preview; wait for Phase 3 validation
- Cross-version snapshot restore: undefined behavior; reject at gate, not at runtime

### Expected Features

The feature research confirms a strict dependency chain: Jailer isolation enables VM lifecycle, which enables vsock execd, which enables snapshotting, which enables everything else. The warm pool (Phase 3's main differentiator) only makes sense once snapshot/restore is reliable.

**Must have (table stakes):**
- VM lifecycle (create, start, stop, destroy) with Jailer — no production sandbox system ships without it
- vsock-based execd connectivity — TCP does not survive snapshot/restore; vsock listen sockets do
- ext4 rootfs provisioning from OCI images — users bring Docker images; they cannot hand-craft VM disks
- Full snapshot create + restore — the core value proposition of the entire milestone
- Pause/resume state machine — user-visible API surface above Firecracker primitives (OSEP-0008)
- Sandbox TTL with pause-instead-of-delete — idle sandboxes pause, restore on next access (OSEP-0009)
- Snapshot list and delete — unbounded growth is a non-starter operationally
- Network connectivity via TAP + egress proxy integration — DNS and outbound requests must work in guest
- CPU template configuration (T2/T2S) — cheap to set early, impossible to retrofit without invalidating all snapshots
- CID uniqueness per VM — vsock driver rejects duplicate CIDs; collisions cause silent failures

**Should have (competitive differentiators):**
- Template VM warm pool (~5-10ms creation vs ~28ms cold restore vs ~1s cold boot) — E2B's competitive moat
- Diff snapshots with dirty page tracking — full snapshots at 8GB RAM = 8GB file per snapshot
- Object storage backend (S3/OSS) — enables multi-node restore and stateful sandbox mobility
- Snapshot retention policies — production systems accumulate snapshots without automated cleanup

**Defer to v2+:**
- COW rootfs overlay (squashfs base + ext4 overlay): useful optimization but adds operational complexity; evaluate post-Phase 1
- Snapshot-based clone / fork semantics: requires entropy re-seeding contract and no upstream API; out of scope
- UFFD lazy restore: faster apparent restore but adds a user-space page-fault handler process; needs Linux >= 6.1
- RL training checkpoint/restore: the use case works with Phase 2 primitives; no additional feature needed

### Architecture Approach

The architecture follows a layered model: the Python `FirecrackerSandboxService` delegates to a Go runtime daemon that uses `firecracker-go-sdk` to control one Firecracker process per VM via a per-VM Unix socket. Each VM has a dedicated TAP device for networking and a vsock UDS path for host-to-guest execd communication. Snapshots are stored as paired `vmstate` and `mem` files on local FS (Phase 1-2) or S3/OSS (Phase 3). The `SnapshotStore` is defined as a Go interface from the start so the Phase 3 S3 implementation is a drop-in. The cold-boot handler chain in `firecracker-go-sdk` branches cleanly into a snapshot restore path by swapping `LoadSnapshotHandler` for `CreateMachineHandler`, keeping both paths in the same VM creation code path.

**Major components:**
1. `FirecrackerSandboxService` (runtime.go) — implements `SandboxService`; orchestrates all other components; the Python server's sole integration point
2. `firecracker-go-sdk Machine` (machine.go) — wraps per-VM Firecracker process via Unix socket; handler chain handles cold boot vs restore
3. Network layer (network.go) — TAP device provisioning via netlink; iptables NAT; one TAP per VM; requires CAP_NET_ADMIN
4. vsock layer (vsock.go) — host-side UDS proxy translating SDK TCP calls to Firecracker's `CONNECT PORT\n` handshake
5. Snapshot Manager (snapshot.go + snapshot_store.go) — pause → create → resume sequencing; `SnapshotStore` interface with local FS and S3 implementations
6. State machine (state.go) — Running / Paused / Terminated with refcount-aware transitions; Paused state is new to OpenSandbox
7. Pool Manager (pool.go, Phase 3) — maintains N pre-warmed VMs restored from template snapshot; fills pool asynchronously
8. Jailer config (jailer.go) — constructs `JailerCfg`; manages chroot structure where all socket, memory, and rootfs paths must live

**Recommended file layout:**
```
server/runtimes/firecracker/
  runtime.go, machine.go, network.go, vsock.go
  rootfs.go, snapshot.go, snapshot_store.go
  pool.go, jailer.go, state.go, config.go
components/execd/vsock_transport.go  (AF_VSOCK listen mode alongside existing TCP)
```

### Critical Pitfalls

1. **Version-coupled snapshot corruption (silently succeeds)** — Firecracker binary version and host kernel version must be stored in every snapshot's metadata; enforce a hard gate at restore time that rejects version mismatches before touching VM state. Treat Firecracker upgrades as breaking migrations: drain pool, invalidate all snapshots, rebuild. Address in Phase 2 snapshot creation path — never retrofit.

2. **Entropy collision across cloned instances** — Every snapshot restore shares the original RNG state, boot ID, and ASLR layout. After every restore, before allowing customer code to run, issue `RNDCLEARPOOL` + `RNDADDENTROPY` + `RNDRESEEDCRNG` via guest agent init. Delete `/var/lib/systemd/random-seed` from template VMs before taking the base snapshot. This is a security requirement, not optional hardening.

3. **Memory file deleted under a live VM (silent guest corruption)** — Firecracker uses `MAP_PRIVATE` to map the memory file at restore time; the file must remain immutable on disk for the entire VM lifetime. Implement reference counting in the snapshot storage layer. GC must decrement refcount only on VM termination, not on snapshot expiry. No exceptions — the first violation produces a non-deterministic, untraceable corruption.

4. **vsock UDS path collision on restore** — Use `vsock_override` in every `LoadSnapshot` call to assign a fresh path per VM instance. Track all live socket paths in a registry; reclaim stale files explicitly on VM termination. Path scheme: `{jailer-root}/{sandbox-id}/v.sock`. Get this right in Phase 1 — the snapshot path must be consistent with the live VM path scheme.

5. **Clock skew causing application-level failures after resume** — The guest TSC jumps forward by pause duration. Applications using monotonic clocks for timeouts may expire all timers at once on resume. After restore, execd must run a clock stabilization step and signal "ready" to the host only after NTP has had a moment to stabilize. The restore API must not return 200 until this health-check ping succeeds over vsock.

6. **Diff snapshot corruption on VMs with > 3 GiB RAM** — A confirmed Firecracker bug (PR #5705, fixed in v1.15.x) silently corrupted diff snapshots on x86 VMs using multiple memory slots (i.e., > 3 GiB). Pin to v1.15.0 and include a write-snapshot-restore-verify CI test at 4 GiB and 8 GiB configurations before shipping diff snapshots.

---

## Implications for Roadmap

Based on combined research, the phase structure is unambiguous because each phase is a hard dependency of the next. The feature dependency graph and architecture build order converge on the same three-phase sequence.

### Phase 1: VM Runtime Foundation

**Rationale:** Nothing else is possible without a working VM lifecycle. This phase establishes all the primitives the snapshot system depends on: VM create/start/stop, TAP networking, vsock execd transport, and the jailer isolation model. The vsock UDS path scheme must be right here — it cannot be changed without invalidating the snapshot path conventions established later.

**Delivers:** A working Firecracker sandbox runtime that passes the same integration tests as the Docker/K8s runtimes. Cold-boot VMs, exec code via vsock, return results, terminate cleanly.

**Addresses (from FEATURES.md):**
- VM lifecycle (create, start, stop, destroy) with Jailer
- vsock-based execd connectivity
- ext4 rootfs provisioning from OCI images
- Network connectivity via TAP + egress proxy
- CPU template configuration (T2/T2S) — set now, cannot retrofit

**Avoids (from PITFALLS.md):**
- vsock UDS path collision: establish path scheme with generation counter from day one
- TAP device leak on crash: verify TAP cleanup in abnormal termination tests
- Jailer seccomp in dev: run Jailer from first integration test, not just in production
- cgroups v1 on host: pre-flight check for cgroups v2 before enabling any snapshot work

**Architecture build order:** rootfs.go → network.go → machine.go → vsock.go → execd vsock transport → runtime.go → jailer.go

**Research flag:** Needs deeper research during planning. Jailer chroot path conventions, TAP naming scheme under network namespaces, and vsock CID allocation strategy all have non-obvious production constraints. Recommend a research-phase pass before Phase 1 task breakdown.

---

### Phase 2: Snapshot and Restore

**Rationale:** This is the core value proposition. Phase 1 must be complete and stable before this phase begins — snapshot/restore adds parallel failure modes (version mismatch, entropy collision, memory file lifecycle) that are impossible to debug on top of an unstable VM lifecycle. The pause/resume state machine extension and the `SnapshotStore` interface must be designed to their final shape here; the Phase 3 S3 backend is a second implementation of the same interface.

**Delivers:** Full snapshot create and restore API. Paused sandbox state. TTL-triggered auto-pause. Snapshot list and delete with reference-counted GC. Diff snapshot support (with CI validation at > 3 GiB). Restore API that returns 200 only after execd health-check confirms guest is responsive.

**Addresses (from FEATURES.md):**
- Full snapshot create and restore
- Pause/resume state machine (OSEP-0008)
- Sandbox TTL with pause-instead-of-delete (OSEP-0009)
- Snapshot list and delete
- Diff snapshots with dirty page tracking (gated by CI validation)

**Avoids (from PITFALLS.md):**
- Version-coupled snapshot corruption: snapshot metadata must include FC version, host kernel, CPU template, rootfs digest on creation
- Entropy collision: entropy reseeding in guest agent init path, verified by integration test
- Memory file mutation under live VM: reference counting in snapshot_store.go from day one
- Clock skew: execd clock stabilization step, restore API health-check gate
- TCP connections stale post-resume: execd treats resume as connection-reset event; client SDK retries
- Diff snapshot corruption > 3 GiB: CI write-snapshot-restore-verify at 4 GiB and 8 GiB

**Architecture build order:** state.go → snapshot.go → snapshot_store.go (local FS) → Lifecycle API extension → diff snapshot flag

**Research flag:** Standard patterns with high-quality official documentation. The Firecracker snapshot-support.md, versioning.md, random-for-clones.md, and handling-page-faults-on-snapshot-resume.md cover the implementation in detail. No additional research phase needed — follow official docs precisely.

---

### Phase 3: Pool Optimization and Multi-Node

**Rationale:** The warm pool is the feature that makes Firecracker economically competitive with Docker for high-concurrency workloads. It only makes sense after Phase 2 is proven reliable — a pool built on flaky snapshot/restore multiplies failures by the pool size. The S3/OSS backend enables cross-node restore and is a prerequisite for the pool to be useful across a fleet.

**Delivers:** Template VM warm pool achieving ~5-10ms sandbox creation time (vs ~28ms from cold restore, vs ~1s cold boot). S3/OSS snapshot storage backend. Snapshot retention policies with background janitor. Diff snapshot pipeline as the primary storage format for warm pool snapshots.

**Addresses (from FEATURES.md):**
- Template VM warm pool (~5-10ms creation)
- Object storage backend (S3/OSS)
- Snapshot retention policies
- Diff snapshots as the production storage format

**Avoids (from PITFALLS.md):**
- Pool entropy reuse: load test 100 concurrent sandbox creates from pool; assert all UUIDs unique
- Object storage restore latency: pre-download and cache snapshots locally; S3 is for mobility/backup, not hot path
- Diff snapshot restore chain: implement diff snapshot creation but use full snapshots for warm pool until merge tooling is validated
- Pool state drift: take template snapshot only after execd reports healthy (not at kernel boot)

**Architecture build order:** pool.go → S3/OSS SnapshotStore implementation → retention policy engine

**Research flag:** Needs deeper research during planning. The warm pool implementation (CoW memory semantics, pool fill strategy, entropy injection per VM, concurrent restore concurrency limits) is complex enough to warrant a research-phase pass. The S3 multipart upload pattern for large memory files and the diff-to-full merge tooling (`snapshot-editor`) also need validation before implementation.

---

### Phase Ordering Rationale

- **Dependency-driven:** The feature dependency graph is a strict chain. Snapshot cannot be created without a paused VM, which cannot exist without a running VM, which cannot exist without TAP networking and rootfs. The architecture build order in ARCHITECTURE.md and the MVP recommendation in FEATURES.md converge independently on the same sequence.
- **Risk front-loading:** The three "never retrofit" pitfalls (version metadata, reference counting, vsock path scheme) are all addressed in Phases 1-2. Phase 3 inherits a correct foundation rather than accumulating debt.
- **Interface stability:** `SnapshotStore` is defined as a Go interface in Phase 2 with a local FS implementation. Phase 3 adds the S3 implementation without modifying the interface or its callers. This is the key architectural seam that makes Phase 3 independently deployable.
- **Jailer from day one:** Running without Jailer in Phase 1 development is acceptable for fast iteration, but the first integration test must use Jailer to catch seccomp filter violations early. Enabling Jailer for the first time in Phase 3 would surface seccomp issues on top of pool complexity.

### Research Flags

Phases needing deeper research during planning:
- **Phase 1:** Jailer chroot path conventions for snapshot + vsock + memory files, TAP naming under network namespaces, vsock CID allocation strategy for concurrent sandbox creation, execd AF_VSOCK transport design (alongside TCP vs replacing TCP)
- **Phase 3:** Warm pool fill strategy and concurrency limits, entropy injection protocol between host and guest agent, diff snapshot merge tooling (`snapshot-editor`) production readiness, S3 multipart upload for large memory files with resume-on-failure

Phases with standard patterns (research-phase optional):
- **Phase 2:** Firecracker official documentation covers the snapshot API, versioning constraints, entropy reseeding protocol, and page-fault handler design in sufficient detail. Implementation should follow the official docs directly without additional research.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Core libraries verified against official pkg.go.dev, Firecracker release notes, and SDK go.mod. Version pins are specific and sourced. |
| Features | HIGH | Firecracker API behavior verified against official docs. Feature dependency graph derived from confirmed API constraints (e.g., pause required before snapshot). |
| Architecture | HIGH | Component boundaries and data flows derived from official Firecracker design docs, vsock docs, and snapshot-support docs. OpenSandbox integration points verified against architecture.md. |
| Pitfalls | HIGH | All critical pitfalls sourced from official Firecracker documentation (versioning.md, random-for-clones.md, handling-page-faults.md). Corroborated by production post-mortems. |

**Overall confidence:** HIGH

### Gaps to Address

- **OpenSandbox server integration points (MEDIUM confidence):** The Python `SandboxService` ABC pattern and factory dispatch were confirmed via DeepWiki inference from source, not direct API reading. During Phase 1 planning, read `server/runtimes/` source directly to confirm hook points and discover any undocumented conventions (e.g., how the server tracks sandbox state, where TTL logic lives).
- **execd vsock transport design:** The research confirms that execd must support AF_VSOCK, but the exact change (add vsock alongside TCP vs replace TCP) depends on whether non-Firecracker runtimes must continue using TCP. Read `components/execd/` source before Phase 1 task breakdown.
- **Diff snapshot merge tooling maturity:** The `snapshot-editor` tool ships with Firecracker but its production readiness for the warm pool restore chain has not been validated. Do not commit to diff snapshots as the warm pool format in Phase 3 until this is tested at target memory sizes.
- **UFFD vs File backend threshold:** Research recommends the File backend (full snapshots) for Phases 1-2. The crossover point where UFFD lazy restore becomes worth its complexity (around > 1 GiB RAM with latency targets < 100ms) should be validated empirically in Phase 3, not assumed.

---

## Sources

### Primary (HIGH confidence)
- `github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md` — Full vs diff snapshots, API sequences, memory file semantics, networking after restore
- `github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/versioning.md` — Snapshot format versioning, cross-version incompatibility
- `github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/random-for-clones.md` — Entropy reseeding protocol for cloned VMs
- `github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/handling-page-faults-on-snapshot-resume.md` — UFFD page fault handler design and failure modes
- `github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md` — vsock UDS bridge, CID addressing, `vsock_override`
- `github.com/firecracker-microvm/firecracker/blob/main/docs/jailer.md` — chroot structure, seccomp filter, cgroups, file path conventions
- `github.com/firecracker-microvm/firecracker/blob/main/docs/design.md` — VMM thread model, device model, one-process-per-VM constraint
- `pkg.go.dev/github.com/firecracker-microvm/firecracker-go-sdk` — Machine struct, handler chain, Config, snapshot methods, Go 1.24 requirement
- `github.com/firecracker-microvm/firecracker/releases` — v1.15.0 as latest stable (March 2025)
- `github.com/alibaba/OpenSandbox/docs/architecture.md` — SandboxService interface, runtime backend contract, execd injection

### Secondary (MEDIUM confidence)
- Marc Brooker, "Seven Years of Firecracker" (2025) — production usage patterns, CoW pool semantics
- ForgeVM / Dev.to: "How I built sandboxes that boot in 28ms" — warm pool implementation patterns
- CodeSandbox engineering blog: memory balloon, lz4 compression, userfaultfd in production
- DeepWiki inference from OpenSandbox source — Python/FastAPI server structure, SandboxService factory pattern

### Tertiary (LOW confidence)
- AI Code Sandbox Benchmark 2026 (superagent.sh) — competitive context only; vendor blog
- arXiv 2102.12892 "Restoring Uniqueness in MicroVM Snapshots" — corroborates official entropy docs; academic

---
*Research completed: 2026-04-04*
*Ready for roadmap: yes*

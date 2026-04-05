# Feature Landscape

**Domain:** Firecracker-based VM sandbox runtime with snapshot/restore
**Project:** OpenSandbox Firecracker Runtime Backend
**Researched:** 2026-04-04
**Overall confidence:** HIGH (Firecracker API behavior verified against official docs; ecosystem patterns verified against multiple production implementations)

---

## Table Stakes

Features users expect. Missing = product feels incomplete or unusable.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| VM lifecycle: create, start, stop, destroy | Every sandbox runtime provides this; users can't do anything without it | Medium | Requires firecracker-go-sdk + Jailer wiring; Jailer is mandatory for production |
| vsock-based execd connectivity | execd currently works over TCP; Firecracker has no tap-accessible exec channel by default — vsock is the natural replacement | Medium | vsock listen sockets survive snapshot/restore; existing open connections are closed; execd must reconnect on resume |
| ext4 rootfs provisioning from OCI images | Users expect to bring Docker images; no one hand-crafts VM disk images | Medium | Tooling exists (firecracker-rootfs-builder, Ignite approach); requires a build pipeline step |
| Guest kernel management | Firecracker requires an explicit kernel image; users must not manage this manually | Low | Pin to a known-good kernel version matching snapshot constraints; kernel = build artifact |
| Snapshot create (full) | Core value proposition of the milestone; without it no pausing, no pool warming | Medium | Requires pause → PUT /snapshot/create → resume or terminate flow |
| Snapshot restore | Same as above — create without restore is useless | Medium | PUT /snapshot/load before VM configuration; memory file must remain immutable |
| Pause / resume state machine | OSEP-0008 draft covers pause/resume; it's the user-visible API surface above the Firecracker primitives | Low | PATCH /vm state: Paused / Resumed; extend OpenSandbox sandbox state machine |
| Sandbox TTL with pause-instead-of-delete | Users expect idle sandboxes to pause rather than disappear; OSEP-0009 (Auto-Renew) sets this expectation | Low | TTL pausing: snapshot on TTL trigger, destroy live VM, restore on next access |
| Jailer isolation in production | Any sandbox system without jailer is not production-safe; competitors use it; reviewers will ask | High | Chroot, cgroup, seccomp; file hard-links into jailed root; significant operational complexity |
| Snapshot list + delete | Users and operators need to clean up snapshots; unbounded growth is a non-starter | Low | Metadata storage + filesystem or object-store deletion |
| Network connectivity from guest | Sandboxes must be able to make outbound requests (egress proxy) | Medium | Tap device + FQDN-based egress; DNS must work inside guest |
| CPU template for portability | Snapshots restored on different hardware require normalized CPUID; without this multi-node restore is undefined behavior | Low | Use T2/T2S/C3 template at snapshot creation time; document requirement |
| CID uniqueness per VM | vsock requires a unique CID per VM on the same host; collisions cause silent failures | Low | Generate CID from sandbox ID or a monotonic counter; track in-use CIDs |

---

## Differentiators

Features that set this product apart from existing OpenSandbox runtimes (Docker, Kata/gVisor).

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Template VM warm pool (~5-10ms creation) | Cold-boot Firecracker VMs take ~1-1.1s; snapshot-restore takes ~28ms; pre-warmed pools eliminate even that; E2B built their competitive moat on this | High | Boot once, snapshot, serve subsequent sandboxes from that snapshot; memory is MAP_PRIVATE CoW — each restored VM gets its own anonymous pages |
| Diff snapshots for efficient incremental saves | Full snapshots = RAM size on disk (8GB VM → 8GB file); diff snapshots capture only dirty pages; essential for any sandbox with large memory allocations | High | Requires track_dirty_pages: true; diff snapshots are developer preview in Firecracker; merge tooling needed for restore; adds restore chain complexity |
| Object storage backend (S3/OSS) for snapshots | Local storage limits snapshot sharing to a single node; object storage enables multi-node restore and true stateful mobility | High | snapshot_path + mem_file_path must be local at load time; requires download-before-restore or network block device; adds latency |
| Snapshot retention policies | Production systems accumulate snapshots; automatic cleanup by age or count avoids runaway disk use; no competitor exposes this at the API level | Low | Policy engine: max_count, max_age, min_restore_count; background janitor |
| COW rootfs overlay (read-only base + writable layer) | Single base ext4 image shared across all sandboxes of same image type; squashfs base + ext4 overlay reduces per-VM disk from ~1GB to ~50MB delta | High | Requires squashfs base + ext4 overlay setup; device mapper or overlayfs; not Firecracker-native but well-documented |
| RL training checkpoint/restore | Firecracker snapshot as RL reset-to-known-state; competitors don't market this but it's the highest-value AI use case | Medium | Snapshot at initialization, restore instead of re-running init; requires < 100ms restore for RL loops to be practical |
| Snapshot-based clone (fork semantics) | Create N identical sandboxes from one snapshot; useful for parallel evaluation, A/B test of agent prompts | Medium | Restore same snapshot multiple times with unique CIDs; security note: unique identifiers / entropy are shared — must re-seed after restore |

---

## Anti-Features

Features to explicitly NOT build. Building these would waste phase capacity, create maintenance debt, or violate upstream constraints.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Kata Containers snapshot integration | Kata wraps Firecracker but does not expose its snapshot API; the entire motivation for direct Firecracker integration is bypassing this gap | Go direct via firecracker-go-sdk; document why Kata is excluded |
| Cross-version snapshot restore | Firecracker snapshots encode binary state tied to a specific Firecracker version; restoring across versions is undefined behavior | Document the constraint; pin Firecracker version per cluster; add version check on restore |
| Cross-kernel snapshot restore | Guest kernel must match at snapshot and restore time; mismatched kernels cause guest crashes | Store kernel version in snapshot metadata; validate on load |
| True live migration | Snapshot/restore on a different host works (with CPU template matching and S3 backend); RDMA-based live migration does not | Document that "migration" means stop-snapshot-restore-start, not zero-downtime migration |
| Windows guest support | Firecracker is Linux-only; the virtio device set is Linux-centric | Out of scope by design; document clearly |
| GPU passthrough | Firecracker's minimal device model (4 devices) does not include GPU passthrough | Use Kubernetes + GPU operator if GPU is needed; separate runtime |
| Pool mode with shared-kernel user multiplexing | ForgeVM's "pool mode" puts N users in one VM with directory scoping; this trades kernel isolation for resource savings | Per-user VM is the OpenSandbox contract; kernel isolation is the product |
| In-band VM configuration API surface | Exposing raw Firecracker VMM API (/boot-source, /drives, etc.) to SDK users adds complexity with no user value | Encapsulate fully inside the runtime backend; users call OpenSandbox lifecycle APIs only |
| Snapshot encryption at rest | Valuable for compliance but adds significant key management complexity with no upstream precedent in OpenSandbox | File-system-level encryption (dm-crypt) is the operator's responsibility; out of scope for this milestone |
| Automated snapshot compression | Reduces storage cost but adds CPU overhead on snapshot creation and wall-clock latency on restore | Let operators compress via storage-tier compression (S3 server-side, ZFS); don't add in the runtime path |

---

## Feature Dependencies

```
Jailer isolation
  └── VM lifecycle (create, start, stop, destroy)
        └── vsock-based execd
              └── Snapshot create (full)
                    ├── Snapshot restore
                    │     ├── Pause / resume state machine
                    │     │     └── TTL pausing
                    │     └── Snapshot list + delete
                    │           └── Snapshot retention policies
                    └── Diff snapshots
                          └── Template VM warm pool
                                └── Snapshot-based clone (fork semantics)

ext4 rootfs provisioning
  └── COW rootfs overlay (optional optimization, separate from snapshot path)

CPU template for portability
  └── Object storage backend (multi-node restore requires CPU template to be set)

CID uniqueness per VM
  └── Snapshot-based clone (multiple VMs from one snapshot on same host)
```

**Critical path:** Jailer → VM lifecycle → vsock execd → full snapshot/restore → everything else. The warm pool is phase 3 work that only makes sense once create+restore works reliably (phase 2).

---

## MVP Recommendation

**Phase 1 (Runtime Foundation) — Prioritize:**
1. VM lifecycle (create, start, stop, destroy) with Jailer
2. ext4 rootfs provisioning from OCI images
3. vsock-based execd connectivity
4. Network connectivity + tap device + egress proxy integration
5. CPU template configuration (T2/T2S/C3) — cheap to do early, expensive to retrofit

**Phase 2 (Snapshot/Restore) — Prioritize:**
1. Full snapshot create + sandbox state machine extension (Paused state)
2. Snapshot restore with auto-resume
3. TTL pausing (snapshot on TTL trigger)
4. Snapshot list + delete
5. Local filesystem storage backend

**Phase 3 (Pool Optimization) — Prioritize:**
1. Template VM warm pool (~5-10ms creation)
2. Diff snapshots with dirty page tracking
3. Object storage backend (S3/OSS)
4. Snapshot retention policies

**Defer indefinitely:**
- COW rootfs overlay: Useful optimization but adds operational complexity; evaluate after Phase 1 stabilizes
- Snapshot-based clone: Requires entropy/uniqueness re-seeding; no upstream API contract; out of scope for initial contribution
- RL training checkpoint/restore: Use case works with Phase 2 primitives; no additional feature needed

---

## Feature Notes by Phase

### Phase 1 Watchpoints

**vsock reconnection on resume:** vsock listen sockets survive pause/resume, but any connection open at snapshot time is closed. execd must tolerate disconnection and re-accept. The execd server already listens in a loop; the client (SDK) must retry on reconnect. This is protocol design, not just networking config.

**Jailer complexity:** The jailer requires hard-linking (not copying) all resources into the chroot before starting. This means the rootfs image, kernel, and vsock socket path must all be inside the jail directory. The jailer creates `<cgroup_base>/<parent>/<id>` per VM. This is the single hardest operational detail of Phase 1.

**CID collision:** With concurrent sandbox creation, CID assignment must be atomic. Use a per-host monotonic counter or derive CID from a hash of the sandbox ID truncated to valid vsock range (3–0xFFFFFFFF, excluding 0, 1=hypervisor, 2=host).

### Phase 2 Watchpoints

**Memory file immutability:** After loading a snapshot, Firecracker holds an mmap of the memory file. The file must not be modified or deleted while any VM restored from it is running. Snapshot management must track refcounts before deleting.

**Clock skew on resume:** Guest TSC jumps on resume. NTP corrects wall-clock drift, but monotonic clocks seen by guest applications do not track correctly. Applications that use monotonic clocks for timeouts (HTTP keep-alive, database connection pools) may behave unexpectedly after resume.

**TCP connection staleness:** Remote peers will have timed out TCP connections during pause. Applications must handle reconnection. This is a user documentation concern, not a feature to build around.

### Phase 3 Watchpoints

**Diff snapshot restore chain:** Firecracker restores full snapshots only; diff snapshots require external merge. The merge tool is not production-hardened. For Phase 3, implement diff snapshot creation but use full snapshots for the warm pool until merge tooling is validated.

**Object storage restore latency:** Downloading a 512MB memory file from S3 before restore adds 1-5 seconds depending on network. For warm pool use, pre-download and cache snapshots locally. Object storage is for mobility and backup, not for the hot path.

---

## Sources

- [Firecracker Snapshot Support Docs](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md) — HIGH confidence; official docs
- [Firecracker vsock Docs](https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md) — HIGH confidence; official docs
- [Firecracker Jailer Docs](https://github.com/firecracker-microvm/firecracker/blob/main/docs/jailer.md) — HIGH confidence; official docs
- [How I built sandboxes that boot in 28ms using Firecracker snapshots](https://dev.to/adwitiya/how-i-built-sandboxes-that-boot-in-28ms-using-firecracker-snapshots-i0k) — MEDIUM confidence; production implementation writeup, single source
- [Seven Years of Firecracker](https://brooker.co.za/blog/2025/09/18/firecracker.html) — HIGH confidence; Marc Brooker (AWS, Firecracker co-author), September 2025
- [Alibaba OpenSandbox Architecture](https://northflank.com/blog/alibaba-opensandbox-architecture-use-cases) — MEDIUM confidence; third-party analysis, consistent with upstream repo
- [Daytona vs E2B in 2026](https://northflank.com/blog/daytona-vs-e2b-ai-code-execution-sandboxes) — MEDIUM confidence; competitive landscape
- [AI Code Sandbox Benchmark 2026](https://www.superagent.sh/blog/ai-code-sandbox-benchmark-2026) — LOW confidence; single vendor blog, useful for competitive context only
- [Incremental diff snapshots GitHub issue](https://github.com/firecracker-microvm/firecracker/issues/2142) — HIGH confidence; official issue tracker
- [Space Efficient Filesystems for Firecracker](https://parandrus.dev/devicemapper/) — MEDIUM confidence; COW overlay patterns
- [Firecracker rootfs and kernel setup](https://github.com/firecracker-microvm/firecracker/blob/main/docs/rootfs-and-kernel-setup.md) — HIGH confidence; official docs

# Pitfalls Research

**Domain:** Firecracker runtime backend with snapshot/restore for OpenSandbox
**Researched:** 2026-04-04
**Confidence:** HIGH (primary sources: official Firecracker docs, versioning.md, snapshot-support.md, random-for-clones.md, vsock.md, and verified community post-mortems)

---

## Critical Pitfalls

### Pitfall 1: Snapshot/Restore Silently Succeeds Across Incompatible Versions

**What goes wrong:**
Attempting to restore a snapshot with a different Firecracker binary version or on a different host kernel version may appear to succeed but produces a VM with undefined behavior — corrupted memory state, incorrect device emulation, or silent crashes under load. Firecracker checks format version compatibility but the range of "compatible" is narrow: every change to microVM state description bumps the major version, and cross-kernel KVM state has different semantics.

**Why it happens:**
Operators upgrade Firecracker for security fixes without realizing that all existing snapshots are invalidated. Rolling deployments mean new and old binary versions coexist briefly, and some restores land on the wrong version. Similarly, cloud hosts silently receive kernel security patches, making the restore host's kernel differ from the snapshot host's.

**How to avoid:**
- Store Firecracker binary version and host kernel version as metadata alongside every snapshot file.
- Enforce a version gate at restore time: read the metadata, reject mismatches before touching the VM state file.
- Pin Firecracker version in the runtime binary (embed it as a build constant); alert on mismatch rather than attempting a potentially corrupt restore.
- Treat Firecracker upgrades as a breaking migration event: invalidate all snapshots, drain the pool, rebuild template VMs.

**Warning signs:**
- VMs restored from snapshot intermittently crash or hang after a host OS update.
- Kernel version divergence between nodes in a fleet after rolling patches.
- No version metadata field in snapshot storage schema.

**Phase to address:**
Phase 2 (Snapshot/Restore API) — version metadata and gate logic must be in the initial snapshot creation path, not retrofitted.

---

### Pitfall 2: Entropy / UUID Collision Across Cloned Snapshot Instances

**What goes wrong:**
When the same snapshot is restored more than once (pool warm-up, VM cloning, retry after failure), all instances start with identical RNG state, `/proc/sys/kernel/random/boot_id`, and any application-level entropy seeds captured at snapshot time. Consequences: duplicate UUIDs in multi-tenant logs, reused nonces in TLS handshakes, predictable ASLR layouts making VM escapes easier.

**Why it happens:**
Developers focus on functional correctness (does the VM boot and run code?) and miss the security invariant that "resume once" vs "resume N times" have completely different guarantees. The MAP_PRIVATE copy-on-write memory model makes cloning trivially easy, which amplifies the risk.

**How to avoid:**
- After every restore, before allowing customer code to run: issue `RNDCLEARPOOL`, `RNDADDENTROPY` (with fresh host bytes), and `RNDRESEEDCRNG` ioctls in the guest agent init path. This is required even on kernels >= 5.18 where VMGenID triggers automatic reseeding — a race window exists between vCPU resumption and CSPRNG reinitialization.
- Delete `/var/lib/systemd/random-seed` from the template VM before taking the base snapshot.
- For pool mode (Phase 3): assign each VM a unique vsock CID immediately on restore; inject a host-provided entropy seed via the guest agent's first message before unblocking the sandbox.
- Never cache random values, UUIDs, or auth tokens in template VM state — generate them post-resume only.

**Warning signs:**
- Boot ID is identical across two concurrently running VMs (`cat /proc/sys/kernel/random/boot_id`).
- Duplicate request IDs appearing in logs across different sandbox instances.
- Template VM has `random-seed` file older than the snapshot creation timestamp.

**Phase to address:**
Phase 2 (basic restore) — entropy reseeding in guest agent init. Phase 3 (pool) — amplifies the risk because cloning is the entire mechanism.

---

### Pitfall 3: vsock UDS Path Collision When Restoring Multiple Snapshots on the Same Host

**What goes wrong:**
Each Firecracker VM's vsock device maps to a Unix Domain Socket path on the host. When the same snapshot is restored multiple times, or when two VMs are created sequentially with the same ID, their UDS paths collide. The second VM fails to bind the socket silently — connection attempts from the host see "connection refused" or stale socket files. With the Jailer, the socket must live inside the jail chroot, making path management even more constraint-sensitive.

**Why it happens:**
Developers use VM ID as the UDS path suffix (e.g., `/var/run/fc-{id}.sock`) without enforcing host-wide uniqueness across the full socket lifecycle. A crashed VM leaves a stale socket file. On restore, the new instance cannot bind the same path.

**How to avoid:**
- Use the `vsock_override` parameter at restore time (introduced for exactly this scenario) to assign a fresh UDS path distinct from the snapshot's original path.
- Enforce a socket path registry: track all live UDS paths in the runtime controller; reclaim stale paths explicitly on VM termination.
- Combine VM ID + restore generation counter + host timestamp as the socket path suffix, not just VM ID.
- In Jailer mode: socket, page fault handler UDS, and memory file must all live inside the jail root; factor this into the path scheme.

**Warning signs:**
- `bind: address already in use` errors in Firecracker logs on VM start.
- execd unreachable immediately after restore with no other errors.
- Stale `.sock` files in the runtime working directory accumulating after VM crashes.

**Phase to address:**
Phase 1 (VM lifecycle) — socket path scheme must be correct before snapshot work begins. Phase 2 (restore) — `vsock_override` usage must be explicit in the restore path.

---

### Pitfall 4: Memory File Mutated After Restore — Silent Guest Corruption

**What goes wrong:**
Firecracker uses `MAP_PRIVATE` to map the snapshot memory file at restore time: reads come from the file, writes go to anonymous copy-on-write memory. This means the original memory file must remain on disk, unmodified, for the entire lifetime of the restored VM. Any external process that modifies, truncates, moves, or deletes the file while the VM is running causes silent guest memory corruption — the guest may appear to run but produces wrong results or crashes non-deterministically.

**Why it happens:**
Storage cleanup jobs, snapshot rotation policies, or a second restore operation that atomically replaces the file all violate the immutability contract without any runtime error from Firecracker.

**How to avoid:**
- Treat the memory file as immutable once it backs a live VM — implement a reference-count-like mechanism: the snapshot storage layer must not delete or replace a memory file while any VM holds a reference to it.
- Use hard links or explicit refcounting in the snapshot storage backend (Phase 2: local FS; Phase 3: S3/OSS object versioning) so GC cannot reclaim a file under a running VM.
- For diff snapshots (Phase 2+): the diff layer depends on the base layer — GC must follow the dependency chain from leaf to root before reclaiming anything.

**Warning signs:**
- Non-deterministic guest panics or "impossible" memory errors that correlate with snapshot GC operations.
- Two restore operations issued for the same memory file within a short window.
- No reference counting in the snapshot storage schema.

**Phase to address:**
Phase 2 (snapshot storage) — the reference counting contract must be in the initial local-FS storage design, not added after the first corruption incident.

---

### Pitfall 5: Clock Skew Causing Application-Level Failures After Resume

**What goes wrong:**
The guest's TSC (Time Stamp Counter) jumps forward by the duration the VM was paused. NTP will eventually correct wall-clock time, but:
1. Applications using monotonic clocks for rate limiting, lock timeouts, or TTL expiry see the TSC jump immediately and may expire all timers at once upon resume.
2. `MSR_IA32_TSC_CTRL` values are not preserved across snapshot/restore without an active CPU template — on x86 without a template, this MSR is overwritten.
3. TSC is marked unstable by the kernel watchdog after a large skew, causing the guest to fall back to a slower clocksource (`hpet` or `acpi_pm`), which degrades performance.

**Why it happens:**
Developers test snapshot/restore with short pause durations (< 1 second) and never observe the timer expiry avalanche that occurs after a multi-minute pause in production.

**How to avoid:**
- Design execd and all sandbox-internal services to use wall-clock time from NTP, not monotonic intervals, for anything that must survive a pause.
- After resume, run a clock stabilization step in the guest agent: sleep briefly to allow NTP sync, then signal "ready" to the host.
- Document to sandbox users that TCP connections and in-flight operations will require reconnection after a pause/resume cycle — do not hide this from the API surface.
- Use a CPU template (T2 or T2S on Intel) to normalize TSC behavior across hosts in the fleet.

**Warning signs:**
- Applications hanging or expiring all timeouts at once immediately after resume.
- `dmesg` showing "Marking clocksource 'tsc' as unstable" after restore.
- Tests only using sub-second pause durations.

**Phase to address:**
Phase 2 (snapshot/restore) — clock stabilization in the resume path and documentation of reconnect semantics for callers.

---

### Pitfall 6: TCP Connections Stale After Resume — No Guest-Side Recovery

**What goes wrong:**
When a VM is paused, remote TCP peers begin their own timeout timers. If the pause duration exceeds the peer's keepalive or connection timeout, the peer closes the connection. Upon resume, the guest still holds socket state for connections the peer considers dead. Applications that do not detect and reconnect will silently send data into the void or block indefinitely on reads.

**Why it happens:**
This is expected Firecracker behavior — it is documented that "network connectivity is not guaranteed after resume" — but implementations often assume the execd HTTP server and active code execution connections will survive a pause transparently.

**How to avoid:**
- The execd agent must treat resume as a restart event for all connection state: close all active HTTP connections, force reconnection of any persistent clients.
- Expose a `POST /sandboxes/{id}/restore` endpoint that returns only after the execd agent has confirmed its internal state is consistent (via a health-check ping over vsock after resume).
- For code execution streaming (SSE): the client must detect connection drop and reconnect; the server must buffer or replay the final execution result for reconnecting clients.
- TCP keepalive on all host-side vsock connections to detect stale sessions quickly.

**Warning signs:**
- `restore` API call succeeds but subsequent `execute` call hangs indefinitely.
- execd logs showing responses to requests that never arrived (requests from before pause replayed by a confused socket state).
- No health-check step in the restore completion path.

**Phase to address:**
Phase 2 (restore API) — execd health-check after resume must be a precondition for the restore API returning 200 to the caller.

---

### Pitfall 7: Diff Snapshot Corruption on x86 VMs with > 3 GiB Memory

**What goes wrong:**
A confirmed Firecracker bug (fixed in PR #5705) caused diff (incremental) snapshots to silently corrupt the memory files of VMs with multiple memory slots — which includes any x86 VM configured with more than 3 GiB of RAM. The corruption was silent: the snapshot appeared to succeed, but the restored VM had wrong memory contents.

**Why it happens:**
x86 architecture places an MMIO gap at the 3.25–4 GiB range, forcing Firecracker to use multiple KVM memory slots for VMs that span or exceed 3 GiB. The bug was in the dirty-page tracking logic for multi-slot configurations.

**How to avoid:**
- Pin to a Firecracker version that includes the fix (verify via changelog).
- For any sandbox configuration allowing > 3 GiB RAM: test diff snapshot fidelity explicitly (write a known pattern, snapshot, restore, verify pattern) in CI.
- Consider defaulting to full snapshots until diff snapshot stability is validated at the target memory size.

**Warning signs:**
- Diff snapshots on VMs with > 3 GiB RAM.
- Using a Firecracker version older than the fix.
- No snapshot integrity test in the CI pipeline.

**Phase to address:**
Phase 2 (diff snapshots) — include memory size boundary in snapshot integration tests.

---

### Pitfall 8: Userfaultfd Page Fault Handler Crash Causes Firecracker to Hang Indefinitely

**What goes wrong:**
Firecracker's on-demand memory loading at restore time uses the Linux `userfaultfd` mechanism. If a custom page fault handler process crashes or exits unexpectedly while Firecracker is loading pages, Firecracker waits indefinitely for a page that will never be delivered. There is no timeout, no automatic recovery, and no error surfaced to the caller. The VM process appears alive but is frozen.

**Why it happens:**
The page fault handler is an external process — Firecracker does not own it. Most implementations do not add watchdog logic to the handler because it is assumed to be simple and stable.

**How to avoid:**
- Implement a watchdog in the runtime controller that monitors the page fault handler process and sends SIGTERM to the Firecracker process if the handler dies.
- Set a maximum restore timeout at the API layer: if the restore operation has not signaled completion within N seconds, kill the VM and return an error.
- In Jailer mode: the handler process, its UDS, and the memory file must all reside inside the jail. Design the chroot path layout to accommodate this from Phase 1.

**Warning signs:**
- Restore operation hangs at the "loading memory" stage with no timeout.
- No process supervision for the page fault handler.
- Firecracker logs frozen at "Loaded VMM snapshot".

**Phase to address:**
Phase 2 (snapshot restore) — timeout and handler watchdog are not optional hardening; they are required for production correctness.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Hardcode VM ID as vsock UDS path | Simple, easy to reason about | Path collision on crash/restore, impossible to run two restores of same snapshot | Never — use ID + generation counter from day one |
| Skip version metadata in snapshot files | Faster MVP | Cannot safely upgrade Firecracker; corrupt restores are untraceable | Never |
| Use cgroups v1 on host | Works without OS upgrade | High snapshot restore latency; documented as unsupported for snapshots | Only in dev/CI; never production |
| Full snapshots only, skip diff support | One code path to test | Memory file = RAM size; 8 GiB sandboxes produce 8 GiB snapshot files, making S3 storage and transfer expensive | Acceptable in Phase 2 MVP; must solve before Phase 3 multi-node |
| Skip entropy reseeding after restore | Saves ~50 ms per restore | Duplicate RNG state across pool instances; UUID and nonce collisions in multi-tenant workloads | Never |
| No reference counting on memory files | Simpler GC | Silent guest memory corruption when GC races with running VM | Never |
| Single static CPU template (T2) for all VMs | Avoids per-host negotiation | Older hosts without T2 support will fail to create VMs; limits available fleet | Acceptable if fleet is homogeneous; revisit before multi-region |

---

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| vsock host → guest | Connect to `<uds_path>` directly and write data | Connect to `<uds_path>`, send `"CONNECT PORT\n"`, read `"OK PORT\n"` before sending any data |
| vsock guest → host | Listen on `<uds_path>` for all ports | Listen on `<uds_path>_<PORT>` — Firecracker creates one socket per port number |
| Jailer + snapshots | Keep memory file outside the jail for easy access | Memory file, page fault handler UDS, and snapshot files must all reside inside the jail root |
| execd over vsock | Use TCP/HTTP directly | vsock requires `AF_VSOCK` socket family; existing HTTP client must be wrapped with a vsock dialer |
| Snapshot restore + TAP devices | Create new TAP device on restore | Restore requires the same TAP device name as the original; create it before calling LoadSnapshot |
| Block devices on restore | Use original block device path | Block device paths in the snapshot config are absolute; they must exist at restore time with the same path |
| Firecracker binary upgrade | In-place upgrade | All existing snapshots are invalidated by version bump; upgrade requires snapshot regeneration and pool drain |

---

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| cgroups v1 on the host | Snapshot restore 2-5x slower than expected | Verify `cat /sys/fs/cgroup/cgroup.controllers` exists (v2) on all nodes | Always — even one v1 node degrades pool fill rate |
| Full snapshots for large VMs | 8 GiB memory file transferred per restore on a different host | Use diff snapshots for warm pool; reserve full snapshots for cold storage | At > 2 GiB VM RAM or multi-node restore |
| Dirty page tracking + huge pages | Near-zero benefit from huge pages | Do not enable both simultaneously | Always — they are mutually exclusive optimizations |
| Eager memory load vs MAP_PRIVATE | Operator deletes memory file after restore assuming it's loaded | Memory file must persist on disk for the full VM lifetime (MAP_PRIVATE reads on page fault) | First time GC runs against a live snapshot |
| Restoring large snapshots without userfaultfd | 500ms+ restore time loading full 4 GiB file | Use userfaultfd on-demand loading for fast restore path | At > 1 GiB RAM |
| Fixed 2-second poll in waitForRunning | Restore appears hung for 2s even if VM is ready in 100ms | Exponential backoff from 100ms; signal readiness from guest via vsock not polling | Always — latency spikes visible in tail percentiles |

---

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Restoring same snapshot N times without entropy reseeding | Identical ASLR, UUID, and nonce state across all N instances; VM escape easier with predictable layout | RNDCLEARPOOL + RNDADDENTROPY + RNDRESEEDCRNG in guest agent on every resume, before unblocking |
| Snapshot files without authentication/encryption | Anyone with storage access can load a sandbox's full memory state | Encrypt memory and state files at rest; sign snapshot manifests; enforce per-tenant access control |
| Not invalidating snapshots after security patch to rootfs | Patched host but restored VM runs pre-patch kernel with known vulnerabilities | Track rootfs image digest and kernel version in snapshot metadata; reject stale image versions |
| Sharing a pool across security boundaries (multi-tenant without isolation) | Pool VMs share kernel exposure; one exploit affects all tenants queued behind same base snapshot | One pool per security tier; refresh pool after security events; per-user UID namespacing inside VM |
| Skipping seccomp in dev and only enabling in production | Seccomp filter violations are silent until production; Firecracker can panic → `mremap` syscall → seccomp violation | Run with Jailer + seccomp from the very first integration test |

---

## "Looks Done But Isn't" Checklist

- [ ] **Snapshot creation:** Often missing version metadata (FC binary version, host kernel, CPU template, rootfs digest) — verify snapshot manifest includes all four fields before Phase 2 ships.
- [ ] **Restore API returning 200:** Often returns before execd is responsive — verify restore completion path includes a vsock health-check ping with a 5-second timeout.
- [ ] **vsock communication:** Often works in happy path but silently fails on restore if UDS path not updated — verify `vsock_override` is used on every restore, not just clone operations.
- [ ] **Entropy reseeding:** Often present in documentation but not wired into guest agent init — verify by checking two concurrent sandbox instances return different UUIDs from `cat /proc/sys/kernel/random/uuid`.
- [ ] **Memory file lifecycle:** Often GC'd by storage cleanup before VM terminates — verify reference count increments on restore and decrements only on VM termination.
- [ ] **TAP device cleanup:** Often leaked on abnormal VM termination — verify TAP device list on host shrinks after VM crash, not just graceful stop.
- [ ] **Diff snapshot integrity:** Often untested at > 3 GiB — verify with a write-snapshot-restore-verify cycle at 4 GiB and 8 GiB RAM configurations.
- [ ] **Pool state drift:** Template VM snapshot taken too early in boot → crashes on restore — verify template snapshot is taken after execd reports healthy, not at kernel boot.

---

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Version mismatch causes corrupt restore | HIGH | Stop all restores; identify FC binary version on each host; drain pool; rebuild template snapshots with pinned version; validate with integrity test before re-enabling |
| Entropy collision detected in production | HIGH | Rotate all cryptographic material generated inside affected sandboxes; audit UUID uniqueness in logs; reseed all live VMs; patch guest agent to reseed on every resume |
| Memory file deleted under live VM | HIGH | Terminate affected VM (already corrupt); restore from a previous clean snapshot; audit GC logic to add reference counting; test before re-enabling GC |
| UDS path collision on restore | MEDIUM | Kill zombie socket file (`rm <path>`); restart Firecracker for that VM with correct UDS path via `vsock_override`; add path uniqueness enforcement to the controller |
| Diff snapshot corruption (> 3 GiB bug) | MEDIUM | Switch to full snapshots at that memory tier; regenerate all diff snapshots at affected sizes; pin to fixed FC version |
| Page fault handler hang | MEDIUM | Kill Firecracker process for the affected VM; return 500 to the restore caller; add timeout + watchdog to prevent recurrence |
| TCP connections stale after resume | LOW | Already expected — document in API response; execd reconnect logic handles it; no data loss if execd is stateless |
| cgroups v1 performance degradation | LOW | Migrate host to v2 (requires OS-level change); interim: increase pool size to compensate for slower restore times |

---

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| Version incompatibility / silent corrupt restore | Phase 2 (snapshot creation API) | CI test: create snapshot on FC v1.x, attempt restore on FC v1.y, assert rejection |
| Entropy collision across clones | Phase 2 (restore path) + Phase 3 (pool) | Integration test: restore same snapshot twice, assert different `/proc/sys/kernel/random/uuid` |
| vsock UDS path collision | Phase 1 (VM lifecycle) | Test: crash VM, attempt restart with same ID, assert no "address in use" error |
| Memory file mutated under live VM | Phase 2 (snapshot storage) | Test: start GC while VM is running, assert VM remains healthy and memory file is untouched |
| Clock skew / timer expiry avalanche | Phase 2 (restore + execd health-check) | Test: pause VM for 60 seconds, resume, assert no timeout errors within first 5 seconds |
| TCP connections stale post-resume | Phase 2 (restore completion gate) | Test: open HTTP connection from host to execd, pause VM, resume, assert reconnect completes |
| Diff snapshot corruption > 3 GiB | Phase 2 (diff snapshot feature) | CI test: 4 GiB VM, write pattern, diff snapshot, restore, verify pattern unchanged |
| Userfaultfd handler hang | Phase 2 (restore infrastructure) | Test: kill page fault handler mid-restore, assert restore returns error within 10 seconds |
| Pool entropy reuse | Phase 3 (pool warm-up) | Load test: 100 concurrent sandbox creates from pool, assert all UUID values unique |
| TAP device leak on crash | Phase 1 (VM lifecycle cleanup) | Test: force-kill Firecracker process, assert TAP device removed from host |
| Cgroups v1 performance regression | Phase 1 (infrastructure validation) | Pre-flight check: assert `cgroups.controllers` exists before enabling snapshots |
| Security of snapshot files at rest | Phase 2 + Phase 3 (storage backends) | Audit: snapshot files not world-readable; S3 bucket policy enforces tenant isolation |

---

## Sources

- [Firecracker snapshot-support.md (official)](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md) — HIGH confidence
- [Firecracker versioning.md (official)](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/versioning.md) — HIGH confidence
- [Firecracker random-for-clones.md (official)](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/random-for-clones.md) — HIGH confidence
- [Firecracker vsock.md (official)](https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md) — HIGH confidence
- [Firecracker handling-page-faults-on-snapshot-resume.md (official)](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/handling-page-faults-on-snapshot-resume.md) — HIGH confidence
- [vsock CID change feature request #3344](https://github.com/firecracker-microvm/firecracker/issues/3344) — MEDIUM confidence
- [Diff snapshot on large memory discussion #2811](https://github.com/firecracker-microvm/firecracker/discussions/2811) — MEDIUM confidence
- [Building a Production-Grade Code Execution Engine with Firecracker MicroVMs (Dadwal, Medium)](https://medium.com/@abhishekdadwal/building-a-production-grade-code-execution-engine-with-firecracker-microvms-21309dadeec9) — MEDIUM confidence (post-mortem, verified against official docs)
- [How I built sandboxes that boot in 28ms using Firecracker snapshots (Dev.to)](https://dev.to/adwitiya/how-i-built-sandboxes-that-boot-in-28ms-using-firecracker-snapshots-i0k) — MEDIUM confidence (practitioner experience)
- [Restoring Uniqueness in MicroVM Snapshots (arXiv 2102.12892)](https://ar5iv.labs.arxiv.org/html/2102.12892) — MEDIUM confidence (academic, corroborates official entropy docs)

---
*Pitfalls research for: Firecracker runtime backend with snapshot/restore (OpenSandbox)*
*Researched: 2026-04-04*

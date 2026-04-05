# Requirements: OpenSandbox Firecracker Runtime with Snapshot/Restore

**Defined:** 2026-04-04
**Core Value:** Sandboxes can be paused, snapshotted, and restored with all in-memory state intact

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### VM Lifecycle

- [ ] **VMLC-01**: Firecracker VM can be created with configurable vCPUs, memory, and boot source via firecracker-go-sdk
- [ ] **VMLC-02**: Firecracker VM can be started and enters Running state
- [ ] **VMLC-03**: Firecracker VM can be stopped and resources are cleaned up (process, socket, tap device)
- [ ] **VMLC-04**: Firecracker VM runs inside Jailer with chroot, seccomp, and cgroup isolation
- [ ] **VMLC-05**: Guest kernel image is managed as a build artifact with pinned version
- [ ] **VMLC-06**: CPU template (T2/T2S/C3) is configurable per sandbox for cross-host snapshot portability

### Rootfs & Images

- [ ] **IMG-01**: ext4 rootfs image can be provisioned from an OCI container image
- [ ] **IMG-02**: Rootfs images are stored in a configurable local path
- [ ] **IMG-03**: Multiple sandbox instances can use the same base image without conflicts

### Networking

- [ ] **NET-01**: Each VM gets a dedicated TAP device for network connectivity
- [ ] **NET-02**: Outbound traffic routes through iptables NAT to host network
- [ ] **NET-03**: DNS resolution works inside the guest VM
- [ ] **NET-04**: Network integrates with OpenSandbox's FQDN-based egress proxy
- [ ] **NET-05**: TAP devices are cleaned up on VM stop/destroy, including abnormal termination

### Host-Guest Communication

- [ ] **VSOCK-01**: Host-guest communication uses vsock instead of TCP
- [ ] **VSOCK-02**: Each VM gets a unique CID (no collisions on same host)
- [ ] **VSOCK-03**: Execd agent inside guest listens on vsock port 44772
- [ ] **VSOCK-04**: Host-side connects via Unix domain socket with CONNECT handshake protocol
- [ ] **VSOCK-05**: vsock UDS paths are unique per VM instance (prevents collision on snapshot restore)

### Snapshot Create

- [ ] **SNAP-01**: Full snapshot can be created (captures VM state + full memory)
- [ ] **SNAP-02**: Diff snapshot can be created (captures only dirty pages since last snapshot, requires track_dirty_pages)
- [ ] **SNAP-03**: VM is automatically paused before snapshot and can be resumed or terminated after
- [ ] **SNAP-04**: Snapshot metadata includes Firecracker version, host kernel version, CPU template, and creation timestamp
- [ ] **SNAP-05**: Snapshot files (vmstate + memory) are stored in configurable local filesystem path

### Snapshot Restore

- [ ] **REST-01**: VM can be restored from a full snapshot with all process state intact
- [ ] **REST-02**: Restore validates Firecracker version and kernel match before loading
- [ ] **REST-03**: Restored VM gets a new unique vsock CID and UDS path (vsock_override)
- [ ] **REST-04**: Execd is reachable after restore (vsock health-check gate before returning success)
- [ ] **REST-05**: Guest entropy is reseeded after every restore (mandatory for multi-tenant security)
- [ ] **REST-06**: Memory file remains immutable while any VM restored from it is running (reference counting)

### Sandbox State Machine

- [ ] **STATE-01**: Sandbox state machine supports Paused state (Running → Paused → Running or Terminated)
- [ ] **STATE-02**: TTL timer pauses while sandbox is in Paused state
- [ ] **STATE-03**: Idle sandboxes auto-pause via snapshot on TTL trigger instead of being deleted
- [ ] **STATE-04**: Paused sandboxes auto-restore on next API access

### Snapshot Management

- [ ] **MGMT-01**: API endpoint lists available snapshots for a sandbox with metadata
- [ ] **MGMT-02**: API endpoint deletes a specific snapshot (with reference count validation)
- [ ] **MGMT-03**: Snapshot retention policies are configurable per-sandbox and globally (max count, max age)
- [ ] **MGMT-04**: Background janitor process enforces retention policies automatically

### Pool Optimization

- [ ] **POOL-01**: Template VM can be booted, configured with common dependencies, and snapshotted
- [ ] **POOL-02**: New sandboxes are created by restoring from template snapshot (~5-10ms) instead of cold boot (~1s)
- [ ] **POOL-03**: Pool pre-warms N sandboxes from template snapshot based on configurable buffer size
- [ ] **POOL-04**: Each pool-created sandbox gets unique CID, vsock UDS path, and reseeded entropy

### Multi-Node Storage

- [ ] **STOR-01**: Object storage backend (S3/OSS) can store and retrieve snapshot files
- [ ] **STOR-02**: Snapshots are downloaded to local filesystem before restore (Firecracker requires local paths)
- [ ] **STOR-03**: Storage backend is pluggable (local filesystem or object storage selected by configuration)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Advanced Features

- **ADV-01**: COW rootfs overlay (read-only squashfs base + writable ext4 layer) to reduce per-VM disk usage
- **ADV-02**: Snapshot-based clone/fork semantics (create N identical sandboxes from one snapshot)
- **ADV-03**: RL training checkpoint/restore (<100ms restore for reinforcement learning reset loops)
- **ADV-04**: UFFD lazy restore for near-zero resume latency (requires Linux kernel >= 6.1)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Kata Containers snapshot integration | Kata doesn't expose Firecracker's snapshot API — direct integration is the whole point |
| Cross-version snapshot restore | Firecracker doesn't guarantee it; undefined behavior, not worth working around |
| Cross-kernel snapshot restore | Guest kernel must match; mismatched kernels cause crashes |
| True live migration | Firecracker supports stop-snapshot-restore, not zero-downtime RDMA migration |
| Windows guest support | Firecracker is Linux-only by design |
| GPU passthrough | Firecracker's minimal device model doesn't support it |
| Shared-kernel user multiplexing | Per-user VM is the OpenSandbox isolation contract |
| Raw Firecracker API exposure | Users call OpenSandbox lifecycle APIs; VMM internals are encapsulated |
| Snapshot encryption at rest | Operator responsibility via dm-crypt or storage-tier encryption |
| Automated snapshot compression | Let operators handle via ZFS/S3 server-side compression; don't add to restore path |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| VMLC-01 | Phase 1 | Pending |
| VMLC-02 | Phase 1 | Pending |
| VMLC-03 | Phase 1 | Pending |
| VMLC-04 | Phase 1 | Pending |
| VMLC-05 | Phase 1 | Pending |
| VMLC-06 | Phase 1 | Pending |
| IMG-01 | Phase 2 | Pending |
| IMG-02 | Phase 2 | Pending |
| IMG-03 | Phase 2 | Pending |
| VSOCK-01 | Phase 3 | Pending |
| VSOCK-02 | Phase 3 | Pending |
| VSOCK-03 | Phase 3 | Pending |
| VSOCK-04 | Phase 3 | Pending |
| VSOCK-05 | Phase 3 | Pending |
| NET-01 | Phase 4 | Pending |
| NET-02 | Phase 4 | Pending |
| NET-03 | Phase 4 | Pending |
| NET-04 | Phase 4 | Pending |
| NET-05 | Phase 4 | Pending |
| SNAP-01 | Phase 5 | Pending |
| SNAP-02 | Phase 5 | Pending |
| SNAP-03 | Phase 5 | Pending |
| SNAP-04 | Phase 5 | Pending |
| SNAP-05 | Phase 5 | Pending |
| REST-01 | Phase 6 | Pending |
| REST-02 | Phase 6 | Pending |
| REST-03 | Phase 6 | Pending |
| REST-04 | Phase 6 | Pending |
| REST-05 | Phase 6 | Pending |
| REST-06 | Phase 6 | Pending |
| STATE-01 | Phase 7 | Pending |
| STATE-02 | Phase 7 | Pending |
| STATE-03 | Phase 7 | Pending |
| STATE-04 | Phase 7 | Pending |
| MGMT-01 | Phase 8 | Pending |
| MGMT-02 | Phase 8 | Pending |
| MGMT-03 | Phase 8 | Pending |
| MGMT-04 | Phase 8 | Pending |
| POOL-01 | Phase 9 | Pending |
| POOL-02 | Phase 9 | Pending |
| POOL-03 | Phase 9 | Pending |
| POOL-04 | Phase 9 | Pending |
| STOR-01 | Phase 10 | Pending |
| STOR-02 | Phase 10 | Pending |
| STOR-03 | Phase 10 | Pending |

**Coverage:**
- v1 requirements: 45 total
- Mapped to phases: 45
- Unmapped: 0 ✓

---
*Requirements defined: 2026-04-04*
*Last updated: 2026-04-04 — traceability updated for fine-grained 10-phase roadmap*

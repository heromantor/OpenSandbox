# Roadmap: OpenSandbox Firecracker Runtime with Snapshot/Restore

## Overview

This roadmap builds a Firecracker runtime backend for OpenSandbox from first principles. Phase 1 establishes the VM lifecycle and Jailer isolation so Firecracker can be used as a runtime at all. Phases 2 and 3 build the two foundational transport layers that every later phase depends on: rootfs image provisioning and vsock-based execd communication. Phase 4 wires up TAP networking and egress proxy integration to make VMs fully network-capable. Together, Phases 1-4 form the Runtime Foundation — a working Firecracker sandbox that can execute code, reach the network, and be stopped cleanly. Phases 5 and 6 add the core value proposition: snapshot creation and restore with all the correctness guarantees (version gating, entropy reseeding, reference counting) that cannot be retrofitted later. Phase 7 extends the sandbox state machine with Paused state and TTL-triggered auto-pause so idle sandboxes survive instead of being deleted. Phase 8 adds snapshot lifecycle management (list, delete, retention). Phase 9 builds the template VM warm pool that achieves ~5-10ms sandbox creation. Phase 10 adds the object storage backend for multi-node snapshot sharing.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3, ...): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

- [x] **Phase 1: VM Lifecycle and Jailer** - Firecracker VM create/start/stop/destroy with production Jailer isolation
- [x] **Phase 2: Rootfs and Image Management** - ext4 rootfs provisioning from OCI images, stored locally, shareable across instances
- [ ] **Phase 3: vsock and Execd Transport** - Host-guest vsock channel; execd listens on vsock; host proxy with CONNECT handshake
- [ ] **Phase 4: TAP Networking and Egress** - Per-VM TAP devices, iptables NAT, DNS, FQDN egress proxy integration
- [ ] **Phase 5: Snapshot Creation** - Full and diff snapshot creation with version metadata; pause/resume sequencing
- [ ] **Phase 6: Snapshot Restore** - Restore from full snapshot with version gating, entropy reseeding, vsock override, health-check gate
- [ ] **Phase 7: Sandbox State Machine and TTL** - Paused state, TTL pause-instead-of-delete, auto-restore on next access
- [ ] **Phase 8: Snapshot Management** - List, delete, retention policies, background janitor with reference-count-aware GC
- [ ] **Phase 9: Pool Optimization** - Template VM warm pool achieving ~5-10ms sandbox creation
- [ ] **Phase 10: Multi-Node Storage** - S3/OSS snapshot backend for cross-node restore and snapshot mobility

## Phase Details

### Phase 1: VM Lifecycle and Jailer
**Goal**: A Firecracker VM can be created, started, stopped, and destroyed with full Jailer production isolation
**Depends on**: Nothing (first phase)
**Requirements**: VMLC-01, VMLC-02, VMLC-03, VMLC-04, VMLC-05, VMLC-06
**Success Criteria** (what must be TRUE):
  1. A VM can be created with configurable vCPUs, memory, and a pinned kernel image, and transitions to Running state
  2. A stopped VM releases all resources (Firecracker process, socket file, jailer chroot) with no leaks
  3. The VM runs inside Jailer with chroot, seccomp filter, and cgroup isolation — verified by observing the jailed process tree
  4. A CPU template (T2, T2S, or C3) is applied at creation time and visible in Firecracker's config response
  5. The guest kernel image version is pinned in a build artifact and reproducibly fetched
**Plans:** 4 plans
Plans:
- [x] 01-01-PLAN.md — Module init, type definitions, interfaces, config validation
- [x] 01-02-PLAN.md — VM lifecycle implementation with Jailer, CPU template, cleanup
- [x] 01-03-PLAN.md — Unit tests and integration test skeleton
- [x] 01-04-PLAN.md — Gap closure: kernel fetch Makefile target with pinned SHA256 (VMLC-05)

### Phase 2: Rootfs and Image Management
**Goal**: ext4 rootfs images can be provisioned from OCI container images and shared safely across VM instances
**Depends on**: Phase 1
**Requirements**: IMG-01, IMG-02, IMG-03
**Success Criteria** (what must be TRUE):
  1. A rootfs image can be built from a named OCI image and stored in a configurable local path
  2. Multiple VMs can boot from the same base rootfs image without filesystem conflicts
  3. The provisioning path is deterministic — the same OCI image tag produces the same ext4 image
**Plans:** 3 plans
Plans:
- [x] 02-01-PLAN.md — Image subpackage foundation: config, store, reference parsing, error types
- [x] 02-02-PLAN.md — Provisioner pipeline: crane.Pull + crane.Export + tar2ext4 + atomic cache write
- [x] 02-03-PLAN.md — VMConfig.ReadOnlyRootfs wiring + integration test + Makefile targets

### Phase 3: vsock and Execd Transport
**Goal**: Host and guest communicate over vsock; execd inside the guest is reachable from the host after boot
**Depends on**: Phase 2
**Requirements**: VSOCK-01, VSOCK-02, VSOCK-03, VSOCK-04, VSOCK-05
**Success Criteria** (what must be TRUE):
  1. Execd inside the guest listens on vsock port 44772 and responds to health-check from the host
  2. Each VM gets a unique CID — two VMs running concurrently on the same host never collide
  3. The host connects to execd via a Unix domain socket with the Firecracker CONNECT handshake protocol
  4. Each VM's vsock UDS path is unique per instance, established at creation with a scheme that is consistent with the snapshot restore path
**Plans:** 3 plans
Plans:
- [x] 03-01-PLAN.md — vsock foundation: CIDAllocator, UDS path helpers, VMConfig/VM/VMResources extensions, validation
- [x] 03-02-PLAN.md — SDK VsockDevices wiring in toFirecrackerConfig + Manager CID auto-allocation
- [x] 03-03-PLAN.md — HTTP-over-vsock transport (NewVsockHTTPClient) + execd health check (WaitForExecd)

### Phase 4: TAP Networking and Egress
**Goal**: VMs have full network connectivity including DNS, outbound internet via NAT, and FQDN egress policy enforcement
**Depends on**: Phase 3
**Requirements**: NET-01, NET-02, NET-03, NET-04, NET-05
**Success Criteria** (what must be TRUE):
  1. A running VM can resolve DNS names for external hosts
  2. Outbound HTTP/S traffic from the guest reaches the internet via iptables NAT on the host
  3. FQDN-based egress policy from OpenSandbox's proxy is enforced — blocked domains are rejected inside the guest
  4. Stopping or crashing a VM removes its TAP device and iptables rules with no leftover netdev entries
**Plans**: TBD

### Phase 5: Snapshot Creation
**Goal**: A running VM's full state can be captured as a snapshot (full or diff), including version metadata required for safe restore
**Depends on**: Phase 4
**Requirements**: SNAP-01, SNAP-02, SNAP-03, SNAP-04, SNAP-05
**Success Criteria** (what must be TRUE):
  1. A full snapshot can be created — vmstate and memory files are written to the configured local path
  2. A diff snapshot can be created capturing only dirty pages since the last snapshot (track_dirty_pages enabled)
  3. The VM is automatically paused before snapshot creation and can be explicitly resumed or left paused after
  4. Snapshot metadata file records Firecracker version, host kernel version, CPU template, and creation timestamp alongside the snapshot files
**Plans**: TBD

### Phase 6: Snapshot Restore
**Goal**: A VM can be restored from a full snapshot with all process state intact; restore is safe, correct, and returns 200 only after execd confirms guest readiness
**Depends on**: Phase 5
**Requirements**: REST-01, REST-02, REST-03, REST-04, REST-05, REST-06
**Success Criteria** (what must be TRUE):
  1. A VM restored from a full snapshot has all original process state intact — a running process inside the guest before snapshot is still running after restore
  2. Restore is rejected with an error if the Firecracker binary version or kernel in the snapshot metadata does not match the current host
  3. Every restored VM gets a new unique vsock CID and UDS path via vsock_override — two VMs restored from the same snapshot can run concurrently without collision
  4. Execd is reachable via vsock after restore — the restore API does not return success until a health-check ping over vsock succeeds
  5. Guest entropy is reseeded after every restore (RNDCLEARPOOL + RNDADDENTROPY + RNDRESEEDCRNG) before any customer code runs
  6. The memory file for a snapshot remains immutable on disk for the full lifetime of any VM restored from it — reference counting prevents deletion of in-use files
**Plans**: TBD

### Phase 7: Sandbox State Machine and TTL
**Goal**: Sandboxes support a Paused state; idle sandboxes are paused instead of deleted; paused sandboxes auto-restore on next access
**Depends on**: Phase 6
**Requirements**: STATE-01, STATE-02, STATE-03, STATE-04
**Success Criteria** (what must be TRUE):
  1. A sandbox transitions Running -> Paused (via snapshot) and Paused -> Running (via restore) and both transitions are visible in the API sandbox state field
  2. A sandbox's TTL timer is suspended while the sandbox is in Paused state and resumes counting on restore
  3. A sandbox that reaches its TTL is paused via snapshot instead of being deleted, and the paused snapshot persists
  4. An API call to a paused sandbox triggers automatic restore and the caller receives a response after execd confirms readiness
**Plans**: TBD

### Phase 8: Snapshot Management
**Goal**: Operators can list, delete, and configure retention for snapshots; a background janitor enforces retention automatically
**Depends on**: Phase 7
**Requirements**: MGMT-01, MGMT-02, MGMT-03, MGMT-04
**Success Criteria** (what must be TRUE):
  1. The API returns a list of snapshots for a sandbox with metadata (type, size, creation timestamp, Firecracker version)
  2. A snapshot can be deleted via API; deletion is rejected if any VM restored from that snapshot is still running
  3. Per-sandbox and global retention policies (max count, max age) are configurable and honored
  4. A background janitor automatically deletes snapshots that violate retention policy, respecting reference counts
**Plans**: TBD

### Phase 9: Pool Optimization
**Goal**: New sandboxes are created by restoring from a pre-warmed template snapshot in ~5-10ms instead of ~1s cold boot
**Depends on**: Phase 8
**Requirements**: POOL-01, POOL-02, POOL-03, POOL-04
**Success Criteria** (what must be TRUE):
  1. A template VM can be booted, configured with common dependencies, and its snapshot captured as the pool template
  2. A sandbox created from the pool is ready (execd health-check passes) in <=10ms from the restore call
  3. The pool pre-warms N standby sandboxes from the template snapshot, where N is configurable, and refills asynchronously after each one is consumed
  4. Every pool-created sandbox has a unique CID, unique vsock UDS path, and freshly reseeded entropy — verified by creating 100 concurrent sandboxes and asserting no UUID or entropy collisions
**Plans**: TBD

### Phase 10: Multi-Node Storage
**Goal**: Snapshots can be stored in and retrieved from S3/OSS, enabling cross-node restore and snapshot mobility across a fleet
**Depends on**: Phase 9
**Requirements**: STOR-01, STOR-02, STOR-03
**Success Criteria** (what must be TRUE):
  1. A snapshot can be uploaded to and downloaded from an S3/OSS bucket
  2. A VM can be restored from a snapshot that was originally created on a different host, after download to local filesystem
  3. The storage backend is selected by configuration — local filesystem and S3/OSS are interchangeable without changing caller code
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7 -> 8 -> 9 -> 10

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. VM Lifecycle and Jailer | 4/4 | Complete | 2026-04-05 |
| 2. Rootfs and Image Management | 0/TBD | Not started | - |
| 3. vsock and Execd Transport | 0/3 | Not started | - |
| 4. TAP Networking and Egress | 0/TBD | Not started | - |
| 5. Snapshot Creation | 0/TBD | Not started | - |
| 6. Snapshot Restore | 0/TBD | Not started | - |
| 7. Sandbox State Machine and TTL | 0/TBD | Not started | - |
| 8. Snapshot Management | 0/TBD | Not started | - |
| 9. Pool Optimization | 0/TBD | Not started | - |
| 10. Multi-Node Storage | 0/TBD | Not started | - |

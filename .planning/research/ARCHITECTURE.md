# Architecture Research

**Domain:** Firecracker runtime backend with snapshot/restore for OpenSandbox
**Researched:** 2026-04-04
**Confidence:** HIGH (Firecracker internals), MEDIUM (OpenSandbox integration points)

## Standard Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Host Orchestration Layer                       │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐ │
│  │  Firecracker │  │   Snapshot   │  │   Pool Manager         │ │
│  │  Runtime Svc │  │   Manager    │  │  (template warm pool)  │ │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬─────────────┘ │
│         │                 │                       │              │
├─────────┴─────────────────┴───────────────────────┴──────────────┤
│                    VM Lifecycle Layer                             │
│  ┌───────────────────────────────────────────────────────────┐   │
│  │  firecracker-go-sdk  (Machine + Handler Chain)            │   │
│  │  - Machine.Start()   - Machine.PauseVM()                  │   │
│  │  - Machine.CreateSnapshot()  - LoadSnapshotHandler        │   │
│  │  - Machine.ResumeVM()  - Machine.Shutdown()               │   │
│  └───────────────────────┬───────────────────────────────────┘   │
│                          │ Unix socket (per-VM)                  │
├──────────────────────────┼────────────────────────────────────────┤
│                    Firecracker Process                            │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────┐            │
│  │ API Thread  │  │  VMM Thread  │  │  vCPU Thread │            │
│  │ (REST ctrl) │  │ (devices/IO) │  │  (guest exec)│            │
│  └─────────────┘  └──────┬───────┘  └──────────────┘            │
│                           │                                      │
│  ┌──────┐  ┌──────────┐  ┌┴─────────┐  ┌────────────────────┐  │
│  │ KVM  │  │ virtio-  │  │ virtio-  │  │ virtio-vsock       │  │
│  │      │  │ block    │  │ net      │  │ (host<→>guest IPC) │  │
│  └──────┘  └──────────┘  └──────────┘  └────────────────────┘  │
│               │                │                  │              │
│          ext4 rootfs      TAP device         UDS socket          │
│           (block dev)   (iptables NAT)     (on host fs)          │
├──────────────────────────────────────────────────────────────────┤
│                     Jailer (optional, prod)                      │
│  chroot + seccomp (24 syscalls) + cgroups + uid/gid drop         │
├──────────────────────────────────────────────────────────────────┤
│                    Guest VM                                       │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  Linux guest kernel + ext4 rootfs                          │  │
│  │  ┌────────────────────────────────────────────────────┐   │  │
│  │  │  execd (AF_VSOCK listener on port 44772)           │   │  │
│  │  │  - HTTP-over-vsock: exec, file ops, code interp    │   │  │
│  │  │  - Jupyter kernel session management               │   │  │
│  │  └────────────────────────────────────────────────────┘   │  │
│  └────────────────────────────────────────────────────────────┘  │
├──────────────────────────────────────────────────────────────────┤
│                  Snapshot Storage Layer                           │
│  ┌──────────────────────┐  ┌────────────────────────────────┐   │
│  │  Local FS            │  │  S3 / OSS (Phase 3)            │   │
│  │  vmstate + mem files │  │  multi-node sharing            │   │
│  └──────────────────────┘  └────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

| Component | Responsibility | Communicates With |
|-----------|----------------|-------------------|
| Firecracker Runtime Service | Implements OpenSandbox `SandboxService` contract; owns VM lifecycle | firecracker-go-sdk, Snapshot Manager, OpenSandbox lifecycle API |
| firecracker-go-sdk `Machine` | Wraps Firecracker REST API; provides handler chain for init, snapshot, restore | Firecracker process via Unix socket |
| Firecracker Process (per VM) | VMM, KVM guest execution, virtio device emulation | Guest kernel via KVM, TAP device, UDS vsock socket |
| Jailer | Applies chroot + seccomp + cgroups around Firecracker process | Host OS; wraps Firecracker binary |
| TAP Device + iptables | Per-VM L2 network interface; NAT for egress | Guest virtio-net, host network stack |
| vsock (virtio + UDS bridge) | Bidirectional IPC between host and guest; no TCP stack needed | Host daemon ↔ guest execd |
| execd (guest-side) | HTTP/SSE service: code execution, file ops, metrics; listens on vsock port | OpenSandbox SDK (via vsock transport) |
| Snapshot Manager | Creates, stores, retrieves, deletes vmstate + memory files | Firecracker Machine API, local FS, S3/OSS |
| Pool Manager (Phase 3) | Keeps N pre-warmed VMs restored from template snapshot ready | Firecracker Runtime Service |

## Recommended Project Structure

```
server/
└── runtimes/
    └── firecracker/               # New runtime backend package
        ├── runtime.go             # FirecrackerSandboxService (implements SandboxService)
        ├── machine.go             # VM lifecycle: create, start, stop, destroy
        ├── network.go             # TAP device provisioning, iptables NAT rules
        ├── vsock.go               # vsock path management, host-side proxy
        ├── rootfs.go              # ext4 image management, OCI-to-ext4 conversion
        ├── snapshot.go            # CreateSnapshot, LoadSnapshot, diff/full logic
        ├── snapshot_store.go      # Storage backends: local FS (Phase 1), S3/OSS (Phase 3)
        ├── pool.go                # Template snapshot warm pool (Phase 3)
        ├── jailer.go              # JailerConfig construction for production mode
        ├── state.go               # Sandbox state machine (Running/Paused/Terminated)
        └── config.go              # Firecracker-specific config (kernel path, CPU template, etc.)

specs/
└── sandbox-lifecycle.yml          # Add: /snapshot, /restore endpoints (extend existing spec)

components/
└── execd/                         # Existing execd — add vsock listen mode
    └── vsock_transport.go         # AF_VSOCK server alongside existing TCP listener
```

### Structure Rationale

- **runtimes/firecracker/:** Mirrors the existing Docker and Kubernetes runtime directories; keeps all Firecracker-specific code isolated behind the `SandboxService` interface
- **network.go separate from machine.go:** TAP device setup requires host privileges and has its own lifecycle (created before VM, torn down after); keeps machine.go focused on VM control
- **snapshot_store.go separate from snapshot.go:** Storage backend is swappable (local → S3/OSS) without touching snapshot creation logic
- **vsock.go on host:** The host needs to proxy incoming execd calls from TCP (SDK) to vsock (VM); this translation layer is its own concern
- **pool.go phase-gated:** Pool logic is independently deployable in Phase 3; phases 1 and 2 can import snapshot.go without pool.go

## Architectural Patterns

### Pattern 1: One Process per VM

**What:** Each Firecracker binary invocation manages exactly one microVM. The host orchestrator spawns one process per sandbox, communicating via a per-VM Unix socket.

**When to use:** Always — this is Firecracker's fundamental constraint and security model.

**Trade-offs:** Process-per-VM isolation is strong but process overhead is real. At 128MB minimum per VM, 100 concurrent sandboxes = ~12.8GB RAM committed plus overhead. Mitigated by pool mode (shared template memory via CoW).

```go
// Each Machine wraps a single Firecracker process
m, err := firecracker.NewMachine(ctx, cfg,
    firecracker.WithProcessRunner(exec.Command(firecrackerBin)),
    firecracker.WithLogger(logger),
)
```

### Pattern 2: Handler Chain for VM Initialization

**What:** firecracker-go-sdk uses a named handler list executed in order during `Start()`. Handlers configure boot source, drives, network, vsock, and optionally load a snapshot. The chain is inspectable and composable.

**When to use:** Any time VM startup needs to branch between cold boot and snapshot restore.

**Trade-offs:** Flexible but order-sensitive. The `LoadSnapshotHandler` must run before `CreateMachineHandler` for restore paths.

```go
// Cold boot handler order (default):
// StartVMMHandler → BootstrapLoggingHandler → CreateMachineHandler
// → CreateBootSourceHandler → AttachDrivesHandler
// → CreateNetworkInterfacesHandler → AddVsocksHandler → StartVMHandler

// Snapshot restore: replace CreateMachineHandler chain with LoadSnapshotHandler
m.Handlers.Prepare.Swap(firecracker.LoadSnapshotHandler)
```

### Pattern 3: vsock as execd Transport

**What:** Instead of connecting to execd via TCP (current OpenSandbox pattern), the host connects to the VM's vsock Unix domain socket and sends a `CONNECT 44772\n` handshake. Firecracker bridges this to the guest's AF_VSOCK listener.

**When to use:** Required for Firecracker — guest has no routable IP without additional setup; vsock is the natural, lower-latency channel.

**Trade-offs:** Requires execd to support AF_VSOCK in addition to or instead of TCP. Host-to-guest direction requires Firecracker's unusual UDS-based initiation protocol (not a standard socket connect).

```
Host connection flow:
  1. Connect to /var/lib/firecracker/{id}/v.sock (Unix socket)
  2. Send: "CONNECT 44772\n"
  3. Receive: "OK 12345\n" (ephemeral port assigned)
  4. Now bidirectional — behaves like a TCP connection to execd

Guest connection flow (guest initiates):
  Guest connects AF_VSOCK to CID=2, PORT=N
  Firecracker forwards to /var/lib/firecracker/{id}/v.sock_N on host
```

### Pattern 4: Pause → Snapshot → Resume Sequence

**What:** Firecracker requires the VM to be paused before snapshotting. The sequence is: PATCH /vm (state=Paused) → PUT /snapshot/create → PATCH /vm (state=Resumed). For diff snapshots, `track_dirty_pages: true` must be set at boot.

**When to use:** All snapshot creation paths, both user-initiated (`POST /sandboxes/{id}/snapshot`) and TTL-based auto-pause.

**Trade-offs:** Pause introduces a brief service interruption. Typical pause time is <100ms; snapshot write time depends on memory size (8GB → ~10s for full). Diff snapshots write only dirty pages (much faster for idle VMs).

```
Snapshot API call sequence:
  PATCH /vm  {"state": "Paused"}
  PUT /snapshot/create  {"mem_file_path": "...", "snapshot_path": "...", "snapshot_type": "Full|Diff"}
  PATCH /vm  {"state": "Resumed"}   ← only if keeping VM alive

Restore API call sequence (new Firecracker process):
  PUT /snapshot/load  {"mem_file_path": "...", "snapshot_path": "...", "enable_diff_snapshots": false}
  PATCH /vm  {"state": "Resumed"}
```

### Pattern 5: TAP Device per VM + iptables NAT

**What:** Each VM gets a dedicated TAP interface (`tap{id}`) on the host. iptables rules in the POSTROUTING and FORWARD chains provide NAT for egress. For FQDN-based egress control (OpenSandbox's egress proxy), traffic is routed through the existing egress proxy.

**When to use:** Every VM needs network — this is the only Firecracker networking model for production (no container networking).

**Trade-offs:** TAP device names must be unique per host. Network namespace isolation (one netns per VM) is recommended for production to prevent interface naming collisions and improve security. Requires CAP_NET_ADMIN on the host orchestrator.

```bash
# Per-VM setup
ip tuntap add tap{id} mode tap
ip addr add 172.16.{n}.1/30 dev tap{id}
ip link set tap{id} up
iptables -t nat -A POSTROUTING -o eth0 -s 172.16.{n}.0/30 -j MASQUERADE
iptables -A FORWARD -i tap{id} -j ACCEPT
```

## Data Flow

### VM Creation Flow (Cold Boot)

```
POST /sandboxes (OpenSandbox Lifecycle API)
    ↓
FirecrackerSandboxService.CreateSandbox()
    ↓
network.ProvisionTAP(sandboxID)   →  TAP device + iptables rules
    ↓
rootfs.PrepareImage(image)         →  ext4 block device mounted
    ↓
firecracker.NewMachine(cfg)        →  Config: kernel, drives, tap, vsock
    ↓
machine.Start(ctx)                 →  Spawns Firecracker process (or jailer)
    ↓ (handler chain)
StartVMMHandler → CreateMachineHandler → AttachDrivesHandler
→ CreateNetworkInterfacesHandler → AddVsocksHandler → StartVMHandler
    ↓
state.WaitForExecd(vsockPath)      →  Connects via UDS, sends CONNECT 44772
    ↓
sandbox state = Running
    ↓
POST /sandboxes/{id}/endpoints/{port} returns vsock proxy address
    ↓
SDK ExecdClient connects through vsock proxy
```

### Snapshot Creation Flow

```
POST /sandboxes/{id}/snapshot (new API endpoint)
    ↓
SnapshotManager.Create(sandboxID, type=Full|Diff)
    ↓
machine.PauseVM(ctx)               →  PATCH /vm {"state": "Paused"}
    ↓
machine.CreateSnapshot(ctx, memPath, statePath)
    ↓  (blocks until files written)
snapshotStore.Store(sandboxID, metadata)
    ↓
machine.ResumeVM(ctx)              →  PATCH /vm {"state": "Resumed"}
    ↓
Returns: snapshot ID, size, type
```

### Snapshot Restore Flow (New VM from snapshot)

```
POST /sandboxes/{id}/restore (new API endpoint)
    ↓
SnapshotManager.Load(snapshotID)   →  Retrieves vmstate + mem file paths
    ↓
network.ProvisionTAP(newSandboxID) →  New TAP device (vsock UDS path changes too)
    ↓
firecracker.NewMachine(cfg,
    WithSnapshot(memPath, statePath),
    WithVsockOverride(newUDSPath))  →  vsock_override prevents UDS collision
    ↓
machine.Start(ctx)
    ↓ (handler chain — LoadSnapshotHandler instead of boot handlers)
StartVMMHandler → LoadSnapshotHandler → CreateNetworkInterfacesHandler
    ↓
machine.ResumeVM(ctx)
    ↓
state.WaitForExecd(newVsockPath)   →  vsock listen sockets survive restore
    ↓
sandbox state = Running
```

### TTL-Based Auto-Pause Flow

```
Sandbox TTL expires (no OSEP-0009 renewal triggered)
    ↓
state machine: Running → Pausing
    ↓
SnapshotManager.Create(sandboxID, type=Diff)   ← incremental if dirty tracking on
    ↓
machine.Shutdown() or machine.StopVMM()
    ↓
network.ReleaseTAP(sandboxID)      →  TAP torn down, iptables rules removed
    ↓
sandbox state = Paused (stored in OpenSandbox server state)
    ↓
On next access: restore flow above
```

### Key Data Flows Summary

1. **Control plane → VM:** OpenSandbox lifecycle API → FirecrackerSandboxService → firecracker-go-sdk → Firecracker Unix socket → KVM
2. **SDK → execd:** ExecdClient HTTP → vsock proxy (host) → Firecracker vsock bridge → guest AF_VSOCK → execd HTTP listener
3. **Snapshot write:** Firecracker process → memory-mapped files on local FS → (Phase 3) streamed to S3/OSS
4. **Snapshot read:** Local FS files → `MAP_PRIVATE` mapping in new Firecracker process → on-demand page loading from file
5. **Guest egress:** Guest virtio-net → TAP device → iptables NAT → existing OpenSandbox egress proxy → internet

## Scaling Considerations

| Scale | Architecture Adjustment |
|-------|------------------------|
| 1-10 VMs (dev/test) | Single host, local FS snapshots, no pool, no jailer required |
| 10-100 VMs | Jailer mandatory; TAP naming scheme with netns per VM; diff snapshots to manage storage; consider memory balloon for snapshot size reduction |
| 100-1000 VMs | Template snapshot warm pool (Phase 3); CoW memory overlays; local NVMe for snapshot storage; object storage for cross-node snapshots; dedicated orchestrator process separate from API server |
| 1000+ VMs | Distributed scheduler; snapshot pre-placement (snapshot near the nodes likely to restore it); memory decompression pipeline (lz4 + userfaultfd pattern used by CodeSandbox) |

### Scaling Priorities

1. **First bottleneck:** Cold boot latency at high concurrency. Fix: template snapshot pool (Phase 3). ~125ms cold boot → ~5-10ms from pool.
2. **Second bottleneck:** Memory snapshot storage at scale. Fix: memory balloon inflation before snapshot + lz4 compression + diff snapshots. 8GB VM → ~745MB compressed.
3. **Third bottleneck:** Cross-host restore requires snapshot replication. Fix: S3/OSS backend (Phase 3).

## Anti-Patterns

### Anti-Pattern 1: Using Kata Containers as the Firecracker wrapper

**What people do:** Use OpenSandbox's existing Kata runtime with Firecracker as the Kata driver, expecting to get snapshot support for free.

**Why it's wrong:** Kata abstracts Firecracker behind its containerd runtime interface. Kata does not expose Firecracker's `/snapshot/create` or `/snapshot/load` APIs. There is no path to snapshot/restore through Kata. This is why the project goes direct.

**Do this instead:** Implement FirecrackerSandboxService directly using firecracker-go-sdk, bypassing Kata entirely.

### Anti-Pattern 2: Sharing the vsock UDS path across restored VMs

**What people do:** Restore multiple VMs from the same snapshot using the same Unix socket path (e.g., `v.sock`).

**Why it's wrong:** The Firecracker process for the new VM tries to bind the same UDS path as the source VM. If the source is still running, it fails. Even if not, concurrent restores collide.

**Do this instead:** Use `vsock_override` in the LoadSnapshot API to specify a unique UDS path per VM instance. Convention: `/var/lib/firecracker/{sandbox-id}/v.sock`.

### Anti-Pattern 3: Taking a snapshot without pausing first

**What people do:** Call `/snapshot/create` on a running (not paused) VM.

**Why it's wrong:** Firecracker rejects snapshot creation on a running VM — the VM must be in the Paused state. Attempting this returns an API error.

**Do this instead:** Always use the Pause → Snapshot → Resume sequence. Wrap this in a single operation in SnapshotManager so callers cannot create snapshots without pausing.

### Anti-Pattern 4: Storing full memory snapshots for every sandbox

**What people do:** Create a full snapshot on every pause event regardless of VM memory size.

**Why it's wrong:** Full snapshots = VM RAM size on disk. 8GB VM = 8GB file per snapshot. At 100 sandboxes this is 800GB just for the latest snapshot, before retention.

**Do this instead:** Phase 1: full snapshots with explicit size limits. Phase 2: diff snapshots after first full snapshot (only dirty pages). Phase 3: memory balloon inflation + compression pipeline. Always enforce retention policy.

### Anti-Pattern 5: TCP-based execd communication for Firecracker VMs

**What people do:** Assign the Firecracker VM an IP, expose execd on TCP port 44772, and use the existing ExecdClient unchanged.

**Why it's wrong:** Requires full L3 routing between host and VM (or additional TAP/bridge config). More attack surface than vsock. Snapshot restore invalidates TCP connections (remote peer sees RST). vsock is the idiomatic Firecracker IPC mechanism.

**Do this instead:** Run execd listening on AF_VSOCK port 44772 in the guest. Add a host-side vsock proxy that the SDK ExecdClient connects to via TCP/Unix, which forwards to the VM's vsock. After restore, vsock listen sockets remain active — execd reconnects work.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Firecracker binary | Spawned as child process via firecracker-go-sdk; controlled via Unix socket REST API | Must be same version for snapshot/restore pairs; jailer wraps binary in prod |
| KVM | Kernel module; Firecracker accesses via `/dev/kvm`; no explicit integration code | Requires KVM-capable host; /dev/kvm must be accessible (jailer hands it in) |
| Linux TAP driver | `ip tuntap` syscalls via `netlink` or shell; iptables via `go-iptables` | Requires CAP_NET_ADMIN; use network namespaces in prod |
| execd (guest) | HTTP-over-vsock; host opens UDS, sends CONNECT handshake, then HTTP | execd must add AF_VSOCK listen in addition to current TCP; same HTTP/SSE protocol |
| OpenSandbox Lifecycle API | FirecrackerSandboxService implements SandboxService; hooks into existing server | Snapshot/restore endpoints are new; pause/resume reuse OSEP-0008 stubs |
| S3/OSS (Phase 3) | Snapshot store backend; stream vmstate + memory files via multipart upload | Same-version constraint: tag snapshot objects with Firecracker version + kernel hash |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| FirecrackerSandboxService ↔ firecracker-go-sdk | Direct Go function calls; Machine struct | Machine is not goroutine-safe; one goroutine per VM lifecycle |
| Host orchestrator ↔ Firecracker process | Unix socket REST (HTTP); managed by firecracker-go-sdk Client | Socket path inside jailer chroot; path must be tracked per sandbox ID |
| Host ↔ Guest execd | vsock (AF_UNIX on host → AF_VSOCK in guest via Firecracker bridge) | After restore, reconnect required; vsock listen sockets survive; connections do not |
| SnapshotManager ↔ Storage | Go interface (SnapshotStore); local FS impl Phase 1, S3 impl Phase 3 | Interface defined in Phase 1; second impl added in Phase 3 without touching caller |
| FirecrackerRuntime ↔ OpenSandbox state machine | State machine extended: Running → Paused → Running/Terminated | OpenSandbox server owns state; runtime calls state transitions; no direct DB writes from runtime |

## Suggested Build Order

Dependencies flow in this order — each phase unblocks the next:

**Phase 1: VM Lifecycle (foundation everything else builds on)**

1. rootfs.go — ext4 image management (before any VM can start)
2. network.go — TAP provisioning (before VM can start with networking)
3. machine.go — VM create/start/stop via firecracker-go-sdk (core lifecycle)
4. vsock.go — host-side vsock proxy for execd communication (before execd works)
5. execd vsock transport — AF_VSOCK listen mode in guest (enables exec in VMs)
6. runtime.go — SandboxService implementation wiring it all together
7. jailer.go — production security wrapper (can be added after baseline works)

**Phase 2: Snapshot/Restore (requires Phase 1 complete)**

8. state.go — extend state machine with Paused state
9. snapshot.go — CreateSnapshot/LoadSnapshot logic (requires machine.go)
10. snapshot_store.go — local FS backend (requires snapshot.go)
11. Lifecycle API extension — /snapshot and /restore endpoints (requires all above)
12. diff snapshot support — track_dirty_pages flag + diff snapshot type (requires full snapshot working)

**Phase 3: Pool + Multi-node (requires Phase 2 complete)**

13. pool.go — template snapshot warm pool (requires snapshot restore working)
14. S3/OSS backend — second SnapshotStore implementation (requires snapshot_store interface)

## Sources

- [Firecracker Design Doc](https://github.com/firecracker-microvm/firecracker/blob/main/docs/design.md) — thread model, VMM architecture, device model (HIGH confidence)
- [Firecracker vsock docs](https://github.com/firecracker-microvm/firecracker/blob/main/docs/vsock.md) — CID/port addressing, UDS bridge, vsock_override for snapshots (HIGH confidence)
- [Firecracker Snapshot Support](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/snapshot-support.md) — file format, full vs diff, API sequence, networking behavior after restore (HIGH confidence)
- [Firecracker Jailer docs](https://github.com/firecracker-microvm/firecracker/blob/main/docs/jailer.md) — chroot structure, seccomp, cgroups, file path conventions (HIGH confidence)
- [firecracker-go-sdk pkg.go.dev](https://pkg.go.dev/github.com/firecracker-microvm/firecracker-go-sdk) — Machine struct, handler chain, Config, snapshot methods (HIGH confidence)
- [OpenSandbox architecture.md](https://raw.githubusercontent.com/alibaba/OpenSandbox/main/docs/architecture.md) — SandboxService interface, runtime backend contract, execd injection (HIGH confidence)
- [ForgeVM sandbox architecture (sandbox 28ms boot)](https://dev.to/adwitiya/how-i-built-sandboxes-that-boot-in-28ms-using-firecracker-snapshots-i0k) — CoW snapshot pooling, guest agent pattern (MEDIUM confidence, community)
- [CodeSandbox memory scaling](https://codesandbox.io/blog/how-we-scale-our-microvm-infrastructure-using-low-latency-memory-decompression) — memory balloon, lz4 compression, userfaultfd (MEDIUM confidence, production case study)

---
*Architecture research for: Firecracker runtime backend with snapshot/restore in OpenSandbox*
*Researched: 2026-04-04*

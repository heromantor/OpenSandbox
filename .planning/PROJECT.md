# OpenSandbox Firecracker Runtime with Snapshot/Restore

## What This Is

A Firecracker runtime backend for OpenSandbox (alibaba/OpenSandbox) that adds first-class snapshot/restore support. This is an upstream contribution that brings full VM state capture and near-instant resume (~5-10ms) to OpenSandbox, starting with a direct Firecracker integration (bypassing Kata's limitations) and building up to snapshot-based pool optimization.

## Core Value

Sandboxes can be paused, snapshotted, and restored with all in-memory state, running processes, and open connections intact — something no existing OpenSandbox runtime backend supports.

## Requirements

### Validated

- ✓ Go SDK client for lifecycle API (create/get/delete sandboxes) — existing
- ✓ Go SDK client for execd API (command execution, code execution, file ops) — existing
- ✓ Execd agent protocol (HTTP on port 44772, SSE streaming, NDJSON) — existing
- ✓ Server proxy mode for environments where sandbox IPs aren't routable — existing
- ✓ Egress proxy with FQDN-based access control — existing

### Active

- [ ] Firecracker VM lifecycle management (create, start, stop, destroy) via firecracker-go-sdk
- [ ] ext4 rootfs image management (conversion from OCI or standalone images)
- [ ] vsock-based host-guest communication channel for execd
- [ ] Tap device networking with integration into OpenSandbox's network setup (FQDN egress, DNS)
- [ ] Jailer support for production security (chroot, seccomp, cgroup isolation)
- [ ] Guest kernel + rootfs image configuration and management
- [ ] Snapshot creation API (`POST /sandboxes/{id}/snapshot`) — full and diff types
- [ ] Snapshot restore API (`POST /sandboxes/{id}/restore`) with auto-resume
- [ ] Snapshot listing and deletion endpoints
- [ ] Local filesystem snapshot storage backend
- [ ] Sandbox state machine extension (Running → Paused → Running/Terminated)
- [ ] TTL pausing for snapshotted sandboxes
- [ ] Dirty page tracking for efficient diff snapshots
- [ ] Template VM snapshots for pool pre-warming (~5-10ms creation vs ~125ms cold boot)
- [ ] Object storage backend (S3/OSS) for multi-node snapshot sharing
- [ ] Snapshot retention policies (per-sandbox and global)

### Out of Scope

- Kata Containers snapshot integration — Kata doesn't expose Firecracker's snapshot API, going direct is the whole point
- Cross-version snapshot restore — Firecracker doesn't guarantee this, not worth working around
- Live migration between hosts — snapshot/restore on different hosts is supported (with CPU template matching), but true live migration is not
- Windows guest support — Firecracker is Linux-only
- GPU passthrough — Firecracker doesn't support it

## Context

- **Parallel effort to Sol:** This is the open-source contribution path alongside Sol (custom Firecracker runtime). Learnings from Sol inform this work and vice versa.
- **OSEP-0008 exists as a draft:** The OpenSandbox project has a draft proposal for Pause/Resume (`POST /pause`, `POST /resume`) but nothing is implemented. This contribution supersedes that with a more capable snapshot/restore approach.
- **OSEP-0003 (Volumes) is implementing:** PVC support for persistent workspace data exists. Volumes survive sandbox restarts — this is the filesystem persistence layer.
- **OSEP-0009 (Auto-Renew) is implemented:** Extends sandbox TTL on traffic detection. Relevant for long-running sandboxes.
- **Execd is HTTP-first:** The current execd agent runs as an HTTP service on port 44772 inside sandboxes. For Firecracker, vsock is the natural transport — execd needs to listen on vsock instead of (or in addition to) a TCP socket.
- **Sparse checkout:** This repo uses sparse checkout — only `sdks/sandbox/go/` and related SDK directories are checked out locally.
- **Current branch (`feat/go-sdk`):** Active work on the Go SDK with tests for PVC volumes, network policy, env vars, and E2E parity.

## Constraints

- **Same Firecracker version:** Snapshot and restore must use the same Firecracker binary version. Cross-version restore is undefined behavior.
- **Same kernel image:** Guest kernel must match between snapshot and restore. Different kernel = undefined behavior.
- **CPU template matching:** Restore host must have same CPU features. Use `cpu_template` (T2, T2S, C3) to normalize CPUID across hosts.
- **Memory file size:** Full snapshot memory file = configured RAM size (e.g., 8GB VM → 8GB file). Diff snapshots are smaller but require the base.
- **Clock skew:** Guest TSC jumps after resume. NTP corrects it, but monotonic clock-dependent apps may notice.
- **TCP connection staleness:** Remote peers time out during pause. Applications must handle reconnection.
- **vsock CID uniqueness:** Each VM from the same snapshot needs a unique CID to avoid conflicts on the same host.
- **Dependency:** `github.com/firecracker-microvm/firecracker-go-sdk` — official Go SDK for Firecracker management.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Direct Firecracker integration (not through Kata) | Kata doesn't expose Firecracker's snapshot API | — Pending |
| vsock for host-guest communication | Natural Firecracker transport, avoids tap networking complexity for exec | — Pending |
| ext4 rootfs images (not OCI containers) | Firecracker uses block devices, not container runtimes | — Pending |
| Local filesystem storage first, S3/OSS later | Simplest path to working snapshots; multi-node comes in Phase 3 | — Pending |
| Three-phase approach (runtime → snapshots → pool) | Each phase is independently valuable; Phase 1 alone adds Firecracker as a runtime option | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-04 after initialization*

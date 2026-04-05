# Phase 1: VM Lifecycle and Jailer - Research

**Researched:** 2026-04-05
**Domain:** Firecracker microVM lifecycle management and Jailer production isolation via firecracker-go-sdk
**Confidence:** HIGH

## Summary

Phase 1 establishes the foundational Firecracker VM lifecycle: create, start, stop, and destroy a microVM with full Jailer production isolation. The primary dependency is `github.com/firecracker-microvm/firecracker-go-sdk` (tagged v1.0.0 but actively developed on `main`, requires Go 1.24+), which provides a `Machine` type wrapping the Firecracker API socket with a handler-based initialization pipeline, and a `JailerConfig` type that manages chroot, cgroup, namespace, and privilege-dropping setup. The SDK handles process lifecycle (start, shutdown, stop), cleanup of socket files, and jailer chroot construction.

The existing OpenSandbox Go SDK at `sdks/sandbox/go/opensandbox/` is a client-side SDK that talks to the OpenSandbox lifecycle/execd/egress APIs over HTTP. Phase 1 builds a **new runtime backend** -- server-side code that actually manages Firecracker processes on the host. This is architecturally separate from the existing client SDK. The new code should live in a new Go module (e.g., `runtime/firecracker/`) at the project root level, not inside `sdks/sandbox/go/`.

**Primary recommendation:** Create a new `runtime/firecracker/` Go module that wraps `firecracker-go-sdk`'s `Machine` and `JailerConfig` types behind an OpenSandbox-specific `VMManager` interface. Use the SDK's handler pipeline for VM startup, configure Jailer with chroot/cgroup/seccomp isolation, support both static and custom CPU templates for snapshot portability, and implement deterministic resource cleanup on stop/destroy.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- discuss phase was skipped per user setting (workflow.skip_discuss). All implementation choices are at Claude's discretion.

### Claude's Discretion
All implementation choices are at Claude's discretion -- discuss phase was skipped per user setting. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None -- discuss phase skipped.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| VMLC-01 | Firecracker VM can be created with configurable vCPUs, memory, and boot source via firecracker-go-sdk | SDK `Config` struct has `MachineCfg.VcpuCount`, `MachineCfg.MemSizeMib`, `KernelImagePath`, `Drives` fields. `NewMachine()` + `Start()` lifecycle verified. |
| VMLC-02 | Firecracker VM can be started and enters Running state | SDK `Machine.Start(ctx)` starts VMM via handler pipeline; `Machine.Wait(ctx)` blocks until exit. PID available via `Machine.PID()`. |
| VMLC-03 | Firecracker VM can be stopped and resources are cleaned up (process, socket, tap device) | SDK `Machine.StopVMM()` sends SIGTERM; `Machine.Shutdown(ctx)` sends Ctrl+Alt+Del. `cleanupFuncs` slice executes in reverse order (socket removal, network teardown). |
| VMLC-04 | Firecracker VM runs inside Jailer with chroot, seccomp, and cgroup isolation | SDK `JailerConfig` configures chroot, UID/GID, cgroups, namespaces. Jailer binary creates chroot at `<base>/<exec-name>/<id>/root/`, creates /dev/kvm and /dev/net/tun, drops privileges. Default seccomp filters built into Firecracker binary. |
| VMLC-05 | Guest kernel image is managed as a build artifact with pinned version | Firecracker's `tools/devtool build_ci_artifacts kernels` builds from config. Pre-built kernels available from CI S3. Kernel version must be pinned in a Makefile/script for reproducibility. |
| VMLC-06 | CPU template (T2/T2S/C3) is configurable per sandbox for cross-host snapshot portability | SDK `MachineCfg.CPUTemplate` field. Static templates (T2/T2S/C3) deprecated since Firecracker v1.5.0 but still functional; JSON custom CPU templates are the replacement. Both paths should be supported. |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/firecracker-microvm/firecracker-go-sdk` | v1.0.0 (use `main` branch via commit hash) | Firecracker VM lifecycle, Jailer integration, handler pipeline | Official SDK from Firecracker team; only maintained Go SDK for Firecracker [VERIFIED: github.com/firecracker-microvm/firecracker-go-sdk] |
| `github.com/sirupsen/logrus` | v1.9.3 | Structured logging (required by firecracker-go-sdk) | Transitive dependency of firecracker-go-sdk; SDK's Machine.Logger() returns `*logrus.Entry` [VERIFIED: firecracker-go-sdk go.mod] |
| `github.com/google/uuid` | v1.6.0 | Unique VM IDs | Already in project; used for sandbox IDs [VERIFIED: existing go.mod] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/hashicorp/go-multierror` | v1.1.1 | Aggregate cleanup errors | When multiple cleanup operations can fail independently [VERIFIED: firecracker-go-sdk go.mod] |
| `github.com/containernetworking/cni` | v1.3.0 | CNI network plugin interface | Phase 4 (networking) -- not needed in Phase 1 but pulled in by SDK [VERIFIED: firecracker-go-sdk go.mod] |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| firecracker-go-sdk | Raw HTTP to Firecracker API socket | More control but enormous maintenance burden; SDK handles handler pipeline, jailer integration, cleanup lifecycle, and keeps pace with API changes |
| logrus | slog (stdlib) | firecracker-go-sdk requires logrus internally; wrapping with slog adapter adds complexity for no gain |
| Static CPU templates | Custom CPU templates only | Custom templates are the future but require maintaining JSON files; support both for flexibility |

**Installation:**

The new runtime module will need its own `go.mod`:
```bash
# From project root
mkdir -p runtime/firecracker
cd runtime/firecracker
go mod init github.com/alibaba/OpenSandbox/runtime/firecracker
go get github.com/firecracker-microvm/firecracker-go-sdk@main
go get github.com/google/uuid@v1.6.0
```

**Version note:** The firecracker-go-sdk's only tagged release is v1.0.0 (Aug 2022), but the `main` branch is actively maintained (Go 1.24.11 in go.mod, up-to-date with Firecracker v1.15.0 API). Use `@main` or pin to a specific commit hash for reproducibility. [VERIFIED: pkg.go.dev and github.com tags page]

## Architecture Patterns

### Recommended Project Structure
```
runtime/
  firecracker/
    go.mod                    # New Go module for Firecracker runtime
    go.sum
    vm.go                     # VMConfig, VM type, Create/Start/Stop/Destroy
    vm_test.go                # Unit tests with mocked Machine
    jailer.go                 # JailerConfig builder, chroot path helpers
    jailer_test.go            # Jailer configuration tests
    kernel.go                 # Kernel image management, version pinning
    kernel_test.go            # Kernel path resolution tests
    cpu_template.go           # CPU template configuration (static + custom)
    cpu_template_test.go      # CPU template tests
    cleanup.go                # Resource cleanup (chroot, socket, cgroup)
    cleanup_test.go           # Cleanup verification tests
    errors.go                 # Runtime-specific error types
    manager.go                # VMManager interface and implementation
    manager_test.go           # Manager lifecycle tests
    testdata/                 # Test fixtures (mock kernel, rootfs stubs)
    integration_test.go       # //go:build integration (requires Linux+KVM)
```

### Pattern 1: VMManager Interface
**What:** An interface abstracting VM lifecycle operations for testability and future runtime backends.
**When to use:** Always -- this is the primary abstraction.
**Example:**
```go
// Source: Project architecture decision
// VMManager manages the lifecycle of Firecracker microVMs.
type VMManager interface {
    Create(ctx context.Context, cfg VMConfig) (*VM, error)
    Start(ctx context.Context, vm *VM) error
    Stop(ctx context.Context, vm *VM) error
    Destroy(ctx context.Context, vm *VM) error
    Get(ctx context.Context, vmID string) (*VM, error)
}
```
[ASSUMED]

### Pattern 2: Config-to-SDK Translation
**What:** A `VMConfig` struct specific to OpenSandbox that translates to `firecracker.Config` internally.
**When to use:** For every VM creation -- isolates OpenSandbox domain from SDK internals.
**Example:**
```go
// Source: firecracker-go-sdk Config struct (github.com/firecracker-microvm/firecracker-go-sdk)
type VMConfig struct {
    ID              string
    VCPUs           int64
    MemoryMiB       int64
    KernelImagePath string
    RootfsPath      string
    KernelArgs      string
    CPUTemplate     string         // "T2", "T2S", "C3", or path to custom JSON
    JailerEnabled   bool
    JailerUID       int
    JailerGID       int
    ChrootBaseDir   string         // Default: /srv/jailer
    CgroupVersion   string         // "1" or "2"
    LogLevel        string         // "Error", "Warning", "Info", "Debug"
    TrackDirtyPages bool           // Enable for future diff snapshots
}

// toFirecrackerConfig translates VMConfig to firecracker.Config.
func (c *VMConfig) toFirecrackerConfig() firecracker.Config {
    drives := firecracker.NewDrivesBuilder(c.RootfsPath).Build()
    
    cfg := firecracker.Config{
        VMID:            c.ID,
        SocketPath:      c.socketPath(),
        KernelImagePath: c.KernelImagePath,
        KernelArgs:      c.KernelArgs,
        Drives:          drives,
        MachineCfg: models.MachineConfiguration{
            VcpuCount:       firecracker.Int64(c.VCPUs),
            MemSizeMib:      firecracker.Int64(c.MemoryMiB),
            TrackDirtyPages: firecracker.Bool(c.TrackDirtyPages),
        },
    }
    
    if c.CPUTemplate != "" {
        cfg.MachineCfg.CPUTemplate = models.CPUTemplate(c.CPUTemplate)
    }
    
    if c.JailerEnabled {
        cfg.JailerCfg = &firecracker.JailerConfig{
            ID:            c.ID,
            UID:           firecracker.Int(c.JailerUID),
            GID:           firecracker.Int(c.JailerGID),
            ExecFile:      "/usr/bin/firecracker",  // Configurable
            ChrootBaseDir: c.ChrootBaseDir,
            CgroupVersion: c.CgroupVersion,
            Daemonize:     true,
        }
    }
    
    return cfg
}
```
[VERIFIED: firecracker-go-sdk Config/JailerConfig/MachineConfiguration struct fields from pkg.go.dev and GitHub source]

### Pattern 3: Handler Pipeline Customization
**What:** The SDK's handler-based startup allows inserting custom setup steps.
**When to use:** When adding pre/post-start hooks (e.g., entropy seeding in Phase 6, vsock setup in Phase 3).
**Example:**
```go
// Source: https://github.com/firecracker-microvm/firecracker-go-sdk/blob/main/handlers.go
// Default handler chain:
// SetupNetwork -> SetupKernelArgs -> StartVMM -> CreateLogFiles 
// -> BootstrapLogging -> CreateMachine -> CreateBootSource 
// -> AttachDrives -> CreateNetworkInterfaces -> AddVsocks -> ConfigMmds

// Custom handler can be appended:
// handlers.Append(firecracker.Handler{
//     Name: "opensandbox.CustomStep",
//     Fn: func(ctx context.Context, m *firecracker.Machine) error { ... },
// })
```
[VERIFIED: firecracker-go-sdk handlers.go source]

### Pattern 4: Deterministic Cleanup
**What:** The SDK accumulates `cleanupFuncs` executed in reverse order on stop. OpenSandbox must add its own cleanup (chroot directory removal, cgroup cleanup).
**When to use:** Every VM stop/destroy operation.
**Example:**
```go
// Source: firecracker-go-sdk machine.go cleanup pattern
// After Machine.Start(), register additional cleanup:
// - Remove jailer chroot directory tree
// - Remove leftover cgroup directories (jailer doesn't always clean these)
// - Remove socket file if StopVMM doesn't
// 
// Pattern: Track all created resources in a VMResources struct,
// clean up in Destroy() even if Stop() panics.
type VMResources struct {
    SocketPath    string
    ChrootDir     string
    CgroupPaths   []string
    LogFifoPath   string
    MetricsFifoPath string
}
```
[ASSUMED]

### Anti-Patterns to Avoid
- **Direct Firecracker API socket calls:** The SDK wraps the HTTP API cleanly; bypassing it loses handler pipeline, validation, cleanup, and error handling.
- **Shared chroot directories:** Each VM MUST have its own chroot under `<base>/<exec-name>/<id>/root`. Sharing leads to filesystem conflicts and security violations.
- **Ignoring cleanup errors:** Use `multierror` to collect all cleanup failures; log each one. A failed socket cleanup blocks the next VM with the same ID.
- **Hardcoded Firecracker binary path:** The jailer `ExecFile` must be configurable; production installs may place the binary in different locations.
- **Running without Jailer in production:** The Jailer provides chroot, cgroup, namespace, seccomp, and privilege-dropping. Without it, the Firecracker process runs with the caller's full privileges.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Firecracker API communication | HTTP client for /machine-config, /actions, etc. | `firecracker-go-sdk` Machine type | SDK handles socket communication, request serialization, error parsing, handler pipeline |
| Jailer chroot setup | Custom chroot/pivot_root code | SDK's `JailerConfig` + jailer binary | Jailer handles mount namespace, dev nodes, privilege dropping, file descriptor closing -- getting any of these wrong is a security vulnerability |
| Process lifecycle management | Manual exec.Cmd + signal handling | SDK's `Machine.Start()`, `StopVMM()`, `Wait()` | SDK handles SIGTERM delivery, process wait, exit channel, cleanup function chain |
| VM ID generation | Custom ID scheme | `github.com/google/uuid` v4 | UUID v4 satisfies jailer's alphanumeric+hyphens constraint and provides uniqueness |
| Drive configuration | Manual /drives API calls | SDK's `DrivesBuilder` | Builder handles IsRootDevice, IsReadOnly, PathOnHost, DriveID assignment |
| Cgroup setup | Manual /sys/fs/cgroup writes | Jailer's `--cgroup` and `--cgroup-version` args | Jailer handles cgroup creation, PID assignment, and cleanup |

**Key insight:** Firecracker's security model depends on getting chroot, namespace, cgroup, and seccomp configuration exactly right. The jailer binary and Go SDK encode years of hardening -- any custom implementation will have security gaps.

## Common Pitfalls

### Pitfall 1: Chroot Path Hard-Link Requirements
**What goes wrong:** Kernel image, rootfs, and FIFO paths fail when using symlinks or paths outside the chroot.
**Why it happens:** The jailer uses hard links to bring files into the chroot. Hard links require same-filesystem, and symlinks are not followed across the pivot_root boundary.
**How to avoid:** Place kernel and rootfs images on the same filesystem as `ChrootBaseDir` (default `/srv/jailer`). Use absolute paths.
**Warning signs:** "No such file or directory" errors after jailer starts; the file exists outside chroot but not inside.

### Pitfall 2: Socket Path Length Limit
**What goes wrong:** VM creation fails with cryptic errors when the Unix socket path exceeds 108 characters.
**Why it happens:** Linux kernel limits Unix domain socket paths to 108 bytes (`sun_path` in `sockaddr_un`). The jailer chroot path (`/srv/jailer/firecracker/<uuid>/root/run/firecracker.socket`) can exceed this with long base dirs or IDs.
**How to avoid:** Keep `ChrootBaseDir` short (default `/srv/jailer` is fine). VM IDs should be standard UUIDs (36 chars). Full path: ~80 chars with default settings.
**Warning signs:** "bind: invalid argument" or "address too long" socket errors.

### Pitfall 3: Stale Chroot Directories After Crash
**What goes wrong:** If the host process crashes without calling cleanup, the chroot directory and cgroup entries persist. The next VM with the same ID fails because the jailer refuses to create an already-existing directory.
**Why it happens:** SDK's `cleanupFuncs` run in `doCleanup()` which requires graceful shutdown. A SIGKILL or host crash skips this.
**How to avoid:** On startup, scan `ChrootBaseDir` for orphaned directories (no corresponding Firecracker process). Implement a "garbage collector" that removes stale chroots. Always generate new UUIDs for VMs (don't reuse IDs).
**Warning signs:** "directory already exists" errors on VM create; accumulating directories under `/srv/jailer/`.

### Pitfall 4: CPU Template Deprecation
**What goes wrong:** Static CPU templates (T2/T2S/C3) via `cpu_template` field will be removed in a future Firecracker release.
**Why it happens:** Firecracker deprecated static templates in v1.5.0 in favor of custom CPU templates via `/cpu-config` API.
**How to avoid:** Support both static templates (for compatibility with current Firecracker versions) and custom CPU template JSON files (for forward compatibility). The SDK's `MachineCfg.CPUTemplate` field still works with current Firecracker versions. Pre-built JSON equivalents of T2/T2S/C3 are available in the Firecracker repo.
**Warning signs:** Deprecation warnings in Firecracker logs mentioning `cpu_template`.

### Pitfall 5: Cgroup v1 vs v2 Mismatch
**What goes wrong:** Jailer fails to create cgroups if the configured cgroup version doesn't match the host.
**Why it happens:** Modern Linux distributions (Ubuntu 22.04+, Fedora 31+) default to cgroup v2. The jailer's `--cgroup-version` must match the host's cgroup hierarchy.
**How to avoid:** Auto-detect cgroup version by checking for `/sys/fs/cgroup/cgroup.controllers` (v2) vs `/sys/fs/cgroup/cpu/` (v1). Set `JailerConfig.CgroupVersion` accordingly.
**Warning signs:** "No such file or directory" errors related to `/sys/fs/cgroup/`.

### Pitfall 6: macOS Development vs Linux Runtime
**What goes wrong:** Code compiles on macOS but none of the Firecracker/jailer code can be tested locally.
**Why it happens:** Firecracker requires Linux KVM. macOS has no /dev/kvm.
**How to avoid:** Use `//go:build linux` build tags for all code that imports firecracker-go-sdk or interacts with KVM/jailer. Unit tests should use interfaces and mocks. Integration tests require a Linux VM or CI environment.
**Warning signs:** Import errors or runtime panics on macOS; tests that silently pass because they're skipped.

## Code Examples

### Example 1: Basic VM Creation with Jailer
```go
// Source: Synthesized from firecracker-go-sdk pkg.go.dev and example_test.go
package firecracker

import (
    "context"
    
    sdk "github.com/firecracker-microvm/firecracker-go-sdk"
    "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
    "github.com/sirupsen/logrus"
)

func createVM(ctx context.Context, cfg VMConfig) (*sdk.Machine, error) {
    fcCfg := sdk.Config{
        VMID:            cfg.ID,
        SocketPath:      cfg.socketPath(),
        KernelImagePath: cfg.KernelImagePath,
        KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off",
        Drives:          sdk.NewDrivesBuilder(cfg.RootfsPath).Build(),
        MachineCfg: models.MachineConfiguration{
            VcpuCount:       sdk.Int64(cfg.VCPUs),
            MemSizeMib:      sdk.Int64(cfg.MemoryMiB),
            TrackDirtyPages: sdk.Bool(cfg.TrackDirtyPages),
        },
    }
    
    if cfg.CPUTemplate != "" {
        fcCfg.MachineCfg.CPUTemplate = models.CPUTemplate(cfg.CPUTemplate)
    }
    
    if cfg.JailerEnabled {
        uid := cfg.JailerUID
        gid := cfg.JailerGID
        fcCfg.JailerCfg = &sdk.JailerConfig{
            ID:            cfg.ID,
            UID:           &uid,
            GID:           &gid,
            ExecFile:      cfg.FirecrackerBinary,
            ChrootBaseDir: cfg.ChrootBaseDir,
            CgroupVersion: cfg.CgroupVersion,
            Daemonize:     true,
        }
    }
    
    logger := logrus.NewEntry(logrus.New())
    
    m, err := sdk.NewMachine(ctx, fcCfg, sdk.WithLogger(logger))
    if err != nil {
        return nil, fmt.Errorf("opensandbox: new machine: %w", err)
    }
    
    if err := m.Start(ctx); err != nil {
        return nil, fmt.Errorf("opensandbox: start machine: %w", err)
    }
    
    return m, nil
}
```
[VERIFIED: firecracker-go-sdk API types and constructor patterns from pkg.go.dev]

### Example 2: Graceful Stop with Cleanup
```go
// Source: Synthesized from firecracker-go-sdk Machine.StopVMM() and Shutdown()
func stopVM(ctx context.Context, m *sdk.Machine) error {
    // Try graceful shutdown first (sends Ctrl+Alt+Del on x86)
    if err := m.Shutdown(ctx); err != nil {
        // If graceful fails, force stop (SIGTERM)
        if stopErr := m.StopVMM(); stopErr != nil {
            return fmt.Errorf("opensandbox: force stop failed: %w (after shutdown error: %v)", stopErr, err)
        }
    }
    
    // Wait for process to exit
    if err := m.Wait(ctx); err != nil {
        // Process may already be gone -- not always an error
        if !isProcessExited(err) {
            return fmt.Errorf("opensandbox: wait for exit: %w", err)
        }
    }
    
    return nil
}
```
[VERIFIED: Machine.Shutdown/StopVMM/Wait methods from pkg.go.dev]

### Example 3: Cgroup Version Auto-Detection
```go
// Source: Standard Linux cgroup detection pattern
func detectCgroupVersion() string {
    // cgroup v2 unified hierarchy has a controllers file at the root
    if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
        return "2"
    }
    return "1"
}
```
[ASSUMED -- standard pattern, not from official docs]

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Static CPU templates (T2/T2S/C3) via `cpu_template` | Custom CPU templates via `/cpu-config` JSON | Firecracker v1.5.0 (deprecated), removal TBD | Must support both for compatibility; static still works in v1.15.0 |
| `--seccomp-level` jailer arg | Default seccomp filters built into Firecracker binary | Firecracker v1.0+ | No configuration needed; filters are baked in. Custom filters via `--seccomp-filter` arg only if overriding defaults. |
| Single cgroup version | Explicit `--cgroup-version 1` or `2` | Jailer change in line with cgroup v2 adoption | Must auto-detect host cgroup version |
| Firecracker Go SDK pre-v1.0 | firecracker-go-sdk v1.0.0 (tagged) + active main branch | v1.0.0 tagged Aug 2022; main branch actively maintained | Use main branch commit pin for latest Firecracker compatibility |

**Deprecated/outdated:**
- Static CPU templates (`cpu_template` field): Deprecated since Firecracker v1.5.0. Still functional in v1.15.0 but scheduled for removal. Use custom CPU templates (`/cpu-config`) for new deployments. [VERIFIED: https://github.com/firecracker-microvm/firecracker/blob/main/docs/cpu_templates/cpu-templates.md]
- `--seccomp-level` jailer argument: Replaced by Firecracker's built-in seccomp filters. [VERIFIED: https://github.com/firecracker-microvm/firecracker/blob/main/docs/seccomp.md]

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | New runtime code should live in `runtime/firecracker/` as a separate Go module, not inside `sdks/sandbox/go/` | Architecture Patterns | If wrong, need to restructure project layout; existing SDK module would need firecracker-go-sdk dependency added |
| A2 | `VMManager` interface is the right abstraction for the runtime | Architecture Patterns | Low risk -- can be refactored; interface is a natural Go pattern for testability |
| A3 | Cgroup version auto-detection via `/sys/fs/cgroup/cgroup.controllers` is reliable | Code Examples | If wrong on exotic distros, would need fallback; standard detection method |
| A4 | Using `@main` branch of firecracker-go-sdk instead of v1.0.0 tag | Standard Stack | v1.0.0 tag is from 2022 and may lack API compatibility with Firecracker v1.15.0; main branch is actively maintained but not tagged |
| A5 | Firecracker binary and jailer binary are pre-installed on the target host | Architecture Patterns | If not, Phase 1 needs an install/download step for both binaries |
| A6 | Guest kernel images will be downloaded from Firecracker CI S3 or built from source | Phase Requirements (VMLC-05) | Other kernel sources may be preferred; the pinning mechanism matters more than the source |

## Open Questions

1. **Where should the runtime code live?**
   - What we know: The existing `sdks/sandbox/go/` is a client SDK. The runtime is server-side code.
   - What's unclear: Whether to use `runtime/firecracker/` at project root or `components/runtime/firecracker/` or another location.
   - Recommendation: Use `runtime/firecracker/` as a new Go module. This keeps the runtime separate from client SDKs and allows independent versioning.

2. **Which firecracker-go-sdk version to pin?**
   - What we know: v1.0.0 is the only tag (Aug 2022). Main branch is actively maintained with Go 1.24+ support.
   - What's unclear: Whether v1.0.0 is compatible with Firecracker v1.15.0 API.
   - Recommendation: Pin to a recent `main` branch commit hash. The go.mod shows Go 1.24.11 which confirms active maintenance.

3. **Firecracker and jailer binary distribution**
   - What we know: Phase 1 needs both `firecracker` and `jailer` binaries on the host.
   - What's unclear: Whether binaries are pre-installed, downloaded as build artifacts, or containerized.
   - Recommendation: Assume pre-installed for Phase 1; add a Makefile target that downloads specific Firecracker release binaries (v1.15.0) for development.

4. **Guest kernel image source and version**
   - What we know: Kernel must be pinned, same kernel used for snapshot/restore (project constraint).
   - What's unclear: Which kernel version (5.10 vs 6.1), which config, and where to download.
   - Recommendation: Use kernel 5.10 (long-term stable, well-tested with Firecracker). Pin version in a `kernel.manifest` or Makefile variable. Download from Firecracker CI artifacts.

5. **Integration test environment**
   - What we know: Development is on macOS (darwin/arm64); Firecracker requires Linux/KVM.
   - What's unclear: CI environment capabilities.
   - Recommendation: Use `//go:build integration && linux` tags. Unit tests mock the SDK interface. Integration tests run on Linux CI only.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All code | Yes | 1.24.5 | -- |
| Make | Build system | Yes | 3.81 | -- |
| Firecracker binary | VM creation | No (macOS host) | -- | Download for Linux CI; cannot run locally |
| Jailer binary | Production isolation | No (macOS host) | -- | Download for Linux CI; cannot run locally |
| /dev/kvm | KVM hypervisor | No (macOS host) | -- | Cannot run Firecracker locally; Linux CI required |
| Linux kernel | Host OS | No (macOS host) | -- | All integration tests require Linux |

**Missing dependencies with no fallback:**
- Firecracker, jailer, /dev/kvm, Linux kernel -- all required for integration testing. Code can be written and unit-tested on macOS with interfaces and mocks, but functional testing requires Linux with KVM support.

**Missing dependencies with fallback:**
- None. All missing dependencies are hard requirements for Firecracker and have no macOS equivalent.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go built-in `testing` package |
| Config file | `runtime/firecracker/go.mod` (new module) |
| Quick run command | `go test ./runtime/firecracker/ -v -short` |
| Full suite command | `go test ./runtime/firecracker/ -v` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| VMLC-01 | VM created with configurable vCPUs, memory, boot source | unit | `go test ./runtime/firecracker/ -run TestVMConfig -v` | No -- Wave 0 |
| VMLC-02 | VM starts and enters Running state | unit + integration | `go test ./runtime/firecracker/ -run TestVMStart -v` | No -- Wave 0 |
| VMLC-03 | VM stop releases all resources | unit + integration | `go test ./runtime/firecracker/ -run TestVMStopCleanup -v` | No -- Wave 0 |
| VMLC-04 | VM runs inside Jailer with chroot/seccomp/cgroup | unit (config) + integration (runtime) | `go test ./runtime/firecracker/ -run TestJailer -v` | No -- Wave 0 |
| VMLC-05 | Kernel image pinned in build artifact | unit | `go test ./runtime/firecracker/ -run TestKernelVersion -v` | No -- Wave 0 |
| VMLC-06 | CPU template configurable per sandbox | unit | `go test ./runtime/firecracker/ -run TestCPUTemplate -v` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./runtime/firecracker/ -v -short` (unit tests only, no KVM needed)
- **Per wave merge:** `go vet ./runtime/firecracker/ && go test ./runtime/firecracker/ -v -short`
- **Phase gate:** Full suite on Linux CI: `go test -tags=integration ./runtime/firecracker/ -v -timeout 3m`

### Wave 0 Gaps
- [ ] `runtime/firecracker/go.mod` -- new module initialization
- [ ] `runtime/firecracker/vm_test.go` -- covers VMLC-01, VMLC-02, VMLC-03
- [ ] `runtime/firecracker/jailer_test.go` -- covers VMLC-04
- [ ] `runtime/firecracker/kernel_test.go` -- covers VMLC-05
- [ ] `runtime/firecracker/cpu_template_test.go` -- covers VMLC-06
- [ ] `runtime/firecracker/integration_test.go` -- integration tests skeleton with `//go:build integration && linux`

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | No | N/A -- runtime operates locally, no user auth |
| V3 Session Management | No | N/A |
| V4 Access Control | Yes | Jailer UID/GID privilege dropping; Firecracker runs as unprivileged user |
| V5 Input Validation | Yes | Validate VMConfig fields (vCPU range 1-32, memory > 0, paths exist, ID format) |
| V6 Cryptography | No | N/A -- no crypto in Phase 1 |

### Known Threat Patterns for Firecracker Runtime

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| VM escape via Firecracker vulnerability | Elevation of Privilege | Jailer chroot + seccomp filters + namespace isolation; keep Firecracker binary updated |
| Shared filesystem between VMs | Information Disclosure | Each VM gets own chroot; rootfs images are per-VM copies (or CoW in Phase 2) |
| Resource exhaustion (fork bomb, memory) | Denial of Service | Jailer cgroup limits (cpu, memory); Firecracker memory balloon |
| Stale process after crash | Denial of Service | Orphan detection on startup; process GC |
| Untrusted kernel image | Tampering | Pin kernel version in build artifact; verify checksum before use |
| Socket file permission | Elevation of Privilege | Socket created inside chroot; inaccessible from outside namespace |

## Sources

### Primary (HIGH confidence)
- [firecracker-go-sdk pkg.go.dev](https://pkg.go.dev/github.com/firecracker-microvm/firecracker-go-sdk) -- Machine, Config, JailerConfig type definitions, v1.0.0 API
- [firecracker-go-sdk GitHub](https://github.com/firecracker-microvm/firecracker-go-sdk) -- go.mod (Go 1.24.11), handlers.go, jailer.go, machine.go, example_test.go source code
- [Firecracker jailer.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/jailer.md) -- Chroot structure, namespace setup, cgroup config, security operations
- [Firecracker CPU templates](https://github.com/firecracker-microvm/firecracker/blob/main/docs/cpu_templates/cpu-templates.md) -- Static template deprecation, custom template replacement
- [Firecracker seccomp.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/seccomp.md) -- Default filter behavior, custom filter override
- [Firecracker rootfs-and-kernel-setup.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/rootfs-and-kernel-setup.md) -- Kernel build instructions, supported versions (5.10, 6.1)
- [Firecracker releases](https://github.com/firecracker-microvm/firecracker/releases) -- Current stable v1.15.0 (March 2025)

### Secondary (MEDIUM confidence)
- [Firecracker static template deprecation discussion](https://github.com/firecracker-microvm/firecracker/discussions/4135) -- Timeline and migration guidance
- [Firecracker production host setup](https://jonathanwoollett-light.github.io/firecracker/book/book/prod-host-setup.html) -- Production deployment recommendations

### Tertiary (LOW confidence)
- None. All critical claims verified against official sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- firecracker-go-sdk is the only official Go SDK; its API is well-documented on pkg.go.dev and verified against source
- Architecture: MEDIUM -- project structure is a reasonable recommendation but the "separate module" approach is an assumption (A1) that should be confirmed
- Pitfalls: HIGH -- all pitfalls verified against official Firecracker documentation (jailer.md, seccomp.md, CPU templates)
- CPU templates: HIGH -- deprecation verified in official docs; both static and custom paths documented

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (30 days -- Firecracker SDK is stable; Firecracker releases are quarterly)

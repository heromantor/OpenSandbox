# Phase 4: TAP Networking and Egress - Research

**Researched:** 2026-04-04
**Domain:** Firecracker TAP networking, iptables NAT, DNS resolution, FQDN egress proxy integration
**Confidence:** HIGH

## Summary

Phase 4 adds network connectivity to Firecracker VMs. Each VM gets a dedicated TAP device on the host, an IP address from a private /30 subnet, and iptables NAT rules for outbound internet access. DNS resolution inside the guest is configured via the firecracker-go-sdk's `IPConfiguration.Nameservers` field which writes nameserver entries to `/proc/net/pnp` at boot time. The guest rootfs must symlink `/etc/resolv.conf` to `/proc/net/pnp` for this to work. FQDN-based egress policy enforcement integrates with OpenSandbox's existing egress proxy sidecar, which runs on the host and intercepts DNS queries to allow/deny domains per policy.

The firecracker-go-sdk already provides `NetworkInterface` with `StaticNetworkConfiguration` and `IPConfiguration` types. The host-side work is: (1) create a TAP device using `vishvananda/netlink` (already an indirect dependency), (2) assign a /30 subnet IP and bring the link up, (3) add iptables MASQUERADE and FORWARD rules using `coreos/go-iptables` (already an indirect dependency), (4) wire the TAP device name and IP configuration into the SDK's `Config.NetworkInterfaces`, and (5) clean up TAP devices and iptables rules on VM stop/destroy including crash recovery. The egress proxy integration passes network policy at sandbox creation time (already part of the lifecycle API) and the proxy enforces it via DNS interception -- no additional code is needed in the Firecracker runtime for policy enforcement itself, only for routing guest DNS through the proxy.

**Primary recommendation:** Use static TAP configuration (not CNI) with `vishvananda/netlink` for TAP lifecycle, `coreos/go-iptables` for NAT rules, /30 subnets per VM from 172.16.0.0/16 address space, and the SDK's `IPConfiguration` for automatic guest-side network setup via kernel boot parameters.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- discuss phase was skipped per user setting.

### Claude's Discretion
All implementation choices are at Claude's discretion -- discuss phase was skipped per user setting. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None -- discuss phase skipped.
</user_constraints>

## Project Constraints (from CLAUDE.md)

- Go 1.24.0 runtime
- `go vet ./...` required; `go build ./...` required
- Standard `gofmt` formatting
- Import grouping: stdlib, blank line, then third-party
- Single-letter receivers for types under 10 methods
- Exported functions have doc comments
- Error wrapping with `fmt.Errorf()` and `%w`
- Constructor pattern: `New{TypeName}()`
- Context always first parameter in async functions
- Test files: `{name}_test.go`; integration tests use build tags
- No global loggers in SDK code; return errors instead of logging
- Dependency: `github.com/firecracker-microvm/firecracker-go-sdk` v1.0.0
- Module path: `github.com/alibaba/OpenSandbox/runtime/firecracker`

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| NET-01 | Each VM gets a dedicated TAP device for network connectivity | `vishvananda/netlink` Tuntap LinkAdd creates TAP device; wired to SDK via `StaticNetworkConfiguration.HostDevName`; unique name per VM: `tap-{short-id}` |
| NET-02 | Outbound traffic routes through iptables NAT to host network | `coreos/go-iptables` adds MASQUERADE in nat/POSTROUTING and ACCEPT in filter/FORWARD chains; IP forwarding enabled via sysctl |
| NET-03 | DNS resolution works inside the guest VM | SDK `IPConfiguration.Nameservers` writes to `/proc/net/pnp`; guest rootfs symlinks `/etc/resolv.conf -> /proc/net/pnp`; nameservers configurable (default: host's DNS or 8.8.8.8) |
| NET-04 | Network integrates with OpenSandbox's FQDN-based egress proxy | Egress proxy runs on host side; guest DNS routed through proxy's address as nameserver; proxy enforces allow/deny based on `NetworkPolicy` from sandbox creation |
| NET-05 | TAP devices are cleaned up on VM stop/destroy, including abnormal termination | `VMResources` extended with `TAPDeviceName` and `IPTablesRules`; `Cleanup()` calls `netlink.LinkDel` and `iptables.Delete`; idempotent removal handles already-deleted devices |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/firecracker-microvm/firecracker-go-sdk` | v1.0.0 | `NetworkInterface`, `StaticNetworkConfiguration`, `IPConfiguration` types; wires TAP device to Firecracker via `Config.NetworkInterfaces` | Already pinned in go.mod; provides the only supported path to configure Firecracker network interfaces [VERIFIED: go.mod, SDK source] |
| `github.com/vishvananda/netlink` | v1.3.1-0.20250303 | TAP device creation (`Tuntap` + `LinkAdd`), IP address assignment (`AddrAdd`), link up (`LinkSetUp`), link deletion (`LinkDel`) | Already an indirect dependency via firecracker-go-sdk; pure Go netlink implementation; no shell commands needed [VERIFIED: go.mod indirect] |
| `github.com/vishvananda/netns` | v0.0.5 | Network namespace management (future snapshot clone isolation) | Already an indirect dependency; used by SDK's network.go for netns-based isolation [VERIFIED: go.mod indirect] |
| `github.com/coreos/go-iptables` | v0.8.0 | iptables rule management: append/delete NAT MASQUERADE, FORWARD ACCEPT rules | Available in go module cache; latest stable is v0.8.0; provides typed API instead of shelling out to iptables binary [VERIFIED: go list -m -versions] |
| `net` | stdlib | `net.IPNet`, `net.IP`, `net.ParseCIDR` for IP addressing | Standard library; required by both netlink and SDK IPConfiguration [VERIFIED: Go stdlib] |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/hashicorp/go-multierror` | v1.1.1 | Aggregate cleanup errors (TAP + iptables + existing resources) | Already a direct dependency; used by existing `VMResources.Cleanup()` [VERIFIED: go.mod] |
| `crypto/rand` | stdlib | Generate MAC addresses for guest network interfaces | Need unique MAC per VM; use locally-administered address (02:xx:xx:xx:xx:xx) |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `vishvananda/netlink` (Go API) | `os/exec` + `ip tuntap add` shell commands | Shell is fragile, harder to test, requires parsing output; netlink is the kernel API |
| `coreos/go-iptables` (Go API) | `os/exec` + `iptables` shell commands | Shell is fragile; go-iptables handles locking, batching, and error parsing |
| Static TAP configuration | CNI configuration (firecracker-go-sdk `CNIConfiguration`) | CNI adds complexity (plugin binaries, conf.d files, cache dirs); static is simpler for single-interface VMs; CNI is better for multi-tenant production with network namespaces |
| /30 subnets per VM | Single bridge + all VMs on same /24 | /30 provides isolation (only host-guest pair); bridge approach risks ARP spoofing between VMs |
| `coreos/go-iptables` v0.8.0 | v0.6.0 (transitive) | v0.8.0 is latest stable; upgrading from the transitive v0.6.0 gets bug fixes and nftables compat |

**Installation:**
```bash
cd runtime/firecracker
# Promote indirect dependencies to direct (netlink, netns already in go.sum)
go get github.com/vishvananda/netlink@v1.3.1-0.20250303224720-0e7078ed04c8
go get github.com/vishvananda/netns@v0.0.5
go get github.com/coreos/go-iptables@v0.8.0
```

## Architecture Patterns

### Recommended Project Structure
```
runtime/firecracker/
  network.go           # NetworkConfig, TAPDevice provisioning, IP allocation, cleanup
  network_linux.go     # Linux-specific TAP/iptables implementation (build-tagged)
  network_test.go      # Unit tests for IP allocation, MAC generation, config validation
  network_linux_test.go # Integration tests for TAP creation (requires root, build-tagged)
  vm.go                # VMConfig extended with NetworkConfig field
  vm_linux.go          # toFirecrackerConfig() extended with NetworkInterfaces
  cleanup.go           # VMResources extended with TAPDeviceName, IPTablesRules
  manager_linux.go     # Manager.Create wires network setup before machine creation
```

### Pattern 1: Static TAP Configuration with SDK Integration
**What:** Create a TAP device on the host using netlink, assign a /30 subnet, and pass the TAP name to the SDK via `StaticNetworkConfiguration.HostDevName`. The SDK then tells Firecracker to use this pre-existing TAP device. The SDK's `IPConfiguration` configures the guest-side IP via the kernel `ip=` boot parameter.
**When to use:** Every VM creation path.
**Example:**
```go
// Source: firecracker-go-sdk network.go + official Firecracker network-setup.md
import (
    "net"
    sdk "github.com/firecracker-microvm/firecracker-go-sdk"
)

networkIfaces := sdk.NetworkInterfaces{{
    StaticConfiguration: &sdk.StaticNetworkConfiguration{
        MacAddress:  "02:FC:00:00:00:01", // locally-administered MAC
        HostDevName: "tap-abc123",         // pre-created TAP device
        IPConfiguration: &sdk.IPConfiguration{
            IPAddr:      net.IPNet{IP: net.ParseIP("172.16.0.2"), Mask: net.CIDRMask(30, 32)},
            Gateway:     net.ParseIP("172.16.0.1"),
            Nameservers: []string{"8.8.8.8", "8.8.4.4"},
        },
    },
}}
cfg.NetworkInterfaces = networkIfaces
```

### Pattern 2: /30 Subnet Allocation Per VM
**What:** Each VM gets a /30 subnet (4 IPs: network, host, guest, broadcast). For VM index N (0-based): host IP = `172.16.{(4*N+1)/256}.{(4*N+1)%256}`, guest IP = `172.16.{(4*N+2)/256}.{(4*N+2)%256}`. This supports 16,384 concurrent VMs in the 172.16.0.0/16 range.
**When to use:** TAP IP address allocation during VM creation.
**Example:**
```go
// Source: Firecracker docs network-setup.md, multiple guests section
type SubnetAllocation struct {
    HostIP    net.IP    // assigned to TAP device on host
    GuestIP   net.IP    // assigned to eth0 inside VM
    Subnet    net.IPNet // /30 subnet
    GatewayIP net.IP    // same as HostIP (host is the gateway)
}

func allocateSubnet(index uint32) SubnetAllocation {
    base := uint32(0xAC100000) // 172.16.0.0
    offset := index * 4
    hostIP := base + offset + 1
    guestIP := base + offset + 2
    return SubnetAllocation{
        HostIP:  uint32ToIP(hostIP),
        GuestIP: uint32ToIP(guestIP),
        Subnet: net.IPNet{
            IP:   uint32ToIP(base + offset),
            Mask: net.CIDRMask(30, 32),
        },
        GatewayIP: uint32ToIP(hostIP),
    }
}
```

### Pattern 3: iptables NAT Rules Per VM
**What:** Three iptables rules per VM: (1) MASQUERADE in nat/POSTROUTING for the guest IP, (2) ACCEPT in filter/FORWARD for the TAP device outbound, (3) ACCEPT in filter/FORWARD for RELATED,ESTABLISHED return traffic. Rules use specific source IP / interface name for precise cleanup.
**When to use:** After TAP device creation, before VM start.
**Example:**
```go
// Source: Firecracker docs network-setup.md + coreos/go-iptables API
import "github.com/coreos/go-iptables/iptables"

ipt, _ := iptables.New()

// NAT: masquerade guest traffic leaving via host's outbound interface
ipt.Append("nat", "POSTROUTING", "-o", hostIface, "-s", guestIP, "-j", "MASQUERADE")

// Forward: allow traffic from TAP to host's outbound interface
ipt.Append("filter", "FORWARD", "-i", tapName, "-o", hostIface, "-j", "ACCEPT")

// Forward: allow return traffic (RELATED,ESTABLISHED)
// NOTE: This rule is typically shared across all VMs; add once, not per-VM.
ipt.AppendUnique("filter", "FORWARD", "-m", "conntrack",
    "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT")
```

### Pattern 4: TAP Device Cleanup with Idempotent Removal
**What:** On VM destroy or crash, remove TAP device (which auto-removes the link and IP), then remove iptables rules. Use idempotent operations (ignore "not found" errors) since the device may already be gone after a crash.
**When to use:** `VMResources.Cleanup()` and Manager.Destroy().
**Example:**
```go
// Source: vishvananda/netlink API
import "github.com/vishvananda/netlink"

func cleanupTAP(tapName string) error {
    link, err := netlink.LinkByName(tapName)
    if err != nil {
        // Device already gone -- idempotent
        return nil
    }
    return netlink.LinkDel(link)
}
```

### Anti-Patterns to Avoid
- **Shell-out for TAP/iptables:** Using `os/exec` to run `ip tuntap add` or `iptables` commands. Fragile, hard to test, no error typing. Use netlink and go-iptables APIs instead.
- **Shared bridge for all VMs:** Putting all VMs on the same L2 bridge. VMs can ARP-spoof each other. Use /30 point-to-point subnets for isolation.
- **Hardcoded outbound interface:** Assuming the host's internet-facing interface is `eth0`. Detect the default route interface programmatically or make it configurable.
- **Global iptables rules without VM-specific scoping:** Rules must be scoped to the specific TAP device name and guest IP so they can be cleanly removed per-VM. Never add a blanket MASQUERADE for all of 172.16.0.0/16.
- **Skipping IP forwarding check:** `net.ipv4.ip_forward` must be 1 on the host. Check at manager startup and fail loudly if disabled.
- **TAP device name > 15 chars:** Linux interface names are limited to 15 characters (IFNAMSIZ). Use `tap-{short-id}` where short-id is the first 11 chars of the VM UUID.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TAP device creation | Shell out to `ip tuntap add` | `vishvananda/netlink` `Tuntap` + `LinkAdd` | Kernel netlink API; no parsing, no PATH dependency, typed errors |
| iptables rule management | Shell out to `iptables` binary | `coreos/go-iptables` `IPTables.Append/Delete` | Handles flock locking, iptables-nft compat, error parsing |
| Guest IP configuration | Boot script that runs `ip addr add` inside guest | SDK `IPConfiguration` via kernel `ip=` boot parameter | Guest-side IP is set by kernel at boot; no init script needed; works with any rootfs |
| MAC address generation | Manual hex string formatting | `crypto/rand` + locally-administered prefix `02:FC:xx:xx:xx:xx` | Ensures uniqueness; `02` bit = locally administered; `FC` = Firecracker mnemonic |
| Subnet arithmetic | Manual IP byte manipulation | `net.IP`, `encoding/binary.BigEndian` | Standard library; handles IPv4/IPv6, no byte-order bugs |
| DNS inside guest | Custom DNS proxy | Nameservers in `IPConfiguration` + symlink `/etc/resolv.conf -> /proc/net/pnp` | Firecracker kernel writes nameservers to /proc/net/pnp at boot; standard pattern |

**Key insight:** The firecracker-go-sdk handles the Firecracker API calls for network interface attachment. Our code only needs to: (a) create the TAP device and iptables rules on the host side BEFORE the SDK attaches the interface, and (b) clean them up AFTER the VM is destroyed. The SDK handles the kernel boot parameter injection for guest-side IP.

## Common Pitfalls

### Pitfall 1: TAP Device Name Length
**What goes wrong:** Linux interface names must be <= 15 characters (IFNAMSIZ-1). A name like `tap-550e8400-e29b-41d4-a716-446655440000` silently truncates or fails.
**Why it happens:** VM IDs are UUIDs (36 chars). Naive `tap-{vmID}` exceeds the limit.
**How to avoid:** Use `tap-{vmID[:11]}` (4 prefix + 11 UUID = 15 chars). Or use a short numeric index: `fc-tap{N}`.
**Warning signs:** `netlink.LinkAdd` returns `EINVAL` or the TAP name doesn't match what was requested.

### Pitfall 2: Forgotten IP Forwarding
**What goes wrong:** TAP device is up, iptables rules are in place, but guest traffic doesn't reach the internet.
**Why it happens:** `net.ipv4.ip_forward` defaults to 0 on many Linux distributions.
**How to avoid:** Check `sysctl net.ipv4.ip_forward` at Manager initialization. Return a clear error if it's 0. Optionally set it to 1 (requires CAP_NET_ADMIN).
**Warning signs:** Guest can ping the host TAP IP but not any external address.

### Pitfall 3: Stale iptables Rules After Crash
**What goes wrong:** If the manager process crashes after creating iptables rules but before cleanup, the rules persist. New VMs with the same TAP name or IP conflict with stale rules.
**Why it happens:** iptables rules survive process death; they're kernel state.
**How to avoid:** On manager startup, scan for orphaned rules matching a known prefix pattern (`tap-` device names, `172.16.x.x` source IPs). Use `iptables.List()` to find and `iptables.Delete()` to remove them. Also, use `iptables.AppendUnique()` for shared rules (RELATED,ESTABLISHED) to avoid duplicates.
**Warning signs:** Duplicate MASQUERADE rules accumulate; `iptables -L -t nat` shows multiple entries for the same IP.

### Pitfall 4: Hardcoded Host Outbound Interface
**What goes wrong:** iptables rules reference `eth0` but the host's internet interface is `ens3`, `enp0s31f6`, or `wlp2s0`.
**Why it happens:** Examples in Firecracker docs use `eth0`. Production hosts have unpredictable interface names.
**How to avoid:** Make the host outbound interface configurable in `NetworkConfig`. Provide a default that detects the interface with the default route (`netlink.RouteList` + find the one with `Dst == nil`).
**Warning signs:** iptables FORWARD rules reference a non-existent interface; guest traffic is silently dropped.

### Pitfall 5: Guest resolv.conf Not Symlinked
**What goes wrong:** DNS doesn't work inside the guest even though nameservers are configured in `IPConfiguration`.
**Why it happens:** The SDK writes nameservers to `/proc/net/pnp` via the kernel `ip=` boot parameter. But standard Linux distributions read `/etc/resolv.conf`, not `/proc/net/pnp`.
**How to avoid:** The rootfs image (provisioned in Phase 2) must have `/etc/resolv.conf` symlinked to `/proc/net/pnp`. Add this to the image provisioner or document it as a rootfs requirement.
**Warning signs:** `cat /proc/net/pnp` shows correct nameservers inside guest, but `nslookup` fails.

### Pitfall 6: Race Between TAP Creation and Machine.Start
**What goes wrong:** `Machine.Start()` is called before the TAP device is fully up with an IP address assigned. Firecracker tries to attach the interface but fails because it doesn't exist yet.
**Why it happens:** TAP creation and IP assignment are asynchronous netlink operations.
**How to avoid:** Create TAP, assign IP, and verify link is UP before calling `sdk.NewMachine()`. The SDK's `CreateNetworkInterfacesHandler` in the handler chain will then find the device ready.
**Warning signs:** `Machine.Start()` returns "host device not found" errors.

### Pitfall 7: Snapshot Restore Networking
**What goes wrong:** After snapshot restore, the guest has the original TAP device name baked into its network config, but the restored VM gets a new TAP device with a different name.
**Why it happens:** Firecracker snapshot preserves the guest's network stack state. The guest's `eth0` expects the original TAP device.
**How to avoid:** For Phase 4, create the TAP device with the same name format that the restore path will use. For snapshot restore (Phase 6), use network namespaces or Firecracker's `network_overrides` API parameter. Document the convention now to avoid rework.
**Warning signs:** Restored VM cannot reach the network despite TAP being created.

## Code Examples

### TAP Device Creation with netlink
```go
// Source: vishvananda/netlink link_test.go TUNTAP examples [VERIFIED: SDK source]
import (
    "fmt"
    "net"
    "github.com/vishvananda/netlink"
)

// createTAPDevice creates a TAP device, assigns an IP, and brings it up.
func createTAPDevice(name string, hostIP net.IP, subnet net.IPNet) error {
    tap := &netlink.Tuntap{
        LinkAttrs: netlink.LinkAttrs{Name: name},
        Mode:      netlink.TUNTAP_MODE_TAP,
    }

    if err := netlink.LinkAdd(tap); err != nil {
        return fmt.Errorf("firecracker: create tap %s: %w", name, err)
    }

    link, err := netlink.LinkByName(name)
    if err != nil {
        return fmt.Errorf("firecracker: find tap %s: %w", name, err)
    }

    addr := &netlink.Addr{
        IPNet: &net.IPNet{IP: hostIP, Mask: subnet.Mask},
    }
    if err := netlink.AddrAdd(link, addr); err != nil {
        return fmt.Errorf("firecracker: add addr to tap %s: %w", name, err)
    }

    if err := netlink.LinkSetUp(link); err != nil {
        return fmt.Errorf("firecracker: bring up tap %s: %w", name, err)
    }

    return nil
}
```

### iptables NAT Rule Management
```go
// Source: coreos/go-iptables API [VERIFIED: go list -m -versions]
import "github.com/coreos/go-iptables/iptables"

type NATRules struct {
    GuestIP   string
    TAPName   string
    HostIface string
}

func (r *NATRules) Apply(ipt *iptables.IPTables) error {
    // MASQUERADE guest traffic
    if err := ipt.AppendUnique("nat", "POSTROUTING",
        "-o", r.HostIface, "-s", r.GuestIP, "-j", "MASQUERADE"); err != nil {
        return fmt.Errorf("firecracker: add masquerade rule: %w", err)
    }

    // Allow forwarding from TAP to outbound
    if err := ipt.AppendUnique("filter", "FORWARD",
        "-i", r.TAPName, "-o", r.HostIface, "-j", "ACCEPT"); err != nil {
        return fmt.Errorf("firecracker: add forward rule: %w", err)
    }

    return nil
}

func (r *NATRules) Remove(ipt *iptables.IPTables) error {
    // Ignore errors -- idempotent cleanup
    _ = ipt.Delete("nat", "POSTROUTING",
        "-o", r.HostIface, "-s", r.GuestIP, "-j", "MASQUERADE")
    _ = ipt.Delete("filter", "FORWARD",
        "-i", r.TAPName, "-o", r.HostIface, "-j", "ACCEPT")
    return nil
}
```

### SDK NetworkInterfaces Integration
```go
// Source: firecracker-go-sdk example_test.go + network.go [VERIFIED: SDK source]
import (
    "net"
    sdk "github.com/firecracker-microvm/firecracker-go-sdk"
)

func buildNetworkInterfaces(tapName, macAddr string, guestIP, gateway net.IP, subnet net.IPNet, nameservers []string) sdk.NetworkInterfaces {
    return sdk.NetworkInterfaces{{
        StaticConfiguration: &sdk.StaticNetworkConfiguration{
            MacAddress:  macAddr,
            HostDevName: tapName,
            IPConfiguration: &sdk.IPConfiguration{
                IPAddr:      net.IPNet{IP: guestIP, Mask: subnet.Mask},
                Gateway:     gateway,
                Nameservers: nameservers, // max 2; written to /proc/net/pnp
            },
        },
    }}
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `iptables` (legacy) | `iptables-nft` (nftables backend) | Debian 10+ (2019), RHEL 8+ | `coreos/go-iptables` handles both transparently; v0.7.0+ added nftables detection [VERIFIED: go-iptables changelog] |
| Manual guest IP via boot script | SDK `IPConfiguration` kernel boot param | firecracker-go-sdk v0.22+ | No guest-side scripts needed; kernel sets IP before init runs |
| CNI with tc-redirect-tap plugin | Static TAP + netlink | Always available | CNI is the recommended path for production multi-tenant; static is simpler for single-interface VMs |

**Deprecated/outdated:**
- Raw `iptables` binary (legacy xtables backend): Still works but `iptables-nft` is the default on modern systems. `coreos/go-iptables` v0.7.0+ auto-detects the backend.
- Network namespaces for Phase 4: Not needed for single VM networking. Required later for snapshot clones (Phase 6). Design the TAP naming convention to be namespace-compatible but don't implement netns in this phase.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Guest rootfs already has `/etc/resolv.conf -> /proc/net/pnp` symlink or will be added in image provisioner | Common Pitfalls (Pitfall 5) | DNS won't work inside guest; need rootfs modification |
| A2 | Host outbound interface can be auto-detected via default route | Architecture Patterns (Pattern 3) | If multiple default routes exist or no default route, auto-detection fails; need explicit config |
| A3 | `coreos/go-iptables` v0.8.0 is compatible with go 1.24.0 / 1.25.7 | Standard Stack | Build failure; would need to pin older version |
| A4 | The egress proxy sidecar runs on the host and can be configured as the guest's DNS nameserver | Phase Requirements (NET-04) | If egress proxy uses a different enforcement mechanism (e.g., iptables FQDN matching instead of DNS interception), routing approach differs |
| A5 | 15-character interface name limit (IFNAMSIZ) applies to TAP devices | Common Pitfalls (Pitfall 1) | Would affect naming scheme |

## Open Questions

1. **Egress Proxy DNS Address**
   - What we know: The egress sidecar runs on port 18080 and enforces FQDN policy. The Go SDK's `DefaultEgressPort = 18080`. Enforcement mode is "dns" per the API response.
   - What's unclear: What IP address does the egress proxy listen on for DNS queries? Is it the host's TAP IP (172.16.x.1)? A separate loopback? The sandbox pod IP?
   - Recommendation: Configure the egress proxy's DNS listener IP as the guest's nameserver in `IPConfiguration.Nameservers`. If the proxy doesn't have a DNS listener, use host DNS (8.8.8.8) and rely on the proxy intercepting at the network level. This needs validation against a running OpenSandbox instance.

2. **Host Outbound Interface Detection**
   - What we know: Firecracker docs use `eth0`. Production hosts have varying interface names.
   - What's unclear: What interface name does the target deployment use?
   - Recommendation: Make it configurable in `ManagerConfig` with auto-detection via `netlink.RouteList(nil, netlink.FAMILY_V4)` + find route with `Dst == nil` as the fallback default.

3. **Shared RELATED,ESTABLISHED Rule**
   - What we know: The FORWARD chain rule `-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT` is needed for return traffic. It's a single global rule, not per-VM.
   - What's unclear: Should we add it per-manager-start and never remove it? Or track whether we added it?
   - Recommendation: Use `iptables.AppendUnique()` at manager startup. Don't remove it at shutdown (other processes may depend on it). Document that this rule is a prerequisite.

## Environment Availability

> This phase requires Linux-only system calls (TAP creation, netlink, iptables). Development happens on macOS.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Linux kernel (TAP/tun) | NET-01 TAP device creation | n/a (macOS dev) | -- | Build-tagged `_linux.go` files; unit tests mock netlink; integration tests require Linux |
| `iptables` / `iptables-nft` binary | NET-02 NAT rules (via go-iptables) | n/a (macOS dev) | -- | Build-tagged `_linux.go`; go-iptables auto-detects backend |
| `net.ipv4.ip_forward` sysctl | NET-02 IP forwarding | n/a (macOS dev) | -- | Check at runtime on Linux; error on macOS |
| Go 1.24.0+ | All code | available | 1.25.7 | -- |
| `vishvananda/netlink` | TAP management | available (indirect) | v1.3.1-pre | Promote to direct dependency |
| `coreos/go-iptables` | iptables rules | in go.sum | v0.6.0 indirect | Add as direct dep at v0.8.0 |

**Missing dependencies with no fallback:**
- Linux kernel: TAP and iptables operations are Linux-only. All network code must use `//go:build linux` build tags. Tests on macOS run unit tests only (mock-based).

**Missing dependencies with fallback:**
- None. All Go libraries are available; Linux kernel is the hard requirement.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` package (built-in) |
| Config file | None (standard `go test`) |
| Quick run command | `go test ./... -v -short` |
| Full suite command | `go test ./... -v -timeout 3m` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| NET-01 | TAP device created with correct name, mode, IP | unit | `go test -run TestNetworkConfig -v` | Wave 0 |
| NET-01 | TAP device creation via netlink (Linux) | integration | `go test -tags=integration -run TestTAPCreate -v` | Wave 0 |
| NET-02 | iptables NAT rules added and removed | unit | `go test -run TestIPTablesRules -v` | Wave 0 |
| NET-03 | IPConfiguration nameservers wired to SDK | unit | `go test -run TestNetworkInterfaces -v` | Wave 0 |
| NET-04 | Egress proxy DNS address used as nameserver | unit | `go test -run TestEgressNameserver -v` | Wave 0 |
| NET-05 | Cleanup removes TAP and iptables (idempotent) | unit | `go test -run TestNetworkCleanup -v` | Wave 0 |
| NET-05 | VMResources.Cleanup includes TAP cleanup | unit | `go test -run TestVMResourcesCleanup -v` | Exists (extend) |

### Sampling Rate
- **Per task commit:** `go test ./... -v -short`
- **Per wave merge:** `go test ./... -v -timeout 3m`
- **Phase gate:** Full suite green before verification

### Wave 0 Gaps
- [ ] `network_test.go` -- unit tests for NetworkConfig, SubnetAllocator, MAC generation, validation
- [ ] `network_linux_test.go` -- build-tagged integration tests for TAP creation/deletion (requires root)
- [ ] No new framework install needed; existing `go test` infrastructure covers all requirements

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | -- |
| V3 Session Management | no | -- |
| V4 Access Control | yes | FQDN-based egress policy enforcement via OpenSandbox egress proxy; deny-by-default |
| V5 Input Validation | yes | VMConfig NetworkConfig field validation (IP ranges, interface names, nameservers) |
| V6 Cryptography | no | -- |

### Known Threat Patterns for TAP Networking

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Guest spoofs source IP to bypass NAT | Spoofing | /30 subnet limits to 1 guest IP; iptables MASQUERADE scoped to specific source IP |
| Guest ARP-poisons other VMs | Tampering | /30 point-to-point subnet (only host+guest); no shared L2 segment between VMs |
| Leaked iptables rules after crash | Information Disclosure | Orphan rule cleanup on manager startup; unique per-VM rule specifications |
| TAP device name collision | Denial of Service | TAP name derived from VM UUID (guaranteed unique); create fails if name exists |
| Guest bypasses egress proxy via raw IP | Elevation of Privilege | Egress proxy enforces DNS-based; for IP-level blocking, iptables DROP rules needed (future enhancement) |

## Sources

### Primary (HIGH confidence)
- [firecracker-go-sdk v1.0.0 source code](/Users/alexandrephilippi/go/pkg/mod/github.com/firecracker-microvm/firecracker-go-sdk@v1.0.0/) -- `NetworkInterface`, `StaticNetworkConfiguration`, `IPConfiguration` types, `createNetworkInterface`, handler chain
- [firecracker-go-sdk go.mod](/Users/alexandrephilippi/work/ale_space/Projects/sharpi/experiments/world/tools/OpenSandbox/runtime/firecracker/go.mod) -- dependency versions, transitive netlink/go-iptables
- [vishvananda/netlink source](/Users/alexandrephilippi/go/pkg/mod/github.com/vishvananda/netlink@v1.3.1-0.20250303224720-0e7078ed04c8/) -- `Tuntap`, `LinkAdd`, `LinkDel`, `AddrAdd`, `LinkSetUp` APIs, `TUNTAP_MODE_TAP` constant
- [Firecracker network-setup.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/network-setup.md) -- TAP creation, iptables NAT, /30 subnets, multiple guests, cleanup

### Secondary (MEDIUM confidence)
- [Firecracker network-for-clones](https://jonathanwoollett-light.github.io/firecracker/book/book/network-for-clones.html) -- Network namespace approach for snapshot clones, veth pairs, SNAT/DNAT
- [OpenSandbox Go SDK egress client](origin/feat/go-sdk:sdks/sandbox/go/opensandbox/egress.go) -- EgressClient, OPENSANDBOX-EGRESS-AUTH header, /policy endpoint
- [OpenSandbox egress API spec](origin/feat/go-sdk:sdks/sandbox/go/opensandbox/api/specs/egress-api.yaml) -- PolicyStatusResponse, NetworkRule, enforcement modes
- [Existing Phase 1-3 research and plans](.planning/phases/) -- established codebase patterns, error types, VMResources, build tags

### Tertiary (LOW confidence)
- [coreos/go-iptables v0.8.0](https://github.com/coreos/go-iptables) -- API assumed from v0.6.0 source in go.sum; latest version not locally verified [ASSUMED]

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries verified in go.mod/go.sum or SDK source; API shapes confirmed from source code
- Architecture: HIGH -- patterns follow official Firecracker network-setup documentation; SDK types verified
- Pitfalls: HIGH -- derived from Firecracker docs, SDK source code analysis, and known Linux networking constraints

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable domain; Firecracker networking API is mature)

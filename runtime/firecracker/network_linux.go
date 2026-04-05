//go:build linux

package firecracker

import (
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/coreos/go-iptables/iptables"
	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/vishvananda/netlink"
)

// CreateTAPDevice creates a TAP device with the given name and assigns the
// host-side IP address from the subnet. The device is brought up before
// returning. On any error after device creation, a cleanup deletion is
// attempted.
func CreateTAPDevice(name string, hostIP net.IP, subnet net.IPNet) error {
	tap := &netlink.Tuntap{
		LinkAttrs: netlink.LinkAttrs{Name: name},
		Mode:      netlink.TUNTAP_MODE_TAP,
	}

	if err := netlink.LinkAdd(tap); err != nil {
		return fmt.Errorf("firecracker: create tap %s: %w", name, err)
	}

	// From here, attempt cleanup on any error.
	link, err := netlink.LinkByName(name)
	if err != nil {
		_ = deleteTAPLink(name)
		return fmt.Errorf("firecracker: find tap %s after creation: %w", name, err)
	}

	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   hostIP,
			Mask: subnet.Mask,
		},
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		_ = deleteTAPLink(name)
		return fmt.Errorf("firecracker: assign address to tap %s: %w", name, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		_ = deleteTAPLink(name)
		return fmt.Errorf("firecracker: bring up tap %s: %w", name, err)
	}

	return nil
}

// DeleteTAPDevice removes a TAP device by name. If the device does not exist,
// nil is returned (idempotent).
func DeleteTAPDevice(name string) error {
	return deleteTAPLink(name)
}

// deleteTAPLink is the internal helper for removing a TAP link by name.
func deleteTAPLink(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		// Device does not exist or cannot be found -- treat as already deleted.
		return nil
	}
	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("firecracker: delete tap %s: %w", name, err)
	}
	return nil
}

// Apply adds iptables NAT (MASQUERADE) and FORWARD rules for the VM's
// guest IP and TAP device. Rules are added idempotently via AppendUnique.
func (r *NATRules) Apply() error {
	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("firecracker: create iptables handle: %w", err)
	}

	// MASQUERADE: guest traffic leaving via host interface gets source NAT.
	if err := ipt.AppendUnique("nat", "POSTROUTING",
		"-o", r.HostIface,
		"-s", r.GuestIP,
		"-j", "MASQUERADE",
	); err != nil {
		return fmt.Errorf("firecracker: add nat masquerade rule: %w", err)
	}

	// FORWARD: allow traffic from TAP to host interface.
	if err := ipt.AppendUnique("filter", "FORWARD",
		"-i", r.TAPName,
		"-o", r.HostIface,
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("firecracker: add nat forward rule: %w", err)
	}

	return nil
}

// Remove deletes iptables NAT (MASQUERADE) and FORWARD rules for this VM.
// Missing rules are silently ignored (idempotent).
func (r *NATRules) Remove() error {
	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("firecracker: create iptables handle: %w", err)
	}

	// Remove MASQUERADE rule -- ignore "rule not found" errors.
	if err := ipt.Delete("nat", "POSTROUTING",
		"-o", r.HostIface,
		"-s", r.GuestIP,
		"-j", "MASQUERADE",
	); err != nil && !isRuleNotFoundError(err) {
		return fmt.Errorf("firecracker: remove nat masquerade rule: %w", err)
	}

	// Remove FORWARD rule -- ignore "rule not found" errors.
	if err := ipt.Delete("filter", "FORWARD",
		"-i", r.TAPName,
		"-o", r.HostIface,
		"-j", "ACCEPT",
	); err != nil && !isRuleNotFoundError(err) {
		return fmt.Errorf("firecracker: remove nat forward rule: %w", err)
	}

	return nil
}

// isRuleNotFoundError checks if an iptables error indicates the rule does
// not exist, which is expected during idempotent removal.
func isRuleNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "No chain/target/match") ||
		strings.Contains(msg, "does a matching rule exist") ||
		strings.Contains(msg, "Bad rule")
}

// EnsureIPForwarding checks that IPv4 forwarding is enabled on the host
// by reading /proc/sys/net/ipv4/ip_forward. Returns a descriptive error
// if forwarding is disabled.
func EnsureIPForwarding() error {
	data, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil {
		return fmt.Errorf("firecracker: check ip forwarding: %w", err)
	}
	val := strings.TrimSpace(string(data))
	if val != "1" {
		return fmt.Errorf("firecracker: ip forwarding disabled: set net.ipv4.ip_forward=1 via sysctl")
	}
	return nil
}

// EnsureSharedForwardRule adds a shared iptables FORWARD rule that allows
// return traffic for established connections. This should be called once at
// Manager startup, not per-VM.
func EnsureSharedForwardRule(hostIface string) error {
	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("firecracker: create iptables handle: %w", err)
	}

	if err := ipt.AppendUnique("filter", "FORWARD",
		"-m", "conntrack",
		"--ctstate", "RELATED,ESTABLISHED",
		"-j", "ACCEPT",
	); err != nil {
		return fmt.Errorf("firecracker: add shared forward rule: %w", err)
	}

	return nil
}

// DefaultHostInterface returns the name of the host's default route network
// interface by inspecting the IPv4 routing table via netlink.
func DefaultHostInterface() (string, error) {
	routes, err := netlink.RouteList(nil, netlink.FAMILY_V4)
	if err != nil {
		return "", fmt.Errorf("firecracker: list routes: %w", err)
	}
	for _, route := range routes {
		if route.Dst == nil {
			// Default route (no destination = 0.0.0.0/0).
			link, err := netlink.LinkByIndex(route.LinkIndex)
			if err != nil {
				return "", fmt.Errorf("firecracker: get link for default route: %w", err)
			}
			return link.Attrs().Name, nil
		}
	}
	return "", fmt.Errorf("firecracker: no default route found")
}

// BuildSDKNetworkInterfaces constructs a firecracker-go-sdk NetworkInterfaces
// value from the given network parameters. The result is suitable for inclusion
// in an sdk.Config.
func BuildSDKNetworkInterfaces(tapName, macAddr string, guestIP, gateway net.IP, subnet net.IPNet, nameservers []string) sdk.NetworkInterfaces {
	return sdk.NetworkInterfaces{
		{
			StaticConfiguration: &sdk.StaticNetworkConfiguration{
				MacAddress:  macAddr,
				HostDevName: tapName,
				IPConfiguration: &sdk.IPConfiguration{
					IPAddr: net.IPNet{
						IP:   guestIP,
						Mask: subnet.Mask,
					},
					Gateway:     gateway,
					Nameservers: nameservers,
				},
			},
		},
	}
}

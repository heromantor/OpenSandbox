//go:build linux

package firecracker

import (
	"net"
	"os"
	"testing"

	"github.com/vishvananda/netlink"
)

// requireRoot skips the test if not running as root.
func requireRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("requires root and Linux")
	}
}

func TestCreateTAPDevice(t *testing.T) {
	requireRoot(t)

	name := "tap-test-unit"
	hostIP := net.ParseIP("172.16.0.1")
	subnet := net.IPNet{
		IP:   net.ParseIP("172.16.0.0"),
		Mask: net.CIDRMask(30, 32),
	}

	// Ensure clean state.
	_ = DeleteTAPDevice(name)

	if err := CreateTAPDevice(name, hostIP, subnet); err != nil {
		t.Fatalf("CreateTAPDevice failed: %v", err)
	}
	defer func() {
		if err := DeleteTAPDevice(name); err != nil {
			t.Errorf("cleanup DeleteTAPDevice failed: %v", err)
		}
	}()

	// Verify device exists.
	link, err := netlink.LinkByName(name)
	if err != nil {
		t.Fatalf("TAP device %s not found after creation: %v", name, err)
	}

	// Verify IP address is assigned.
	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err != nil {
		t.Fatalf("failed to list addresses on %s: %v", name, err)
	}
	found := false
	for _, addr := range addrs {
		if addr.IP.Equal(hostIP) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected IP %s on %s, addresses: %v", hostIP, name, addrs)
	}

	// Verify link is up.
	attrs := link.Attrs()
	if attrs.OperState != netlink.OperUp && attrs.Flags&net.FlagUp == 0 {
		t.Errorf("expected TAP device to be UP, state: %s, flags: %v", attrs.OperState, attrs.Flags)
	}
}

func TestDeleteTAPDevice_Idempotent(t *testing.T) {
	requireRoot(t)

	// Deleting a non-existent device should return nil.
	err := DeleteTAPDevice("tap-nonexistent-999")
	if err != nil {
		t.Fatalf("DeleteTAPDevice of non-existent device should return nil, got: %v", err)
	}
}

func TestEnsureIPForwarding(t *testing.T) {
	requireRoot(t)

	// Just verify it doesn't panic -- the result depends on the system config.
	err := EnsureIPForwarding()
	if err != nil {
		t.Logf("EnsureIPForwarding returned error (may be expected): %v", err)
	} else {
		t.Logf("IP forwarding is enabled")
	}
}

func TestDefaultHostInterface(t *testing.T) {
	// This test doesn't require root but requires a system with a default route.
	iface, err := DefaultHostInterface()
	if err != nil {
		t.Skipf("no default route found (expected in some CI environments): %v", err)
	}
	if iface == "" {
		t.Error("DefaultHostInterface returned empty string")
	}
	t.Logf("default host interface: %s", iface)
}

func TestBuildSDKNetworkInterfaces(t *testing.T) {
	guestIP := net.ParseIP("172.16.0.2")
	gateway := net.ParseIP("172.16.0.1")
	subnet := net.IPNet{
		IP:   net.ParseIP("172.16.0.0"),
		Mask: net.CIDRMask(30, 32),
	}
	nameservers := []string{"8.8.8.8", "8.8.4.4"}

	ifaces := BuildSDKNetworkInterfaces("tap-test", "02:FC:00:11:22:33", guestIP, gateway, subnet, nameservers)

	if len(ifaces) != 1 {
		t.Fatalf("expected 1 network interface, got %d", len(ifaces))
	}
	iface := ifaces[0]
	if iface.StaticConfiguration == nil {
		t.Fatal("expected StaticConfiguration to be non-nil")
	}
	sc := iface.StaticConfiguration
	if sc.MacAddress != "02:FC:00:11:22:33" {
		t.Errorf("MacAddress: expected 02:FC:00:11:22:33, got %s", sc.MacAddress)
	}
	if sc.HostDevName != "tap-test" {
		t.Errorf("HostDevName: expected tap-test, got %s", sc.HostDevName)
	}
	if sc.IPConfiguration == nil {
		t.Fatal("expected IPConfiguration to be non-nil")
	}
	ipCfg := sc.IPConfiguration
	if ipCfg.IPAddr.IP.String() != "172.16.0.2" {
		t.Errorf("IPAddr.IP: expected 172.16.0.2, got %s", ipCfg.IPAddr.IP)
	}
	if ones, _ := ipCfg.IPAddr.Mask.Size(); ones != 30 {
		t.Errorf("IPAddr.Mask: expected /30, got /%d", ones)
	}
	if ipCfg.Gateway.String() != "172.16.0.1" {
		t.Errorf("Gateway: expected 172.16.0.1, got %s", ipCfg.Gateway)
	}
	if len(ipCfg.Nameservers) != 2 {
		t.Fatalf("Nameservers: expected 2, got %d", len(ipCfg.Nameservers))
	}
	if ipCfg.Nameservers[0] != "8.8.8.8" || ipCfg.Nameservers[1] != "8.8.4.4" {
		t.Errorf("Nameservers: expected [8.8.8.8, 8.8.4.4], got %v", ipCfg.Nameservers)
	}
}

func TestBuildSDKNetworkInterfaces_CustomNameservers(t *testing.T) {
	guestIP := net.ParseIP("172.16.0.2")
	gateway := net.ParseIP("172.16.0.1")
	subnet := net.IPNet{
		IP:   net.ParseIP("172.16.0.0"),
		Mask: net.CIDRMask(30, 32),
	}
	nameservers := []string{"1.1.1.1"}

	ifaces := BuildSDKNetworkInterfaces("tap-custom", "02:FC:AA:BB:CC:DD", guestIP, gateway, subnet, nameservers)
	if len(ifaces) != 1 {
		t.Fatalf("expected 1 network interface, got %d", len(ifaces))
	}
	ipCfg := ifaces[0].StaticConfiguration.IPConfiguration
	if len(ipCfg.Nameservers) != 1 || ipCfg.Nameservers[0] != "1.1.1.1" {
		t.Errorf("Nameservers: expected [1.1.1.1], got %v", ipCfg.Nameservers)
	}
}

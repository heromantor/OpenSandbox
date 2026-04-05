package firecracker

import (
	"errors"
	"net"
	"strings"
	"testing"
)

func TestSubnetAllocator_Index0(t *testing.T) {
	alloc, err := AllocateSubnet(0)
	if err != nil {
		t.Fatalf("AllocateSubnet(0) returned error: %v", err)
	}
	if got := alloc.HostIP.String(); got != "172.16.0.1" {
		t.Errorf("HostIP: expected 172.16.0.1, got %s", got)
	}
	if got := alloc.GuestIP.String(); got != "172.16.0.2" {
		t.Errorf("GuestIP: expected 172.16.0.2, got %s", got)
	}
	if got := alloc.Subnet.String(); got != "172.16.0.0/30" {
		t.Errorf("Subnet: expected 172.16.0.0/30, got %s", got)
	}
	if got := alloc.GatewayIP.String(); got != "172.16.0.1" {
		t.Errorf("GatewayIP: expected 172.16.0.1, got %s", got)
	}
}

func TestSubnetAllocator_Index1(t *testing.T) {
	alloc, err := AllocateSubnet(1)
	if err != nil {
		t.Fatalf("AllocateSubnet(1) returned error: %v", err)
	}
	if got := alloc.HostIP.String(); got != "172.16.0.5" {
		t.Errorf("HostIP: expected 172.16.0.5, got %s", got)
	}
	if got := alloc.GuestIP.String(); got != "172.16.0.6" {
		t.Errorf("GuestIP: expected 172.16.0.6, got %s", got)
	}
	if got := alloc.Subnet.String(); got != "172.16.0.4/30" {
		t.Errorf("Subnet: expected 172.16.0.4/30, got %s", got)
	}
}

func TestSubnetAllocator_Index255_CrossesOctetBoundary(t *testing.T) {
	// Index 255: offset = 255*4 = 1020
	// Base: 172.16.0.0 + 1020 = 172.16.3.252
	// HostIP: 172.16.3.253, GuestIP: 172.16.3.254
	alloc, err := AllocateSubnet(255)
	if err != nil {
		t.Fatalf("AllocateSubnet(255) returned error: %v", err)
	}
	if got := alloc.HostIP.String(); got != "172.16.3.253" {
		t.Errorf("HostIP: expected 172.16.3.253, got %s", got)
	}
	if got := alloc.GuestIP.String(); got != "172.16.3.254" {
		t.Errorf("GuestIP: expected 172.16.3.254, got %s", got)
	}
	if got := alloc.Subnet.String(); got != "172.16.3.252/30" {
		t.Errorf("Subnet: expected 172.16.3.252/30, got %s", got)
	}
}

func TestSubnetAllocator_MaxIndex(t *testing.T) {
	// Index 16383: offset = 16383*4 = 65532
	// Base: 172.16.0.0 + 65532 = 172.16.255.252
	// HostIP: 172.16.255.253, GuestIP: 172.16.255.254
	alloc, err := AllocateSubnet(MaxSubnetIndex)
	if err != nil {
		t.Fatalf("AllocateSubnet(%d) returned error: %v", MaxSubnetIndex, err)
	}
	if got := alloc.HostIP.String(); got != "172.16.255.253" {
		t.Errorf("HostIP: expected 172.16.255.253, got %s", got)
	}
	if got := alloc.GuestIP.String(); got != "172.16.255.254" {
		t.Errorf("GuestIP: expected 172.16.255.254, got %s", got)
	}
	if got := alloc.Subnet.String(); got != "172.16.255.252/30" {
		t.Errorf("Subnet: expected 172.16.255.252/30, got %s", got)
	}
}

func TestSubnetAllocator_ExceedsRange(t *testing.T) {
	_, err := AllocateSubnet(MaxSubnetIndex + 1)
	if err == nil {
		t.Fatal("AllocateSubnet(16384) should return error, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error should mention 'exceeds', got: %v", err)
	}
}

func TestGenerateMAC_Format(t *testing.T) {
	mac, err := GenerateMAC()
	if err != nil {
		t.Fatalf("GenerateMAC() returned error: %v", err)
	}
	// Must parse as a valid MAC address.
	hw, parseErr := net.ParseMAC(mac)
	if parseErr != nil {
		t.Fatalf("GenerateMAC() returned invalid MAC %q: %v", mac, parseErr)
	}
	// Must be 6 bytes.
	if len(hw) != 6 {
		t.Errorf("expected 6-byte MAC, got %d bytes", len(hw))
	}
	// Must have locally-administered bit set (02:FC prefix).
	if hw[0] != 0x02 {
		t.Errorf("expected first byte 0x02, got 0x%02x", hw[0])
	}
	if hw[1] != 0xFC {
		t.Errorf("expected second byte 0xFC, got 0x%02x", hw[1])
	}
}

func TestGenerateMAC_Uniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		mac, err := GenerateMAC()
		if err != nil {
			t.Fatalf("GenerateMAC() iteration %d returned error: %v", i, err)
		}
		if seen[mac] {
			t.Fatalf("duplicate MAC at iteration %d: %s", i, mac)
		}
		seen[mac] = true
	}
}

func TestTAPDeviceName_UUID(t *testing.T) {
	name := TAPDeviceName("550e8400-e29b-41d4-a716-446655440000")
	if name != "tap-550e8400-e2" {
		t.Errorf("expected tap-550e8400-e2, got %q", name)
	}
	if len(name) > maxTAPNameLen {
		t.Errorf("name exceeds %d chars: %q (len=%d)", maxTAPNameLen, name, len(name))
	}
}

func TestTAPDeviceName_ShortID(t *testing.T) {
	name := TAPDeviceName("abc")
	if name != "tap-abc" {
		t.Errorf("expected tap-abc, got %q", name)
	}
	if len(name) > maxTAPNameLen {
		t.Errorf("name exceeds %d chars: %q (len=%d)", maxTAPNameLen, name, len(name))
	}
}

func TestTAPDeviceName_ExactBoundary(t *testing.T) {
	// 11 char ID + "tap-" = 15 exactly
	name := TAPDeviceName("12345678901")
	if name != "tap-12345678901" {
		t.Errorf("expected tap-12345678901, got %q", name)
	}
	if len(name) != maxTAPNameLen {
		t.Errorf("expected exactly %d chars, got %d", maxTAPNameLen, len(name))
	}
}

func TestTAPDeviceName_LongerThan11(t *testing.T) {
	// 12 char ID should be truncated to 11
	name := TAPDeviceName("123456789012")
	if len(name) > maxTAPNameLen {
		t.Errorf("name exceeds %d chars: %q (len=%d)", maxTAPNameLen, name, len(name))
	}
	if name != "tap-12345678901" {
		t.Errorf("expected tap-12345678901, got %q", name)
	}
}

func TestNetworkConfig_Validate_RejectsEmptyHostInterface(t *testing.T) {
	cfg := NetworkConfig{
		SubnetIndex:   0,
		HostInterface: "",
		Nameservers:   []string{"8.8.8.8"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty HostInterface, got nil")
	}
	var cfgErr *InvalidVMConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected InvalidVMConfigError, got %T: %v", err, err)
	}
	if cfgErr.Field != "NetworkConfig.HostInterface" {
		t.Errorf("expected Field=NetworkConfig.HostInterface, got %s", cfgErr.Field)
	}
}

func TestNetworkConfig_Validate_RejectsInvalidSubnetIndex(t *testing.T) {
	cfg := NetworkConfig{
		SubnetIndex:   MaxSubnetIndex + 1,
		HostInterface: "eth0",
		Nameservers:   []string{"8.8.8.8"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for SubnetIndex > MaxSubnetIndex, got nil")
	}
}

func TestNetworkConfig_Validate_RejectsTooManyNameservers(t *testing.T) {
	cfg := NetworkConfig{
		SubnetIndex:   0,
		HostInterface: "eth0",
		Nameservers:   []string{"8.8.8.8", "8.8.4.4", "1.1.1.1"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for >2 nameservers, got nil")
	}
}

func TestNetworkConfig_Validate_AcceptsValid(t *testing.T) {
	cfg := NetworkConfig{
		SubnetIndex:   0,
		HostInterface: "eth0",
		Nameservers:   []string{"8.8.8.8", "8.8.4.4"},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error for valid config, got: %v", err)
	}
}

func TestNetworkConfig_Validate_AcceptsEmptyNameservers(t *testing.T) {
	cfg := NetworkConfig{
		SubnetIndex:   0,
		HostInterface: "eth0",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error for empty nameservers, got: %v", err)
	}
}

// BuildSDKNetworkInterfaces tests are in network_linux_test.go since the
// firecracker-go-sdk types only compile on Linux.

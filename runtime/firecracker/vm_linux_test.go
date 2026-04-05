//go:build linux

package firecracker

import (
	"os"
	"strings"
	"testing"
)

// TestToFirecrackerConfig_ReadOnlyRootfs verifies that VMConfig.ReadOnlyRootfs
// propagates to the firecracker-go-sdk Drive's IsReadOnly field. Covers both
// the Phase 1 default (writable) and the Phase 2 read-only shared drive path.
func TestToFirecrackerConfig_ReadOnlyRootfs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		readOnly bool
	}{
		{"writable_default", false},
		{"read_only_shared", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := VMConfig{
				ID:              "test-vm",
				VCPUs:           1,
				MemoryMiB:       256,
				KernelImagePath: "/tmp/kernel",
				RootfsPath:      "/tmp/rootfs.ext4",
				ReadOnlyRootfs:  tc.readOnly,
			}.withDefaults()
			sdkCfg, err := cfg.toFirecrackerConfig()
			if err != nil {
				t.Fatalf("toFirecrackerConfig: %v", err)
			}
			if len(sdkCfg.Drives) == 0 {
				t.Fatal("no drives")
			}
			got := sdkCfg.Drives[0].IsReadOnly
			if got == nil {
				t.Fatal("IsReadOnly is nil")
			}
			if *got != tc.readOnly {
				t.Fatalf("IsReadOnly = %v, want %v", *got, tc.readOnly)
			}
		})
	}
}

// TestToFirecrackerConfig_VsockDevice verifies that VMConfig.VsockCID correctly
// populates or omits VsockDevices in the firecracker-go-sdk Config.
func TestToFirecrackerConfig_VsockDevice(t *testing.T) {
	for _, tc := range []struct {
		name           string
		vsockCID       uint32
		jailerEnabled  bool
		expectDevices  int
		expectCID      uint32
		expectPathHas  string // substring expected in Path
		expectPathExact string // exact Path match (if non-empty)
	}{
		{
			name:          "enabled_cid5",
			vsockCID:      5,
			jailerEnabled: false,
			expectDevices: 1,
			expectCID:     5,
			expectPathHas: "firecracker-vsock-test-vm.sock",
		},
		{
			name:          "disabled_cid0",
			vsockCID:      0,
			jailerEnabled: false,
			expectDevices: 0,
		},
		{
			name:            "jailed_cid3",
			vsockCID:        3,
			jailerEnabled:   true,
			expectDevices:   1,
			expectCID:       3,
			expectPathExact: "vsock.sock",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kernel := writeTemp(t, "vmlinux", "fake-kernel")
			rootfs := writeTemp(t, "rootfs.ext4", "fake-rootfs")

			cfg := VMConfig{
				ID:              "test-vm",
				VCPUs:           1,
				MemoryMiB:       256,
				KernelImagePath: kernel,
				RootfsPath:      rootfs,
				VsockCID:        tc.vsockCID,
				JailerEnabled:   tc.jailerEnabled,
			}.withDefaults()

			// For jailed VMs, supply valid jailer opts so toFirecrackerConfig succeeds.
			if tc.jailerEnabled {
				cfg.Jailer = JailerOpts{
					UID:           1000,
					GID:           1000,
					ChrootBaseDir: "/srv/jailer",
				}
			}

			sdkCfg, err := cfg.toFirecrackerConfig()
			if err != nil {
				t.Fatalf("toFirecrackerConfig: %v", err)
			}

			if len(sdkCfg.VsockDevices) != tc.expectDevices {
				t.Fatalf("VsockDevices len = %d, want %d", len(sdkCfg.VsockDevices), tc.expectDevices)
			}

			if tc.expectDevices == 0 {
				return
			}

			dev := sdkCfg.VsockDevices[0]
			if dev.ID != "vsock0" {
				t.Errorf("VsockDevices[0].ID = %q, want %q", dev.ID, "vsock0")
			}
			if dev.CID != tc.expectCID {
				t.Errorf("VsockDevices[0].CID = %d, want %d", dev.CID, tc.expectCID)
			}
			if tc.expectPathExact != "" && dev.Path != tc.expectPathExact {
				t.Errorf("VsockDevices[0].Path = %q, want %q", dev.Path, tc.expectPathExact)
			}
			if tc.expectPathHas != "" && !strings.Contains(dev.Path, tc.expectPathHas) {
				t.Errorf("VsockDevices[0].Path = %q, want to contain %q", dev.Path, tc.expectPathHas)
			}
		})
	}
}

// TestToFirecrackerConfig_NilNetworkConfig verifies backward compatibility:
// when NetworkConfig is nil, cfg.NetworkInterfaces remains nil.
func TestToFirecrackerConfig_NilNetworkConfig(t *testing.T) {
	cfg := VMConfig{
		ID:              "test-vm",
		VCPUs:           1,
		MemoryMiB:       256,
		KernelImagePath: "/tmp/kernel",
		RootfsPath:      "/tmp/rootfs.ext4",
		NetworkConfig:   nil,
	}.withDefaults()

	sdkCfg, err := cfg.toFirecrackerConfig()
	if err != nil {
		t.Fatalf("toFirecrackerConfig: %v", err)
	}
	if sdkCfg.NetworkInterfaces != nil {
		t.Fatalf("expected nil NetworkInterfaces when NetworkConfig is nil, got %v", sdkCfg.NetworkInterfaces)
	}
}

// TestToFirecrackerConfig_WithNetworkConfig verifies that a valid NetworkConfig
// produces cfg.NetworkInterfaces with correct StaticConfiguration fields.
func TestToFirecrackerConfig_WithNetworkConfig(t *testing.T) {
	cfg := VMConfig{
		ID:              "net-test-vm",
		VCPUs:           1,
		MemoryMiB:       256,
		KernelImagePath: "/tmp/kernel",
		RootfsPath:      "/tmp/rootfs.ext4",
		NetworkConfig: &NetworkConfig{
			SubnetIndex:   0,
			HostInterface: "eth0",
		},
	}.withDefaults()

	sdkCfg, err := cfg.toFirecrackerConfig()
	if err != nil {
		t.Fatalf("toFirecrackerConfig: %v", err)
	}
	if len(sdkCfg.NetworkInterfaces) == 0 {
		t.Fatal("expected non-empty NetworkInterfaces")
	}

	ni := sdkCfg.NetworkInterfaces[0]
	if ni.StaticConfiguration == nil {
		t.Fatal("expected non-nil StaticConfiguration")
	}
	if ni.StaticConfiguration.HostDevName == "" {
		t.Error("expected non-empty HostDevName")
	}
	if ni.StaticConfiguration.MacAddress == "" {
		t.Error("expected non-empty MacAddress")
	}
	if ni.StaticConfiguration.IPConfiguration == nil {
		t.Fatal("expected non-nil IPConfiguration")
	}

	ipCfg := ni.StaticConfiguration.IPConfiguration
	// Subnet index 0: GuestIP = 172.16.0.2, Gateway = 172.16.0.1
	if got := ipCfg.IPAddr.IP.String(); got != "172.16.0.2" {
		t.Errorf("expected GuestIP 172.16.0.2, got %s", got)
	}
	if got := ipCfg.Gateway.String(); got != "172.16.0.1" {
		t.Errorf("expected Gateway 172.16.0.1, got %s", got)
	}

	// Default nameservers should be used when NetworkConfig.Nameservers is nil.
	if len(ipCfg.Nameservers) != 2 {
		t.Fatalf("expected 2 default nameservers, got %d", len(ipCfg.Nameservers))
	}
	if ipCfg.Nameservers[0] != "8.8.8.8" {
		t.Errorf("expected first nameserver 8.8.8.8, got %s", ipCfg.Nameservers[0])
	}
}

// TestToFirecrackerConfig_NetworkConfigCustomNameservers verifies that custom
// nameservers from NetworkConfig are passed through to IPConfiguration.
func TestToFirecrackerConfig_NetworkConfigCustomNameservers(t *testing.T) {
	cfg := VMConfig{
		ID:              "dns-test-vm",
		VCPUs:           1,
		MemoryMiB:       256,
		KernelImagePath: "/tmp/kernel",
		RootfsPath:      "/tmp/rootfs.ext4",
		NetworkConfig: &NetworkConfig{
			SubnetIndex:   1,
			HostInterface: "eth0",
			Nameservers:   []string{"1.1.1.1", "9.9.9.9"},
		},
	}.withDefaults()

	sdkCfg, err := cfg.toFirecrackerConfig()
	if err != nil {
		t.Fatalf("toFirecrackerConfig: %v", err)
	}
	if len(sdkCfg.NetworkInterfaces) == 0 {
		t.Fatal("expected non-empty NetworkInterfaces")
	}

	ipCfg := sdkCfg.NetworkInterfaces[0].StaticConfiguration.IPConfiguration
	if len(ipCfg.Nameservers) != 2 {
		t.Fatalf("expected 2 custom nameservers, got %d", len(ipCfg.Nameservers))
	}
	if ipCfg.Nameservers[0] != "1.1.1.1" {
		t.Errorf("expected first nameserver 1.1.1.1, got %s", ipCfg.Nameservers[0])
	}
	if ipCfg.Nameservers[1] != "9.9.9.9" {
		t.Errorf("expected second nameserver 9.9.9.9, got %s", ipCfg.Nameservers[1])
	}
}

// writeTemp creates a temporary file with the given name and content, returning its path.
func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := t.TempDir() + "/" + name
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

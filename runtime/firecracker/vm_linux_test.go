//go:build linux

package firecracker

import "testing"

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

package firecracker

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validConfig returns a VMConfig with all required fields set, using temporary
// files for kernel and rootfs paths.
func validConfig(t *testing.T) VMConfig {
	t.Helper()
	kernel := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernel, []byte("fake-kernel"), 0644); err != nil {
		t.Fatalf("write fake kernel: %v", err)
	}
	rootfs := filepath.Join(t.TempDir(), "rootfs.ext4")
	if err := os.WriteFile(rootfs, []byte("fake-rootfs"), 0644); err != nil {
		t.Fatalf("write fake rootfs: %v", err)
	}
	return VMConfig{
		VCPUs:           2,
		MemoryMiB:       256,
		KernelImagePath: kernel,
		RootfsPath:      rootfs,
	}
}

func TestVMConfigValidate_ValidConfig(t *testing.T) {
	cfg := validConfig(t)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected nil error for valid config, got: %v", err)
	}
}

func TestVMConfigValidate_VCPUsOutOfRange(t *testing.T) {
	tests := []struct {
		name  string
		vcpus int64
	}{
		{"zero", 0},
		{"negative", -1},
		{"above_max", 33},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.VCPUs = tt.vcpus
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for VCPUs=%d, got nil", tt.vcpus)
			}
			if !strings.Contains(err.Error(), "VCPUs") {
				t.Errorf("error should mention VCPUs, got: %v", err)
			}
			var cfgErr *InvalidVMConfigError
			if ok := errors.As(err, &cfgErr); !ok {
				t.Fatalf("expected InvalidVMConfigError, got %T", err)
			}
			if cfgErr.Field != "VCPUs" {
				t.Errorf("expected Field=VCPUs, got %s", cfgErr.Field)
			}
		})
	}
}

func TestVMConfigValidate_VCPUsBoundaryValues(t *testing.T) {
	// VCPUs=1 (lower bound) should be valid.
	cfg := validConfig(t)
	cfg.VCPUs = 1
	if err := cfg.Validate(); err != nil {
		t.Errorf("VCPUs=1 should be valid, got: %v", err)
	}

	// VCPUs=32 (upper bound) should be valid.
	cfg2 := validConfig(t)
	cfg2.VCPUs = 32
	if err := cfg2.Validate(); err != nil {
		t.Errorf("VCPUs=32 should be valid, got: %v", err)
	}
}

func TestVMConfigValidate_MemoryTooLow(t *testing.T) {
	tests := []struct {
		name string
		mem  int64
	}{
		{"zero", 0},
		{"below_min", 64},
		{"boundary_below", 127},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.MemoryMiB = tt.mem
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for MemoryMiB=%d, got nil", tt.mem)
			}
			if !strings.Contains(err.Error(), "MemoryMiB") {
				t.Errorf("error should mention MemoryMiB, got: %v", err)
			}
		})
	}
}

func TestVMConfigValidate_MemoryBoundary(t *testing.T) {
	cfg := validConfig(t)
	cfg.MemoryMiB = 128
	if err := cfg.Validate(); err != nil {
		t.Errorf("MemoryMiB=128 should be valid, got: %v", err)
	}
}

func TestVMConfigValidate_MissingKernel(t *testing.T) {
	cfg := validConfig(t)
	cfg.KernelImagePath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty KernelImagePath, got nil")
	}
	if !strings.Contains(err.Error(), "KernelImagePath") {
		t.Errorf("error should mention KernelImagePath, got: %v", err)
	}
}

func TestVMConfigValidate_MissingRootfs(t *testing.T) {
	cfg := validConfig(t)
	cfg.RootfsPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty RootfsPath, got nil")
	}
	if !strings.Contains(err.Error(), "RootfsPath") {
		t.Errorf("error should mention RootfsPath, got: %v", err)
	}
}

func TestVMConfigValidate_InvalidID(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"special_chars", "vm!@#"},
		{"spaces", "vm 123"},
		{"dots", "vm.123"},
		{"underscores", "vm_123"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.ID = tt.id
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for ID=%q, got nil", tt.id)
			}
			if !strings.Contains(err.Error(), "ID") {
				t.Errorf("error should mention ID, got: %v", err)
			}
		})
	}
}

func TestVMConfigValidate_ValidIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"empty_auto_gen", ""},
		{"alphanumeric", "abc123"},
		{"with_hyphens", "vm-123-abc"},
		{"uuid_style", "550e8400-e29b-41d4-a716-446655440000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.ID = tt.id
			if err := cfg.Validate(); err != nil {
				t.Errorf("expected valid ID=%q, got error: %v", tt.id, err)
			}
		})
	}
}

func TestVMConfig_ReadOnlyRootfs_DefaultsFalse(t *testing.T) {
	cfg := VMConfig{}.withDefaults()
	if cfg.ReadOnlyRootfs {
		t.Error("expected ReadOnlyRootfs=false by default (Phase 1 writable behavior)")
	}
}

func TestVMConfig_ReadOnlyRootfs_Preserved(t *testing.T) {
	cfg := VMConfig{ReadOnlyRootfs: true}.withDefaults()
	if !cfg.ReadOnlyRootfs {
		t.Error("expected ReadOnlyRootfs=true to be preserved after withDefaults")
	}
}

func TestVMConfigDefaults(t *testing.T) {
	cfg := VMConfig{}
	filled := cfg.withDefaults()

	if filled.ID == "" {
		t.Error("expected non-empty ID after withDefaults")
	}
	if filled.VCPUs != 1 {
		t.Errorf("expected VCPUs=1, got %d", filled.VCPUs)
	}
	if filled.MemoryMiB != 256 {
		t.Errorf("expected MemoryMiB=256, got %d", filled.MemoryMiB)
	}
	if !strings.Contains(filled.KernelArgs, "console=ttyS0") {
		t.Errorf("expected KernelArgs to contain 'console=ttyS0', got %q", filled.KernelArgs)
	}
	if filled.LogLevel != "Error" {
		t.Errorf("expected LogLevel=Error, got %q", filled.LogLevel)
	}
	if filled.FirecrackerBin != "/usr/bin/firecracker" {
		t.Errorf("expected FirecrackerBin=/usr/bin/firecracker, got %q", filled.FirecrackerBin)
	}
	if filled.JailerBin != "/usr/bin/jailer" {
		t.Errorf("expected JailerBin=/usr/bin/jailer, got %q", filled.JailerBin)
	}
}

func TestVMConfigDefaults_PreservesExplicitValues(t *testing.T) {
	cfg := VMConfig{
		VCPUs:     4,
		MemoryMiB: 512,
		LogLevel:  "Debug",
	}
	filled := cfg.withDefaults()

	if filled.VCPUs != 4 {
		t.Errorf("expected VCPUs=4 preserved, got %d", filled.VCPUs)
	}
	if filled.MemoryMiB != 512 {
		t.Errorf("expected MemoryMiB=512 preserved, got %d", filled.MemoryMiB)
	}
	if filled.LogLevel != "Debug" {
		t.Errorf("expected LogLevel=Debug preserved, got %q", filled.LogLevel)
	}
	// ID should still be auto-generated since it was empty.
	if filled.ID == "" {
		t.Error("expected non-empty ID after withDefaults even with explicit values")
	}
}

func TestVMState_Constants(t *testing.T) {
	// Verify state constants have expected string representations.
	if StateCreated != "Created" {
		t.Errorf("expected StateCreated='Created', got %q", StateCreated)
	}
	if StateStarting != "Starting" {
		t.Errorf("expected StateStarting='Starting', got %q", StateStarting)
	}
	if StateRunning != "Running" {
		t.Errorf("expected StateRunning='Running', got %q", StateRunning)
	}
	if StateStopping != "Stopping" {
		t.Errorf("expected StateStopping='Stopping', got %q", StateStopping)
	}
	if StateStopped != "Stopped" {
		t.Errorf("expected StateStopped='Stopped', got %q", StateStopped)
	}
}


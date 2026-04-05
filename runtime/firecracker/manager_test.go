package firecracker

import (
	"errors"
	"testing"
)

func TestManagerConfig_Defaults(t *testing.T) {
	cfg := ManagerConfig{}
	filled := cfg.withDefaults()

	if filled.ChrootBaseDir != "/srv/jailer" {
		t.Errorf("expected ChrootBaseDir=/srv/jailer, got %q", filled.ChrootBaseDir)
	}
	if filled.DefaultVCPUs != 1 {
		t.Errorf("expected DefaultVCPUs=1, got %d", filled.DefaultVCPUs)
	}
	if filled.DefaultMemoryMiB != 256 {
		t.Errorf("expected DefaultMemoryMiB=256, got %d", filled.DefaultMemoryMiB)
	}
	if filled.LogLevel != "Error" {
		t.Errorf("expected LogLevel=Error, got %q", filled.LogLevel)
	}
}

func TestManagerConfig_PreservesExplicitValues(t *testing.T) {
	cfg := ManagerConfig{
		ChrootBaseDir:  "/custom/jailer",
		DefaultVCPUs:   4,
		DefaultMemoryMiB: 1024,
		LogLevel:       "Debug",
	}
	filled := cfg.withDefaults()

	if filled.ChrootBaseDir != "/custom/jailer" {
		t.Errorf("expected ChrootBaseDir=/custom/jailer preserved, got %q", filled.ChrootBaseDir)
	}
	if filled.DefaultVCPUs != 4 {
		t.Errorf("expected DefaultVCPUs=4 preserved, got %d", filled.DefaultVCPUs)
	}
	if filled.DefaultMemoryMiB != 1024 {
		t.Errorf("expected DefaultMemoryMiB=1024 preserved, got %d", filled.DefaultMemoryMiB)
	}
	if filled.LogLevel != "Debug" {
		t.Errorf("expected LogLevel=Debug preserved, got %q", filled.LogLevel)
	}
}

// Error type tests -- these are cross-platform since error types are in errors.go.

func TestVMNotFoundError(t *testing.T) {
	err := &VMNotFoundError{VMID: "test-vm-123"}
	msg := err.Error()
	if msg != "firecracker: vm not found: test-vm-123" {
		t.Errorf("unexpected error message: %s", msg)
	}

	// Verify errors.As works.
	var target *VMNotFoundError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should find VMNotFoundError")
	}
	if target.VMID != "test-vm-123" {
		t.Errorf("expected VMID=test-vm-123, got %s", target.VMID)
	}
}

func TestVMAlreadyExistsError(t *testing.T) {
	err := &VMAlreadyExistsError{VMID: "dup-vm"}
	msg := err.Error()
	if msg != "firecracker: vm already exists: dup-vm" {
		t.Errorf("unexpected error message: %s", msg)
	}

	var target *VMAlreadyExistsError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should find VMAlreadyExistsError")
	}
	if target.VMID != "dup-vm" {
		t.Errorf("expected VMID=dup-vm, got %s", target.VMID)
	}
}

func TestInvalidVMConfigError(t *testing.T) {
	err := &InvalidVMConfigError{Field: "VCPUs", Message: "must be in range [1, 32]"}
	msg := err.Error()
	expected := "firecracker: invalid config: VCPUs: must be in range [1, 32]"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}

	var target *InvalidVMConfigError
	if !errors.As(err, &target) {
		t.Fatal("errors.As should find InvalidVMConfigError")
	}
	if target.Field != "VCPUs" {
		t.Errorf("expected Field=VCPUs, got %s", target.Field)
	}
}

func TestVMStartError(t *testing.T) {
	cause := errors.New("connection refused")
	err := &VMStartError{VMID: "start-vm", Cause: cause}
	msg := err.Error()
	if msg != "firecracker: vm start failed: start-vm: connection refused" {
		t.Errorf("unexpected error message: %s", msg)
	}

	// Test Unwrap.
	if !errors.Is(err, cause) {
		t.Error("Unwrap should expose the underlying cause")
	}
}

func TestVMStopError(t *testing.T) {
	cause := errors.New("process not running")
	err := &VMStopError{VMID: "stop-vm", Cause: cause}
	msg := err.Error()
	if msg != "firecracker: vm stop failed: stop-vm: process not running" {
		t.Errorf("unexpected error message: %s", msg)
	}

	if !errors.Is(err, cause) {
		t.Error("Unwrap should expose the underlying cause")
	}
}

func TestCleanupError(t *testing.T) {
	errs := []error{
		errors.New("remove socket: permission denied"),
		errors.New("remove chroot: device busy"),
	}
	err := &CleanupError{VMID: "cleanup-vm", Errors: errs}
	msg := err.Error()
	if msg != "firecracker: cleanup failed: cleanup-vm: remove socket: permission denied; remove chroot: device busy" {
		t.Errorf("unexpected error message: %s", msg)
	}
}

func TestCleanupError_NoErrors(t *testing.T) {
	err := &CleanupError{VMID: "empty-vm", Errors: nil}
	msg := err.Error()
	if msg != "firecracker: cleanup failed: empty-vm: no errors" {
		t.Errorf("unexpected error message: %s", msg)
	}
}

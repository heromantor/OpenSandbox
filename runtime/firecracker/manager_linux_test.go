//go:build linux

package firecracker

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestManagerCreate_ValidConfig(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)

	vm, err := mgr.Create(context.Background(), cfg)
	if runtime.GOOS != "linux" {
		// sdk.NewMachine may fail on non-Linux -- that's expected.
		if err != nil {
			t.Skipf("Manager.Create requires Linux: %v", err)
		}
	}
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if vm.State != StateCreated {
		t.Errorf("expected StateCreated, got %s", vm.State)
	}
	if vm.ID == "" {
		t.Error("expected non-empty VM ID")
	}
}

func TestManagerCreate_InvalidConfig(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := VMConfig{
		VCPUs: 0, // invalid
	}

	_, err := mgr.Create(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
}

func TestManagerCreate_DuplicateID(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.ID = "duplicate-vm"

	_, err := mgr.Create(context.Background(), cfg)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	_, err = mgr.Create(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
	var existsErr *VMAlreadyExistsError
	if !errors.As(err, &existsErr) {
		t.Fatalf("expected VMAlreadyExistsError, got %T: %v", err, err)
	}
	if existsErr.VMID != "duplicate-vm" {
		t.Errorf("expected VMID=duplicate-vm, got %s", existsErr.VMID)
	}
}

func TestManagerGet_Exists(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.ID = "get-vm"

	created, err := mgr.Create(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := mgr.Get(context.Background(), "get-vm")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("expected ID=%s, got %s", created.ID, got.ID)
	}
	if got.State != StateCreated {
		t.Errorf("expected StateCreated, got %s", got.State)
	}
}

func TestManagerGet_ReturnsCopy(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.ID = "copy-vm"

	_, err := mgr.Create(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := mgr.Get(context.Background(), "copy-vm")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Modify the returned copy.
	got.State = StateRunning

	// Re-get and verify original is unchanged.
	got2, err := mgr.Get(context.Background(), "copy-vm")
	if err != nil {
		t.Fatalf("Get (second) failed: %v", err)
	}
	if got2.State != StateCreated {
		t.Errorf("Get should return a copy; modifying it should not affect stored VM. expected StateCreated, got %s", got2.State)
	}
}

func TestManagerGet_NotFound(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	_, err := mgr.Get(context.Background(), "nonexistent")
	var nfErr *VMNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected VMNotFoundError, got %T: %v", err, err)
	}
	if nfErr.VMID != "nonexistent" {
		t.Errorf("expected VMID=nonexistent, got %s", nfErr.VMID)
	}
}

func TestManagerStart_NotFound(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	err := mgr.Start(context.Background(), "nonexistent")
	var nfErr *VMNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected VMNotFoundError, got %T: %v", err, err)
	}
}

func TestManagerStop_NotFound(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	err := mgr.Stop(context.Background(), "nonexistent")
	var nfErr *VMNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected VMNotFoundError, got %T: %v", err, err)
	}
}

func TestManagerDestroy_NotFound(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	err := mgr.Destroy(context.Background(), "nonexistent")
	var nfErr *VMNotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected VMNotFoundError, got %T: %v", err, err)
	}
}

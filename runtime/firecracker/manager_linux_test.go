//go:build linux

package firecracker

import (
	"context"
	"errors"
	"runtime"
	"strings"
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

// skipIfCreateFails is a helper that creates a VM and skips the test if
// sdk.NewMachine fails (expected on non-Linux or when Firecracker is absent).
func skipIfCreateFails(t *testing.T, mgr *Manager, cfg VMConfig) *VM {
	t.Helper()
	vm, err := mgr.Create(context.Background(), cfg)
	if err != nil {
		if runtime.GOOS != "linux" {
			t.Skipf("Manager.Create requires Linux: %v", err)
		}
		t.Fatalf("Create failed: %v", err)
	}
	return vm
}

func TestManager_Create_AutoAssignsCID(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.VsockCID = 0 // auto-assign

	vm := skipIfCreateFails(t, mgr, cfg)

	if vm.VsockCID < MinGuestCID {
		t.Errorf("expected VsockCID >= %d, got %d", MinGuestCID, vm.VsockCID)
	}
}

func TestManager_Create_TwoCIDsUnique(t *testing.T) {
	mgr := NewManager(ManagerConfig{})

	cfg1 := validConfig(t)
	cfg1.VsockCID = 0
	vm1 := skipIfCreateFails(t, mgr, cfg1)

	cfg2 := validConfig(t)
	cfg2.VsockCID = 0
	vm2 := skipIfCreateFails(t, mgr, cfg2)

	if vm1.VsockCID == vm2.VsockCID {
		t.Errorf("expected unique CIDs, both got %d", vm1.VsockCID)
	}
}

func TestManager_Create_ExplicitCID(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.VsockCID = 42

	vm := skipIfCreateFails(t, mgr, cfg)

	if vm.VsockCID != 42 {
		t.Errorf("expected VsockCID=42, got %d", vm.VsockCID)
	}
}

func TestManager_Create_SetsVsockUDSPath(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.VsockCID = 0

	vm := skipIfCreateFails(t, mgr, cfg)

	if vm.VsockUDSPath == "" {
		t.Error("expected non-empty VsockUDSPath")
	}
	if !strings.Contains(vm.VsockUDSPath, vm.ID) {
		t.Errorf("expected VsockUDSPath to contain VM ID %q, got %q", vm.ID, vm.VsockUDSPath)
	}
}

func TestManager_Create_TracksVsockUDSInResources(t *testing.T) {
	mgr := NewManager(ManagerConfig{})
	cfg := validConfig(t)
	cfg.VsockCID = 0

	vm := skipIfCreateFails(t, mgr, cfg)

	if vm.Resources.VsockUDSPath == "" {
		t.Error("expected non-empty Resources.VsockUDSPath")
	}
	if vm.Resources.VsockUDSPath != vm.VsockUDSPath {
		t.Errorf("expected Resources.VsockUDSPath=%q to match VsockUDSPath=%q",
			vm.Resources.VsockUDSPath, vm.VsockUDSPath)
	}
}

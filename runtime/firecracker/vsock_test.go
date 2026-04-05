package firecracker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCIDAllocator_StartsAtMinGuestCID(t *testing.T) {
	alloc := NewCIDAllocator(MinGuestCID)
	got := alloc.Allocate()
	if got != MinGuestCID {
		t.Fatalf("expected first CID=%d, got %d", MinGuestCID, got)
	}
}

func TestCIDAllocator_Monotonic(t *testing.T) {
	alloc := NewCIDAllocator(MinGuestCID)
	c1 := alloc.Allocate()
	c2 := alloc.Allocate()
	c3 := alloc.Allocate()
	if c1 != 3 || c2 != 4 || c3 != 5 {
		t.Fatalf("expected CIDs 3,4,5, got %d,%d,%d", c1, c2, c3)
	}
}

func TestCIDAllocator_ConcurrentUniqueness(t *testing.T) {
	alloc := NewCIDAllocator(MinGuestCID)
	const n = 1000

	var mu sync.Mutex
	seen := make(map[uint32]bool, n)
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			cid := alloc.Allocate()
			mu.Lock()
			if seen[cid] {
				t.Errorf("duplicate CID: %d", cid)
			}
			seen[cid] = true
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Fatalf("expected %d unique CIDs, got %d", n, len(seen))
	}
}

func TestCIDAllocator_CustomStart(t *testing.T) {
	alloc := NewCIDAllocator(100)
	got := alloc.Allocate()
	if got != 100 {
		t.Fatalf("expected first CID=100, got %d", got)
	}
}

func TestMinGuestCID_Value(t *testing.T) {
	if MinGuestCID != 3 {
		t.Fatalf("expected MinGuestCID=3, got %d", MinGuestCID)
	}
}

func TestExecdPort_Value(t *testing.T) {
	if ExecdPort != 44772 {
		t.Fatalf("expected ExecdPort=44772, got %d", ExecdPort)
	}
}

func TestVsockUDSPath_NonJailed(t *testing.T) {
	path := vsockUDSPath("test-vm-123", false)
	expected := filepath.Join(os.TempDir(), "firecracker-vsock-test-vm-123.sock")
	if path != expected {
		t.Fatalf("expected %q, got %q", expected, path)
	}
}

func TestVsockUDSPath_Jailed(t *testing.T) {
	path := vsockUDSPath("test-vm-123", true)
	if path != "vsock.sock" {
		t.Fatalf("expected \"vsock.sock\", got %q", path)
	}
}

func TestVsockUDSPath_UniquePerID(t *testing.T) {
	p1 := vsockUDSPath("vm-aaa", false)
	p2 := vsockUDSPath("vm-bbb", false)
	if p1 == p2 {
		t.Fatalf("expected unique paths, both were %q", p1)
	}
}

func TestVsockUDSPath_LengthValidation(t *testing.T) {
	// A 36-character UUID ID should produce a path within 108 chars.
	uuidID := "550e8400-e29b-41d4-a716-446655440000"
	path := vsockUDSPath(uuidID, false)
	if len(path) > maxUnixSocketPathLen {
		t.Fatalf("path too long (%d > %d): %s", len(path), maxUnixSocketPathLen, path)
	}
	// Verify it contains the expected pattern.
	expectedSuffix := fmt.Sprintf("firecracker-vsock-%s.sock", uuidID)
	if !strings.HasSuffix(path, expectedSuffix) {
		t.Fatalf("path %q does not end with %q", path, expectedSuffix)
	}
}

func TestVMConfigValidate_VsockCID_RejectsReserved(t *testing.T) {
	tests := []struct {
		name string
		cid  uint32
	}{
		{"cid_1", 1},
		{"cid_2", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig(t)
			cfg.VsockCID = tt.cid
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for VsockCID=%d, got nil", tt.cid)
			}
			if !strings.Contains(err.Error(), "VsockCID") {
				t.Errorf("error should mention VsockCID, got: %v", err)
			}
		})
	}
}

func TestVMConfigValidate_VsockCID_AcceptsZero(t *testing.T) {
	cfg := validConfig(t)
	cfg.VsockCID = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("VsockCID=0 (auto) should be valid, got: %v", err)
	}
}

func TestVMConfigValidate_VsockCID_AcceptsMinimum(t *testing.T) {
	cfg := validConfig(t)
	cfg.VsockCID = MinGuestCID
	if err := cfg.Validate(); err != nil {
		t.Fatalf("VsockCID=%d should be valid, got: %v", MinGuestCID, err)
	}
}

func TestVMConfigValidate_VsockCID_AcceptsLarge(t *testing.T) {
	cfg := validConfig(t)
	cfg.VsockCID = 1000
	if err := cfg.Validate(); err != nil {
		t.Fatalf("VsockCID=1000 should be valid, got: %v", err)
	}
}

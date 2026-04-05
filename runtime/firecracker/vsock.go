package firecracker

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
)

const (
	// MinGuestCID is the minimum valid guest Context Identifier for vsock.
	// CID 0 is reserved for the hypervisor, CID 1 is reserved, and CID 2
	// is assigned to the host. Guest CIDs must be >= 3.
	MinGuestCID uint32 = 3

	// ExecdPort is the well-known port number where the execd agent listens
	// inside the guest VM over vsock.
	ExecdPort uint32 = 44772
)

// CIDAllocator provides atomic, monotonically increasing guest Context
// Identifiers for vsock devices. Each Allocate call returns a unique CID,
// safe for concurrent use.
type CIDAllocator struct {
	next atomic.Uint32
}

// NewCIDAllocator creates a CIDAllocator that starts allocating from firstCID.
func NewCIDAllocator(firstCID uint32) *CIDAllocator {
	a := &CIDAllocator{}
	a.next.Store(firstCID)
	return a
}

// Allocate returns the next unique CID and advances the counter atomically.
func (a *CIDAllocator) Allocate() uint32 {
	return a.next.Add(1) - 1
}

// vsockUDSPath returns the Unix domain socket path for a VM's vsock device.
// When jailerEnabled is true, the path is relative inside the chroot (Firecracker
// creates it there). When false, the path is an absolute path in the system
// temp directory, unique per VM ID.
func vsockUDSPath(id string, jailerEnabled bool) string {
	if jailerEnabled {
		return "vsock.sock"
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("firecracker-vsock-%s.sock", id))
}

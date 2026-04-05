// Package firecracker provides types and interfaces for managing Firecracker
// microVM lifecycle, jailer configuration, and resource cleanup within the
// OpenSandbox runtime.
package firecracker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/google/uuid"
)

// VMState represents the current state of a Firecracker microVM.
type VMState string

const (
	// StateCreated indicates the VM has been created but not yet started.
	StateCreated VMState = "Created"
	// StateStarting indicates the VM is in the process of starting.
	StateStarting VMState = "Starting"
	// StateRunning indicates the VM is running and operational.
	StateRunning VMState = "Running"
	// StateStopping indicates the VM is in the process of stopping.
	StateStopping VMState = "Stopping"
	// StateStopped indicates the VM has been stopped.
	StateStopped VMState = "Stopped"
)

// VMConfig holds the configuration for creating a Firecracker microVM.
type VMConfig struct {
	// ID is the VM identifier (UUID). If empty, auto-generated.
	ID string
	// VCPUs is the number of virtual CPUs (1-32, default 1).
	VCPUs int64
	// MemoryMiB is the memory in MiB (minimum 128, default 256).
	MemoryMiB int64
	// KernelImagePath is the path to the guest kernel image (required).
	KernelImagePath string
	// RootfsPath is the path to the root filesystem ext4 image (required).
	RootfsPath string
	// ReadOnlyRootfs marks the root drive as read-only. When true, Firecracker
	// opens the rootfs ext4 file read-only and multiple VMs can safely share
	// the same image file without filesystem conflicts. Default false (Phase 1
	// writable behavior). Recommended true for images produced by the
	// runtime/firecracker/image Provisioner. Added in Phase 2 for IMG-03.
	// Note: zero-value (false) is the correct default — no entry needed in
	// withDefaults(). Phase 1 consumers get writable drives; Phase 2 consumers
	// opt in explicitly.
	ReadOnlyRootfs bool
	// KernelArgs are the kernel boot arguments (default: "console=ttyS0 reboot=k panic=1 pci=off").
	KernelArgs string
	// CPUTemplate configures CPU template for snapshot portability.
	CPUTemplate CPUTemplateConfig
	// TrackDirtyPages enables dirty page tracking for future diff snapshots.
	TrackDirtyPages bool
	// JailerEnabled runs the VM inside Jailer (recommended for production).
	JailerEnabled bool
	// Jailer holds the jailer configuration (used when JailerEnabled=true).
	Jailer JailerOpts
	// LogLevel sets the Firecracker log level: "Error", "Warning", "Info", "Debug" (default "Error").
	LogLevel string
	// FirecrackerBin is the path to the firecracker binary (default "/usr/bin/firecracker").
	FirecrackerBin string
	// JailerBin is the path to the jailer binary (default "/usr/bin/jailer").
	JailerBin string
}

// validVMIDPattern matches valid VM IDs: alphanumeric characters and hyphens.
var validVMIDPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// Validate checks that the VMConfig fields are within acceptable ranges and
// required fields are set. Returns an InvalidVMConfigError for the first
// validation failure encountered.
func (c *VMConfig) Validate() error {
	if c.ID != "" && !validVMIDPattern.MatchString(c.ID) {
		return &InvalidVMConfigError{
			Field:   "ID",
			Message: "must contain only alphanumeric characters and hyphens",
		}
	}
	if c.VCPUs < 1 || c.VCPUs > 32 {
		return &InvalidVMConfigError{
			Field:   "VCPUs",
			Message: fmt.Sprintf("must be in range [1, 32], got %d", c.VCPUs),
		}
	}
	if c.MemoryMiB < 128 {
		return &InvalidVMConfigError{
			Field:   "MemoryMiB",
			Message: fmt.Sprintf("must be >= 128, got %d", c.MemoryMiB),
		}
	}
	if c.KernelImagePath == "" {
		return &InvalidVMConfigError{
			Field:   "KernelImagePath",
			Message: "must not be empty",
		}
	}
	if c.RootfsPath == "" {
		return &InvalidVMConfigError{
			Field:   "RootfsPath",
			Message: "must not be empty",
		}
	}
	return nil
}

// withDefaults returns a copy of VMConfig with zero-value fields filled with
// sensible defaults.
func (c VMConfig) withDefaults() VMConfig {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.VCPUs == 0 {
		c.VCPUs = 1
	}
	if c.MemoryMiB == 0 {
		c.MemoryMiB = 256
	}
	if c.KernelArgs == "" {
		c.KernelArgs = DefaultKernelArgs
	}
	if c.LogLevel == "" {
		c.LogLevel = "Error"
	}
	if c.FirecrackerBin == "" {
		c.FirecrackerBin = "/usr/bin/firecracker"
	}
	if c.JailerBin == "" {
		c.JailerBin = "/usr/bin/jailer"
	}
	return c
}

// socketPath returns the API socket path for this VM configuration.
// When Jailer is enabled, the socket path is relative inside the chroot.
// When Jailer is disabled, the socket path is an absolute temp path.
func (c *VMConfig) socketPath() string {
	if c.JailerEnabled {
		return socketPathInChroot()
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("firecracker-%s.socket", c.ID))
}

// VM represents a Firecracker microVM instance with its current state and
// tracked resources.
type VM struct {
	// ID is the unique identifier for this VM.
	ID string
	// State is the current lifecycle state of the VM.
	State VMState
	// Config is the configuration used to create this VM.
	Config VMConfig
	// PID is the Firecracker process PID (0 if not started).
	PID int
	// SocketPath is the API socket path for this VM.
	SocketPath string
	// Resources tracks all resources needing cleanup.
	Resources VMResources

	// mu protects mutable VM fields during concurrent access.
	mu sync.Mutex
}

// VMManager defines the interface for managing Firecracker microVM lifecycle.
// Implementations handle VM creation, startup, shutdown, destruction, and
// state queries.
type VMManager interface {
	// Create creates a new VM with the given configuration. Returns the VM
	// in StateCreated. The VM is not yet started.
	Create(ctx context.Context, cfg VMConfig) (*VM, error)
	// Start starts a previously created VM, transitioning it to StateRunning.
	Start(ctx context.Context, vmID string) error
	// Stop gracefully stops a running VM, transitioning it to StateStopped.
	Stop(ctx context.Context, vmID string) error
	// Destroy destroys a VM and cleans up all associated resources.
	Destroy(ctx context.Context, vmID string) error
	// Get retrieves the current state of a VM by ID.
	Get(ctx context.Context, vmID string) (*VM, error)
}

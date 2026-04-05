package firecracker

import (
	"context"
	"errors"
	"sync"
)

// ManagerConfig holds configuration for the VM Manager.
type ManagerConfig struct {
	// ChrootBaseDir is the base directory for jailer chroots (default "/srv/jailer").
	ChrootBaseDir string
	// DefaultVCPUs is the default number of vCPUs for new VMs (default 1).
	DefaultVCPUs int64
	// DefaultMemoryMiB is the default memory in MiB for new VMs (default 256).
	DefaultMemoryMiB int64
	// LogLevel is the default log level (default "Error").
	LogLevel string
}

// withDefaults returns a copy of ManagerConfig with zero-value fields filled
// with sensible defaults.
func (c ManagerConfig) withDefaults() ManagerConfig {
	if c.ChrootBaseDir == "" {
		c.ChrootBaseDir = "/srv/jailer"
	}
	if c.DefaultVCPUs == 0 {
		c.DefaultVCPUs = 1
	}
	if c.DefaultMemoryMiB == 0 {
		c.DefaultMemoryMiB = 256
	}
	if c.LogLevel == "" {
		c.LogLevel = "Error"
	}
	return c
}

// Manager implements VMManager, managing the lifecycle of Firecracker microVMs.
// It maintains an in-memory registry of VMs and coordinates creation, startup,
// shutdown, and destruction operations.
type Manager struct {
	config ManagerConfig
	vms    map[string]*VM
	mu     sync.RWMutex
}

// Compile-time interface check: Manager must implement VMManager.
var _ VMManager = (*Manager)(nil)

// NewManager creates a new Manager with the given configuration. Zero-value
// config fields are filled with defaults.
func NewManager(cfg ManagerConfig) *Manager {
	cfg = cfg.withDefaults()
	return &Manager{
		config: cfg,
		vms:    make(map[string]*VM),
	}
}

// Create creates a new VM with the given configuration.
// This is a stub implementation that will be completed in Plan 02.
func (m *Manager) Create(ctx context.Context, cfg VMConfig) (*VM, error) {
	return nil, errors.New("not implemented")
}

// Start starts a previously created VM.
// This is a stub implementation that will be completed in Plan 02.
func (m *Manager) Start(ctx context.Context, vmID string) error {
	return errors.New("not implemented")
}

// Stop gracefully stops a running VM.
// This is a stub implementation that will be completed in Plan 02.
func (m *Manager) Stop(ctx context.Context, vmID string) error {
	return errors.New("not implemented")
}

// Destroy destroys a VM and cleans up all associated resources.
// This is a stub implementation that will be completed in Plan 02.
func (m *Manager) Destroy(ctx context.Context, vmID string) error {
	return errors.New("not implemented")
}

// Get retrieves the current state of a VM by ID.
// This is a stub implementation that will be completed in Plan 02.
func (m *Manager) Get(ctx context.Context, vmID string) (*VM, error) {
	return nil, errors.New("not implemented")
}

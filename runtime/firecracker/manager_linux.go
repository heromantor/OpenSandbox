//go:build linux

package firecracker

import (
	"context"
	"fmt"
	"strings"
	"sync"

	sdk "github.com/firecracker-microvm/firecracker-go-sdk"
	log "github.com/sirupsen/logrus"
)

// Manager implements VMManager, managing the lifecycle of Firecracker microVMs.
// It maintains an in-memory registry of VMs and coordinates creation, startup,
// shutdown, and destruction operations.
type Manager struct {
	config   ManagerConfig
	vms      map[string]*VM
	machines map[string]*sdk.Machine
	cidAlloc *CIDAllocator
	mu       sync.RWMutex
}

// Compile-time interface check: Manager must implement VMManager.
var _ VMManager = (*Manager)(nil)

// NewManager creates a new Manager with the given configuration. Zero-value
// config fields are filled with defaults.
func NewManager(cfg ManagerConfig) *Manager {
	cfg = cfg.withDefaults()
	return &Manager{
		config:   cfg,
		vms:      make(map[string]*VM),
		machines: make(map[string]*sdk.Machine),
		cidAlloc: NewCIDAllocator(MinGuestCID),
	}
}

// Create creates a new VM with the given configuration. It validates the config,
// translates it to a firecracker-go-sdk Config, and creates an SDK Machine
// instance. The VM is returned in StateCreated and must be started with Start().
func (m *Manager) Create(ctx context.Context, cfg VMConfig) (*VM, error) {
	cfg = cfg.withDefaults()

	// Auto-assign vsock CID if not explicitly set.
	if cfg.VsockCID == 0 {
		cfg.VsockCID = m.cidAlloc.Allocate()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("firecracker: create vm: %w", err)
	}
	if err := cfg.CPUTemplate.Validate(); err != nil {
		return nil, fmt.Errorf("firecracker: create vm: %w", err)
	}
	if err := ValidateKernelImage(cfg.KernelImagePath); err != nil {
		return nil, fmt.Errorf("firecracker: create vm: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.vms[cfg.ID]; exists {
		return nil, &VMAlreadyExistsError{VMID: cfg.ID}
	}

	fcCfg, err := cfg.toFirecrackerConfig()
	if err != nil {
		return nil, fmt.Errorf("firecracker: create vm: %w", err)
	}

	// Create a logrus logger for the SDK Machine.
	logger := log.New()
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		logger.SetLevel(log.DebugLevel)
	case "info":
		logger.SetLevel(log.InfoLevel)
	case "warning", "warn":
		logger.SetLevel(log.WarnLevel)
	default:
		logger.SetLevel(log.ErrorLevel)
	}

	machine, err := sdk.NewMachine(ctx, fcCfg, sdk.WithLogger(log.NewEntry(logger)))
	if err != nil {
		return nil, &VMStartError{
			VMID:  cfg.ID,
			Cause: fmt.Errorf("create machine: %w", err),
		}
	}

	// Build VMResources for cleanup tracking.
	var socketPath, chrootDirPath string
	if cfg.JailerEnabled {
		chrootDirPath = chrootDir(cfg.Jailer.ChrootBaseDir, cfg.ID)
		if cfg.Jailer.ChrootBaseDir == "" {
			chrootDirPath = chrootDir("/srv/jailer", cfg.ID)
		}
		socketPath = chrootDirPath + "/" + socketPathInChroot()
	} else {
		socketPath = cfg.socketPath()
	}

	// Compute vsock UDS path. For non-jailed VMs the helper returns a full
	// absolute path. For jailed VMs the helper returns a relative "vsock.sock"
	// but we need the full host path for cleanup, so prepend the chroot dir.
	vsockPath := vsockUDSPath(cfg.ID, cfg.JailerEnabled)
	if cfg.JailerEnabled {
		vsockPath = chrootDirPath + "/vsock.sock"
	}

	vm := &VM{
		ID:           cfg.ID,
		State:        StateCreated,
		Config:       cfg,
		SocketPath:   socketPath,
		VsockCID:     cfg.VsockCID,
		VsockUDSPath: vsockPath,
		Resources: VMResources{
			SocketPath:   socketPath,
			ChrootDir:    chrootDirPath,
			VsockUDSPath: vsockPath,
		},
	}

	m.vms[cfg.ID] = vm
	m.machines[cfg.ID] = machine

	return vm, nil
}

// Start starts a previously created VM, transitioning it from StateCreated to
// StateRunning. It calls Machine.Start() to boot the Firecracker microVM and
// records the process PID.
func (m *Manager) Start(ctx context.Context, vmID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return &VMNotFoundError{VMID: vmID}
	}

	if vm.State != StateCreated {
		return fmt.Errorf("firecracker: cannot start vm %s: current state is %s, expected %s",
			vmID, vm.State, StateCreated)
	}

	vm.State = StateStarting

	machine, exists := m.machines[vmID]
	if !exists {
		vm.State = StateCreated
		return &VMNotFoundError{VMID: vmID}
	}

	if err := machine.Start(ctx); err != nil {
		vm.State = StateCreated
		return &VMStartError{VMID: vmID, Cause: err}
	}

	pid, err := machine.PID()
	if err == nil {
		vm.PID = pid
	}

	vm.State = StateRunning
	return nil
}

// Stop gracefully stops a running VM, transitioning it from StateRunning to
// StateStopped. It first attempts a graceful shutdown (Ctrl+Alt+Del), then
// falls back to SIGTERM via StopVMM, and waits for the process to exit.
// Resources are NOT cleaned up here -- use Destroy for full cleanup.
func (m *Manager) Stop(ctx context.Context, vmID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return &VMNotFoundError{VMID: vmID}
	}

	if vm.State != StateRunning {
		return fmt.Errorf("firecracker: cannot stop vm %s: current state is %s, expected %s",
			vmID, vm.State, StateRunning)
	}

	vm.State = StateStopping

	machine, exists := m.machines[vmID]
	if !exists {
		return &VMNotFoundError{VMID: vmID}
	}

	// Try graceful shutdown first (sends Ctrl+Alt+Del to the guest).
	if err := machine.Shutdown(ctx); err != nil {
		// Graceful shutdown failed; fall back to SIGTERM.
		if stopErr := machine.StopVMM(); stopErr != nil {
			vm.State = StateRunning
			return &VMStopError{
				VMID:  vmID,
				Cause: fmt.Errorf("shutdown failed: %v; stopVMM failed: %w", err, stopErr),
			}
		}
	}

	// Wait for the process to exit. Ignore "already exited" style errors
	// since that means the process terminated successfully.
	if err := machine.Wait(ctx); err != nil {
		if !isProcessExitedError(err) {
			// Non-fatal: the VM may have already exited between Shutdown and Wait.
			// Log-worthy but don't fail the Stop operation.
			_ = err
		}
	}

	vm.State = StateStopped
	vm.PID = 0
	return nil
}

// Destroy destroys a VM and cleans up all associated resources. If the VM is
// still running, it is stopped first. The VM is removed from the manager's
// registry after cleanup.
func (m *Manager) Destroy(ctx context.Context, vmID string) error {
	m.mu.Lock()
	vm, exists := m.vms[vmID]
	if !exists {
		m.mu.Unlock()
		return &VMNotFoundError{VMID: vmID}
	}

	// If the VM is running, stop it first.
	if vm.State == StateRunning {
		m.mu.Unlock()
		if err := m.Stop(ctx, vmID); err != nil {
			// If stop fails, still try to clean up resources.
			_ = err
		}
		m.mu.Lock()
	}

	// Clean up all tracked resources.
	var cleanupErr error
	if err := vm.Resources.Cleanup(); err != nil {
		cleanupErr = err
	}

	// Remove from registry.
	delete(m.vms, vmID)
	delete(m.machines, vmID)
	m.mu.Unlock()

	if cleanupErr != nil {
		return &CleanupError{
			VMID:   vmID,
			Errors: []error{cleanupErr},
		}
	}
	return nil
}

// Get retrieves the current state of a VM by ID. Returns a shallow copy of the
// VM to prevent callers from mutating internal manager state.
func (m *Manager) Get(ctx context.Context, vmID string) (*VM, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return nil, &VMNotFoundError{VMID: vmID}
	}

	// Return a shallow copy to prevent external mutation of internal state.
	vmCopy := *vm
	return &vmCopy, nil
}

// isProcessExitedError checks if an error indicates the process has already
// exited, which is expected during Stop when the VM terminates between
// Shutdown and Wait.
func isProcessExitedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "process already finished") ||
		strings.Contains(msg, "process already exited") ||
		strings.Contains(msg, "signal: killed") ||
		strings.Contains(msg, "wait: no child processes")
}

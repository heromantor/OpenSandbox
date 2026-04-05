package firecracker

import (
	"fmt"
	"os"

	"github.com/hashicorp/go-multierror"
)

// VMResources tracks all resources associated with a VM that need cleanup
// on stop or destroy.
type VMResources struct {
	// SocketPath is the path to the Firecracker API socket.
	SocketPath string
	// ChrootDir is the path to the jailer chroot directory.
	ChrootDir string
	// CgroupPaths are the paths to cgroup directories.
	CgroupPaths []string
	// LogFifoPath is the path to the log FIFO.
	LogFifoPath string
	// MetricsFifoPath is the path to the metrics FIFO.
	MetricsFifoPath string
	// VsockUDSPath is the path to the vsock Unix domain socket.
	VsockUDSPath string
}

// Cleanup removes all tracked resources. Errors from individual cleanup
// operations are aggregated using multierror so that all resources are
// attempted even if some fail. Returns nil if all cleanups succeed or
// all paths are empty.
func (r *VMResources) Cleanup() error {
	var result *multierror.Error

	// Remove socket file.
	if r.SocketPath != "" {
		if err := os.Remove(r.SocketPath); err != nil && !os.IsNotExist(err) {
			result = multierror.Append(result,
				fmt.Errorf("remove socket %s: %w", r.SocketPath, err))
		}
	}

	// Remove vsock UDS file.
	if r.VsockUDSPath != "" {
		if err := os.Remove(r.VsockUDSPath); err != nil && !os.IsNotExist(err) {
			result = multierror.Append(result,
				fmt.Errorf("remove vsock uds %s: %w", r.VsockUDSPath, err))
		}
	}

	// Remove chroot directory tree.
	if r.ChrootDir != "" {
		if err := os.RemoveAll(r.ChrootDir); err != nil {
			result = multierror.Append(result,
				fmt.Errorf("remove chroot %s: %w", r.ChrootDir, err))
		}
	}

	// Remove cgroup directories.
	for _, cgPath := range r.CgroupPaths {
		if cgPath == "" {
			continue
		}
		if err := os.RemoveAll(cgPath); err != nil {
			result = multierror.Append(result,
				fmt.Errorf("remove cgroup %s: %w", cgPath, err))
		}
	}

	// Remove log FIFO.
	if r.LogFifoPath != "" {
		if err := os.Remove(r.LogFifoPath); err != nil && !os.IsNotExist(err) {
			result = multierror.Append(result,
				fmt.Errorf("remove log fifo %s: %w", r.LogFifoPath, err))
		}
	}

	// Remove metrics FIFO.
	if r.MetricsFifoPath != "" {
		if err := os.Remove(r.MetricsFifoPath); err != nil && !os.IsNotExist(err) {
			result = multierror.Append(result,
				fmt.Errorf("remove metrics fifo %s: %w", r.MetricsFifoPath, err))
		}
	}

	return result.ErrorOrNil()
}

// IsEmpty returns true if all resource paths are zero-value, indicating
// no resources are tracked for cleanup.
func (r *VMResources) IsEmpty() bool {
	return r.SocketPath == "" &&
		r.ChrootDir == "" &&
		len(r.CgroupPaths) == 0 &&
		r.LogFifoPath == "" &&
		r.MetricsFifoPath == "" &&
		r.VsockUDSPath == ""
}

package firecracker

import (
	"fmt"
	"os"
	"path/filepath"
)

// JailerOpts holds configuration for running a Firecracker VM inside the Jailer.
type JailerOpts struct {
	// UID is the unprivileged user ID for the jailed process (required, must be > 0).
	UID int
	// GID is the unprivileged group ID for the jailed process (required, must be > 0).
	GID int
	// ChrootBaseDir is the base chroot directory (default "/srv/jailer").
	ChrootBaseDir string
	// CgroupVersion is "1" or "2" (auto-detected if empty).
	CgroupVersion string
	// Daemonize runs the jailer as a daemon (default true).
	Daemonize bool
	// NumaNode is the NUMA node to pin to (-1 = no pinning, default -1).
	NumaNode int
}

// detectCgroupVersion returns "2" if the host uses cgroup v2 (unified hierarchy),
// "1" otherwise. Detection is based on the presence of the cgroup.controllers file
// at the cgroup v2 mount point.
func detectCgroupVersion() string {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return "2"
	}
	return "1"
}

// chrootDir returns the full chroot directory path for a VM following the
// Jailer convention: <base>/<exec-name>/<id>/root/
func chrootDir(baseDir, vmID string) string {
	return filepath.Join(baseDir, "firecracker", vmID, "root")
}

// socketPathInChroot returns the relative socket path inside the chroot root,
// following the Jailer convention.
func socketPathInChroot() string {
	return "run/firecracker.socket"
}

// maxUnixSocketPathLen is the maximum length of a Unix domain socket path
// on Linux (sun_path in sockaddr_un).
const maxUnixSocketPathLen = 108

// validateJailerOpts checks that the JailerOpts fields are valid for use
// with the Firecracker jailer.
func validateJailerOpts(opts JailerOpts) error {
	if opts.UID <= 0 {
		return &InvalidVMConfigError{
			Field:   "Jailer.UID",
			Message: fmt.Sprintf("must be > 0, got %d", opts.UID),
		}
	}
	if opts.GID <= 0 {
		return &InvalidVMConfigError{
			Field:   "Jailer.GID",
			Message: fmt.Sprintf("must be > 0, got %d", opts.GID),
		}
	}
	if opts.ChrootBaseDir == "" {
		return &InvalidVMConfigError{
			Field:   "Jailer.ChrootBaseDir",
			Message: "must not be empty",
		}
	}

	// Validate that the full socket path fits within the Unix socket path limit.
	// Use a 36-character UUID placeholder to estimate maximum path length.
	// Format: <base>/firecracker/<uuid>/root/run/firecracker.socket
	placeholderUUID := "00000000-0000-0000-0000-000000000000" // 36 chars
	fullSocketPath := filepath.Join(
		chrootDir(opts.ChrootBaseDir, placeholderUUID),
		socketPathInChroot(),
	)
	if len(fullSocketPath) > maxUnixSocketPathLen {
		return &InvalidVMConfigError{
			Field:   "Jailer.ChrootBaseDir",
			Message: fmt.Sprintf("socket path too long (%d chars, max %d): %s", len(fullSocketPath), maxUnixSocketPathLen, fullSocketPath),
		}
	}

	return nil
}

// resolvedJailerConfig holds the resolved jailer configuration with all defaults
// applied. This is an intermediate representation used before translating to
// the firecracker-go-sdk JailerConfig in Plan 02.
type resolvedJailerConfig struct {
	id            string
	uid           int
	gid           int
	execFile      string
	chrootBaseDir string
	cgroupVersion string
	daemonize     bool
	numaNode      int
}

// resolveJailerConfig resolves a JailerOpts into a fully populated configuration
// by applying defaults and validating the result.
func resolveJailerConfig(vmID string, opts JailerOpts, firecrackerBin string) (*resolvedJailerConfig, error) {
	// Apply defaults.
	if opts.ChrootBaseDir == "" {
		opts.ChrootBaseDir = "/srv/jailer"
	}
	if opts.CgroupVersion == "" {
		opts.CgroupVersion = detectCgroupVersion()
	}
	// Daemonize defaults to true. Since bool zero-value is false, we need to
	// handle this explicitly. The JailerOpts struct uses Daemonize=false as
	// "not set", so we default to true here.
	// NOTE: Callers who explicitly want Daemonize=false must set it after
	// calling resolveJailerConfig. In practice, daemonize is almost always true.
	if !opts.Daemonize {
		opts.Daemonize = true
	}
	// NumaNode defaults to -1 (no pinning). Zero value means "use the default".
	if opts.NumaNode == 0 {
		opts.NumaNode = -1
	}

	if err := validateJailerOpts(opts); err != nil {
		return nil, err
	}

	return &resolvedJailerConfig{
		id:            vmID,
		uid:           opts.UID,
		gid:           opts.GID,
		execFile:      firecrackerBin,
		chrootBaseDir: opts.ChrootBaseDir,
		cgroupVersion: opts.CgroupVersion,
		daemonize:     opts.Daemonize,
		numaNode:      opts.NumaNode,
	}, nil
}


package firecracker

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

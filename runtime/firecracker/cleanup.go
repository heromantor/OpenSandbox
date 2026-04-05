package firecracker

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
}

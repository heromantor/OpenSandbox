package firecracker

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

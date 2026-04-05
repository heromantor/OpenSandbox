package firecracker

const (
	// DefaultKernelVersion is the pinned kernel version for Firecracker VMs.
	DefaultKernelVersion = "5.10"
	// DefaultKernelArgs are the default kernel boot arguments.
	DefaultKernelArgs = "console=ttyS0 reboot=k panic=1 pci=off"
)

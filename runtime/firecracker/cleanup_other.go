//go:build !linux

package firecracker

// cleanupNetwork is a no-op on non-Linux platforms. TAP device and iptables
// cleanup require Linux-specific netlink and go-iptables syscalls.
func (r *VMResources) cleanupNetwork() error {
	return nil
}

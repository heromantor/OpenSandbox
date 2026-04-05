//go:build integration

package firecracker

import (
	"testing"
)

// TestIntegrationVMLifecycle tests the full VM lifecycle: create, start, stop, destroy.
// This test requires:
// - Linux with /dev/kvm
// - Firecracker binary at /usr/bin/firecracker
// - Jailer binary at /usr/bin/jailer
// - A guest kernel image
// - A rootfs ext4 image
func TestIntegrationVMLifecycle(t *testing.T) {
	t.Skip("TODO: requires Firecracker binary and Linux/KVM -- implement when CI is ready")
}

// TestIntegrationJailerIsolation tests that the Jailer creates a proper chroot
// environment and the VM runs inside it with the correct UID/GID.
func TestIntegrationJailerIsolation(t *testing.T) {
	t.Skip("TODO: requires Firecracker binary, jailer, and Linux/KVM")
}

// TestIntegrationCPUTemplate tests that CPU templates (T2, T2S, C3) are properly
// applied to the VM configuration and affect the CPUID visible to the guest.
func TestIntegrationCPUTemplate(t *testing.T) {
	t.Skip("TODO: requires Firecracker binary and Intel CPU for T2/T2S/C3 templates")
}

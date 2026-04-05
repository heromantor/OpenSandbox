package firecracker

import (
	"fmt"
	"os"
)

const (
	// DefaultKernelVersion is the pinned kernel version for Firecracker VMs.
	DefaultKernelVersion = "5.10"
	// DefaultKernelArgs are the default kernel boot arguments.
	DefaultKernelArgs = "console=ttyS0 reboot=k panic=1 pci=off"
)

// KernelManifest declares a pinned kernel version and its location on disk.
type KernelManifest struct {
	// Version is the kernel version (e.g., "5.10").
	Version string
	// ImagePath is the path to the vmlinux binary.
	ImagePath string
	// Checksum is the SHA256 checksum of the kernel image (hex-encoded).
	Checksum string
}

// ValidateKernelImage checks that the kernel image at the given path exists,
// is not a directory, and is not empty.
func ValidateKernelImage(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("firecracker: kernel image: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("firecracker: kernel image: %s is a directory", path)
	}
	if info.Size() == 0 {
		return fmt.Errorf("firecracker: kernel image: %s is empty", path)
	}
	return nil
}

// ResolveKernelPath validates and returns the kernel image path from a
// KernelManifest. Returns an error if ImagePath is empty or the file does
// not exist.
func ResolveKernelPath(manifest KernelManifest) (string, error) {
	if manifest.ImagePath == "" {
		return "", fmt.Errorf("firecracker: kernel manifest: ImagePath must not be empty")
	}
	if err := ValidateKernelImage(manifest.ImagePath); err != nil {
		return "", err
	}
	return manifest.ImagePath, nil
}

package firecracker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// DefaultKernelVersion is the pinned kernel version for Firecracker VMs.
	DefaultKernelVersion = "5.10"
	// DefaultKernelArgs are the default kernel boot arguments.
	DefaultKernelArgs = "console=ttyS0 reboot=k panic=1 pci=off"
	// DefaultKernelURL is the upstream Firecracker CI artifact URL for the
	// pinned x86_64 kernel image (vmlinux-5.10). The `fetch-kernel` Makefile
	// target downloads from this URL.
	DefaultKernelURL = "https://s3.amazonaws.com/spec.ccfc.min/firecracker-ci/v1.10/x86_64/vmlinux-5.10.225"
	// DefaultKernelSHA256 is the SHA256 checksum of the kernel image located
	// at DefaultKernelURL. Used by VerifyKernelChecksum and the
	// `verify-kernel` Makefile target to detect tampering or corruption.
	DefaultKernelSHA256 = "89370d37ec2e0ee8e15df72a131ba04e3a20bba76d4ce07daad15f7e3f8e3c4f"
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

// VerifyKernelChecksum computes the SHA256 of the file at path and compares
// it (case-insensitive) to the expected hex-encoded digest. Returns nil when
// the digests match. Returns a descriptive error when the file cannot be
// read, the expected digest is empty, or the digests differ.
func VerifyKernelChecksum(path, expectedHex string) error {
	if expectedHex == "" {
		return fmt.Errorf("firecracker: kernel checksum: expected digest must not be empty")
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("firecracker: kernel checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("firecracker: kernel checksum: read %s: %w", path, err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expectedHex) {
		return fmt.Errorf("firecracker: kernel checksum: mismatch for %s: expected %s, got %s", path, strings.ToLower(expectedHex), actual)
	}
	return nil
}

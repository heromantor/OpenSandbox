// Package image provides OCI-image-to-ext4 rootfs provisioning for
// Firecracker VMs. It pulls OCI images via crane, flattens their
// layers, and writes them as ext4 block device files via tar2ext4.
package image

import (
	"fmt"
)

// DefaultRootfsCacheDir is the FHS-aligned default cache path.
const DefaultRootfsCacheDir = "/var/lib/opensandbox/rootfs"

// DefaultMaxImageSize caps the ext4 image at 2 GiB unless overridden.
const DefaultMaxImageSize int64 = 2 * 1024 * 1024 * 1024

// DefaultPlatform pins the OCI pull to Linux/amd64.
// Firecracker is Linux-only; amd64 is the v1 target.
const DefaultPlatform = "linux/amd64"

// MinMaxImageSize is the smallest permitted MaxImageSize (32 MiB).
// Below this the ext4 metadata overhead crowds out real content.
const MinMaxImageSize int64 = 32 * 1024 * 1024

// ProvisionerConfig holds configuration for the image Provisioner.
type ProvisionerConfig struct {
	// RootfsCacheDir is the local directory where ext4 images are stored.
	RootfsCacheDir string
	// MaxImageSize caps the ext4 image file size in bytes.
	MaxImageSize int64
	// DefaultPlatform sets the OCI platform for pulls ("linux/amd64").
	DefaultPlatform string
}

// withDefaults returns a copy of ProvisionerConfig with zero-value fields
// filled with sensible defaults.
func (c ProvisionerConfig) withDefaults() ProvisionerConfig {
	if c.RootfsCacheDir == "" {
		c.RootfsCacheDir = DefaultRootfsCacheDir
	}
	if c.MaxImageSize == 0 {
		c.MaxImageSize = DefaultMaxImageSize
	}
	if c.DefaultPlatform == "" {
		c.DefaultPlatform = DefaultPlatform
	}
	return c
}

// Validate checks that ProvisionerConfig fields are within acceptable
// ranges. Returns an InvalidProvisionerConfigError for the first
// validation failure encountered. Zero values for MaxImageSize are
// accepted (callers typically apply withDefaults before Validate, but
// Validate must not reject an unpopulated MaxImageSize — the defaulting
// step will fill it later).
func (c *ProvisionerConfig) Validate() error {
	if c.RootfsCacheDir == "" {
		return &InvalidProvisionerConfigError{
			Field:   "RootfsCacheDir",
			Message: "must not be empty",
		}
	}
	if c.MaxImageSize != 0 && c.MaxImageSize < MinMaxImageSize {
		return &InvalidProvisionerConfigError{
			Field:   "MaxImageSize",
			Message: fmt.Sprintf("must be >= %d (32 MiB), got %d", MinMaxImageSize, c.MaxImageSize),
		}
	}
	return nil
}

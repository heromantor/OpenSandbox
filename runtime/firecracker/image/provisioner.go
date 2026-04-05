// Package image provides OCI-image-to-ext4 rootfs provisioning for
// Firecracker VMs. It pulls OCI images via crane, flattens their
// layers, and writes them as ext4 block device files via tar2ext4.
package image

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim/ext4/tar2ext4"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
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

// Provisioner builds ext4 rootfs images from OCI references and caches
// them via the Store. Provisioner is safe for concurrent use.
type Provisioner struct {
	cfg     ProvisionerConfig
	store   *Store
	fetcher ImageFetcher

	// digestCache maps canonical ref -> v1.Hash so repeated Provision
	// calls for the same ref skip the network fetch entirely.
	digestMu    sync.RWMutex
	digestCache map[string]v1.Hash
}

// NewProvisioner constructs a Provisioner with the given configuration.
// Uses the production craneFetcher for registry pulls.
func NewProvisioner(cfg ProvisionerConfig) (*Provisioner, error) {
	return newProvisionerWithFetcher(cfg, NewCraneFetcher())
}

// newProvisionerWithFetcher is the test-visible constructor that
// allows injecting a custom ImageFetcher.
func newProvisionerWithFetcher(cfg ProvisionerConfig, fetcher ImageFetcher) (*Provisioner, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store := NewStore(cfg.RootfsCacheDir)
	if err := store.Init(); err != nil {
		return nil, err
	}
	return &Provisioner{
		cfg:         cfg,
		store:       store,
		fetcher:     fetcher,
		digestCache: make(map[string]v1.Hash),
	}, nil
}

// Store returns the underlying cache store (for callers that need
// to enumerate cache entries or build paths from digests).
func (p *Provisioner) Store() *Store { return p.store }

// Provision resolves ref to a manifest digest, builds the ext4 image
// if not already cached, and returns the absolute path to the ext4
// file. Safe for concurrent callers.
func (p *Provisioner) Provision(ctx context.Context, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", &InvalidProvisionerConfigError{Field: "ref", Message: "must not be empty"}
	}
	parsed, err := ParseReference(ref)
	if err != nil {
		return "", err
	}

	// Fast path: if we already resolved this canonical ref to a digest
	// and the ext4 file is on disk, skip the network fetch entirely.
	p.digestMu.RLock()
	cached, ok := p.digestCache[parsed.Canonical]
	p.digestMu.RUnlock()
	if ok && p.store.Exists(cached) {
		return p.store.PathFor(cached), nil
	}

	platform, perr := parsePlatform(p.cfg.DefaultPlatform)
	if perr != nil {
		return "", perr
	}

	img, err := p.fetcher.Fetch(ctx, parsed.Canonical, platform)
	if err != nil {
		return "", err
	}
	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("firecracker: image: digest: %w", err)
	}

	// Record the ref -> digest mapping for future fast-path lookups.
	p.digestMu.Lock()
	p.digestCache[parsed.Canonical] = digest
	p.digestMu.Unlock()

	// Cache hit short-circuit (another goroutine may have built it).
	if p.store.Exists(digest) {
		return p.store.PathFor(digest), nil
	}

	// Miss: stream crane.Export -> tar2ext4 -> atomic write via Store.
	if err := p.buildAndStore(ctx, img, digest); err != nil {
		return "", err
	}
	return p.store.PathFor(digest), nil
}

func (p *Provisioner) buildAndStore(_ context.Context, img v1.Image, digest v1.Hash) error {
	// Use a temp file (not in-memory) because tar2ext4 requires
	// io.ReadWriteSeeker and ext4 images can exceed memory budget.
	scratch, err := os.CreateTemp(p.cfg.RootfsCacheDir, digest.Hex+".ext4.scratch-*")
	if err != nil {
		return &CacheError{Op: "scratch", Cause: err}
	}
	scratchPath := scratch.Name()
	defer func() { _ = scratch.Close(); _ = os.Remove(scratchPath) }()

	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		// crane.Export flattens image layers into a single tar stream.
		exportErr := crane.Export(img, pw)
		_ = pw.CloseWithError(exportErr)
		errCh <- exportErr
	}()

	convErr := tar2ext4.ConvertTarToExt4(pr, scratch,
		tar2ext4.ConvertWhiteout,
		tar2ext4.MaximumDiskSize(p.cfg.MaxImageSize),
	)
	if convErr != nil {
		return &Ext4ConvertError{Cause: convErr}
	}
	if exportErr := <-errCh; exportErr != nil {
		return &ImagePullError{Ref: digest.String(), Cause: exportErr}
	}
	if err := scratch.Sync(); err != nil {
		return &CacheError{Op: "sync", Cause: err}
	}
	if _, err := scratch.Seek(0, io.SeekStart); err != nil {
		return &CacheError{Op: "seek", Cause: err}
	}

	return p.store.AtomicWrite(digest, scratch)
}

// parsePlatform parses "linux/amd64" into a *v1.Platform.
func parsePlatform(s string) (*v1.Platform, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, &InvalidProvisionerConfigError{
			Field:   "DefaultPlatform",
			Message: fmt.Sprintf("must be 'os/arch', got %q", s),
		}
	}
	return &v1.Platform{OS: parts[0], Architecture: parts[1]}, nil
}

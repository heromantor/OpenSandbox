package image

import (
	"io"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// Store manages the on-disk cache of ext4 rootfs images. Path layout:
//
//	{CacheDir}/{algorithm}/{hex}.ext4
//
// The cache is content-addressed via the OCI manifest digest, so
// concurrent writers producing identical content are safe under
// last-rename-wins.
type Store struct {
	cacheDir string
}

// NewStore constructs a Store rooted at the given cache directory. The
// directory is NOT created here; call Init() to create it. Defaults for
// the cache directory live on ProvisionerConfig, not on Store.
func NewStore(cacheDir string) *Store {
	return &Store{cacheDir: cacheDir}
}

// Init creates the cache directory tree with 0o750 perms. Idempotent:
// calling Init more than once on an existing directory returns nil.
func (s *Store) Init() error {
	if err := os.MkdirAll(s.cacheDir, 0o750); err != nil {
		return &CacheError{Op: "init", Cause: err}
	}
	return nil
}

// PathFor returns the cache path for a given manifest digest. The path
// is constructed as {cacheDir}/{algorithm}/{hex}.ext4.
func (s *Store) PathFor(h v1.Hash) string {
	return filepath.Join(s.cacheDir, h.Algorithm, h.Hex+".ext4")
}

// Exists reports whether a cached ext4 file exists for this digest.
// Returns false if the file does not exist or stat fails for any reason.
func (s *Store) Exists(h v1.Hash) bool {
	_, err := os.Stat(s.PathFor(h))
	return err == nil
}

// AtomicWrite writes src's contents to the cache path for h, using a
// temp file + rename. The final file is 0o444 (read-only). Safe under
// concurrent callers writing the same digest: partial writes are never
// visible at the final path, and last-rename-wins is content-preserving
// because writers for the same digest produce identical bytes by
// construction.
func (s *Store) AtomicWrite(h v1.Hash, src io.Reader) error {
	dst := s.PathFor(h)
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return &CacheError{Op: "write", Cause: err}
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(dst)+".tmp-*")
	if err != nil {
		return &CacheError{Op: "write", Cause: err}
	}
	tmpName := tmp.Name()
	// Ensure temp file is cleaned up on any failure path. os.Remove after
	// a successful rename is a no-op returning ENOENT, which we ignore.
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return &CacheError{Op: "write", Cause: err}
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return &CacheError{Op: "write", Cause: err}
	}
	if err := tmp.Chmod(0o444); err != nil {
		tmp.Close()
		return &CacheError{Op: "write", Cause: err}
	}
	if err := tmp.Close(); err != nil {
		return &CacheError{Op: "write", Cause: err}
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return &CacheError{Op: "rename", Cause: err}
	}
	return nil
}

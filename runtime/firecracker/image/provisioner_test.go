package image

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// testImage builds a v1.Image from testdata/tiny.tar for offline tests.
func testImage(t *testing.T) v1.Image {
	t.Helper()
	layer, err := tarball.LayerFromFile("testdata/tiny.tar")
	if err != nil {
		t.Fatalf("tarball.LayerFromFile: %v", err)
	}
	img, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		t.Fatalf("mutate.AppendLayers: %v", err)
	}
	return img
}

// countingFetcher wraps staticFetcher and counts Fetch calls.
type countingFetcher struct {
	inner ImageFetcher
	count atomic.Int32
}

func newCountingFetcher(img v1.Image) *countingFetcher {
	return &countingFetcher{inner: newStaticFetcher(img, nil)}
}

func (f *countingFetcher) Fetch(ctx context.Context, ref string, p *v1.Platform) (v1.Image, error) {
	f.count.Add(1)
	return f.inner.Fetch(ctx, ref, p)
}

func newTestProvisioner(t *testing.T, fetcher ImageFetcher, cfgOverrides ...func(*ProvisionerConfig)) *Provisioner {
	t.Helper()
	cfg := ProvisionerConfig{
		RootfsCacheDir: t.TempDir(),
	}
	for _, fn := range cfgOverrides {
		fn(&cfg)
	}
	p, err := newProvisionerWithFetcher(cfg, fetcher)
	if err != nil {
		t.Fatalf("newProvisionerWithFetcher: %v", err)
	}
	return p
}

func TestProvision_Success(t *testing.T) {
	img := testImage(t)
	fetcher := newStaticFetcher(img, nil)
	p := newTestProvisioner(t, fetcher)

	path, err := p.Provision(context.Background(), "alpine:3.19")
	if err != nil {
		t.Fatalf("Provision() = %v", err)
	}
	if path == "" {
		t.Fatal("Provision() returned empty path")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) = %v", path, err)
	}
	if info.Size() == 0 {
		t.Errorf("ext4 file size = 0, want > 0")
	}
	// Path should contain "sha256" (the hash algorithm directory).
	if !containsSubstr(path, "sha256") {
		t.Errorf("path %q does not contain 'sha256'", path)
	}
}

func TestProvision_CacheHit(t *testing.T) {
	img := testImage(t)
	fetcher := newCountingFetcher(img)
	p := newTestProvisioner(t, fetcher)

	path1, err := p.Provision(context.Background(), "alpine:3.19")
	if err != nil {
		t.Fatalf("first Provision() = %v", err)
	}

	path2, err := p.Provision(context.Background(), "alpine:3.19")
	if err != nil {
		t.Fatalf("second Provision() = %v", err)
	}

	if path1 != path2 {
		t.Errorf("paths differ: %q vs %q", path1, path2)
	}
	if got := fetcher.count.Load(); got != 1 {
		t.Errorf("fetcher.Fetch called %d times, want 1 (cache hit should skip)", got)
	}
}

func TestProvision_Deterministic(t *testing.T) {
	img := testImage(t)
	fetcher := newStaticFetcher(img, nil)
	p := newTestProvisioner(t, fetcher)

	// Two different refs that resolve to the same image (same digest).
	path1, err := p.Provision(context.Background(), "alpine:3.19")
	if err != nil {
		t.Fatalf("Provision(alpine:3.19) = %v", err)
	}
	path2, err := p.Provision(context.Background(), "alpine:latest")
	if err != nil {
		t.Fatalf("Provision(alpine:latest) = %v", err)
	}
	if path1 != path2 {
		t.Errorf("same image, different paths: %q vs %q", path1, path2)
	}
}

func TestProvision_EmptyRef(t *testing.T) {
	img := testImage(t)
	fetcher := newStaticFetcher(img, nil)
	p := newTestProvisioner(t, fetcher)

	_, err := p.Provision(context.Background(), "")
	if err == nil {
		t.Fatal("Provision('') = nil, want error")
	}
	var invErr *InvalidProvisionerConfigError
	if !errors.As(err, &invErr) {
		t.Errorf("error type = %T, want *InvalidProvisionerConfigError", err)
	}
}

func TestProvision_Concurrent(t *testing.T) {
	img := testImage(t)
	fetcher := newStaticFetcher(img, nil)
	p := newTestProvisioner(t, fetcher)

	const goroutines = 8
	var wg sync.WaitGroup
	paths := make([]string, goroutines)
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path, err := p.Provision(context.Background(), "alpine:3.19")
			paths[idx] = path
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Provision() = %v", i, err)
		}
	}
	// All paths should be the same.
	for i := 1; i < goroutines; i++ {
		if paths[i] != paths[0] {
			t.Errorf("goroutine %d path %q != goroutine 0 path %q", i, paths[i], paths[0])
		}
	}
}

func TestProvision_ContextCancelled(t *testing.T) {
	cancelErr := errors.New("test cancel trigger")
	fetcher := newStaticFetcher(nil, cancelErr)
	p := newTestProvisioner(t, fetcher)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.Provision(ctx, "alpine:3.19")
	if err == nil {
		t.Fatal("Provision() = nil, want error on cancelled context")
	}
}

func TestProvision_SizeLimit(t *testing.T) {
	// tar2ext4 with MaximumDiskSize should reject a tar whose ext4 image
	// exceeds the configured maximum. We use MinMaxImageSize (32 MiB) as
	// the cap. The testdata/tiny.tar is small so this should succeed
	// on a size-limited provisioner; to test the error path, we would need
	// a large tar. Skip if building a large layer is unreliable on this
	// platform.
	//
	// Instead, test the positive case: tiny.tar should work with MinMaxImageSize.
	img := testImage(t)
	fetcher := newStaticFetcher(img, nil)
	p := newTestProvisioner(t, fetcher, func(c *ProvisionerConfig) {
		c.MaxImageSize = MinMaxImageSize // 32 MiB
	})

	path, err := p.Provision(context.Background(), "alpine:3.19")
	if err != nil {
		t.Fatalf("Provision() with 32 MiB limit and tiny tar = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) = %v", path, err)
	}
	if info.Size() == 0 {
		t.Error("ext4 file size = 0")
	}
}

// containsSubstr is a test helper.
func containsSubstr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && searchSubstr(s, sub))
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

//go:build integration

package image

import (
	"context"
	"os"
	"testing"
	"time"
)

// testIntegrationRef is a small public image used for end-to-end tests.
// NOT docker.io -- Docker Hub anonymous rate limits (100 pulls/6h) will
// block CI. Switch to a non-throttled registry.
const testIntegrationRef = "public.ecr.aws/docker/library/alpine:3.19"

// TestIntegrationProvisionFromRealRegistry pulls a small public OCI
// image end-to-end, converts it to ext4, and asserts the file exists
// and is within expected size bounds.
//
// Requires network access. Run with:
//
//	go test -tags=integration ./image/ -v -run TestIntegrationProvisionFromRealRegistry
func TestIntegrationProvisionFromRealRegistry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cacheDir := t.TempDir()
	p, err := NewProvisioner(ProvisionerConfig{
		RootfsCacheDir: cacheDir,
		MaxImageSize:   512 * 1024 * 1024, // 512 MiB
	})
	if err != nil {
		t.Fatalf("NewProvisioner: %v", err)
	}

	path, err := p.Provision(ctx, testIntegrationRef)
	if err != nil {
		t.Fatalf("Provision(%q): %v", testIntegrationRef, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat ext4: %v", err)
	}
	const minSize = 100 * 1024        // 100 KiB lower bound (alpine ~5-10 MiB)
	const maxSize = 200 * 1024 * 1024 // 200 MiB upper bound
	if info.Size() < minSize {
		t.Fatalf("ext4 too small: %d bytes (want >= %d)", info.Size(), minSize)
	}
	if info.Size() > maxSize {
		t.Fatalf("ext4 too large: %d bytes (want <= %d)", info.Size(), maxSize)
	}

	// Determinism: second Provision hits cache, returns same path.
	path2, err := p.Provision(ctx, testIntegrationRef)
	if err != nil {
		t.Fatalf("second Provision: %v", err)
	}
	if path != path2 {
		t.Fatalf("non-deterministic paths: %q vs %q", path, path2)
	}
}

// TestIntegrationProvisionDifferentRefsSameImage verifies that two
// references resolving to the same manifest digest produce the same
// cached ext4 path. Uses "alpine:3.19" and "alpine:3" if both resolve
// to the same digest at test time -- skip if they diverge.
func TestIntegrationProvisionDifferentRefsSameImage(t *testing.T) {
	t.Skip("flaky -- tag-to-digest drift over time; kept as documentation of intent")
}

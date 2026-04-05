package image

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestStore_PathFor(t *testing.T) {
	s := NewStore("/tmp/fake")
	h := v1.Hash{Algorithm: "sha256", Hex: "abc123"}
	got := s.PathFor(h)
	want := "/tmp/fake/sha256/abc123.ext4"
	if got != want {
		t.Errorf("PathFor(%v) = %q, want %q", h, got, want)
	}
}

func TestStore_PathFor_UsesConfiguredDirVerbatim(t *testing.T) {
	// NewStore("") should use empty dir verbatim - defaults live on ProvisionerConfig.
	s := NewStore("")
	h := v1.Hash{Algorithm: "sha256", Hex: "abc"}
	got := s.PathFor(h)
	want := "sha256/abc.ext4"
	if got != want {
		t.Errorf("PathFor with empty dir = %q, want %q", got, want)
	}
}

func TestStore_Init(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("first Init() = %v, want nil", err)
	}
	// Second call: idempotent.
	if err := s.Init(); err != nil {
		t.Fatalf("second Init() = %v, want nil", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(%s) = %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%s is not a directory", dir)
	}
	// Permission bits should include 0o750.
	mode := info.Mode().Perm()
	if mode&0o750 != 0o750 {
		t.Errorf("mode = %o, want 0o750 bits set", mode)
	}
	// And should NOT have world-read/write/exec bits.
	if mode&0o007 != 0 {
		t.Errorf("mode = %o, world bits should be clear", mode)
	}
}

func TestStore_Exists(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	h := v1.Hash{Algorithm: "sha256", Hex: "deadbeef"}
	if s.Exists(h) {
		t.Errorf("Exists(h) = true before write, want false")
	}
	if err := s.AtomicWrite(h, strings.NewReader("hi")); err != nil {
		t.Fatalf("AtomicWrite() = %v", err)
	}
	if !s.Exists(h) {
		t.Errorf("Exists(h) = false after write, want true")
	}
}

func TestStore_AtomicWrite_SingleWriter(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	h := v1.Hash{Algorithm: "sha256", Hex: "feedface"}
	if err := s.AtomicWrite(h, strings.NewReader("hello")); err != nil {
		t.Fatalf("AtomicWrite() = %v", err)
	}
	// Verify the final file exists with correct content length.
	final := s.PathFor(h)
	info, err := os.Stat(final)
	if err != nil {
		t.Fatalf("Stat(%s) = %v", final, err)
	}
	if info.Size() != 5 {
		t.Errorf("size = %d, want 5", info.Size())
	}
	// Final file should be 0o444 (read-only).
	mode := info.Mode().Perm()
	if mode != 0o444 {
		t.Errorf("mode = %o, want 0o444", mode)
	}
	// Verify no .tmp-* files remain in the dir.
	subDir := filepath.Dir(final)
	entries, err := os.ReadDir(subDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) = %v", subDir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("found leftover tmp file: %s", e.Name())
		}
	}
}

func TestStore_AtomicWrite_Concurrent(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	if err := s.Init(); err != nil {
		t.Fatalf("Init() = %v", err)
	}
	h := v1.Hash{Algorithm: "sha256", Hex: "cafebabe"}
	const writers = 8
	var wg sync.WaitGroup
	errCh := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.AtomicWrite(h, strings.NewReader("shared-content")); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent AtomicWrite returned error: %v", err)
	}
	// Final file exists.
	final := s.PathFor(h)
	if _, err := os.Stat(final); err != nil {
		t.Errorf("final file missing: %v", err)
	}
	// No .tmp-* leftovers.
	subDir := filepath.Dir(final)
	entries, err := os.ReadDir(subDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) = %v", subDir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("found leftover tmp file after concurrent writes: %s", e.Name())
		}
	}
}

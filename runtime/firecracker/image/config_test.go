package image

import (
	"errors"
	"testing"
)

func TestProvisionerConfig_WithDefaults_AllZero(t *testing.T) {
	c := ProvisionerConfig{}.withDefaults()
	if c.RootfsCacheDir != DefaultRootfsCacheDir {
		t.Errorf("RootfsCacheDir = %q, want %q", c.RootfsCacheDir, DefaultRootfsCacheDir)
	}
	if c.MaxImageSize != DefaultMaxImageSize {
		t.Errorf("MaxImageSize = %d, want %d", c.MaxImageSize, DefaultMaxImageSize)
	}
	const wantSize int64 = 2 * 1024 * 1024 * 1024
	if c.MaxImageSize != wantSize {
		t.Errorf("MaxImageSize = %d, want %d (2 GiB)", c.MaxImageSize, wantSize)
	}
	if c.DefaultPlatform != DefaultPlatform {
		t.Errorf("DefaultPlatform = %q, want %q", c.DefaultPlatform, DefaultPlatform)
	}
	if c.DefaultPlatform != "linux/amd64" {
		t.Errorf("DefaultPlatform = %q, want %q", c.DefaultPlatform, "linux/amd64")
	}
}

func TestProvisionerConfig_WithDefaults_PreservesRootfsCacheDir(t *testing.T) {
	c := ProvisionerConfig{RootfsCacheDir: "/tmp/x"}.withDefaults()
	if c.RootfsCacheDir != "/tmp/x" {
		t.Errorf("RootfsCacheDir = %q, want %q", c.RootfsCacheDir, "/tmp/x")
	}
	if c.MaxImageSize != DefaultMaxImageSize {
		t.Errorf("MaxImageSize = %d, want %d (filled from default)", c.MaxImageSize, DefaultMaxImageSize)
	}
	if c.DefaultPlatform != DefaultPlatform {
		t.Errorf("DefaultPlatform = %q, want %q (filled from default)", c.DefaultPlatform, DefaultPlatform)
	}
}

func TestProvisionerConfig_Validate_TooSmallMaxImageSize(t *testing.T) {
	c := ProvisionerConfig{
		RootfsCacheDir:  "/x",
		MaxImageSize:    16 * 1024 * 1024, // 16 MiB (below MinMaxImageSize)
		DefaultPlatform: "linux/amd64",
	}
	err := c.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want non-nil error for MaxImageSize below minimum")
	}
	var invErr *InvalidProvisionerConfigError
	if !errors.As(err, &invErr) {
		t.Fatalf("Validate() returned %T, want *InvalidProvisionerConfigError", err)
	}
	if invErr.Field != "MaxImageSize" {
		t.Errorf("Field = %q, want %q", invErr.Field, "MaxImageSize")
	}
}

func TestProvisionerConfig_Validate_Valid(t *testing.T) {
	c := ProvisionerConfig{
		RootfsCacheDir:  "/x",
		MaxImageSize:    64 * 1024 * 1024,
		DefaultPlatform: "linux/amd64",
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestProvisionerConfig_Validate_EmptyRootfsCacheDir(t *testing.T) {
	c := ProvisionerConfig{
		RootfsCacheDir:  "",
		MaxImageSize:    64 * 1024 * 1024,
		DefaultPlatform: "linux/amd64",
	}
	err := c.Validate()
	if err == nil {
		t.Fatalf("Validate() = nil, want non-nil error for empty RootfsCacheDir")
	}
	var invErr *InvalidProvisionerConfigError
	if !errors.As(err, &invErr) {
		t.Fatalf("Validate() returned %T, want *InvalidProvisionerConfigError", err)
	}
	if invErr.Field != "RootfsCacheDir" {
		t.Errorf("Field = %q, want %q", invErr.Field, "RootfsCacheDir")
	}
}

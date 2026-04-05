package firecracker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateKernelImage_MissingFile(t *testing.T) {
	err := ValidateKernelImage("/nonexistent/path/vmlinux")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestValidateKernelImage_Directory(t *testing.T) {
	dir := t.TempDir()
	err := ValidateKernelImage(dir)
	if err == nil {
		t.Fatal("expected error for directory, got nil")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error should mention 'directory', got: %v", err)
	}
}

func TestValidateKernelImage_ValidFile(t *testing.T) {
	kernelPath := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernelPath, []byte("fake-kernel-binary-data"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := ValidateKernelImage(kernelPath); err != nil {
		t.Fatalf("expected nil for valid kernel file, got: %v", err)
	}
}

func TestValidateKernelImage_EmptyFile(t *testing.T) {
	kernelPath := filepath.Join(t.TempDir(), "vmlinux-empty")
	if err := os.WriteFile(kernelPath, []byte{}, 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	err := ValidateKernelImage(kernelPath)
	if err == nil {
		t.Fatal("expected error for empty file, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestKernelManifest_Constants(t *testing.T) {
	if DefaultKernelVersion != "5.10" {
		t.Errorf("expected DefaultKernelVersion='5.10', got %q", DefaultKernelVersion)
	}
	if !strings.Contains(DefaultKernelArgs, "console=ttyS0") {
		t.Errorf("expected DefaultKernelArgs to contain 'console=ttyS0', got %q", DefaultKernelArgs)
	}
	if !strings.Contains(DefaultKernelArgs, "reboot=k") {
		t.Errorf("expected DefaultKernelArgs to contain 'reboot=k', got %q", DefaultKernelArgs)
	}
	if !strings.Contains(DefaultKernelArgs, "pci=off") {
		t.Errorf("expected DefaultKernelArgs to contain 'pci=off', got %q", DefaultKernelArgs)
	}
}

func TestResolveKernelPath_Valid(t *testing.T) {
	kernelPath := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernelPath, []byte("fake-kernel"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	manifest := KernelManifest{
		Version:   "5.10",
		ImagePath: kernelPath,
	}
	resolved, err := ResolveKernelPath(manifest)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if resolved != kernelPath {
		t.Errorf("expected %q, got %q", kernelPath, resolved)
	}
}

func TestResolveKernelPath_EmptyImagePath(t *testing.T) {
	manifest := KernelManifest{
		Version:   "5.10",
		ImagePath: "",
	}
	_, err := ResolveKernelPath(manifest)
	if err == nil {
		t.Fatal("expected error for empty ImagePath, got nil")
	}
	if !strings.Contains(err.Error(), "ImagePath") {
		t.Errorf("error should mention ImagePath, got: %v", err)
	}
}

func TestResolveKernelPath_NonExistentFile(t *testing.T) {
	manifest := KernelManifest{
		Version:   "5.10",
		ImagePath: "/nonexistent/vmlinux",
	}
	_, err := ResolveKernelPath(manifest)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestDefaultKernelURL_Constants(t *testing.T) {
	if DefaultKernelURL == "" {
		t.Fatal("DefaultKernelURL must not be empty")
	}
	if !strings.HasPrefix(DefaultKernelURL, "https://") {
		t.Errorf("DefaultKernelURL should use https scheme, got %q", DefaultKernelURL)
	}
	if !strings.Contains(DefaultKernelURL, "vmlinux") {
		t.Errorf("DefaultKernelURL should reference a vmlinux artifact, got %q", DefaultKernelURL)
	}
	if !strings.Contains(DefaultKernelURL, "5.10") {
		t.Errorf("DefaultKernelURL should reference the pinned 5.10 kernel, got %q", DefaultKernelURL)
	}
	if DefaultKernelSHA256 == "" {
		t.Fatal("DefaultKernelSHA256 must not be empty")
	}
	if len(DefaultKernelSHA256) != 64 {
		t.Errorf("DefaultKernelSHA256 should be a 64-char hex SHA256, got len=%d (%q)", len(DefaultKernelSHA256), DefaultKernelSHA256)
	}
	for _, r := range DefaultKernelSHA256 {
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
		if !isHex {
			t.Errorf("DefaultKernelSHA256 contains non-hex character %q: %q", r, DefaultKernelSHA256)
			break
		}
	}
}

func TestVerifyKernelChecksum_Matches(t *testing.T) {
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	kernelPath := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernelPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if err := VerifyKernelChecksum(kernelPath, expected); err != nil {
		t.Fatalf("expected nil error on matching checksum, got: %v", err)
	}
}

func TestVerifyKernelChecksum_CaseInsensitive(t *testing.T) {
	kernelPath := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernelPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	upperExpected := "2CF24DBA5FB0A30E26E83B2AC5B9E29E1B161E5C1FA7425E73043362938B9824"
	if err := VerifyKernelChecksum(kernelPath, upperExpected); err != nil {
		t.Fatalf("expected case-insensitive match, got: %v", err)
	}
}

func TestVerifyKernelChecksum_Mismatch(t *testing.T) {
	kernelPath := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernelPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	wrong := "0000000000000000000000000000000000000000000000000000000000000000"
	err := VerifyKernelChecksum(kernelPath, wrong)
	if err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error should mention 'mismatch', got: %v", err)
	}
}

func TestVerifyKernelChecksum_MissingFile(t *testing.T) {
	err := VerifyKernelChecksum("/nonexistent/vmlinux", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestVerifyKernelChecksum_EmptyExpected(t *testing.T) {
	kernelPath := filepath.Join(t.TempDir(), "vmlinux")
	if err := os.WriteFile(kernelPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	err := VerifyKernelChecksum(kernelPath, "")
	if err == nil {
		t.Fatal("expected error for empty expected digest, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestVerifyKernelChecksum_EmptyFile(t *testing.T) {
	// sha256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
	kernelPath := filepath.Join(t.TempDir(), "vmlinux-empty")
	if err := os.WriteFile(kernelPath, []byte{}, 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if err := VerifyKernelChecksum(kernelPath, expected); err != nil {
		t.Fatalf("expected empty-file SHA256 to match, got: %v", err)
	}
}

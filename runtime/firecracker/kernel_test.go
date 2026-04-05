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

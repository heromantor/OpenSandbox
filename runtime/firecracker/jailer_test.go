package firecracker

import (
	"strings"
	"testing"
)

func TestJailerOpts_ValidateRejectsZeroUID(t *testing.T) {
	opts := JailerOpts{
		UID:           0,
		GID:           1000,
		ChrootBaseDir: "/srv/jailer",
	}
	err := validateJailerOpts(opts)
	if err == nil {
		t.Fatal("expected error for UID=0, got nil")
	}
	if !strings.Contains(err.Error(), "UID") {
		t.Errorf("error should mention UID, got: %v", err)
	}
}

func TestJailerOpts_ValidateRejectsZeroGID(t *testing.T) {
	opts := JailerOpts{
		UID:           1000,
		GID:           0,
		ChrootBaseDir: "/srv/jailer",
	}
	err := validateJailerOpts(opts)
	if err == nil {
		t.Fatal("expected error for GID=0, got nil")
	}
	if !strings.Contains(err.Error(), "GID") {
		t.Errorf("error should mention GID, got: %v", err)
	}
}

func TestJailerOpts_ValidateRejectsEmptyChroot(t *testing.T) {
	opts := JailerOpts{
		UID:           1000,
		GID:           1000,
		ChrootBaseDir: "",
	}
	err := validateJailerOpts(opts)
	if err == nil {
		t.Fatal("expected error for empty ChrootBaseDir, got nil")
	}
	if !strings.Contains(err.Error(), "ChrootBaseDir") {
		t.Errorf("error should mention ChrootBaseDir, got: %v", err)
	}
}

func TestJailerOpts_ValidateSocketPathLength(t *testing.T) {
	// Create a ChrootBaseDir long enough that the full socket path exceeds 108 chars.
	// Format: <base>/firecracker/<uuid>/root/run/firecracker.socket
	// UUID placeholder is 36 chars. We need the total to exceed 108.
	longBase := "/" + strings.Repeat("a", 80)
	opts := JailerOpts{
		UID:           1000,
		GID:           1000,
		ChrootBaseDir: longBase,
	}
	err := validateJailerOpts(opts)
	if err == nil {
		t.Fatal("expected error for long ChrootBaseDir, got nil")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "108") && !strings.Contains(errMsg, "socket") && !strings.Contains(errMsg, "path") {
		t.Errorf("error should mention socket path length, got: %v", err)
	}
}

func TestJailerOpts_ValidateAcceptsValidOpts(t *testing.T) {
	opts := JailerOpts{
		UID:           1000,
		GID:           1000,
		ChrootBaseDir: "/srv/jailer",
	}
	if err := validateJailerOpts(opts); err != nil {
		t.Fatalf("expected nil error for valid opts, got: %v", err)
	}
}

func TestDetectCgroupVersion(t *testing.T) {
	version := detectCgroupVersion()
	if version != "1" && version != "2" {
		t.Fatalf("expected cgroup version '1' or '2', got %q", version)
	}
}

func TestResolveJailerConfig_ValidOpts(t *testing.T) {
	resolved, err := resolveJailerConfig("test-vm-123", JailerOpts{
		UID:           1000,
		GID:           1000,
		ChrootBaseDir: "/srv/jailer",
		CgroupVersion: "2",
	}, "/usr/bin/firecracker")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if resolved.id != "test-vm-123" {
		t.Errorf("expected id=test-vm-123, got %s", resolved.id)
	}
	if resolved.uid != 1000 {
		t.Errorf("expected uid=1000, got %d", resolved.uid)
	}
	if resolved.gid != 1000 {
		t.Errorf("expected gid=1000, got %d", resolved.gid)
	}
	if resolved.execFile != "/usr/bin/firecracker" {
		t.Errorf("expected execFile=/usr/bin/firecracker, got %s", resolved.execFile)
	}
	if resolved.chrootBaseDir != "/srv/jailer" {
		t.Errorf("expected chrootBaseDir=/srv/jailer, got %s", resolved.chrootBaseDir)
	}
	if resolved.cgroupVersion != "2" {
		t.Errorf("expected cgroupVersion=2, got %s", resolved.cgroupVersion)
	}
	if !resolved.daemonize {
		t.Error("expected daemonize=true (default)")
	}
	if resolved.numaNode != -1 {
		t.Errorf("expected numaNode=-1 (default), got %d", resolved.numaNode)
	}
}

func TestResolveJailerConfig_RejectsInvalidOpts(t *testing.T) {
	// UID=0 should fail validation even after resolveJailerConfig applies defaults.
	_, err := resolveJailerConfig("test-vm", JailerOpts{
		UID:           0,
		GID:           1000,
		ChrootBaseDir: "/srv/jailer",
		CgroupVersion: "2",
	}, "/usr/bin/firecracker")
	if err == nil {
		t.Fatal("expected error for UID=0, got nil")
	}
}

func TestResolveJailerConfig_AutoDetectsCgroupVersion(t *testing.T) {
	// Empty CgroupVersion should be auto-detected.
	resolved, err := resolveJailerConfig("test-vm", JailerOpts{
		UID:           1000,
		GID:           1000,
		ChrootBaseDir: "/srv/jailer",
		CgroupVersion: "", // should auto-detect
	}, "/usr/bin/firecracker")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if resolved.cgroupVersion != "1" && resolved.cgroupVersion != "2" {
		t.Errorf("expected auto-detected cgroup version '1' or '2', got %q", resolved.cgroupVersion)
	}
}

func TestChrootDir(t *testing.T) {
	result := chrootDir("/srv/jailer", "abc-123")
	expected := "/srv/jailer/firecracker/abc-123/root"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestChrootDir_DifferentBase(t *testing.T) {
	result := chrootDir("/var/lib/jailer", "my-vm-456")
	expected := "/var/lib/jailer/firecracker/my-vm-456/root"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestSocketPathInChroot(t *testing.T) {
	result := socketPathInChroot()
	expected := "run/firecracker.socket"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

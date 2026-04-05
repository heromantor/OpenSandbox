package image

import (
	"strings"
	"testing"
)

func TestParseReference_Canonical(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		wantCanonicalHas  string
		wantRegistryHas   string
		wantRepositoryHas string
	}{
		{
			name:              "alpine short form expands to docker.io default registry",
			input:             "alpine:3.19",
			wantCanonicalHas:  "index.docker.io",
			wantRegistryHas:   "docker.io",
			wantRepositoryHas: "alpine",
		},
		{
			name:              "ghcr.io explicit registry preserved",
			input:             "ghcr.io/foo/bar:v1",
			wantCanonicalHas:  "ghcr.io/foo/bar:v1",
			wantRegistryHas:   "ghcr.io",
			wantRepositoryHas: "foo/bar",
		},
		{
			name:              "public.ecr.aws explicit registry preserved",
			input:             "public.ecr.aws/x/y:z",
			wantCanonicalHas:  "public.ecr.aws/x/y:z",
			wantRegistryHas:   "public.ecr.aws",
			wantRepositoryHas: "x/y",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ref, err := ParseReference(tc.input)
			if err != nil {
				t.Fatalf("ParseReference(%q) returned error: %v", tc.input, err)
			}
			if ref.Raw != tc.input {
				t.Errorf("Raw = %q, want %q", ref.Raw, tc.input)
			}
			if ref.Canonical == "" {
				t.Errorf("Canonical is empty")
			}
			if !strings.Contains(ref.Canonical, tc.wantCanonicalHas) {
				t.Errorf("Canonical = %q, want to contain %q", ref.Canonical, tc.wantCanonicalHas)
			}
			if ref.Registry == "" {
				t.Errorf("Registry is empty")
			}
			if !strings.Contains(ref.Registry, tc.wantRegistryHas) {
				t.Errorf("Registry = %q, want to contain %q", ref.Registry, tc.wantRegistryHas)
			}
			if ref.Repository == "" {
				t.Errorf("Repository is empty")
			}
			if !strings.Contains(ref.Repository, tc.wantRepositoryHas) {
				t.Errorf("Repository = %q, want to contain %q", ref.Repository, tc.wantRepositoryHas)
			}
			if ref.Identifier == "" {
				t.Errorf("Identifier is empty")
			}
		})
	}
}

func TestParseReference_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty string", input: ""},
		{name: "invalid chars", input: "NOT VALID!"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseReference(tc.input)
			if err == nil {
				t.Fatalf("ParseReference(%q) returned nil error, want non-nil", tc.input)
			}
		})
	}
}

func TestErrors_InvalidProvisionerConfigError(t *testing.T) {
	e := &InvalidProvisionerConfigError{Field: "MaxImageSize", Message: "must be >= 32 MiB"}
	got := e.Error()
	want := "firecracker: image: invalid config: MaxImageSize: must be >= 32 MiB"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestErrors_ImagePullError(t *testing.T) {
	cause := &stubError{msg: "network down"}
	e := &ImagePullError{Ref: "alpine:3.19", Cause: cause}
	got := e.Error()
	want := "firecracker: image: pull alpine:3.19: network down"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if e.Unwrap() != cause {
		t.Errorf("Unwrap() did not return the wrapped cause")
	}
}

func TestErrors_Ext4ConvertError(t *testing.T) {
	cause := &stubError{msg: "disk full"}
	e := &Ext4ConvertError{Cause: cause}
	got := e.Error()
	want := "firecracker: image: ext4 convert: disk full"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if e.Unwrap() != cause {
		t.Errorf("Unwrap() did not return the wrapped cause")
	}
}

func TestErrors_CacheError(t *testing.T) {
	cause := &stubError{msg: "permission denied"}
	e := &CacheError{Op: "write", Cause: cause}
	got := e.Error()
	want := "firecracker: image: cache: write: permission denied"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if e.Unwrap() != cause {
		t.Errorf("Unwrap() did not return the wrapped cause")
	}
}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

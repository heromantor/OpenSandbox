package image

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestCraneFetcher_Implements_ImageFetcher(t *testing.T) {
	var _ ImageFetcher = NewCraneFetcher()
}

func TestStaticFetcher_Implements_ImageFetcher(t *testing.T) {
	var _ ImageFetcher = newStaticFetcher(nil, nil)
}

func TestStaticFetcher_ReturnsImage(t *testing.T) {
	wantErr := errors.New("boom")
	f := newStaticFetcher(nil, wantErr)
	_, err := f.Fetch(context.Background(), "test", nil)
	if !errors.Is(err, wantErr) {
		t.Errorf("Fetch() error = %v, want %v", err, wantErr)
	}
}

func TestCraneFetcher_WrapsErrorInImagePullError(t *testing.T) {
	f := NewCraneFetcher()
	// Pull a non-existent reference to trigger an error.
	_, err := f.Fetch(context.Background(), "invalid-ref-!!", &v1.Platform{OS: "linux", Architecture: "amd64"})
	if err == nil {
		t.Fatal("Fetch() = nil, want ImagePullError for invalid ref")
	}
	var pullErr *ImagePullError
	if !errors.As(err, &pullErr) {
		t.Errorf("Fetch() error type = %T, want *ImagePullError", err)
	}
}

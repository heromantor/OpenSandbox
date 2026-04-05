package image

import (
	"context"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// ImageFetcher resolves an OCI reference into a v1.Image. This seam
// lets tests swap the real registry-backed fetcher (craneFetcher) for
// an offline static implementation.
type ImageFetcher interface {
	Fetch(ctx context.Context, ref string, platform *v1.Platform) (v1.Image, error)
}

// craneFetcher is the production ImageFetcher: it pulls from an
// OCI registry via go-containerregistry.
type craneFetcher struct{}

// NewCraneFetcher returns the production registry-backed fetcher.
func NewCraneFetcher() ImageFetcher { return craneFetcher{} }

func (craneFetcher) Fetch(ctx context.Context, ref string, platform *v1.Platform) (v1.Image, error) {
	opts := []crane.Option{}
	if platform != nil {
		opts = append(opts, crane.WithPlatform(platform))
	}
	opts = append(opts, crane.WithContext(ctx))
	img, err := crane.Pull(ref, opts...)
	if err != nil {
		return nil, &ImagePullError{Ref: ref, Cause: err}
	}
	return img, nil
}

// staticFetcher returns a preloaded image for tests. Exported at package
// scope (not in _test.go) because provisioner_test.go uses it across files.
type staticFetcher struct {
	img v1.Image
	err error
}

func newStaticFetcher(img v1.Image, err error) ImageFetcher {
	return &staticFetcher{img: img, err: err}
}

func (f *staticFetcher) Fetch(_ context.Context, _ string, _ *v1.Platform) (v1.Image, error) {
	return f.img, f.err
}

package image

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
)

// Reference wraps name.Reference so callers don't need to import name
// directly. Fields are exported to make parsed components available for
// logging, metrics, and cache-key construction.
type Reference struct {
	// Raw is the user-supplied reference string (pre-normalization).
	Raw string
	// Canonical is the fully-qualified reference as rendered by
	// name.Reference.Name(), which normalizes short refs so "alpine:3.19"
	// becomes "index.docker.io/library/alpine:3.19".
	Canonical string
	// Registry is the registry host (e.g. "index.docker.io", "ghcr.io").
	Registry string
	// Repository is the repository path (e.g. "library/alpine", "foo/bar").
	Repository string
	// Identifier is the tag or digest hex portion of the reference.
	Identifier string
}

// ParseReference parses and normalizes an OCI reference string using the
// go-containerregistry name package. It returns a non-nil error for empty
// or malformed references. name.ParseReference's defaults expand short
// refs such as "alpine:3.19" to "index.docker.io/library/alpine:3.19".
func ParseReference(ref string) (Reference, error) {
	parsed, err := name.ParseReference(ref)
	if err != nil {
		return Reference{}, fmt.Errorf("firecracker: image: parse reference %q: %w", ref, err)
	}
	ctx := parsed.Context()
	return Reference{
		Raw:        ref,
		Canonical:  parsed.Name(),
		Registry:   ctx.RegistryStr(),
		Repository: ctx.RepositoryStr(),
		Identifier: parsed.Identifier(),
	}, nil
}

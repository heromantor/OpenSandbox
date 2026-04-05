package image

import "fmt"

// InvalidProvisionerConfigError is returned when a ProvisionerConfig field
// has an invalid value. Field names the offending field; Message describes
// the constraint violated.
type InvalidProvisionerConfigError struct {
	Field   string
	Message string
}

// Error returns the error message for InvalidProvisionerConfigError.
func (e *InvalidProvisionerConfigError) Error() string {
	return fmt.Sprintf("firecracker: image: invalid config: %s: %s", e.Field, e.Message)
}

// ImagePullError wraps errors encountered while pulling an OCI image by
// reference. Ref is the reference string that was attempted; Cause is the
// underlying error from the registry client.
type ImagePullError struct {
	Ref   string
	Cause error
}

// Error returns the error message for ImagePullError.
func (e *ImagePullError) Error() string {
	return fmt.Sprintf("firecracker: image: pull %s: %v", e.Ref, e.Cause)
}

// Unwrap returns the underlying cause of the pull failure.
func (e *ImagePullError) Unwrap() error { return e.Cause }

// Ext4ConvertError wraps errors encountered during tar -> ext4 conversion.
type Ext4ConvertError struct {
	Cause error
}

// Error returns the error message for Ext4ConvertError.
func (e *Ext4ConvertError) Error() string {
	return fmt.Sprintf("firecracker: image: ext4 convert: %v", e.Cause)
}

// Unwrap returns the underlying cause of the conversion failure.
func (e *Ext4ConvertError) Unwrap() error { return e.Cause }

// CacheError wraps errors from the on-disk ext4 cache (Store). Op is a
// short verb naming the cache operation (e.g. "write", "rename", "stat",
// "init"); Cause is the underlying filesystem error.
type CacheError struct {
	Op    string
	Cause error
}

// Error returns the error message for CacheError.
func (e *CacheError) Error() string {
	return fmt.Sprintf("firecracker: image: cache: %s: %v", e.Op, e.Cause)
}

// Unwrap returns the underlying cause of the cache failure.
func (e *CacheError) Unwrap() error { return e.Cause }

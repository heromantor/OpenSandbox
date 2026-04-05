package firecracker

import (
	"fmt"
	"strings"
)

// VMNotFoundError is returned when a VM with the given ID does not exist
// in the manager's registry.
type VMNotFoundError struct {
	VMID string
}

// Error returns the error message for VMNotFoundError.
func (e *VMNotFoundError) Error() string {
	return fmt.Sprintf("firecracker: vm not found: %s", e.VMID)
}

// VMAlreadyExistsError is returned when attempting to create a VM with an
// ID that already exists in the manager's registry.
type VMAlreadyExistsError struct {
	VMID string
}

// Error returns the error message for VMAlreadyExistsError.
func (e *VMAlreadyExistsError) Error() string {
	return fmt.Sprintf("firecracker: vm already exists: %s", e.VMID)
}

// InvalidVMConfigError is returned when a VMConfig field has an invalid value.
type InvalidVMConfigError struct {
	Field   string
	Message string
}

// Error returns the error message for InvalidVMConfigError.
func (e *InvalidVMConfigError) Error() string {
	return fmt.Sprintf("firecracker: invalid config: %s: %s", e.Field, e.Message)
}

// VMStartError is returned when a VM fails to start.
type VMStartError struct {
	VMID  string
	Cause error
}

// Error returns the error message for VMStartError.
func (e *VMStartError) Error() string {
	return fmt.Sprintf("firecracker: vm start failed: %s: %v", e.VMID, e.Cause)
}

// Unwrap returns the underlying cause of the start failure.
func (e *VMStartError) Unwrap() error { return e.Cause }

// VMStopError is returned when a VM fails to stop.
type VMStopError struct {
	VMID  string
	Cause error
}

// Error returns the error message for VMStopError.
func (e *VMStopError) Error() string {
	return fmt.Sprintf("firecracker: vm stop failed: %s: %v", e.VMID, e.Cause)
}

// Unwrap returns the underlying cause of the stop failure.
func (e *VMStopError) Unwrap() error { return e.Cause }

// CleanupError is returned when one or more resource cleanup operations fail
// during VM destruction.
type CleanupError struct {
	VMID   string
	Errors []error
}

// Error returns the error message for CleanupError, listing all sub-errors.
func (e *CleanupError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("firecracker: cleanup failed: %s: no errors", e.VMID)
	}
	msgs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		msgs[i] = err.Error()
	}
	return fmt.Sprintf("firecracker: cleanup failed: %s: %s", e.VMID, strings.Join(msgs, "; "))
}

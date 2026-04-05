//go:build linux

package firecracker

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
)

// cleanupNetwork removes the TAP device and iptables NAT/FORWARD rules
// associated with this VM. Both operations are idempotent: missing devices
// and rules are silently ignored.
func (r *VMResources) cleanupNetwork() error {
	var result *multierror.Error
	if r.TAPDeviceName != "" {
		if err := DeleteTAPDevice(r.TAPDeviceName); err != nil {
			result = multierror.Append(result,
				fmt.Errorf("remove tap %s: %w", r.TAPDeviceName, err))
		}
	}
	if r.NATRules != nil {
		if err := r.NATRules.Remove(); err != nil {
			result = multierror.Append(result,
				fmt.Errorf("remove nat rules: %w", err))
		}
	}
	return result.ErrorOrNil()
}

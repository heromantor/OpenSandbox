// Package firecracker provides types and interfaces for managing Firecracker
// microVM networking, including subnet allocation, TAP device naming, MAC
// address generation, and iptables NAT rule definitions.
package firecracker

import (
	"net"
)

const (
	// DefaultSubnetBase is 172.16.0.0 expressed as a uint32 for arithmetic.
	DefaultSubnetBase uint32 = 0xAC100000

	// MaxSubnetIndex is the maximum valid index for /30 subnet allocation
	// within the 172.16.0.0/16 range. There are 16384 /30 subnets available.
	MaxSubnetIndex uint32 = 16383

	// maxTAPNameLen is the maximum length of a TAP device name (IFNAMSIZ - 1).
	maxTAPNameLen = 15
)

// DefaultNameservers are the DNS nameservers configured for guests by default.
var DefaultNameservers = []string{"8.8.8.8", "8.8.4.4"}

// SubnetAllocation holds the IP addresses and subnet computed for a VM's
// point-to-point /30 network segment.
type SubnetAllocation struct {
	HostIP    net.IP
	GuestIP   net.IP
	Subnet    net.IPNet
	GatewayIP net.IP
}

// AllocateSubnet maps a zero-based index to a /30 subnet within 172.16.0.0/16.
// Index 0 returns 172.16.0.0/30 (host .1, guest .2). Returns error if index
// exceeds MaxSubnetIndex.
func AllocateSubnet(index uint32) (SubnetAllocation, error) {
	return SubnetAllocation{}, nil
}

// GenerateMAC returns a locally-administered MAC address with the prefix
// 02:FC (Firecracker mnemonic). The remaining 4 bytes are crypto-random.
func GenerateMAC() (string, error) {
	return "", nil
}

// TAPDeviceName returns a TAP device name derived from the VM ID, truncated
// to respect the IFNAMSIZ limit of 15 characters.
func TAPDeviceName(vmID string) string {
	return ""
}

// NetworkConfig holds the network parameters for creating a VM's network
// environment (TAP device, iptables rules, DNS).
type NetworkConfig struct {
	SubnetIndex   uint32
	HostInterface string
	Nameservers   []string
}

// Validate checks that NetworkConfig fields are within acceptable ranges.
func (c *NetworkConfig) Validate() error {
	return nil
}

// NATRules holds the parameters for iptables NAT rules associated with a VM.
type NATRules struct {
	GuestIP   string
	TAPName   string
	HostIface string
}

// Package firecracker provides types and interfaces for managing Firecracker
// microVM networking, including subnet allocation, TAP device naming, MAC
// address generation, and iptables NAT rule definitions.
package firecracker

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync/atomic"
)

const (
	// DefaultSubnetBase is 172.16.0.0 expressed as a uint32 for arithmetic.
	DefaultSubnetBase uint32 = 0xAC100000

	// MaxSubnetIndex is the maximum valid index for /30 subnet allocation
	// within the 172.16.0.0/16 range. There are 16384 /30 subnets available.
	MaxSubnetIndex uint32 = 16383

	// maxTAPNameLen is the maximum length of a TAP device name (IFNAMSIZ - 1).
	maxTAPNameLen = 15

	// tapPrefix is the prefix for TAP device names.
	tapPrefix = "tap-"
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
	if index > MaxSubnetIndex {
		return SubnetAllocation{}, fmt.Errorf(
			"firecracker: subnet index %d exceeds maximum %d for 172.16.0.0/16",
			index, MaxSubnetIndex,
		)
	}

	offset := index * 4
	baseAddr := DefaultSubnetBase + offset

	return SubnetAllocation{
		HostIP:    uint32ToIP(baseAddr + 1),
		GuestIP:   uint32ToIP(baseAddr + 2),
		GatewayIP: uint32ToIP(baseAddr + 1),
		Subnet: net.IPNet{
			IP:   uint32ToIP(baseAddr),
			Mask: net.CIDRMask(30, 32),
		},
	}, nil
}

// uint32ToIP converts a uint32 in host byte order to a net.IP (4-byte IPv4).
func uint32ToIP(addr uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, addr)
	return ip
}

// GenerateMAC returns a locally-administered MAC address with the prefix
// 02:FC (Firecracker mnemonic). The remaining 4 bytes are crypto-random.
func GenerateMAC() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("firecracker: generate mac: %w", err)
	}
	return fmt.Sprintf("02:fc:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3]), nil
}

// TAPDeviceName returns a TAP device name derived from the VM ID, truncated
// to respect the IFNAMSIZ limit of 15 characters. The format is "tap-{id}"
// where id is truncated to fit within the limit.
func TAPDeviceName(vmID string) string {
	maxIDLen := maxTAPNameLen - len(tapPrefix)
	if len(vmID) > maxIDLen {
		vmID = vmID[:maxIDLen]
	}
	return tapPrefix + vmID
}

// NetworkConfig holds the network parameters for creating a VM's network
// environment (TAP device, iptables rules, DNS).
type NetworkConfig struct {
	// SubnetIndex selects which /30 subnet from 172.16.0.0/16 to allocate.
	SubnetIndex uint32
	// HostInterface is the host outbound interface for iptables NAT rules.
	HostInterface string
	// Nameservers are the DNS nameservers for guest resolution (max 2).
	Nameservers []string
}

// Validate checks that NetworkConfig fields are within acceptable ranges.
// Returns an InvalidVMConfigError for the first validation failure.
func (c *NetworkConfig) Validate() error {
	if c.HostInterface == "" {
		return &InvalidVMConfigError{
			Field:   "NetworkConfig.HostInterface",
			Message: "must not be empty",
		}
	}
	if c.SubnetIndex > MaxSubnetIndex {
		return &InvalidVMConfigError{
			Field:   "NetworkConfig.SubnetIndex",
			Message: fmt.Sprintf("must be <= %d, got %d", MaxSubnetIndex, c.SubnetIndex),
		}
	}
	if len(c.Nameservers) > 2 {
		return &InvalidVMConfigError{
			Field:   "NetworkConfig.Nameservers",
			Message: fmt.Sprintf("max 2 nameservers supported by SDK, got %d", len(c.Nameservers)),
		}
	}
	return nil
}

// NATRules holds the parameters for iptables NAT rules associated with a VM.
// Apply and Remove methods are defined in network_linux.go.
type NATRules struct {
	GuestIP   string
	TAPName   string
	HostIface string
}

// SubnetIndexAllocator provides atomic, monotonically increasing subnet indices
// for TAP network allocation. Each Allocate call returns a unique index starting
// from 0, safe for concurrent use.
type SubnetIndexAllocator struct {
	next atomic.Uint32
}

// NewSubnetIndexAllocator creates a SubnetIndexAllocator that starts at 0.
func NewSubnetIndexAllocator() *SubnetIndexAllocator {
	return &SubnetIndexAllocator{}
}

// Allocate returns the next unique subnet index and advances the counter atomically.
func (a *SubnetIndexAllocator) Allocate() uint32 {
	return a.next.Add(1) - 1
}

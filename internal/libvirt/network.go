package libvirt

import (
	"fmt"
	"net"

	libvirtxml "github.com/libvirt/libvirt-go-xml"
)

const (
	DefaultNetworkName = "shuttle"
	DefaultSubnet      = "192.168.122"
	DefaultGateway     = "192.168.122.1"
	DefaultDHCPStart   = "192.168.122.100"
	DefaultDHCPEnd     = "192.168.122.254"
)

// NetworkConfig holds the NAT network configuration.
type NetworkConfig struct {
	Name      string
	Subnet    string
	Gateway   string
	DHCPStart string
	DHCPEnd   string
}

// NetworkGateway returns the IPv4 gateway advertised by a libvirt network.
func (c *Client) NetworkGateway(name string) (string, error) {
	network, err := c.l.NetworkLookupByName(name)
	if err != nil {
		return "", fmt.Errorf("network %q not found: %w", name, err)
	}
	networkXML, err := c.l.NetworkGetXMLDesc(network, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get network %q XML: %w", name, err)
	}
	return networkGateway(networkXML)
}

func networkGateway(networkXML string) (string, error) {
	var definition libvirtxml.Network
	if err := definition.Unmarshal(networkXML); err != nil {
		return "", fmt.Errorf("failed to parse network XML: %w", err)
	}
	for _, ip := range definition.IPs {
		if parsed := net.ParseIP(ip.Address); parsed != nil && parsed.To4() != nil {
			return ip.Address, nil
		}
	}
	return "", fmt.Errorf("network has no IPv4 gateway")
}

// DefaultNetworkConfig returns the default NAT network configuration.
func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		Name:      DefaultNetworkName,
		Subnet:    DefaultSubnet,
		Gateway:   DefaultGateway,
		DHCPStart: DefaultDHCPStart,
		DHCPEnd:   DefaultDHCPEnd,
	}
}

// CreateNetwork creates a NAT network for the runner VMs.
func (c *Client) CreateNetwork(cfg NetworkConfig) error {
	// Just use existing default network - no need to create
	return nil
}

// NetworkExists checks if a network exists.
func (c *Client) NetworkExists(name string) bool {
	_, err := c.l.NetworkLookupByName(name)
	return err == nil
}

// DeleteNetwork deletes a network.
func (c *Client) DeleteNetwork(name string) error {
	net, err := c.l.NetworkLookupByName(name)
	if err != nil {
		return fmt.Errorf("network not found: %w", err)
	}

	active, err := c.l.NetworkIsActive(net)
	if err != nil {
		return fmt.Errorf("failed to check network status: %w", err)
	}

	if active != 0 {
		if err := c.l.NetworkDestroy(net); err != nil {
			return fmt.Errorf("failed to destroy network: %w", err)
		}
	}

	if err := c.l.NetworkUndefine(net); err != nil {
		return fmt.Errorf("failed to undefine network: %w", err)
	}

	return nil
}

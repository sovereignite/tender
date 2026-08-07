package libvirt

import (
	"fmt"

	"github.com/libvirt/libvirt-go-xml"
)

const (
	DefaultNetworkName = "gh-runners"
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
	network := libvirtxml.Network{
		Name: cfg.Name,
		Forward: &libvirtxml.NetworkForward{
			Mode: "nat",
			NAT: &libvirtxml.NetworkForwardNAT{
				Addresses: []libvirtxml.NetworkForwardNATAddress{
					{Start: cfg.Subnet + ".1", End: cfg.Subnet + ".254"},
				},
			},
		},
		Bridge: &libvirtxml.NetworkBridge{
			Name:  "virbr-" + cfg.Name,
			STP:   "on",
			Delay: "0",
		},
			IPs: []libvirtxml.NetworkIP{
				{
					Address: cfg.Gateway,
					Netmask: "255.255.255.0",
					DHCP: &libvirtxml.NetworkDHCP{
						Ranges: []libvirtxml.NetworkDHCPRange{
							{
								Start: cfg.DHCPStart,
								End:   cfg.DHCPEnd,
							},
						},
					},
				},
			},
	}

	xml, err := network.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal network XML: %w", err)
	}

	net, err := c.l.NetworkDefineXML(xml)
	if err != nil {
		return fmt.Errorf("failed to define network: %w", err)
	}

	if err := c.l.NetworkCreate(net); err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}

	if err := c.l.NetworkSetAutostart(net, 1); err != nil {
		return fmt.Errorf("failed to set network autostart: %w", err)
	}

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

package libvirt

import (
	"fmt"

	"github.com/digitalocean/go-libvirt"
	"github.com/digitalocean/go-libvirt/socket/dialers"
)

// Client wraps the go-libvirt client with convenience methods.
type Client struct {
	l *libvirt.Libvirt
}

// NewClient creates a new libvirt client connected to the local system.
func NewClient() (*Client, error) {
	l := libvirt.NewWithDialer(dialers.NewLocal())
	if err := l.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to libvirt: %w", err)
	}
	return &Client{l: l}, nil
}

// Close disconnects from libvirt.
func (c *Client) Close() error {
	return c.l.Disconnect()
}

// Libvirt returns the underlying go-libvirt client.
func (c *Client) Libvirt() *libvirt.Libvirt {
	return c.l
}

// IsAlive checks if the libvirt connection is alive.
func (c *Client) IsAlive() bool {
	_, err := c.l.ConnectGetLibVersion()
	return err == nil
}

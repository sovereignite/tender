package libvirt

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/libvirt/libvirt-go-xml"
)

// DomainConfig holds the VM configuration.
type DomainConfig struct {
	Name       string
	MemoryMB   uint
	CPUs       uint
	DiskPath   string
	NetworkName string
	MACAddress string
}

// DefaultDomainConfig returns the default VM configuration.
func DefaultDomainConfig(name string) DomainConfig {
	return DomainConfig{
		Name:       name,
		MemoryMB:   4096,
		CPUs:       2,
		NetworkName: "default",
	}
}

// DomainStatus represents the status of a domain.
type DomainStatus struct {
	Name     string
	State    string
	UUID     string
	MemoryMB uint
	CPUs     uint
	IP       string
}

// CreateDomain creates a new VM.
func (c *Client) CreateDomain(cfg DomainConfig) error {
	// Delete existing domain if it exists
	existingDom, err := c.l.DomainLookupByName(cfg.Name)
	if err == nil {
		c.l.DomainDestroy(existingDom)
		c.l.DomainUndefine(existingDom)
	}

	domain := libvirtxml.Domain{
		Type: "kvm",
		Name: cfg.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: cfg.MemoryMB,
			Unit:  "MiB",
		},
		VCPU: &libvirtxml.DomainVCPU{
			Value: cfg.CPUs,
		},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Arch:    "x86_64",
				Machine: "pc",
				Type:    "hvm",
			},
		},
		Devices: &libvirtxml.DomainDeviceList{
			Disks: []libvirtxml.DomainDisk{
				{
					Device: "disk",
					Driver: &libvirtxml.DomainDiskDriver{
						Name: "qemu",
						Type: "qcow2",
					},
					Source: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: cfg.DiskPath,
						},
					},
					Target: &libvirtxml.DomainDiskTarget{
						Dev: "vda",
						Bus: "virtio",
					},
				},
			},
			Interfaces: []libvirtxml.DomainInterface{
				{
					Source: &libvirtxml.DomainInterfaceSource{
						Network: &libvirtxml.DomainInterfaceSourceNetwork{
							Network: cfg.NetworkName,
						},
					},
					Model: &libvirtxml.DomainInterfaceModel{
						Type: "virtio",
					},
				},
			},
			Graphics: []libvirtxml.DomainGraphic{
				{
					Spice: &libvirtxml.DomainGraphicSpice{},
				},
			},
			Channels: []libvirtxml.DomainChannel{
				{
					Source: &libvirtxml.DomainChardevSource{
						UNIX: &libvirtxml.DomainChardevSourceUNIX{
							Mode: "bind",
						},
					},
					Target: &libvirtxml.DomainChannelTarget{
						VirtIO: &libvirtxml.DomainChannelTargetVirtIO{
							Name: "org.qemu.guest_agent.0",
						},
					},
				},
			},
		},
	}

	xml, err := domain.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal domain XML: %w", err)
	}

	_, err = c.l.DomainDefineXML(xml)
	if err != nil {
		return fmt.Errorf("failed to define domain: %w", err)
	}

	return nil
}

// DomainExists checks if a domain exists.
func (c *Client) DomainExists(name string) bool {
	_, err := c.l.DomainLookupByName(name)
	return err == nil
}

// StartDomain starts a domain.
func (c *Client) StartDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := c.l.DomainCreate(dom); err != nil {
		return fmt.Errorf("failed to start domain: %w", err)
	}

	return nil
}

// StopDomain stops a domain gracefully.
func (c *Client) StopDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := c.l.DomainShutdown(dom); err != nil {
		return fmt.Errorf("failed to stop domain: %w", err)
	}

	return nil
}

// ForceStopDomain forcefully stops a domain.
func (c *Client) ForceStopDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	if err := c.l.DomainDestroy(dom); err != nil {
		return fmt.Errorf("failed to force stop domain: %w", err)
	}

	return nil
}

// DestroyDomain destroys and undefines a domain.
func (c *Client) DestroyDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}

	// Try to stop if running
	c.l.DomainDestroy(dom)

	if err := c.l.DomainUndefine(dom); err != nil {
		return fmt.Errorf("failed to undefine domain: %w", err)
	}

	return nil
}

// GetDomainStatus returns the status of a domain.
func (c *Client) GetDomainStatus(name string) (*DomainStatus, error) {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return nil, fmt.Errorf("domain not found: %w", err)
	}

	rState, maxMem, _, nrVirtCPU, _, err := c.l.DomainGetInfo(dom)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain info: %w", err)
	}

	state := libvirt.DomainState(rState)
	stateStr := "unknown"
	switch state {
	case libvirt.DomainNostate:
		stateStr = "nostate"
	case libvirt.DomainRunning:
		stateStr = "running"
	case libvirt.DomainBlocked:
		stateStr = "blocked"
	case libvirt.DomainPaused:
		stateStr = "paused"
	case libvirt.DomainShutdown:
		stateStr = "shutdown"
	case libvirt.DomainShutoff:
		stateStr = "shutoff"
	case libvirt.DomainCrashed:
		stateStr = "crashed"
	case libvirt.DomainPmsuspended:
		stateStr = "pmsuspended"
	}

	return &DomainStatus{
		Name:     dom.Name,
		State:    stateStr,
		UUID:     fmt.Sprintf("%x", dom.UUID),
		MemoryMB: uint(maxMem / 1024),
		CPUs:     uint(nrVirtCPU),
	}, nil
}

// WaitForDomainIP waits for a domain to get an IP address.
func (c *Client) WaitForDomainIP(name string, timeout time.Duration) (string, error) {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return "", fmt.Errorf("domain not found: %w", err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ifaces, err := c.l.DomainInterfaceAddresses(dom, 0, 0)
		if err == nil {
			for _, iface := range ifaces {
				for _, addr := range iface.Addrs {
					if addr.Type == 0 && addr.Addr != "127.0.0.1" {
						return addr.Addr, nil
					}
				}
			}
		}
		time.Sleep(time.Second)
	}

	return "", fmt.Errorf("timeout waiting for IP")
}

func generateMAC() string {
	buf := make([]byte, 6)
	rand.Read(buf)
	// Set local bit and clear multicast bit
	buf[0] = (buf[0] | 0x02) & 0xfe
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
}

// ListDomains lists all domains.
func (c *Client) ListDomains() ([]DomainStatus, error) {
	flags := libvirt.ConnectListDomainsActive | libvirt.ConnectListDomainsInactive
	domains, _, err := c.l.ConnectListAllDomains(1, flags)
	if err != nil {
		return nil, fmt.Errorf("failed to list domains: %w", err)
	}

	var statuses []DomainStatus
	for _, dom := range domains {
		rState, maxMem, _, nrVirtCPU, _, err := c.l.DomainGetInfo(dom)
		if err != nil {
			continue
		}

		state := libvirt.DomainState(rState)
		stateStr := "unknown"
		switch state {
		case libvirt.DomainRunning:
			stateStr = "running"
		case libvirt.DomainShutoff:
			stateStr = "shutoff"
		case libvirt.DomainPaused:
			stateStr = "paused"
		}

		statuses = append(statuses, DomainStatus{
			Name:     dom.Name,
			State:    stateStr,
			UUID:     fmt.Sprintf("%x", dom.UUID),
			MemoryMB: uint(maxMem / 1024),
			CPUs:     uint(nrVirtCPU),
		})
	}

	return statuses, nil
}

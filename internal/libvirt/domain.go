package libvirt

import (
	"fmt"
	"io"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/libvirt/libvirt-go-xml"
)

func (c *Client) OpenConsole(name string, output io.Writer) error {
	domain, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}
	if err := c.l.DomainOpenConsole(domain, nil, output, uint32(libvirt.DomainConsoleForce)); err != nil {
		return fmt.Errorf("failed to open domain console: %w", err)
	}
	return nil
}

type DomainConfig struct {
	Name        string
	MemoryMB    uint
	CPUs        uint
	DiskPath    string
	SeedPath    string
	NetworkName string
}

type DomainStatus struct {
	Name     string
	State    string
	UUID     string
	MemoryMB uint
	CPUs     uint
	IP       string
}

func (c *Client) CreateDomain(cfg DomainConfig) error {
	existingDom, err := c.l.DomainLookupByName(cfg.Name)
	if err == nil {
		_ = c.l.DomainDestroy(existingDom)
		_ = c.l.DomainUndefine(existingDom)
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
		CPU: &libvirtxml.DomainCPU{
			Mode: "host-model",
		},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Arch:    "x86_64",
				Machine: "q35",
				Type:    "hvm",
			},
			Firmware: "efi",
		},
		Features: &libvirtxml.DomainFeatureList{
			ACPI: &libvirtxml.DomainFeature{},
			SMM:  &libvirtxml.DomainFeatureSMM{State: "on"},
		},
		Devices: &libvirtxml.DomainDeviceList{
			Disks: []libvirtxml.DomainDisk{{
				Source: &libvirtxml.DomainDiskSource{
					File: &libvirtxml.DomainDiskSourceFile{File: cfg.DiskPath},
				},
				Target: &libvirtxml.DomainDiskTarget{Dev: "vda", Bus: "virtio"},
				Driver: &libvirtxml.DomainDiskDriver{Name: "qemu", Type: "qcow2"},
			}},
			Interfaces: []libvirtxml.DomainInterface{{
				Source: &libvirtxml.DomainInterfaceSource{
					Network: &libvirtxml.DomainInterfaceSourceNetwork{Network: cfg.NetworkName},
				},
				Model: &libvirtxml.DomainInterfaceModel{Type: "virtio"},
			}},
			Serials:  []libvirtxml.DomainSerial{{}},
			Consoles: []libvirtxml.DomainConsole{{}},
			TPMs: []libvirtxml.DomainTPM{{
				Backend: &libvirtxml.DomainTPMBackend{
					Emulator: &libvirtxml.DomainTPMBackendEmulator{Version: "2.0"},
				},
			}},
			VSock: &libvirtxml.DomainVSock{},
			Graphics: []libvirtxml.DomainGraphic{{
				Spice: &libvirtxml.DomainGraphicSpice{},
			}},
			Videos: []libvirtxml.DomainVideo{{
				Model: libvirtxml.DomainVideoModel{Type: "virtio"},
			}},
		},
	}
	if cfg.SeedPath != "" {
		domain.Devices.Disks = append(domain.Devices.Disks, libvirtxml.DomainDisk{
			Device: "cdrom",
			Driver: &libvirtxml.DomainDiskDriver{Name: "qemu", Type: "raw"},
			Source: &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{File: cfg.SeedPath},
			},
			Target:   &libvirtxml.DomainDiskTarget{Dev: "sda", Bus: "sata"},
			ReadOnly: &libvirtxml.DomainDiskReadOnly{},
		})
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

func (c *Client) DomainExists(name string) bool {
	_, err := c.l.DomainLookupByName(name)
	return err == nil
}

func (c *Client) StartDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}
	return c.l.DomainCreate(dom)
}

func (c *Client) StopDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}
	return c.l.DomainShutdown(dom)
}

func (c *Client) ForceStopDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}
	return c.l.DomainDestroy(dom)
}

func (c *Client) DestroyDomain(name string) error {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return fmt.Errorf("domain not found: %w", err)
	}
	_ = c.l.DomainDestroy(dom)
	if err := c.l.DomainUndefineFlags(dom, libvirt.DomainUndefineNvram); err != nil {
		_ = c.l.DomainUndefine(dom)
	}
	return nil
}

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

func (c *Client) GetDomainXML(name string) (string, error) {
	dom, err := c.l.DomainLookupByName(name)
	if err != nil {
		return "", fmt.Errorf("domain not found: %w", err)
	}
	xmlStr, err := c.l.DomainGetXMLDesc(dom, 0)
	if err != nil {
		return "", fmt.Errorf("failed to get domain XML: %w", err)
	}
	return xmlStr, nil
}

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

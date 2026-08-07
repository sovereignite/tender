package runner

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/sovereignite/gh-workers/internal/libvirt"
)

// Manager manages GitHub Actions runner VMs.
type Manager struct {
	client *libvirt.Client
}

// NewManager creates a new runner manager.
func NewManager(client *libvirt.Client) *Manager {
	return &Manager{client: client}
}

// EnsureInfrastructure ensures the libvirt infrastructure is ready.
func (m *Manager) EnsureInfrastructure() error {
	// Create network
	netCfg := libvirt.DefaultNetworkConfig()
	if !m.client.NetworkExists(netCfg.Name) {
		if err := m.client.CreateNetwork(netCfg); err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}
	}

	// Create storage pool
	poolCfg := libvirt.DefaultStorageConfig()
	if !m.client.PoolExists(poolCfg.Name) {
		if err := m.client.CreatePool(poolCfg); err != nil {
			return fmt.Errorf("failed to create pool: %w", err)
		}
	}

	// Download base image if needed
	if err := m.client.DownloadBaseImage(poolCfg.Name); err != nil {
		return fmt.Errorf("failed to download base image: %w", err)
	}

	return nil
}

// Create creates a new runner VM.
func (m *Manager) Create(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Clone volume from base image
	diskName := cfg.Name + ".qcow2"
	if err := m.client.CloneVolume(cfg.PoolName, diskName); err != nil {
		return fmt.Errorf("failed to clone volume: %w", err)
	}

	// Create domain
	domCfg := libvirt.DefaultDomainConfig(cfg.Name)
	domCfg.DiskPath = filepath.Join(libvirt.DefaultPoolPath, diskName)
	domCfg.MemoryMB = cfg.MemoryMB
	domCfg.CPUs = cfg.CPUs
	domCfg.NetworkName = cfg.NetworkName

	if err := m.client.CreateDomain(domCfg); err != nil {
		return fmt.Errorf("failed to create domain: %w", err)
	}

	return nil
}

// Start starts a runner VM.
func (m *Manager) Start(name string) error {
	return m.client.StartDomain(name)
}

// Stop stops a runner VM gracefully.
func (m *Manager) Stop(name string) error {
	return m.client.StopDomain(name)
}

// ForceStop forcefully stops a runner VM.
func (m *Manager) ForceStop(name string) error {
	return m.client.ForceStopDomain(name)
}

// Destroy destroys a runner VM and its disk.
func (m *Manager) Destroy(name string) error {
	// Destroy domain
	if err := m.client.DestroyDomain(name); err != nil {
		return fmt.Errorf("failed to destroy domain: %w", err)
	}

	// Delete volume
	diskName := name + ".qcow2"
	if err := m.client.DeleteVolume(libvirt.DefaultPoolName, diskName); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	return nil
}

// Status returns the status of a runner VM.
func (m *Manager) Status(name string) (*libvirt.DomainStatus, error) {
	return m.client.GetDomainStatus(name)
}

// List lists all runner VMs.
func (m *Manager) List() ([]libvirt.DomainStatus, error) {
	return m.client.ListDomains()
}

// WaitForReady waits for a runner VM to be ready (get an IP).
func (m *Manager) WaitForReady(name string, timeout time.Duration) (string, error) {
	return m.client.WaitForDomainIP(name, timeout)
}

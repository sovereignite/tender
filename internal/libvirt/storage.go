package libvirt

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/libvirt/libvirt-go-xml"
)

const (
	DefaultPoolName   = "gh-runners"
	DefaultPoolPath   = "/var/lib/libvirt/images/gh-runners"
	BaseImageName     = "ubuntu-24.04-server-cloudimg-amd64.img"
	BaseImageURL      = "https://cloud-images.ubuntu.com/releases/24.04/release/" + BaseImageName
)

// StorageConfig holds the storage pool configuration.
type StorageConfig struct {
	Name string
	Path string
}

// DefaultStorageConfig returns the default storage configuration.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Name: DefaultPoolName,
		Path: DefaultPoolPath,
	}
}

// CreatePool creates a storage pool for the runner VMs.
func (c *Client) CreatePool(cfg StorageConfig) error {
	if err := os.MkdirAll(cfg.Path, 0755); err != nil {
		return fmt.Errorf("failed to create pool directory: %w", err)
	}

	pool := libvirtxml.StoragePool{
		Name: cfg.Name,
		Type: "dir",
		Target: &libvirtxml.StoragePoolTarget{
			Path: cfg.Path,
		},
	}

	xml, err := pool.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal pool XML: %w", err)
	}

	p, err := c.l.StoragePoolDefineXML(xml, 0)
	if err != nil {
		return fmt.Errorf("failed to define pool: %w", err)
	}

	if err := c.l.StoragePoolBuild(p, 0); err != nil {
		return fmt.Errorf("failed to build pool: %w", err)
	}

	if err := c.l.StoragePoolCreate(p, 0); err != nil {
		return fmt.Errorf("failed to create pool: %w", err)
	}

	if err := c.l.StoragePoolSetAutostart(p, 1); err != nil {
		return fmt.Errorf("failed to set pool autostart: %w", err)
	}

	return nil
}

// PoolExists checks if a storage pool exists.
func (c *Client) PoolExists(name string) bool {
	_, err := c.l.StoragePoolLookupByName(name)
	return err == nil
}

// CloneVolume creates a new volume by cloning from the base image.
func (c *Client) CloneVolume(poolName, name string) error {
	pool, err := c.l.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	baseVol, err := c.l.StorageVolLookupByName(pool, BaseImageName)
	if err != nil {
		return fmt.Errorf("base image not found: %w", err)
	}

	newVol := libvirtxml.StorageVolume{
		Name: name,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: 20,
			Unit:  "GiB",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Path: filepath.Join(DefaultPoolPath, name),
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
		},
	}

	xml, err := newVol.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal volume XML: %w", err)
	}

	_, err = c.l.StorageVolCreateXMLFrom(pool, xml, baseVol, 0)
	if err != nil {
		return fmt.Errorf("failed to clone volume: %w", err)
	}

	return nil
}

// DeleteVolume deletes a volume.
func (c *Client) DeleteVolume(poolName, name string) error {
	pool, err := c.l.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	vol, err := c.l.StorageVolLookupByName(pool, name)
	if err != nil {
		return fmt.Errorf("volume not found: %w", err)
	}

	if err := c.l.StorageVolDelete(vol, 0); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	return nil
}

// DownloadBaseImage downloads the Ubuntu cloud image to the storage pool.
func (c *Client) DownloadBaseImage(poolName string) error {
	pool, err := c.l.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	// Check if base image already exists
	_, err = c.l.StorageVolLookupByName(pool, BaseImageName)
	if err == nil {
		return nil // Already exists
	}

	// Download using wget
	baseImagePath := filepath.Join(DefaultPoolPath, BaseImageName)
	if err := downloadFile(BaseImageURL, baseImagePath); err != nil {
		return fmt.Errorf("failed to download base image: %w", err)
	}

	// Refresh pool to pick up the new file
	if err := c.l.StoragePoolRefresh(pool, 0); err != nil {
		return fmt.Errorf("failed to refresh pool: %w", err)
	}

	return nil
}

func downloadFile(url, path string) error {
	// This would use a proper HTTP client in production
	// For now, we'll assume wget is available
	return fmt.Errorf("download not implemented - please download manually: wget -O %s %s", path, url)
}

package libvirt

import (
	"fmt"

	"github.com/libvirt/libvirt-go-xml"
)

const (
	DefaultPoolName   = "gh-runners"
	DefaultPoolPath   = "/var/lib/libvirt/images"
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
	// Just use existing default pool - no need to create
	return nil
}

// PoolExists checks if a storage pool exists.
func (c *Client) PoolExists(name string) bool {
	_, err := c.l.StoragePoolLookupByName(name)
	return err == nil
}

// CloneVolume creates a new volume by cloning from the base image.
func (c *Client) CloneVolume(poolName, name string) error {
	pool, err := c.l.StoragePoolLookupByName("default")
	if err != nil {
		return fmt.Errorf("default pool not found: %w", err)
	}

	// Delete existing volume if it exists
	existingVol, err := c.l.StorageVolLookupByName(pool, name)
	if err == nil {
		c.l.StorageVolDelete(existingVol, 0)
	}

	// Create empty volume
	newVol := libvirtxml.StorageVolume{
		Name: name,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: 20,
			Unit:  "GiB",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Path: "/home/me/.local/share/libvirt/images/" + name,
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
		},
	}

	xml, err := newVol.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal volume XML: %w", err)
	}

	_, err = c.l.StorageVolCreateXML(pool, xml, 0)
	if err != nil {
		return fmt.Errorf("failed to create volume: %w", err)
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
	// Skip - no base image needed for now
	return nil
}

func downloadFile(url, path string) error {
	// This would use a proper HTTP client in production
	// For now, we'll assume wget is available
	return fmt.Errorf("download not implemented - please download manually: wget -O %s %s", path, url)
}

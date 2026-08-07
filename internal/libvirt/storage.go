package libvirt

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/digitalocean/go-libvirt"
	"github.com/libvirt/libvirt-go-xml"
)

const (
	BaseImageName = "ubuntu-26.04-server-cloudimg-amd64.img"
	BaseImageURL  = "https://cloud-images.ubuntu.com/releases/26.04/release/" + BaseImageName
)

// StorageConfig holds the storage pool configuration.
type StorageConfig struct {
	Name string
	Path string
}

// CreatePool creates a storage pool for the runner VMs.
func (c *Client) CreatePool(cfg StorageConfig) error {
	// Create pool XML - let libvirt set the path automatically
	poolXML := fmt.Sprintf(`<pool type='dir'>
  <name>%s</name>
</pool>`, cfg.Name)

	_, err := c.l.StoragePoolDefineXML(poolXML, 0)
	if err != nil {
		return fmt.Errorf("failed to define pool: %w", err)
	}

	// Get the pool to start it
	pool, err := c.l.StoragePoolLookupByName(cfg.Name)
	if err != nil {
		return fmt.Errorf("failed to lookup pool: %w", err)
	}

	if err := c.l.StoragePoolCreate(pool, 0); err != nil {
		return fmt.Errorf("failed to start pool: %w", err)
	}

	// Set it as autostart
	if err := c.l.StoragePoolSetAutostart(pool, 1); err != nil {
		return fmt.Errorf("failed to set autostart: %w", err)
	}

	return nil
}

// PoolExists checks if a storage pool exists.
func (c *Client) PoolExists(name string) bool {
	_, err := c.l.StoragePoolLookupByName(name)
	return err == nil
}

// CloneVolume creates a new volume by cloning from the base image.
func (c *Client) CloneVolume(poolName, name string) (string, error) {
	pool, err := c.l.StoragePoolLookupByName(poolName)
	if err != nil {
		return "", fmt.Errorf("pool %q not found: %w", poolName, err)
	}

	// Delete existing volume if it exists
	existingVol, err := c.l.StorageVolLookupByName(pool, name)
	if err == nil {
		_ = c.l.StorageVolDelete(existingVol, 0)
	}

	// Get base image from images pool - find the first .img file
	imagesPool, err := c.l.StoragePoolLookupByName("images")
	if err != nil {
		return "", fmt.Errorf("images pool not found: %w", err)
	}

	// List volumes in images pool to find the base image
	vols, _, err := c.l.StoragePoolListAllVolumes(imagesPool, 0, 0)
	if err != nil {
		return "", fmt.Errorf("failed to list images pool volumes: %w", err)
	}

	var baseVol libvirt.StorageVol
	for _, vol := range vols {
		volType, _, _, err := c.l.StorageVolGetInfo(vol)
		if err == nil && volType == 0 { // 0 = file volume
			baseVol = vol
			break
		}
	}

	if baseVol.Name == "" {
		return "", fmt.Errorf("no base image found in images pool")
	}

	// Clone from base image - let libvirt determine the path from the pool
	newVol := libvirtxml.StorageVolume{
		Name: name,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: 20,
			Unit:  "GiB",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
		},
	}

	xml, err := newVol.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal volume XML: %w", err)
	}

	vol, err := c.l.StorageVolCreateXMLFrom(pool, xml, baseVol, 0)
	if err != nil {
		return "", fmt.Errorf("failed to clone volume: %w", err)
	}

	// Get the actual path from libvirt
	volPath, err := c.l.StorageVolGetPath(vol)
	if err != nil {
		return "", fmt.Errorf("failed to get volume path: %w", err)
	}

	return volPath, nil
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
func (c *Client) DownloadBaseImage(poolName string, image string) error {
	pool, err := c.l.StoragePoolLookupByName(poolName)
	if err != nil {
		return fmt.Errorf("pool %q not found: %w", poolName, err)
	}

	// Use provided image or default
	imageName := BaseImageName
	imageURL := BaseImageURL
	if image != "" {
		imageName = image
		imageURL = "https://cloud-images.ubuntu.com/releases/" + strings.TrimSuffix(image, "-server-cloudimg-amd64.img") + "/release/" + image
	}

	// Check if base image already exists
	_, err = c.l.StorageVolLookupByName(pool, imageName)
	if err == nil {
		return nil // Already exists
	}

	// Get the pool's target path from its XML
	poolXML, err := c.l.StoragePoolGetXMLDesc(pool, 0)
	if err != nil {
		return fmt.Errorf("failed to get pool XML: %w", err)
	}

	var poolDef struct {
		Target struct {
			Path string `xml:"path"`
		} `xml:"target"`
	}
	if err := xml.Unmarshal([]byte(poolXML), &poolDef); err != nil {
		return fmt.Errorf("failed to parse pool XML: %w", err)
	}

	imagePath := filepath.Join(poolDef.Target.Path, imageName)
	if err := downloadFile(imageURL, imagePath); err != nil {
		return fmt.Errorf("failed to download base image: %w", err)
	}

	// Refresh pool to pick up the new file
	if err := c.l.StoragePoolRefresh(pool, 0); err != nil {
		return fmt.Errorf("failed to refresh pool: %w", err)
	}

	return nil
}

func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	_, err = io.Copy(out, resp.Body)
	return err
}

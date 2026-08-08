package libvirt

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/digitalocean/go-libvirt"
	"github.com/libvirt/libvirt-go-xml"
	"github.com/sovereignite/gh-workers/internal/images"
	"github.com/sovereignite/gh-workers/internal/isoimage"
)

const (
	runnerPoolName     = "default"
	imagesPoolName     = "images"
	runnerDiskCapacity = 20 * 1024 * 1024 * 1024
)

type VolumeInfo struct {
	Capacity   uint64
	Allocation uint64
}

// EnsureImagesPool creates and starts the singleton base-image pool.
func (c *Client) EnsureImagesPool() error {
	if err := c.ensurePoolReady(runnerPoolName); err != nil {
		return fmt.Errorf("runner storage pool is not ready: %w", err)
	}

	exists, err := c.poolExists(imagesPoolName)
	if err != nil {
		return err
	}
	if exists {
		return c.ensurePoolReady(imagesPoolName)
	}

	reference, err := c.l.StoragePoolLookupByName(runnerPoolName)
	if err != nil {
		return fmt.Errorf("reference pool %q not found: %w", runnerPoolName, err)
	}

	referenceXML, err := c.l.StoragePoolGetXMLDesc(reference, 0)
	if err != nil {
		return fmt.Errorf("failed to get reference pool %q XML: %w", runnerPoolName, err)
	}

	poolXML, err := derivedPoolXML(referenceXML, imagesPoolName)
	if err != nil {
		return fmt.Errorf("failed to derive pool %q from %q: %w", imagesPoolName, runnerPoolName, err)
	}

	pool, err := c.l.StoragePoolDefineXML(poolXML, 0)
	if err != nil {
		return fmt.Errorf("failed to define pool: %w", err)
	}

	if err := c.l.StoragePoolCreate(pool, libvirt.StoragePoolCreateWithBuild); err != nil {
		_ = c.l.StoragePoolUndefine(pool)
		return fmt.Errorf("failed to start pool: %w", err)
	}

	// Set it as autostart
	if err := c.l.StoragePoolSetAutostart(pool, 1); err != nil {
		return fmt.Errorf("failed to set autostart: %w", err)
	}

	return nil
}

func (c *Client) ensurePoolReady(name string) error {
	pool, err := c.l.StoragePoolLookupByName(name)
	if err != nil {
		return fmt.Errorf("pool %q not found: %w", name, err)
	}

	poolXML, err := c.l.StoragePoolGetXMLDesc(pool, 0)
	if err != nil {
		return fmt.Errorf("failed to get pool %q XML: %w", name, err)
	}
	if _, err := dirPoolDefinition(poolXML); err != nil {
		return fmt.Errorf("invalid pool %q: %w", name, err)
	}

	active, err := c.l.StoragePoolIsActive(pool)
	if err != nil {
		return fmt.Errorf("failed to check whether pool %q is active: %w", name, err)
	}
	if active == 0 {
		if err := c.l.StoragePoolCreate(pool, 0); err != nil {
			return fmt.Errorf("failed to start pool %q: %w", name, err)
		}
	}

	autostart, err := c.l.StoragePoolGetAutostart(pool)
	if err != nil {
		return fmt.Errorf("failed to get pool %q autostart: %w", name, err)
	}
	if autostart == 0 {
		if err := c.l.StoragePoolSetAutostart(pool, 1); err != nil {
			return fmt.Errorf("failed to set pool %q autostart: %w", name, err)
		}
	}

	return nil
}

func derivedPoolXML(referenceXML, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || name == "." {
		return "", fmt.Errorf("invalid pool name %q", name)
	}

	reference, err := dirPoolDefinition(referenceXML)
	if err != nil {
		return "", err
	}

	referencePath := filepath.Clean(reference.Target.Path)
	targetPath := filepath.Join(filepath.Dir(referencePath), "gh-workers-"+name)

	definition := libvirtxml.StoragePool{
		Type: "dir",
		Name: name,
		Target: &libvirtxml.StoragePoolTarget{
			Path: targetPath,
		},
	}
	return definition.Marshal()
}

func dirPoolDefinition(poolXML string) (*libvirtxml.StoragePool, error) {
	var definition libvirtxml.StoragePool
	if err := definition.Unmarshal(poolXML); err != nil {
		return nil, fmt.Errorf("failed to parse pool XML: %w", err)
	}
	if definition.Type != "dir" {
		return nil, fmt.Errorf("expected type dir, got %q", definition.Type)
	}
	if definition.Target == nil || definition.Target.Path == "" {
		return nil, fmt.Errorf("directory pool has no target path")
	}
	if !filepath.IsAbs(definition.Target.Path) {
		return nil, fmt.Errorf("directory pool target %q is not absolute", definition.Target.Path)
	}
	return &definition, nil
}

func (c *Client) poolExists(name string) (bool, error) {
	_, err := c.l.StoragePoolLookupByName(name)
	if err == nil {
		return true, nil
	}

	var libvirtErr libvirt.Error
	if errors.As(err, &libvirtErr) && libvirtErr.Code == uint32(libvirt.ErrNoStoragePool) {
		return false, nil
	}
	return false, fmt.Errorf("failed to look up pool %q: %w", name, err)
}

// CloneVolume creates a new volume by cloning from the base image.
func (c *Client) CloneVolume(name, baseImageName string) (string, error) {
	pool, err := c.l.StoragePoolLookupByName(runnerPoolName)
	if err != nil {
		return "", fmt.Errorf("pool %q not found: %w", runnerPoolName, err)
	}

	// Delete existing volume if it exists
	existingVol, err := c.l.StorageVolLookupByName(pool, name)
	if err == nil {
		_ = c.l.StorageVolDelete(existingVol, 0)
	}

	imagesPool, err := c.l.StoragePoolLookupByName(imagesPoolName)
	if err != nil {
		return "", fmt.Errorf("images pool not found: %w", err)
	}

	baseVol, err := c.l.StorageVolLookupByName(imagesPool, baseImageName)
	if err != nil {
		return "", fmt.Errorf("base image %q not found in images pool: %w", baseImageName, err)
	}

	// Clone from base image - let libvirt determine the path from the pool
	newVol := libvirtxml.StorageVolume{
		Name: name,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: runnerDiskCapacity,
			Unit:  "bytes",
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
	if err := c.l.StorageVolResize(vol, runnerDiskCapacity, 0); err != nil {
		_ = c.l.StorageVolDelete(vol, 0)
		return "", fmt.Errorf("failed to expand cloned volume: %w", err)
	}

	// Get the actual path from libvirt
	volPath, err := c.l.StorageVolGetPath(vol)
	if err != nil {
		return "", fmt.Errorf("failed to get volume path: %w", err)
	}

	return volPath, nil
}

// DeleteVolume deletes a volume.
func (c *Client) DeleteVolume(name string) error {
	pool, err := c.l.StoragePoolLookupByName(runnerPoolName)
	if err != nil {
		return fmt.Errorf("pool not found: %w", err)
	}

	vol, err := c.l.StorageVolLookupByName(pool, name)
	if err != nil {
		if isLibvirtError(err, libvirt.ErrNoStorageVol) {
			return nil
		}
		return fmt.Errorf("volume not found: %w", err)
	}

	if err := c.l.StorageVolDelete(vol, 0); err != nil {
		return fmt.Errorf("failed to delete volume: %w", err)
	}

	return nil
}

func (c *Client) GetVolumeInfo(name string) (*VolumeInfo, error) {
	pool, err := c.l.StoragePoolLookupByName(runnerPoolName)
	if err != nil {
		return nil, fmt.Errorf("pool %q not found: %w", runnerPoolName, err)
	}
	volume, err := c.l.StorageVolLookupByName(pool, name)
	if err != nil {
		return nil, fmt.Errorf("volume %q not found: %w", name, err)
	}
	_, capacity, allocation, err := c.l.StorageVolGetInfo(volume)
	if err != nil {
		return nil, fmt.Errorf("failed to get volume %q info: %w", name, err)
	}
	return &VolumeInfo{Capacity: capacity, Allocation: allocation}, nil
}

// CreateSeedVolume stores a cloud-init seed in the runner storage pool.
func (c *Client) CreateSeedVolume(name string, seed []byte) (string, error) {
	pool, err := c.l.StoragePoolLookupByName(runnerPoolName)
	if err != nil {
		return "", fmt.Errorf("pool %q not found: %w", runnerPoolName, err)
	}
	if existing, err := c.l.StorageVolLookupByName(pool, name); err == nil {
		if err := c.l.StorageVolDelete(existing, 0); err != nil {
			return "", fmt.Errorf("failed to replace seed volume: %w", err)
		}
	} else if !isLibvirtError(err, libvirt.ErrNoStorageVol) {
		return "", fmt.Errorf("failed to look up seed volume: %w", err)
	}

	volumeDef := libvirtxml.StorageVolume{
		Name: name,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: uint64(len(seed)),
			Unit:  "bytes",
		},
		Allocation: &libvirtxml.StorageVolumeSize{
			Value: uint64(len(seed)),
			Unit:  "bytes",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{Type: "raw"},
		},
	}
	volumeXML, err := volumeDef.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal seed volume XML: %w", err)
	}
	volume, err := c.l.StorageVolCreateXML(pool, volumeXML, 0)
	if err != nil {
		return "", fmt.Errorf("failed to create seed volume: %w", err)
	}
	if err := c.l.StorageVolUpload(volume, bytes.NewReader(seed), 0, uint64(len(seed)), 0); err != nil {
		_ = c.l.StorageVolDelete(volume, 0)
		return "", fmt.Errorf("failed to stream seed into libvirt volume: %w", err)
	}
	path, err := c.l.StorageVolGetPath(volume)
	if err != nil {
		_ = c.l.StorageVolDelete(volume, 0)
		return "", fmt.Errorf("failed to get seed volume path: %w", err)
	}
	return path, nil
}

// CacheRunnerTools builds and caches a read-only ISO containing the runner archive.
func (c *Client) CacheRunnerTools(release images.RunnerRelease, callbackPath string) (string, error) {
	callbackHash, err := validateCallbackBinary(callbackPath)
	if err != nil {
		return "", err
	}
	cacheName, err := runnerToolsCacheName(release, callbackHash)
	if err != nil {
		return "", err
	}
	pool, err := c.l.StoragePoolLookupByName(imagesPoolName)
	if err != nil {
		return "", fmt.Errorf("pool %q not found: %w", imagesPoolName, err)
	}
	if volume, err := c.l.StorageVolLookupByName(pool, cacheName); err == nil {
		return c.l.StorageVolGetPath(volume)
	} else if !isLibvirtError(err, libvirt.ErrNoStorageVol) {
		return "", fmt.Errorf("failed to look up runner tools image %q: %w", cacheName, err)
	}

	dir, err := os.MkdirTemp("", "gh-runner-tools-")
	if err != nil {
		return "", fmt.Errorf("failed to create runner tools workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	workspace := filepath.Join(dir, "root")
	if err := os.Mkdir(workspace, 0700); err != nil {
		return "", fmt.Errorf("failed to create runner tools root: %w", err)
	}
	archivePath := filepath.Join(workspace, "actions-runner.tar.gz")
	if err := downloadFile(release.URL, archivePath); err != nil {
		return "", fmt.Errorf("failed to download GitHub Actions runner %s: %w", release.Version, err)
	}
	if err := copyFile(callbackPath, filepath.Join(workspace, "gh-runner-phone-home"), 0755); err != nil {
		return "", fmt.Errorf("failed to stage phone-home binary: %w", err)
	}
	isoPath := filepath.Join(dir, cacheName)
	if err := isoimage.Build(isoPath, workspace, "GH_RUNNER_TOOLS"); err != nil {
		return "", fmt.Errorf("failed to build runner tools ISO: %w", err)
	}

	iso, err := os.Open(isoPath)
	if err != nil {
		return "", fmt.Errorf("failed to open runner tools ISO: %w", err)
	}
	defer func() { _ = iso.Close() }()
	info, err := iso.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to inspect runner tools ISO: %w", err)
	}

	volumeDef := libvirtxml.StorageVolume{
		Name: cacheName,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: uint64(info.Size()),
			Unit:  "bytes",
		},
		Allocation: &libvirtxml.StorageVolumeSize{
			Value: uint64(info.Size()),
			Unit:  "bytes",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{Type: "raw"},
		},
	}
	volumeXML, err := volumeDef.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal runner tools volume XML: %w", err)
	}
	volume, err := c.l.StorageVolCreateXML(pool, volumeXML, 0)
	if err != nil {
		return "", fmt.Errorf("failed to create runner tools volume: %w", err)
	}
	keepVolume := false
	defer func() {
		if !keepVolume {
			_ = c.l.StorageVolDelete(volume, 0)
		}
	}()
	if err := c.l.StorageVolUpload(volume, iso, 0, uint64(info.Size()), 0); err != nil {
		return "", fmt.Errorf("failed to upload runner tools ISO: %w", err)
	}
	path, err := c.l.StorageVolGetPath(volume)
	if err != nil {
		return "", fmt.Errorf("failed to get runner tools ISO path: %w", err)
	}
	keepVolume = true
	return path, nil
}

func runnerToolsCacheName(release images.RunnerRelease, callbackHash []byte) (string, error) {
	if release.Version == "" || filepath.Base(release.Version) != release.Version || release.Version == "." {
		return "", fmt.Errorf("invalid GitHub Actions runner version %q", release.Version)
	}
	parsedURL, err := url.Parse(release.URL)
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host != "github.com" {
		return "", fmt.Errorf("runner download URL must use https://github.com")
	}
	if len(callbackHash) != sha256.Size {
		return "", fmt.Errorf("invalid phone-home binary SHA-256")
	}
	return fmt.Sprintf("actions-runner-%s-%s-linux-x64.iso", release.Version, hex.EncodeToString(callbackHash[:8])), nil
}

func validateCallbackBinary(path string) ([]byte, error) {
	binary, err := elf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open phone-home binary: %w", err)
	}
	defer func() { _ = binary.Close() }()
	if binary.Class != elf.ELFCLASS64 || binary.Machine != elf.EM_X86_64 {
		return nil, fmt.Errorf("phone-home binary must target Linux x86-64")
	}
	if binary.Section(".interp") != nil {
		return nil, fmt.Errorf("phone-home binary must be statically linked")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open phone-home binary for hashing: %w", err)
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, fmt.Errorf("hash phone-home binary: %w", err)
	}
	return hash.Sum(nil), nil
}

func copyFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func downloadFile(sourceURL, destination string) error {
	request, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %s", response.Status)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// CacheImage downloads and verifies an image on first use.
func (c *Client) CacheImage(image images.Image) (string, error) {
	cacheName, err := imageCacheName(image)
	if err != nil {
		return "", err
	}

	pool, err := c.l.StoragePoolLookupByName(imagesPoolName)
	if err != nil {
		return "", fmt.Errorf("pool %q not found: %w", imagesPoolName, err)
	}

	_, err = c.l.StorageVolLookupByName(pool, cacheName)
	if err == nil {
		return cacheName, nil
	}
	if !isLibvirtError(err, libvirt.ErrNoStorageVol) {
		return "", fmt.Errorf("failed to look up cached image %q: %w", cacheName, err)
	}

	request, err := http.NewRequest(http.MethodGet, image.URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create image request: %w", err)
	}
	response, err := (&http.Client{Timeout: 30 * time.Minute}).Do(request)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download returned %s", response.Status)
	}
	if response.ContentLength >= 0 && uint64(response.ContentLength) != image.Size {
		return "", fmt.Errorf("image size is %d bytes, expected %d", response.ContentLength, image.Size)
	}

	volumeDef := libvirtxml.StorageVolume{
		Name: cacheName,
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: image.Size,
			Unit:  "bytes",
		},
		Allocation: &libvirtxml.StorageVolumeSize{
			Value: 0,
			Unit:  "bytes",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{Type: "raw"},
		},
	}
	volumeXML, err := volumeDef.Marshal()
	if err != nil {
		return "", fmt.Errorf("failed to marshal image volume XML: %w", err)
	}
	volume, err := c.l.StorageVolCreateXML(pool, volumeXML, 0)
	if err != nil {
		return "", fmt.Errorf("failed to create image volume: %w", err)
	}
	keepVolume := false
	defer func() {
		if !keepVolume {
			_ = c.l.StorageVolDelete(volume, 0)
		}
	}()

	hash := sha256.New()
	reader := io.TeeReader(response.Body, hash)
	if err := c.l.StorageVolUpload(volume, reader, 0, image.Size, 0); err != nil {
		return "", fmt.Errorf("failed to stream image into libvirt volume: %w", err)
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return "", fmt.Errorf("failed to finish reading image: %w", err)
	}
	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualChecksum, image.Checksum) {
		return "", fmt.Errorf("image checksum is %s, expected %s", actualChecksum, image.Checksum)
	}

	if err := c.l.StoragePoolRefresh(pool, 0); err != nil {
		return "", fmt.Errorf("failed to refresh images pool: %w", err)
	}

	keepVolume = true
	return cacheName, nil
}

func imageCacheName(image images.Image) (string, error) {
	parsedURL, err := url.Parse(image.URL)
	if err != nil {
		return "", fmt.Errorf("invalid image URL: %w", err)
	}
	if parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return "", fmt.Errorf("image URL must use HTTPS")
	}
	if image.Name == "" || filepath.Base(image.Name) != image.Name {
		return "", fmt.Errorf("invalid image name %q", image.Name)
	}
	if !strings.EqualFold(image.ChecksumType, "sha256") {
		return "", fmt.Errorf("unsupported image checksum type %q", image.ChecksumType)
	}
	checksum, err := hex.DecodeString(image.Checksum)
	if err != nil || len(checksum) != sha256.Size {
		return "", fmt.Errorf("invalid SHA-256 checksum")
	}
	if image.Size == 0 {
		return "", fmt.Errorf("image size is missing")
	}
	return hex.EncodeToString(checksum[:8]) + "-" + image.Name, nil

}

func isLibvirtError(err error, code libvirt.ErrorNumber) bool {
	var libvirtErr libvirt.Error
	return errors.As(err, &libvirtErr) && libvirtErr.Code == uint32(code)
}

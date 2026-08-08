package libvirt

import (
	"strings"
	"testing"

	libvirtxml "github.com/libvirt/libvirt-go-xml"
	"github.com/sovereignite/gh-workers/internal/images"
)

func TestDerivedPoolXMLUsesReferenceParent(t *testing.T) {
	referenceXML := `<pool type="dir">
  <name>Downloads</name>
  <target><path>/home/example/Downloads</path></target>
</pool>`

	poolXML, err := derivedPoolXML(referenceXML, "images")
	if err != nil {
		t.Fatalf("derivedPoolXML() error = %v", err)
	}

	var pool libvirtxml.StoragePool
	if err := pool.Unmarshal(poolXML); err != nil {
		t.Fatalf("failed to parse derived pool XML: %v", err)
	}
	if pool.Type != "dir" {
		t.Errorf("pool type = %q, want dir", pool.Type)
	}
	if pool.Name != "images" {
		t.Errorf("pool name = %q, want images", pool.Name)
	}
	if pool.Target == nil || pool.Target.Path != "/home/example/gh-workers-images" {
		t.Fatalf("pool target = %#v, want /home/example/gh-workers-images", pool.Target)
	}
}

func TestDerivedPoolXMLDoesNotOverlapConventionalImagesTarget(t *testing.T) {
	referenceXML := `<pool type="dir">
  <name>default</name>
  <target><path>/var/lib/libvirt/images</path></target>
</pool>`

	poolXML, err := derivedPoolXML(referenceXML, "images")
	if err != nil {
		t.Fatalf("derivedPoolXML() error = %v", err)
	}

	var pool libvirtxml.StoragePool
	if err := pool.Unmarshal(poolXML); err != nil {
		t.Fatalf("failed to parse derived pool XML: %v", err)
	}
	if pool.Target == nil || pool.Target.Path != "/var/lib/libvirt/gh-workers-images" {
		t.Fatalf("pool target = %#v, want /var/lib/libvirt/gh-workers-images", pool.Target)
	}
}

func TestDerivedPoolXMLRejectsInvalidReference(t *testing.T) {
	tests := map[string]string{
		"non-directory pool": `<pool type="logical"><name>vg</name></pool>`,
		"missing target":     `<pool type="dir"><name>default</name></pool>`,
		"relative target":    `<pool type="dir"><name>default</name><target><path>images</path></target></pool>`,
	}

	for name, referenceXML := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := derivedPoolXML(referenceXML, "images"); err == nil {
				t.Fatal("derivedPoolXML() error = nil, want an error")
			}
		})
	}
}

func TestDerivedPoolXMLRejectsPathLikeName(t *testing.T) {
	referenceXML := `<pool type="dir">
  <name>default</name>
  <target><path>/var/lib/libvirt/images</path></target>
</pool>`

	if _, err := derivedPoolXML(referenceXML, "../images"); err == nil {
		t.Fatal("derivedPoolXML() error = nil, want an error")
	}
}

func TestImageCacheName(t *testing.T) {
	image := images.Image{
		Name:         "ubuntu.img",
		URL:          "https://cloud-images.ubuntu.com/ubuntu.img",
		ChecksumType: "sha256",
		Checksum:     strings.Repeat("ab", 32),
		Size:         1024,
	}

	name, err := imageCacheName(image)
	if err != nil {
		t.Fatal(err)
	}
	if name != "abababababababab-ubuntu.img" {
		t.Fatalf("imageCacheName() = %q", name)
	}
}

func TestImageCacheNameRejectsInvalidMetadata(t *testing.T) {
	valid := images.Image{
		Name:         "ubuntu.img",
		URL:          "https://cloud-images.ubuntu.com/ubuntu.img",
		ChecksumType: "sha256",
		Checksum:     strings.Repeat("ab", 32),
		Size:         1024,
	}
	tests := map[string]images.Image{
		"insecure URL": func() images.Image { image := valid; image.URL = "http://example.com/image"; return image }(),
		"path name":    func() images.Image { image := valid; image.Name = "../image"; return image }(),
		"checksum type": func() images.Image {
			image := valid
			image.ChecksumType = "sha512"
			return image
		}(),
		"checksum": func() images.Image { image := valid; image.Checksum = "bad"; return image }(),
		"size":     func() images.Image { image := valid; image.Size = 0; return image }(),
	}

	for name, image := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := imageCacheName(image); err == nil {
				t.Fatal("imageCacheName() error = nil, want an error")
			}
		})
	}
}

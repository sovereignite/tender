package images

import (
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		a, b string
		want int
	}{
		{"26.04", "8.04", 1},
		{"20260803-2559", "20260803-999", 1},
		{"44", "43", 1},
		{"24.04", "24.04", 0},
	} {
		got := compareVersions(test.a, test.b)
		if got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}

func TestSelectImageDefaultsToLatestSupportedUbuntu(t *testing.T) {
	t.Parallel()
	selected, err := selectImage([]Image{
		{Distro: "ubuntu", Release: "27.04", Arch: "x86_64", Format: "qcow2", Supported: false, Name: "unsupported"},
		{Distro: "ubuntu", Release: "26.04", BuildID: "20260801", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "latest"},
		{Distro: "ubuntu", Release: "24.04", BuildID: "20260807", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "older"},
		{Distro: "debian", Release: "13", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "other-distro"},
	}, Selector{Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name != "latest" {
		t.Fatalf("selected %q", selected.Name)
	}
}

func TestSelectImageAcceptsPartialOverrides(t *testing.T) {
	t.Parallel()
	candidates := []Image{
		{Distro: "debian", Release: "13", Codename: "Trixie", BuildID: "20260803-2559", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "trixie"},
		{Distro: "debian", Release: "12", Codename: "Bookworm", BuildID: "20260806-2562", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "bookworm"},
	}
	selected, err := selectImage(candidates, Selector{Distro: "debian", Release: "bookworm", Arch: "x86_64"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name != "bookworm" {
		t.Fatalf("selected %q", selected.Name)
	}
}

func TestSelectImageUsesLatestBuildDeterministically(t *testing.T) {
	t.Parallel()
	candidates := []Image{
		{Distro: "ubuntu", Release: "26.04", BuildID: "20260731", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "old"},
		{Distro: "ubuntu", Release: "26.04", BuildID: "20260801", Arch: "x86_64", Format: "qcow2", Supported: true, Name: "new"},
	}
	selected, err := selectImage(candidates, Selector{Arch: "x86_64", Format: "qcow2"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Name != "new" {
		t.Fatalf("selected %q", selected.Name)
	}
}

func TestParseUbuntu(t *testing.T) {
	t.Parallel()
	catalog := ubuntuCatalog{Products: map[string]ubuntuProduct{
		"com.ubuntu.cloud:server:26.04:amd64": {
			Arch:            "amd64",
			OS:              "ubuntu",
			Release:         "resolute",
			ReleaseCodename: "Resolute Raccoon",
			ReleaseTitle:    "26.04 LTS",
			SupportEOL:      "2031-05-29",
			Supported:       true,
			Version:         "26.04",
			Versions: map[string]ubuntuVersion{
				"20260731": {Label: "release", Items: map[string]ubuntuItem{
					"disk1.img": {FType: "disk1.img", Path: "server/releases/resolute/release-20260731/ubuntu-26.04-server-cloudimg-amd64.img", SHA256: "abc", Size: 42},
				}},
			},
		},
	}}

	images := parseUbuntu(catalog, map[string]time.Time{"resolute": time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)})
	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}
	image := images[0]
	if image.Release != "26.04" || image.Arch != "x86_64" || !image.Supported || !image.IsLTS {
		t.Fatalf("unexpected image: %+v", image)
	}
	if image.ReleaseDate.Format("2006-01-02") != "2026-04-23" {
		t.Fatalf("unexpected release date: %s", image.ReleaseDate)
	}
	if image.URL != "https://cloud-images.ubuntu.com/server/releases/resolute/release-20260731/ubuntu-26.04-server-cloudimg-amd64.img" {
		t.Fatalf("unexpected URL: %s", image.URL)
	}
}

func TestParseUbuntuReleaseDates(t *testing.T) {
	t.Parallel()
	dates := parseUbuntuReleaseDates("Dist: resolute\nName: Resolute Raccoon\nVersion: 26.04 LTS\nDate: Thu, 23 April 2026 00:26:04 UTC\nSupported: 1\n")
	if got := dates["resolute"].Format("2006-01-02"); got != "2026-04-23" {
		t.Fatalf("release date = %q", got)
	}
}

func TestParseDebianReleasesAndSupport(t *testing.T) {
	t.Parallel()
	releases, err := parseDebianReleases(strings.NewReader("version,codename,series,created,release,eol,eol-lts,eol-elts\n13,Trixie,trixie,2023-06-10,2025-08-09,2028-08-09,2030-06-30,2035-06-30\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 {
		t.Fatalf("got %d releases, want 1", len(releases))
	}
	status, supported := debianSupport(releases[0], time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC))
	if status != "supported" || !supported {
		t.Fatalf("status = %q, supported = %v", status, supported)
	}
}

func TestParseDebianReleasesAllowsMissingLifecycleColumns(t *testing.T) {
	t.Parallel()
	releases, err := parseDebianReleases(strings.NewReader("version,codename,series,created,release,eol,eol-lts,eol-elts\n13,Trixie,trixie,2023-06-10,2025-08-09,2028-08-09,2030-06-30,2035-06-30\n14,Forky,forky,2025-08-09\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].Version != "13" {
		t.Fatalf("unexpected releases: %+v", releases)
	}
}

func TestLatestDebianBuildUsesNumericBuildSuffix(t *testing.T) {
	t.Parallel()
	latest, ok := latestDebianBuild([]debianSearchResult{
		{ID: "old", Release: "trixie", Version: "20260803-999"},
		{ID: "new", Release: "trixie", Version: "20260803-2559"},
	}, "trixie")
	if !ok || latest.ID != "new" {
		t.Fatalf("latest = %+v, ok = %v", latest, ok)
	}
}

func TestParseFedora(t *testing.T) {
	t.Parallel()
	current := map[string]struct {
		released string
		eol      string
	}{"44": {released: "2026-04-28", eol: "2027-05-15"}}
	artifacts := []fedoraArtifact{
		{Version: "44", Arch: "x86_64", Link: "https://example.test/Fedora-Cloud-Base-Generic-44-1.7.x86_64.qcow2", Variant: "Cloud", Subvariant: "Cloud_Base", SHA256: "abc", Size: "42"},
		{Version: "44", Arch: "x86_64", Link: "https://example.test/Fedora-Cloud-Base-UEFI-UKI-44-1.7.x86_64.qcow2", Variant: "Cloud", Subvariant: "Cloud_Base_UKI"},
	}
	images := parseFedora(current, artifacts)
	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}
	image := images[0]
	if image.BuildID != "44-1.7" || image.Arch != "x86_64" || image.Format != "qcow2" || image.Size != 42 {
		t.Fatalf("unexpected image: %+v", image)
	}
}

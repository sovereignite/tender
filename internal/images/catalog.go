package images

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	ubuntuReleasesURL  = "https://cloud-images.ubuntu.com/releases/"
	ubuntuLifecycleURL = "https://changelogs.ubuntu.com/meta-release"
	debianFinderURL    = "https://cloud-image-finder.debian.net/api/v1"
	debianLifecycleURL = "https://debian.pages.debian.net/distro-info-data/debian.csv"
	fedoraImagesURL    = "https://fedoraproject.org/releases.json"
	fedoraReleasesURL  = "https://bodhi.fedoraproject.org/releases/?state=current&rows_per_page=100"
)

type Image struct {
	Distro       string
	Release      string
	Codename     string
	Support      string
	Supported    bool
	IsLTS        bool
	ReleaseDate  time.Time
	SupportEOL   time.Time
	BuildID      string
	BuildDate    time.Time
	Arch         string
	Format       string
	Name         string
	URL          string
	Checksum     string
	ChecksumType string
	Size         uint64
}

type Filter struct {
	Distro  string
	Release string
	LTS     bool
	Arch    string
}

func ListImages(filter Filter) ([]Image, error) {
	return listImages(context.Background(), &http.Client{Timeout: 30 * time.Second}, filter)
}

func listImages(ctx context.Context, client *http.Client, filter Filter) ([]Image, error) {
	distro := strings.ToLower(strings.TrimSpace(filter.Distro))
	if distro == "" {
		distro = "ubuntu"
	}

	var sources []func(context.Context, *http.Client) ([]Image, error)
	switch distro {
	case "ubuntu":
		sources = []func(context.Context, *http.Client) ([]Image, error){listUbuntu}
	case "debian":
		sources = []func(context.Context, *http.Client) ([]Image, error){listDebian}
	case "fedora":
		sources = []func(context.Context, *http.Client) ([]Image, error){listFedora}
	case "all":
		sources = []func(context.Context, *http.Client) ([]Image, error){listUbuntu, listDebian, listFedora}
	default:
		return nil, fmt.Errorf("unsupported distro %q", filter.Distro)
	}

	var result []Image
	for _, source := range sources {
		images, err := source(ctx, client)
		if err != nil {
			return nil, err
		}
		for _, image := range images {
			if filter.Release != "" && !strings.EqualFold(image.Release, filter.Release) && !strings.EqualFold(image.Codename, filter.Release) {
				continue
			}
			if filter.LTS && !image.IsLTS {
				continue
			}
			if filter.Arch != "" && normalizeArch(filter.Arch) != image.Arch {
				continue
			}
			result = append(result, image)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Distro != result[j].Distro {
			return result[i].Distro < result[j].Distro
		}
		return compareVersions(result[i].Release, result[j].Release) > 0
	})
	return result, nil
}

func GetLatestLTS() (*Image, error) {
	images, err := ListImages(Filter{Distro: "ubuntu", LTS: true, Arch: "x86_64"})
	if err != nil {
		return nil, err
	}
	for i := range images {
		if images[i].Supported {
			return &images[i], nil
		}
	}
	return nil, fmt.Errorf("no supported Ubuntu LTS image found")
}

func compareVersions(a, b string) int {
	pa := versionParts(a)
	pb := versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var av, bv int
		if i < len(pa) {
			av = pa[i]
		}
		if i < len(pb) {
			bv = pb[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

func versionParts(version string) []int {
	fields := strings.FieldsFunc(version, func(r rune) bool { return r < '0' || r > '9' })
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		value, _ := strconv.Atoi(field)
		parts = append(parts, value)
	}
	return parts
}

func normalizeArch(arch string) string {
	switch strings.ToLower(arch) {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	case "ppc64el", "ppc64le":
		return "ppc64le"
	default:
		return strings.ToLower(arch)
	}
}

func parseDate(value string) time.Time {
	for _, layout := range []string{"2006-01-02", "20060102", time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

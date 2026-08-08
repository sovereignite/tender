package images

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

type ubuntuIndex struct {
	Index map[string]struct {
		Datatype string `json:"datatype"`
		Path     string `json:"path"`
	} `json:"index"`
}

type ubuntuCatalog struct {
	Products map[string]ubuntuProduct `json:"products"`
}

type ubuntuProduct struct {
	Aliases         string                   `json:"aliases"`
	Arch            string                   `json:"arch"`
	OS              string                   `json:"os"`
	Release         string                   `json:"release"`
	ReleaseCodename string                   `json:"release_codename"`
	ReleaseTitle    string                   `json:"release_title"`
	SupportEOL      string                   `json:"support_eol"`
	Supported       bool                     `json:"supported"`
	Version         string                   `json:"version"`
	Versions        map[string]ubuntuVersion `json:"versions"`
}

type ubuntuVersion struct {
	Label string                `json:"label"`
	Items map[string]ubuntuItem `json:"items"`
}

type ubuntuItem struct {
	FType  string `json:"ftype"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   uint64 `json:"size"`
}

func listUbuntu(ctx context.Context, client *http.Client) ([]Image, error) {
	var index ubuntuIndex
	if err := get(ctx, client, ubuntuReleasesURL+"streams/v1/index.json", &index); err != nil {
		return nil, fmt.Errorf("ubuntu catalog index: %w", err)
	}
	entry, ok := index.Index["com.ubuntu.cloud:released:download"]
	if !ok || entry.Datatype != "image-downloads" || entry.Path == "" {
		return nil, fmt.Errorf("ubuntu download catalog is absent from the index")
	}

	catalogURL, err := url.JoinPath(ubuntuReleasesURL, entry.Path)
	if err != nil {
		return nil, err
	}
	var catalog ubuntuCatalog
	if err := get(ctx, client, catalogURL, &catalog); err != nil {
		return nil, fmt.Errorf("ubuntu image catalog: %w", err)
	}
	metaRelease, err := getText(ctx, client, ubuntuLifecycleURL)
	if err != nil {
		return nil, fmt.Errorf("ubuntu release catalog: %w", err)
	}
	return parseUbuntu(catalog, parseUbuntuReleaseDates(metaRelease)), nil
}

func parseUbuntu(catalog ubuntuCatalog, releaseDates map[string]time.Time) []Image {
	var result []Image
	for _, product := range catalog.Products {
		if product.OS != "ubuntu" {
			continue
		}
		builds := make([]string, 0, len(product.Versions))
		for build, version := range product.Versions {
			if version.Label == "release" {
				if _, ok := version.Items["disk1.img"]; ok {
					builds = append(builds, build)
				}
			}
		}
		if len(builds) == 0 {
			continue
		}
		sort.Strings(builds)
		build := builds[len(builds)-1]
		item := product.Versions[build].Items["disk1.img"]
		status := "unsupported"
		if product.Supported {
			status = "supported"
		}
		buildDate := timeFromBuild(build)
		result = append(result, Image{
			Distro:       "ubuntu",
			Release:      product.Version,
			Codename:     product.ReleaseCodename,
			Support:      status,
			Supported:    product.Supported,
			IsLTS:        strings.Contains(product.ReleaseTitle, "LTS"),
			ReleaseDate:  releaseDates[product.Release],
			SupportEOL:   parseDate(product.SupportEOL),
			BuildID:      build,
			BuildDate:    buildDate,
			Arch:         normalizeArch(product.Arch),
			Format:       "qcow2",
			Name:         path.Base(item.Path),
			URL:          "https://cloud-images.ubuntu.com/" + strings.TrimPrefix(item.Path, "/"),
			Checksum:     item.SHA256,
			ChecksumType: "sha256",
			Size:         item.Size,
		})
	}
	return result
}

func parseUbuntuReleaseDates(content string) map[string]time.Time {
	result := make(map[string]time.Time)
	for _, paragraph := range strings.Split(content, "\n\n") {
		fields := make(map[string]string)
		for _, line := range strings.Split(paragraph, "\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok {
				fields[key] = strings.TrimSpace(value)
			}
		}
		if fields["Dist"] == "" || fields["Date"] == "" {
			continue
		}
		for _, layout := range []string{"Mon, 02 Jan 2006 15:04:05 MST", "Mon, 02 January 2006 15:04:05 MST"} {
			if parsed, err := time.Parse(layout, fields["Date"]); err == nil {
				result[fields["Dist"]] = parsed
				break
			}
		}
	}
	return result
}

func timeFromBuild(build string) time.Time {
	if len(build) < 8 {
		return time.Time{}
	}
	return parseDate(build[:8])
}

package images

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

type fedoraArtifact struct {
	Version    string `json:"version"`
	Arch       string `json:"arch"`
	Link       string `json:"link"`
	Variant    string `json:"variant"`
	Subvariant string `json:"subvariant"`
	SHA256     string `json:"sha256"`
	Size       string `json:"size"`
}

type fedoraLifecycle struct {
	Releases []struct {
		Name       string `json:"name"`
		IDPrefix   string `json:"id_prefix"`
		Version    string `json:"version"`
		State      string `json:"state"`
		ReleasedOn string `json:"released_on"`
		EOL        string `json:"eol"`
	} `json:"releases"`
}

var fedoraBuildPattern = regexp.MustCompile(`-(\d+)-(\d+(?:\.\d+)*)\.[^.]+\.qcow2$`)

func listFedora(ctx context.Context, client *http.Client) ([]Image, error) {
	var lifecycle fedoraLifecycle
	if err := get(ctx, client, fedoraReleasesURL, &lifecycle); err != nil {
		return nil, fmt.Errorf("fedora lifecycle catalog: %w", err)
	}
	current := make(map[string]struct {
		released string
		eol      string
	})
	for _, release := range lifecycle.Releases {
		if release.State == "current" && release.IDPrefix == "FEDORA" && strings.HasPrefix(release.Name, "F") {
			current[release.Version] = struct {
				released string
				eol      string
			}{release.ReleasedOn, release.EOL}
		}
	}

	var artifacts []fedoraArtifact
	if err := get(ctx, client, fedoraImagesURL, &artifacts); err != nil {
		return nil, fmt.Errorf("fedora image catalog: %w", err)
	}
	return parseFedora(current, artifacts), nil
}

func parseFedora(current map[string]struct {
	released string
	eol      string
}, artifacts []fedoraArtifact) []Image {
	var result []Image
	for _, artifact := range artifacts {
		lifecycle, ok := current[artifact.Version]
		if !ok || artifact.Variant != "Cloud" || artifact.Subvariant != "Cloud_Base" || !strings.HasSuffix(artifact.Link, ".qcow2") || !strings.Contains(artifact.Link, "-Cloud-Base-Generic-") {
			continue
		}
		parsedURL, err := url.Parse(artifact.Link)
		if err != nil {
			continue
		}
		size, _ := strconv.ParseUint(artifact.Size, 10, 64)
		buildID := artifact.Version
		if match := fedoraBuildPattern.FindStringSubmatch(path.Base(parsedURL.Path)); len(match) == 3 {
			buildID = match[1] + "-" + match[2]
		}
		result = append(result, Image{
			Distro:       "fedora",
			Release:      artifact.Version,
			Support:      "supported",
			Supported:    true,
			ReleaseDate:  parseDate(lifecycle.released),
			SupportEOL:   parseDate(lifecycle.eol),
			BuildID:      buildID,
			Arch:         normalizeArch(artifact.Arch),
			Format:       "qcow2",
			Name:         path.Base(parsedURL.Path),
			URL:          artifact.Link,
			Checksum:     artifact.SHA256,
			ChecksumType: "sha256",
			Size:         size,
		})
	}
	return result
}

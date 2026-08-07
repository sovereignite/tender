package images

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type debianSearch struct {
	Results []debianSearchResult `json:"results"`
}

type debianSearchResult struct {
	ID      string `json:"id"`
	Arch    string `json:"arch"`
	Release string `json:"release"`
	Version string `json:"version"`
}

type debianDetail struct {
	Items []struct {
		Kind string `json:"kind"`
		Data struct {
			Ref string `json:"ref"`
		} `json:"data"`
		Metadata struct {
			Annotations map[string]string `json:"annotations"`
			Labels      map[string]string `json:"labels"`
		} `json:"metadata"`
	} `json:"items"`
}

type debianRelease struct {
	Version     string
	Codename    string
	Series      string
	ReleaseDate time.Time
	EOL         time.Time
	EOLLTS      time.Time
}

func listDebian(ctx context.Context, client *http.Client) ([]Image, error) {
	releases, err := fetchDebianReleases(ctx, client)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var result []Image
	for _, release := range releases {
		status, supported := debianSupport(release, now)
		if !supported {
			continue
		}
		query := url.Values{
			"vendor":        {"generic"},
			"arch":          {"amd64"},
			"release":       {release.Series},
			"exclude_daily": {"true"},
			"size":          {"50"},
		}
		var search debianSearch
		if err := get(ctx, client, debianFinderURL+"/images/search?"+query.Encode(), &search); err != nil {
			return nil, fmt.Errorf("debian %s image search: %w", release.Series, err)
		}
		latest, ok := latestDebianBuild(search.Results, release.Series)
		if !ok {
			continue
		}
		var detail debianDetail
		if err := get(ctx, client, debianFinderURL+"/images/"+url.PathEscape(latest.ID), &detail); err != nil {
			return nil, fmt.Errorf("debian image %s: %w", latest.ID, err)
		}
		image, ok := debianImage(detail, latest, release, status)
		if ok {
			result = append(result, image)
		}
	}
	return result, nil
}

func fetchDebianReleases(ctx context.Context, client *http.Client) ([]debianRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, debianLifecycleURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("debian lifecycle data: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("debian lifecycle data: %s", resp.Status)
	}
	return parseDebianReleases(resp.Body)
}

func parseDebianReleases(reader io.Reader) ([]debianRelease, error) {
	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
	var result []debianRelease
	for _, record := range records[1:] {
		if len(record) < 7 || record[0] == "" || record[4] == "" {
			continue
		}
		result = append(result, debianRelease{
			Version:     record[0],
			Codename:    record[1],
			Series:      record[2],
			ReleaseDate: parseDate(record[4]),
			EOL:         parseDate(record[5]),
			EOLLTS:      parseDate(record[6]),
		})
	}
	return result, nil
}

func debianSupport(release debianRelease, now time.Time) (string, bool) {
	if release.ReleaseDate.IsZero() || now.Before(release.ReleaseDate) {
		return "unreleased", false
	}
	if release.EOL.IsZero() || !now.After(release.EOL) {
		return "supported", true
	}
	if !release.EOLLTS.IsZero() && !now.After(release.EOLLTS) {
		return "lts", true
	}
	return "unsupported", false
}

func latestDebianBuild(results []debianSearchResult, series string) (debianSearchResult, bool) {
	var latest debianSearchResult
	for _, result := range results {
		if result.Release != series {
			continue
		}
		if latest.ID == "" || compareVersions(result.Version, latest.Version) > 0 {
			latest = result
		}
	}
	return latest, latest.ID != ""
}

func debianImage(detail debianDetail, build debianSearchResult, release debianRelease, status string) (Image, bool) {
	for _, item := range detail.Items {
		if item.Kind != "Upload" || item.Metadata.Labels["upload.cloud.debian.org/image-format"] != "qcow2" || item.Metadata.Labels["upload.cloud.debian.org/type"] != "release" || item.Metadata.Labels["cloud.debian.org/vendor"] != "generic" {
			continue
		}
		return Image{
			Distro:       "debian",
			Release:      release.Version,
			Codename:     release.Codename,
			Support:      status,
			Supported:    true,
			IsLTS:        status == "lts",
			ReleaseDate:  release.ReleaseDate,
			SupportEOL:   release.EOLLTS,
			BuildID:      build.Version,
			BuildDate:    timeFromBuild(build.Version),
			Arch:         normalizeArch(build.Arch),
			Format:       "qcow2",
			Name:         path.Base(item.Data.Ref),
			URL:          "https://cloud.debian.org/images/cloud/" + strings.TrimPrefix(item.Data.Ref, "/"),
			Checksum:     strings.TrimPrefix(item.Metadata.Annotations["cloud.debian.org/digest"], "sha512:"),
			ChecksumType: "sha512-base64",
		}, true
	}
	return Image{}, false
}

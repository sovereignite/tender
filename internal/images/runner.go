package images

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const githubRunnerLatestReleaseURL = "https://api.github.com/repos/actions/runner/releases/latest"

type RunnerRelease struct {
	Version string
	URL     string
}

func LatestRunnerRelease() (RunnerRelease, error) {
	return latestRunnerRelease(context.Background(), &http.Client{Timeout: 30 * time.Second}, githubRunnerLatestReleaseURL)
}

func latestRunnerRelease(ctx context.Context, client *http.Client, releaseURL string) (RunnerRelease, error) {
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := get(ctx, client, releaseURL, &release); err != nil {
		return RunnerRelease{}, fmt.Errorf("get latest GitHub Actions runner release: %w", err)
	}
	version := strings.TrimPrefix(release.TagName, "v")
	if version == "" || strings.ContainsAny(version, "/\\") {
		return RunnerRelease{}, fmt.Errorf("invalid GitHub Actions runner version %q", release.TagName)
	}
	return RunnerRelease{
		Version: version,
		URL: fmt.Sprintf(
			"https://github.com/actions/runner/releases/download/v%s/actions-runner-linux-x64-%s.tar.gz",
			version,
			version,
		),
	}, nil
}

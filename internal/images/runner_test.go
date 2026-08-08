package images

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLatestRunnerRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v2.327.1"}`))
	}))
	defer server.Close()

	release, err := latestRunnerRelease(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "2.327.1" {
		t.Fatalf("version is %q", release.Version)
	}
	if release.URL != "https://github.com/actions/runner/releases/download/v2.327.1/actions-runner-linux-x64-2.327.1.tar.gz" {
		t.Fatalf("URL is %q", release.URL)
	}
}

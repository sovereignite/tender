package github

import (
	"errors"
	"strings"
	"testing"
)

func TestCLIGetRunnerRegistrationToken(t *testing.T) {
	client := NewCLI("sovereignite")
	client.run = func(args ...string) ([]byte, error) {
		want := []string{"api", "--method", "POST", "orgs/sovereignite/actions/runners/registration-token"}
		if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("arguments = %q, want %q", args, want)
		}
		return []byte(`{"token":"registration-token","expires_at":"2026-08-07T20:00:00Z"}`), nil
	}

	token, err := client.GetRunnerRegistrationToken()
	if err != nil {
		t.Fatal(err)
	}
	if token.Token != "registration-token" {
		t.Fatalf("token = %q", token.Token)
	}
}

func TestCLIGetRunnerRegistrationTokenReportsAuthorization(t *testing.T) {
	client := NewCLI("sovereignite")
	client.run = func(args ...string) ([]byte, error) {
		return []byte("HTTP 403"), errors.New("exit status 1")
	}

	_, err := client.GetRunnerRegistrationToken()
	if err == nil || !strings.Contains(err.Error(), "gh auth refresh") {
		t.Fatalf("error = %v", err)
	}
}

func TestCLIGetRunnerRegistrationTokenRequiresOrganization(t *testing.T) {
	client := NewCLI("")
	if _, err := client.GetRunnerRegistrationToken(); err == nil {
		t.Fatal("error = nil, want an error")
	}
}

package github

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

// TokenProvider supplies short-lived GitHub runner registration tokens.
type TokenProvider interface {
	GetRunnerRegistrationToken() (*Token, error)
}

type commandRunner func(args ...string) ([]byte, error)

// CLI uses the authenticated GitHub CLI credential without exposing it.
type CLI struct {
	org string
	run commandRunner
}

func NewCLI(org string) *CLI {
	return &CLI{
		org: org,
		run: func(args ...string) ([]byte, error) {
			return exec.Command("gh", args...).CombinedOutput()
		},
	}
}

func (c *CLI) GetRunnerRegistrationToken() (*Token, error) {
	if strings.TrimSpace(c.org) == "" {
		return nil, fmt.Errorf("GitHub organization is required")
	}

	endpoint := fmt.Sprintf("orgs/%s/actions/runners/registration-token", url.PathEscape(c.org))
	output, err := c.run("api", "--method", "POST", endpoint)
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("failed to mint runner token with gh: %s; authorize runner management with `gh auth refresh -h github.com -s admin:org`", message)
	}

	var token Token
	if err := json.Unmarshal(output, &token); err != nil {
		return nil, fmt.Errorf("failed to decode runner token response: %w", err)
	}
	if token.Token == "" {
		return nil, fmt.Errorf("GitHub returned an empty runner token")
	}
	return &token, nil
}

package cloudinit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/sovereignite/shuttle/internal/isoimage"
)

// Config holds the cloud-init configuration for a GitHub Actions runner.
type Config struct {
	// Runner configuration
	RunnerName   string
	Organization string
	Repository   string
	Token        string
	Labels       []string
	Group        string

	// System configuration
	Hostname string
	Username string

	// Network configuration
	IP      string
	Gateway string
	DNS     []string

	// Phone home
	PhoneHomePort uint32
}

// DefaultConfig returns a default cloud-init configuration.
func DefaultConfig(name, org, token string) Config {
	return Config{
		RunnerName:   name,
		Organization: org,
		Token:        token,
		Hostname:     name,
		Username:     "ubuntu",
		DNS:          []string{"8.8.8.8", "8.8.4.4"},
	}
}

// GenerateUserData generates cloud-init user data for a GitHub Actions runner.
func GenerateUserData(cfg Config) (string, error) {
	tmpl := `#cloud-config
hostname: {{ .Hostname }}
manage_etc_hosts: true

users:
  - name: {{ .Username }}
    sudo: ALL=(ALL) NOPASSWD:ALL
    shell: /bin/bash
    lock_passwd: true
    ssh_import_id:
      - gh:{{ .Username }}

packages:
  - curl
  - tar
  - jq
  - unzip

runcmd:
  - |
    set -eux
    install -d -o {{ .Username }} -g {{ .Username }} /opt/actions-runner
    install -d /mnt/gh-runner-tools
    mount -o ro LABEL=GH_RUNNER_TOOLS /mnt/gh-runner-tools
    install -m 0755 /mnt/gh-runner-tools/distaff /usr/local/libexec/distaff
    cd /opt/actions-runner
    tar xzf /mnt/gh-runner-tools/actions-runner.tar.gz
    chown -R {{ .Username }}:{{ .Username }} /opt/actions-runner
    sudo -u {{ .Username }} ./config.sh --url https://github.com/{{ .Organization }} --token {{ .Token }} --name {{ .RunnerName }}{{ if .Labels }} --labels {{ joinLabels .Labels }}{{ end }}{{ if .Group }} --runnergroup {{ .Group }}{{ end }} --work _work --replace --unattended
    ./svc.sh install {{ .Username }}
    ./svc.sh start
    /usr/local/libexec/distaff --instance-id {{ printf "%q" .RunnerName }} --port {{ .PhoneHomePort }}

final_message: "GitHub Actions runner {{ .RunnerName }} is ready!"
`

	t, err := template.New("cloudinit").Funcs(template.FuncMap{
		"joinLabels": func(labels []string) string {
			result := ""
			for i, l := range labels {
				if i > 0 {
					result += ","
				}
				result += l
			}
			return result
		},
		"joinDNS": func(dns []string) string {
			result := ""
			for i, d := range dns {
				if i > 0 {
					result += ", "
				}
				result += d
			}
			return result
		},
	}).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GenerateMetaConfig generates cloud-init meta config.
func GenerateMetaConfig(cfg Config) string {
	return fmt.Sprintf(`instance-id: %s
local-hostname: %s
`, cfg.RunnerName, cfg.Hostname)
}

// BuildSeedImage creates a NoCloud seed disk and returns its bytes.
func BuildSeedImage(userData, metaData string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "gh-runner-seed-")
	if err != nil {
		return nil, fmt.Errorf("failed to create seed workspace: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	userDataPath := filepath.Join(dir, "user-data")
	metaDataPath := filepath.Join(dir, "meta-data")
	seedPath := dir + ".img"
	defer func() { _ = os.Remove(seedPath) }()
	if err := os.WriteFile(userDataPath, []byte(userData), 0600); err != nil {
		return nil, fmt.Errorf("failed to write user-data: %w", err)
	}
	if err := os.WriteFile(metaDataPath, []byte(metaData), 0600); err != nil {
		return nil, fmt.Errorf("failed to write meta-data: %w", err)
	}

	if err := isoimage.Build(seedPath, dir, "cidata"); err != nil {
		return nil, fmt.Errorf("failed to build cloud-init seed: %w", err)
	}
	seed, err := os.ReadFile(seedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read seed image: %w", err)
	}
	return seed, nil
}

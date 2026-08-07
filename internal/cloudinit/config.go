package cloudinit

import (
	"bytes"
	"fmt"
	"text/template"
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
	Hostname   string
	Username   string
	Password   string

	// Network configuration
	IP      string
	Gateway string
	DNS     []string
}

// DefaultConfig returns a default cloud-init configuration.
func DefaultConfig(name, org, token string) Config {
	return Config{
		RunnerName:   name,
		Organization: org,
		Token:        token,
		Labels:       []string{"self-hosted", "linux", "x64"},
		Hostname:     name,
		Username:     "runner",
		Password:     "runner",
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
    lock_passwd: false
    passwd: {{ .Password }}

packages:
  - curl
  - tar
  - jq
  - unzip
  - systemd

runcmd:
  - mkdir -p /opt/actions-runner
  - cd /opt/actions-runner
  - curl -L -o actions-runner-linux-x64.tar.gz https://github.com/actions/runner/releases/download/v2.319.1/actions-runner-linux-x64-2.319.1.tar.gz
  - tar xzf actions-runner-linux-x64.tar.gz
  - rm actions-runner-linux-x64.tar.gz
  - chown -R {{ .Username }}:{{ .Username }} /opt/actions-runner
  - ./config.sh --url https://github.com/{{ .Organization }} --token {{ .Token }} --name {{ .RunnerName }} --labels {{ joinLabels .Labels }} --runnergroup {{ .Group }} --work _work --replace --unattended
  - ./svc.sh install {{ .Username }}
  - ./svc.sh start

write_files:
  - path: /etc/systemd/system/github-runner.service
    content: |
      [Unit]
      Description=GitHub Actions Runner
      After=network.target

      [Service]
      Type=simple
      User={{ .Username }}
      WorkingDirectory=/opt/actions-runner
      ExecStart=/opt/actions-runner/run.sh
      Restart=always
      RestartSec=0

      [Install]
      WantedBy=multi-user.target

  - path: /etc/netplan/01-netcfg.yaml
    content: |
      network:
        version: 2
        ethernets:
          ens3:
            dhcp4: true
            nameservers:
              addresses: [{{ joinDNS .DNS }}]

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

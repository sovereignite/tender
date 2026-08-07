# AGENTS.md

## Project

Configure self-hosted GitHub Actions runner VMs for the Sovereignite organization using **virsh/libvirt** — implemented as a **Go program** using libvirt bindings.

## Environment

- Nix flake + direnv provides the dev shell (run `direnv allow` to activate)
- The Nix shell supplies tools; the actual work is **libvirt/virsh VM management**
- Application source code is in `cmd/gh-runner/` and `internal/`

## Key Tools in Dev Shell

- **VM/infra**: opentofu, cfssl, openssl, tpm2-tools
- **Go toolchain**: go, golangci-lint, gopls, protoc-gen-go
- **K8s**: kubectl, helm, kustomize, kpt, kubeconform, kind
- **Containers**: podman, ko
- **Build**: bazel, gnumake, go-task, just
- **GitHub**: gh CLI
- **Other**: jq, dasel, nodejs

## Repo Quirks

- `config.allowUnfree = true` is set in the flake
- Module path: `github.com/sovereignite/gh-workers`
- Go dependencies: `go-libvirt` (pure Go, no CGo), `go-libvirt-xml`, `cobra`

## Go Program Architecture

```
gh-workers/
├── cmd/gh-runner/main.go        # CLI entrypoint (cobra)
├── internal/
│   ├── libvirt/                 # Libvirt client wrapper
│   │   ├── client.go            # Connection management
│   │   ├── domain.go            # VM lifecycle
│   │   ├── network.go           # NAT network (192.168.122.0/24)
│   │   └── storage.go           # Disk/volume management
│   ├── runner/                  # GitHub runner management
│   │   ├── config.go            # Runner configuration
│   │   └── manager.go           # High-level VM operations
│   ├── cloudinit/               # Cloud-init configuration
│   │   └── config.go            # User data generation
│   ├── github/                  # GitHub App integration
│   │   └── app.go               # Token generation
│   ├── health/                  # Health checking
│   │   └── checker.go           # Auto-recovery
│   ├── config/                  # Configuration management
│   │   └── config.go            # JSON config file
│   └── logging/                 # Logging
│       └── logger.go            # Level-based logging
```

## CLI Commands

```bash
# Build
go build -o gh-runner ./cmd/gh-runner

# VM lifecycle
./gh-runner create [name] --count=4 --org=sovereignite --labels=self-hosted,linux,x64
./gh-runner list
./gh-runner status [name]
./gh-runner start [name]
./gh-runner stop [name]
./gh-runner destroy [name]
./gh-runner wait [name]  # Wait for IP

# Health checking
./gh-runner health  # Check all runner health

# With GitHub App integration
./gh-runner create runner-1 \
  --org sovereignignite \
  --app-id 12345 \
  --private-key /path/to/key.pem \
  --cloud-init

# With config file
./gh-runner --config /path/to/config.json create runner-1

# With logging
./gh-runner --log-level debug create runner-1
```

## VM Configuration

- **Network**: NAT (192.168.122.0/24), DHCP .100-.254
- **Storage**: `/var/lib/libvirt/images/gh-runners/`
- **Base image**: Ubuntu 24.04 cloud image (auto-downloaded)
- **Default VM**: 4GB RAM, 2 CPUs, 20GB disk
- **Ephemeral**: Clone-on-create, discard on destroy

## Auto-Reboot Pattern

VMs use `--ephemeral` runner flag + systemd reboot:
1. Runner completes one job → exits
2. systemd restarts runner → re-registers with GitHub
3. Fresh state for every build

## Commands

```bash
# Enter dev shell (if direnv not used)
nix develop

# Update flake inputs
nix flake update

# Check flake evaluates cleanly
nix flake check

# Build and run
go build -o gh-runner ./cmd/gh-runner
./gh-runner --help
```

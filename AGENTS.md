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

- **Network**: Uses existing `default` network (must be active)
- **Storage**: Uses existing `default` pool (`/home/me/.local/share/libvirt/images/`)
- **Base image**: Ubuntu 24.04 cloud image (auto-downloaded)
- **Runner tools**: Versioned `GH_RUNNER_TOOLS` ISO cached in the shared images pool and attached read-only to every VM
- **Default VM**: 4GB RAM, 2 CPUs, 20GB disk
- **Ephemeral**: Clone-on-create, discard on destroy

## Auto-Reboot Pattern

VMs use `--ephemeral` runner flag + systemd reboot:
1. Runner completes one job → exits
2. systemd restarts runner → re-registers with GitHub
3. Fresh state for every build

## Runner Readiness: Vsock Phone Home

Runner readiness uses virtio-vsock, not HTTP or the libvirt network:

1. `gh-runner create` opens an `AF_VSOCK` listener on a dynamically assigned port before defining or starting the VM.
2. The generated cloud-init seed contains that vsock port and a standalone `/usr/local/libexec/gh-runner-phone-home.py` callback program.
3. The VM mounts the shared `GH_RUNNER_TOOLS` ISO and extracts the cached runner archive into its writable root disk.
4. Cloud-init configures and starts the GitHub Actions runner service without downloading runner binaries.
5. The callback connects to host CID `2` and sends newline-delimited JSON containing `instance_id`, `hostname`, `fqdn`, `pub_key_rsa`, `pub_key_ecdsa`, and `pub_key_ed25519`.
6. The Go listener records the guest CID and metadata, replies `OK`, and marks the matching runner ready.
7. The `create` command exits successfully only after receiving that acknowledged callback.

Do not replace this path with a callback to the libvirt gateway. Vsock deliberately avoids dependency on guest IP assignment, routing, NAT, DNS, or host firewall configuration.

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

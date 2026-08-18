# DIG: github.com/sovereignite/tender

Repo root: /var/lib/hermes/src/github.com/sovereignite/tender
Remote:     https://github.com/sovereignite/tender.git  (branch: master)
Language:   Go (go 1.26.5), ~4,380 LOC of Go across 39 `.go` files
License:    GPLv2 (LICENSE.md)
Reviewed:   2026-08-17

================================================================================
1. WHAT THE PROJECT DOES
================================================================================

Shuttle is a CLI tool that builds and manages disposable GitHub Actions
self-hosted runner VMs on a local libvirt host. The "loom" metaphor
(README) names future components (Warp/Distaff/Heddle/Bobbin/Spindle/Weft/Flyer/
Reed), but the ACTUAL implemented product today is narrower than the metaphor:

  - `shuttle`      : host CLI that provisions, starts, stops, destroys, and
                     health-checks GitHub Actions runner VMs via libvirt.
  - `distaff`      : tiny guest-side agent. After boot it collects the guest's
                     hostname/FQDN and SSH host public keys and "phones home"
                     to the host over virtio-vsock (host CID 2).
  - `repo-check`   : git pre-commit / CI policy hook that rejects submodules,
                     secret/binary filenames, oversized files, non-text content,
                     and GitHub-hosted runners in workflows.

The intended end-state (tracked in GitHub issue #1 and sub-issues, per
AGENTS.md and docs/roles-and-skills.md) is a declarative, resource-based
system that builds installable Linux systems for many targets (bare metal,
cloud, Raspberry Pi, libvirt) from composable resources and a `shuttle deploy`
command. NONE of that declarative model, deployment engine, or `shuttle deploy`
command exists yet. The current code is imperative Cobra commands + JSON config
+ cloud-init + a shared runner-tools ISO, orchestrated by `runner.Manager`.

Do not describe the planned resource/build/deploy model as implemented.

================================================================================
2. FULL DIRECTORY TREE (tracked files; .git excluded)
================================================================================

shuttle/
├── AGENTS.md                         # Agent/AI contributor instructions (158 lines)
├── README.md                         # Loom-metaphor overview
├── FINDINGS.md                       # Security/correctness review (12 issues, 2026-08-08)
├── LICENSE.md                        # GPLv2
├── go.mod / go.sum                   # Module: github.com/sovereignite/tender
├── flake.nix / flake.lock            # Nix dev shell (huge tool set)
├── .env / .envrc                     # Secret-bearing env files (untracked content; .env.local is the user one)
├── .gitignore                        # Ignores bin/, .direnv/, .env.local, etc.
├── .pre-commit-config.yaml           # repo-check + gitleaks + go test + golangci-lint
├── cmd/
│   ├── shuttle/
│   │   ├── main.go                   # Host CLI (Cobra), all subcommands
│   │   └── main_test.go
│   ├── distaff/
│   │   ├── main.go                   # Guest phone-home binary
│   │   └── main_test.go
│   └── repo-check/
│       ├── main.go                   # File-policy hook
│       └── main_test.go
├── internal/
│   ├── config/config.go              # JSON Config + DefaultConfig + Load/Save
│   ├── cloudinit/
│   │   ├── config.go                 # User-data/meta-data templates, seed ISO builder
│   │   └── config_test.go
│   ├── github/
│   │   ├── app.go                    # GitHub App JWT + runner registration token
│   │   ├── cli.go                    # gh CLI token provider
│   │   └── cli_test.go
│   ├── health/
│   │   └── checker.go                # One-shot + looped health checks, auto-recover
│   ├── images/
│   │   ├── catalog.go                # Ubuntu/Debian/Fedora catalog + ListImages/SelectImage
│   │   ├── ubuntu.go / debian.go / fedora.go
│   │   ├── http.go                   # HTTP fetch helpers
│   │   ├── runner.go                 # LatestRunnerRelease (actions/runner)
│   │   ├── catalog_test.go / runner_test.go
│   ├── isoimage/
│   │   ├── isoimage.go               # In-process ISO9660 (RockRidge) builder
│   │   └── isoimage_test.go
│   ├── libvirt/
│   │   ├── client.go                 # go-libvirt connection wrapper
│   │   ├── domain.go                 # Domain create/start/stop/destroy/status/IP/XML
│   │   ├── network.go                # NAT network config constants + lookups
│   │   ├── storage.go                # Pools, volume clone, seed + runner-tools ISO cache
│   │   ├── network_test.go / storage_test.go
│   ├── logging/
│   │   └── logger.go                 # Leveled stdlib logger
│   ├── phonehome/
│   │   ├── phonehome.go              # Collect + vsock Send (host CID 2)
│   │   └── phonehome_test.go
│   └── runner/
│       ├── manager.go                # High-level orchestration (the "weft")
│       └── config.go                 # Runner Config + name generation + Validate
├── docs/
│   └── roles-and-skills.md           # Role boundaries + planned skill ownership
├── .github/
│   ├── workflows/
│   │   ├── ci.yaml                   # Main pipeline: changes→commands→lint→test→build→scan→sign→attest→release→publish
│   │   ├── build.yaml / scan.yaml / sign.yaml / attest.yaml / publish.yaml
│   │   ├── image-build.yaml / image-scan.yaml / image-sign.yaml / image-attest.yaml
│   └── actions/
│       ├── changes / commands / lint / test / build / scan / sign / attest / build(2) / image-* / scan(2)
│       └── (reusable composite actions referenced by the workflows above)
└── .opencode/
    ├── opencode.json
    └── agent/                        # One personality file per role:
        ├── warp.md distaff.md heddle.md bobbin.md spindle.md weft.md flyer.md reed.md

================================================================================
3. KEY SOURCE FILES & BEHAVIOR
================================================================================

--- cmd/shuttle/main.go (host CLI) -------------------------------------------
Cobra root command `shuttle`. Persistent flags: --config, --log-level (info).
Subcommands (all connect to libvirt via libvirt.NewClient(), qemu:///system):

  create  [flags]  Builds everything end-to-end:
        - flags: -l/--labels, -m/--memory (4096), -c/--cpus (2),
          -o/--org (env GH_RUNNER_ORG), -r/--repo, --app-id, --private-key,
          -g/--group, --token, -u/--username (env GH_USERNAME)
        - Resolves a GitHub App (if app-id+private-key) else uses `gh` CLI
          (github.NewCLI) to mint a runner registration token.
        - EnsureInfrastructure: ensures `images` pool, selects base cloud
          image, caches it (qcow2 clone), downloads latest actions-runner
          tarball, builds a read-only runner-tools ISO containing distaff.
        - Generates cloud-init user-data + meta-data, builds a NoCloud seed
          ISO, creates+starts the domain, then waits up to 15m for phone-home
          over vsock (and prints the vsock CID).
  start/stop/destroy [name]   Lifecycle ops (destroy deletes disk+seed volumes).
  list / status [name] / wait [name] / health / console [name] / dumpxml [name]
  images list   (flags: -d distro, -r release, -l LTS, -a arch)
  images select [distro]  (flags: -r release, -a arch, -f format, -l LTS)

Note: `create` takes NO positional name and generates `runner-<hex>` if none
is derived; name uniqueness is NOT enforced (see FINDINGS #3).

--- internal/runner/manager.go (orchestration) ------------------------------
*Manager holds the libvirt client + an in-process vsock phone-home server.
- EnsureInfrastructure: image pool, SelectImage, CacheImage (clone),
  LatestRunnerRelease, CacheRunnerTools (downloads actions-runner tarball,
  copies distaff binary, builds GH_RUNNER_TOOLS ISO, uploads to `images` pool).
- CreateWithCloudInit: starts vsock listener, renders cloud-init (cloudinit),
  builds seed ISO into the `default` pool, then Create() clones base qcow2
  into a 20 GiB runner volume and defines the domain.
- WaitForPhoneHome polls an in-memory map for up to timeout.
- distaff binary is located as a sibling of the shuttle executable (`distaff`).

--- internal/libvirt/domain.go ----------------------------------------------
DefineXML for a KVM/q35 domain:
  - EFI firmware, host-model CPU, ACPI+SMM (secure boot posture),
    virtio disk (vda/qcow2 from cloned base), cloud-init seed (sda/sata
    cdrom), runner-tools ISO (sdb/sata cdrom, read-only+shareable),
    virtio network on cfg.NetworkName, virtio serial/console, emulated TPM
    2.0, virtio VSock, SPICE graphics, virtio video.
  - Arch is hard-coded x86_64.
  - CreateDomain destroys+undefines any same-named existing domain (unowned).
  - DestroyDomain ignores DomainDestroy error and swallows undefine errors.

--- internal/libvirt/storage.go ---------------------------------------------
  - Pools: `default` (runner volumes) and `images` (base images + tools ISO).
  - EnsureImagesPool derives `shuttle-images` dir pool from `default`.
  - CloneVolume: 20 GiB qcow2, deletes existing same-named volume first.
  - CacheImage / imageCacheName: verifies download URL host (https://github.com
    for runner; cloud-image hosts for distros) and caches by content name.
  - CacheRunnerTools: validates the distaff callback binary is a statically
    linked Linux x86-64 ELF (no .interp), SHA-256 hashes it, names the ISO
    `actions-runner-<ver>-<hash8>-linux-x64.iso`, uploads to images pool.
  - downloadFile / copyFile / validateCallbackBinary helpers.

--- internal/cloudinit/config.go --------------------------------------------
Renders `#cloud-config` user-data via text/template:
  - user `ubuntu` (sudo NOPASSWD, lock_passwd, ssh_import_id: gh:<user>),
  - installs curl/tar/jq/unzip,
  - mounts LABEL=GH_RUNNER_TOOLS, installs distaff to
    /usr/local/libexec/distaff, unpacks actions-runner, runs
    ./config.sh with --token, --labels, --runnergroup, --replace --unattended,
    installs+starts svc.sh, then runs distaff --instance-id --port <vsockPort>.
  - Token, org, name, labels, group are interpolated into YAML and a root
    shell command (see FINDINGS #4: injection risk).
  - BuildSeedImage writes user-data/meta-data and builds a cidata ISO.

--- internal/phonehome/phonehome.go -----------------------------------------
Collect(InstanceID): hostname, FQDN (CNAME lookup), and ssh_host_rsa/ecdsa/
ed25519 public keys from /etc/ssh.
Send: dials vsock HostCID=2 on the given port, retries up to 10x with 3s
backoff within a 2m context deadline, expects an "OK\n" ack.

--- internal/github/app.go + cli.go ------------------------------------------
  - App: parses PKCS1 RSA private key, mints a 10-min RS256 JWT, exchanges it
    for an installation token, then a runner registration token via the
    GitHub REST API (api.github.com).
  - CLI: shells out to `gh api --method POST orgs/<org>/actions/runners/
    registration-token`. TokenProvider interface unifies both.

--- internal/images/* -------------------------------------------------------
catalog.go + per-distro files (ubuntu/debian/fedora) query upstream catalog
APIs (cloud-images.ubuntu.com, cloud-image-finder.debian.net,
fedoraproject.org) and produce a normalized Image list with checksums.
runner.go fetches the latest actions/runner release tag and builds the
linux-x64 tarball URL. http.go is a shared GET helper.

--- internal/health/checker.go ----------------------------------------------
One-shot HealthCheck (used by `shuttle health`) and a looped Start() that
recovers shut-off domains by starting them (`recoverDomain`). It iterates
ListDomains() results WITHOUT ownership filtering (FINDINGS #1).

--- internal/config/config.go / runner/config.go ----------------------------
JSON Config (github/libvirt/runner/health/logging). Defaults: libvirt
qemu:///system, network "shuttle", runner 4096MiB/2cpu, labels
self-hosted/linux/x64, health 30s/2m, log info. Config file path default
~/.config/shuttle/config.json. Runner Config.Validate only requires
org OR repo; name auto-generated from crypto/rand if empty.

--- cmd/distaff/main.go / repo-check/main.go --------------------------------
  - distaff: flags --instance-id (required) --port (required, <=uint32 max);
    collects payload and sends over vsock.
  - repo-check: reads the git index (`git ls-files --stage -z`), rejects
    submodules, forbidden extensions (.key/.pem/.exe/.so/...), .env.local,
    id_rsa*, >1 MiB files, binary/NUL content, executables without shebang,
    and `runs-on: ubuntu|windows|macos-*` in .github/workflows (no GH-hosted
    runners). Used by pre-commit and CI.

================================================================================
4. DEPENDENCIES (go.mod)
================================================================================

Direct:
  github.com/digitalocean/go-libvirt   (pure-Go libvirt client — no CGo)
  github.com/diskfs/go-diskfs          (in-process ISO9660 builder)
  github.com/golang-jwt/jwt/v5         (GitHub App JWT)
  github.com/libvirt/libvirt-go-xml    (libvirt domain/network/volume XML)
  github.com/mdlayher/vsock            (virtio-vsock transport)
  github.com/spf13/cobra               (CLI)

Indirect:
  github.com/djherbis/times, github.com/inconshreveable/mousetrap,
  github.com/mdlayher/socket, github.com/spf13/pflag,
  golang.org/x/crypto, golang.org/x/net, golang.org/x/sync, golang.org/x/sys

Hard requirement (AGENTS.md): pure-Go client only — do NOT add a CGo
requirement without concrete need.

Nix flake (flake.nix) provides a very large dev shell: go, golangci-lint,
gitleaks, gotools, gopls, bazel, ko, kubectl, helm, kustomize, opentofu,
kind, tpm2-tools, cfssl, podman, gh, nodejs, uv, opencode, and many
k8s/KRM validation tools. Inputs: numtide/flake-utils, NixOS/nixpkgs
nixos-unstable (allowUnfree=true).

================================================================================
5. BUILD / TEST / VALIDATION COMMANDS
================================================================================

From repo root (AGENTS.md):

  go build -o bin/shuttle ./cmd/shuttle
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/distaff ./cmd/distaff
  ./bin/shuttle --help

Tests / checks:
  go test ./...
  go test -race ./...
  go vet ./...
  golangci-lint run
  go mod verify
  go run ./cmd/repo-check          # file-policy hook

Pre-commit (.pre-commit-config.yaml) runs: repo-check, gitleaks,
go test ./..., golangci-lint run. CI (ci.yaml) runs change-selected
command gates on self-hosted runners (lint→test→build→scan→sign→attest)
and publishes direct binaries from version tags (v*).

WARNING (AGENTS.md): libvirt ops mutate host VM/network/storage state. Do not
run live lifecycle ops or destructive integration tests by default. The repo
ships NO unit tests that touch a live libvirt; tests cover catalog/runner,
cloudinit, isoimage, libvirt network/storage helpers, phonehome, github CLI,
and repo-check logic.

================================================================================
6. CONFIGURATION & NOTABLE DETAILS
================================================================================

- Config file: JSON (config.Load/Save). Env vars GH_RUNNER_ORG, GH_USERNAME
  feed CLI defaults. .env / .envrc exist (secret-bearing; .env.local is the
  intended untracked user file — never commit keys/tokens).
- libvirt URI: qemu:///system. Network: "shuttle" (CreateNetwork is currently
  a no-op that just uses the existing default network). Subnet defaults
  192.168.122.0/24 gateway .1, DHCP .100–.254.
- Runner disk: 20 GiB qcow2 cloned from the selected base cloud image.
- Architectures: code works for x86_64 today. Planned support (docs/roles):
  amd64, arm64, riscv64, ppc64le, s390x, loong64 (no 386).
- Security posture baked into domain XML: EFI + SMM (secure boot), emulated
  TPM 2.0, virtio everywhere.
- distaff is deliberately tiny and statically linked; the host verifies it is
  a static Linux x86-64 ELF before caching it into the tools ISO.
- Phone-home uses virtio-vsock to host CID 2; the host runs an in-process
  vsock listener and records (CID, hostname, FQDN, SSH host keys, ready).

================================================================================
7. SECURITY / CORRECTNESS FINDINGS (FINDINGS.md, review 2026-08-08)
================================================================================

A reviewed set of 12 issues (2 Critical, several High). Highlights:

CRITICAL
  1. Health checks can start unrelated libvirt VMs — ListDomains returns ALL
     domains; checker starts every shut-off one (no ownership filtering).
  2. Failed destruction can still delete storage — DestroyDomain swallows
     DomainDestroy/Undefine errors, then Destroy deletes the disk+seed anyway.

HIGH
  3. Duplicate creation silently replaces an existing VM (CloneVolume deletes
     same-named disk; CreateDomain destroys same-named domain) with no
     ownership check or uniqueness enforcement.
  4. Cloud-init values permit root shell and YAML injection (org, username,
     name, token, labels, group interpolated unescaped into YAML + root cmd).
  5. Registration tokens remain exposed inside the VM (cloud-init user-data
     carries the GitHub token at rest in the seed ISO).
  ...plus lifecycle failure-order, resource-ownership, artifact verification,
  and secret-handling gaps through to item #12.

Action: read FINDINGS.md before changing lifecycle/provisioning/auth/health/
phone-home/image-cache/storage code. AGENTS.md repeats these as hard rules
(verify ownership, reject duplicate names, delete only after confirmed
shutdown+undefine, roll back on failure in reverse order, never retain tokens,
validate values before placing them in YAML/shell/URLs/paths/XML, authenticate
phone-home and bound payload sizes/deadlines, verify downloaded artifacts).

================================================================================
8. ROLES & SKILL MODEL (docs/roles-and-skills.md)
================================================================================

Nine named roles (Warp=coordinator, Distaff=bounded worker, Heddle=technical
investigation, Bobbin=repeatable routines, Spindle=presentation, Weft=resource
accounting, Flyer=human/social/commits, Reed=status/reporting). Each has an
OpenCode agent persona under .opencode/agent/. Skills are to be named
verb-object (resolve-resources, fetch-resources, build-system-image,
run-libvirt-target, etc.) and OWNED by roles via a mapping table — but the
table describes INTENDED capability areas, not shipped tools. Architecture
decisions (Kubernetes-style Shuttle YAML, shuttle deploy, arch-specific
independent artifacts, libvirt as first target) are recorded as intended
outcome; current Go code is still imperative and must not be described as the
planned declarative system.

================================================================================
9. NOTABLE / ODDBALL OBSERVATIONS
================================================================================

- The README loom metaphor wildly oversells vs. the implemented reality
  (only libvirt GitHub-runner provisioning exists). Keep the two separate.
- `CreateNetwork` is a stub that does nothing (just reuses the default net).
- Domain arch is hard-coded x86_64 even though multi-arch is "planned".
- The repo documents a heavyweight Nix/OpenCode/k8s tooling world (bazel, ko,
  helm, kustomize, tpm2, cfssl...) that is far larger than what the current
  ~4.4k-LOC Go actually uses — much of flake.nix is future-looking scaffolding.
- FINDINGS.md is a real, detailed security review and is the authoritative
  risk list for this codebase.
- `.env` / `.envrc` are present but their contents are secret-bearing and were
  not read; .env.local is the intended untracked user env file.
- CI is fully self-hosted (runs-on: self-hosted) and includes binary signing
  + attestation + scanning gates before any v* tag release.

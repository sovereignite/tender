# AGENTS.md

## Project Overview

Shuttle builds installable Linux systems from resources and exercises them with
development targets. The current implementation provisions GitHub Actions
runner VMs through libvirt. `shuttle` is the host CLI; `distaff` is the small
guest-side agent that reports readiness to the host over virtio-vsock.

The intended resource and delivery model is tracked in GitHub issue #1 and its
sub-issues. It introduces declarative resources, architecture-specific OS
images and system extensions, and a generic deployment-target interface. Do
not describe that design as implemented until its code exists.

Role definitions and routing live in `docs/roles-and-skills.md`. OpenCode agent
definitions live in `.opencode/agent/`. Keep this file focused on repository
instructions shared by every coding agent.

## Development Environment

- The repository is a Go module: `github.com/sovereignite/shuttle`.
- A Nix flake supplies the development tools. Direnv users should run
  `direnv allow`; otherwise use `nix develop`.
- Libvirt operations mutate host VM, network, and storage state. Do not run a
  live lifecycle operation unless the task requires it and its target is known
  to be safe.
- The host implementation uses the pure-Go `go-libvirt` client. Do not add a
  CGo requirement without a concrete need.
- `.env.local` is intentionally untracked. Never commit credentials, runner
  registration tokens, GitHub App private keys, or generated secrets.

## Repository Layout

- `cmd/shuttle/`: host CLI entrypoint.
- `cmd/distaff/`: guest phone-home binary.
- `cmd/repo-check/`: repository file-policy hook.
- `internal/images/`: OS image and GitHub runner release discovery.
- `internal/libvirt/`: libvirt connection, domain, network, and storage work.
- `internal/runner/`: current high-level runner VM orchestration.
- `internal/cloudinit/`: cloud-init data and seed image generation.
- `internal/phonehome/`: guest metadata collection and vsock transport.
- `internal/isoimage/`: in-process ISO generation.
- `docs/roles-and-skills.md`: role boundaries and planned skill ownership.
- `FINDINGS.md`: reviewed correctness, security, and test gaps.

## Build Commands

Run commands from the repository root.

```bash
go build -o bin/shuttle ./cmd/shuttle
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/distaff ./cmd/distaff
./bin/shuttle --help
```

Build generated executables under `bin/`; the directory is ignored. Do not
write build outputs into the repository root or commit generated binaries.

## Test and Validation Commands

Run the narrowest relevant test first, then the full checks before finishing a
substantial change.

```bash
go test ./...
go test -race ./...
go vet ./...
golangci-lint run
go mod verify
go run ./cmd/repo-check
```

The pre-commit configuration runs repository policy checks, `gitleaks`, Go
tests, and `golangci-lint`. `.github/workflows/ci.yaml` runs change-selected
command gates on self-hosted runners and publishes direct binaries from version
tags. Do not imply that CI replaces the broader local checks listed above.

Do not perform destructive libvirt integration tests by default. If a task
requires one, state which domain, storage pool, volumes, and network may be
modified before running it.

## Current CLI

These commands exist in `cmd/shuttle/main.go` today:

```bash
./shuttle create --org example-org
./shuttle start NAME
./shuttle stop NAME
./shuttle destroy NAME
./shuttle list
./shuttle status NAME
./shuttle wait NAME
./shuttle health
./shuttle console NAME
./shuttle dumpxml NAME
./shuttle images list
./shuttle images select [DISTRO]
```

`create` accepts flags and no positional name. Use `./shuttle <command> --help`
as the source of truth for current flags. The planned `shuttle deploy` command
does not exist yet.

## Code Style and Conventions

- Follow standard Go formatting and idioms; run `gofmt` on changed Go files.
- Keep changes small and explicit. Prefer extending existing packages over
  adding abstraction before it has a concrete consumer.
- Wrap errors with operation context using `%w`.
- Pass `context.Context` through blocking, network, and lifecycle operations.
- Keep host orchestration out of `distaff`; it is a bounded guest-side worker.
- Generate structured YAML or XML through serializers rather than unescaped
  string interpolation.
- Use Go architecture names in declarations and translate them explicitly at
  external boundaries. Planned supported architectures are `amd64`, `arm64`,
  `riscv64`, `ppc64le`, `s390x`, and `loong64`; do not add `386`.
- Add or update tests for behavior changes, especially lifecycle failure order,
  resource ownership, artifact verification, and secret handling.

## Security and Safety

Read `FINDINGS.md` before changing lifecycle, provisioning, authentication,
health, phone-home, image-cache, or storage code. In particular:

- Never operate on a libvirt domain or volume without verifying Shuttle
  ownership. Current list and health paths do not enforce this safely.
- Reject duplicate names by default. Do not silently replace domains or disks.
- Delete storage only after domain shutdown and undefinition are confirmed.
- Roll back resources created by a failed create or deployment in reverse order.
- Never log or retain runner registration tokens longer than required.
- Validate values before placing them in cloud-init, YAML, shell commands, URLs,
  filesystem paths, or libvirt XML.
- Authenticate phone-home events and bound payload sizes and I/O deadlines.
- Verify downloaded artifacts before publishing or trusting cached entries.

## Documentation Instructions

- Document current behavior as current and proposed behavior as planned.
- Source command syntax from Cobra definitions or `--help`, not memory.
- Source defaults from code and name the defining file when the distinction
  matters.
- Keep detailed architecture decisions in design docs or issues, not in this
  operational instruction file.
- Update documentation when renaming commands, packages, binaries, resources,
  or configuration fields.
- Do not duplicate agent personalities here; update `.opencode/agent/` and
  `docs/roles-and-skills.md` when role boundaries change.

## Commit and Pull Request Guidance

- Do not commit, amend, push, or open a pull request unless explicitly asked.
- Before committing, inspect status and diffs and stage only intended files.
- Use concise conventional commit subjects consistent with repository history,
  such as `docs: align agent instructions with current behavior`.
- Include tests or validation performed in the pull request description.
- Call out skipped live libvirt testing and any remaining safety risk.
- Never bypass hooks to make a change pass.

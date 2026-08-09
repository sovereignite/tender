# Repository Review Findings

Review date: 2026-08-08

Scope: Go application correctness, security, dependencies, tests, and framework/library practices. Nix-related findings are intentionally excluded.

## Critical

### 1. Health checks can start unrelated libvirt VMs

References:

- `internal/libvirt/domain.go:259-293`
- `internal/health/checker.go:91-100`
- `internal/health/checker.go:110-118`
- `cmd/shuttle/main.go:330-361`

`ListDomains` returns every active and inactive libvirt domain without checking runner ownership. The health checker then starts every shut-off domain it receives. On a shared host, `gh-runner health` can boot intentionally stopped, unrelated machines.

Add namespaced libvirt metadata to managed runner domains and filter list, health, recovery, and destructive lifecycle operations by that metadata.

### 2. Failed destruction can still delete VM storage

References:

- `internal/libvirt/domain.go:170-179`
- `internal/runner/manager.go:240-252`

`DestroyDomain` ignores `DomainDestroy`. If `DomainUndefineFlags` fails, it also ignores the fallback `DomainUndefine` error and returns success. `Manager.Destroy` then deletes the runner disk and seed volume.

If QEMU cannot be stopped or the domain cannot be undefined, this can remove storage from a still-running or still-defined VM while reporting success. Return the libvirt errors, verify the domain is inactive and undefined, and only then remove its volumes.

## High

### 3. Duplicate creation silently replaces an existing VM

References:

- `internal/libvirt/storage.go:172-183`
- `internal/libvirt/domain.go:42-47`
- `internal/runner/manager.go:155-184`

`CloneVolume` deletes an existing named disk, while `CreateDomain` destroys and undefines an existing domain with the same name. The operations ignore errors and do not verify that the resources belong to gh-workers.

A name collision, concurrent creation, or deliberate name reuse can destroy an unrelated or healthy VM before the replacement is viable. Creation should reject existing names by default. Any replacement operation should be explicit, ownership-checked, serialized, and transactional.

### 4. Cloud-init values permit root shell and YAML injection

References:

- `internal/cloudinit/config.go:48-117`
- `internal/runner/config.go:38-45`
- `cmd/shuttle/main.go:163-172`

Organization, username, runner name, token, labels, and group are interpolated directly into YAML and root shell commands. Validation does not restrict these values. Shell metacharacters can execute commands as root, while spaces and YAML metacharacters can corrupt provisioning.

Generate YAML structurally and pass provisioning arguments through a safely encoded argument or environment file. Validate names, organizations, repositories, usernames, labels, and groups at the CLI boundary.

### 5. Registration tokens remain exposed inside the VM

References:

- `internal/cloudinit/config.go:68-81`
- `internal/libvirt/domain.go:104-113`
- `internal/runner/manager.go:215-225`

Provisioning uses `set -x`, which can place the registration token in cloud-init logs. The token is also embedded in `user-data` and retained in an attached, read-only seed volume. Workflow jobs run under a passwordless-sudo account and can read or mount that seed while the organization-wide token remains valid.

Do not trace commands containing secrets. Detach and securely delete the seed before accepting jobs, or use a just-in-time runner configuration that does not retain a reusable registration token.

### 7. Phone-home readiness can be spoofed or exhausted by another guest

References:

- `internal/runner/manager.go:66-84`
- `internal/runner/manager.go:88-118`
- `internal/phonehome/phonehome.go:18-25`

The host trusts any valid payload and indexes readiness solely by the caller-provided `instance_id`. It records the peer CID but does not compare it with an expected domain. Another guest can claim a newly created runner's identity and cause `create` to report readiness before the real VM is configured.

Accepted connections also have no deadline or payload limit, allowing a guest to hold goroutines indefinitely or submit oversized JSON.

Put an unpredictable per-create nonce in the seed and phone-home payload, verify the nonce and expected CID, use a bounded decoder, set read/write deadlines, and mark the event ready only after sending the acknowledgement successfully.

### 8. Runner and image artifacts are not safely authenticated when cached

References:

- `internal/libvirt/storage.go:318-336`
- `internal/libvirt/storage.go:344-405`
- `internal/libvirt/storage.go:488-506`
- `internal/libvirt/storage.go:543-571`

Existing cached volumes are accepted solely by name. A process or host crash after creating the final volume can leave a truncated artifact that future runs treat as valid. The GitHub Actions runner archive is downloaded and installed as root without checking a published asset digest.

Upload under a temporary volume name, verify the complete size, structure, and digest, and only then publish the final cache entry atomically. Verify the runner archive against GitHub's published digest before including it in the tools ISO.

## Medium

### 9. Repository-scoped runner creation does not work

References:

- `cmd/shuttle/main.go:112-124`
- `cmd/shuttle/main.go:135-144`
- `internal/runner/manager.go:202-208`
- `internal/cloudinit/config.go:16-21`
- `internal/cloudinit/config.go:78`
- `internal/github/cli.go:33-39`

The CLI exposes `--repo`, but token acquisition always uses the organization endpoint. `CreateWithCloudInit` does not copy the repository into cloud-init, and `config.sh` always receives an organization URL.

With only `--repo`, token acquisition fails because the organization is empty. With both flags, the command creates an organization runner instead. Model the target as exactly one organization or `owner/repository`, mint the corresponding token, and generate the corresponding registration URL.

### 10. Almost all JSON configuration settings are ignored

References:

- `internal/config/config.go:10-85`
- `cmd/shuttle/main.go:43-68`
- `cmd/shuttle/main.go:95-144`

The configuration file is loaded, but creation reconstructs runner settings from flag variables. Only `Logging.File` affects behavior. GitHub App settings, organization, libvirt URI/network, runner labels/group/resources, health settings, logging level, and logging format are silently ignored.

Define and implement precedence between defaults, configuration files, environment variables, and explicitly changed flags. Remove or reject unsupported settings rather than accepting them without effect.

### 11. Failed creation leaks domains, disks, seeds, and tokens

References:

- `internal/runner/manager.go:155-225`
- `cmd/shuttle/main.go:144-155`

There is no rollback if seed creation succeeds but disk cloning, domain definition, or startup fails. A phone-home timeout also leaves the VM and its token-bearing seed in place.

Track resources created during the operation and clean them up in reverse order unless creation reaches a committed state. Consider an explicit keep-on-failure option for diagnostics.

### 12. Registration tokens are minted before lengthy infrastructure work

References:

- `cmd/shuttle/main.go:118-133`
- `internal/runner/manager.go:122-148`

The short-lived runner registration token is obtained before downloading and caching the cloud image and runner tools. Slow infrastructure preparation can consume the token lifetime before cloud-init invokes `config.sh`.

Prepare infrastructure first, then mint the token immediately before seed generation and VM startup.

### 13. Health checks overlap and do not honor cancellation fully

References:

- `internal/health/checker.go:75-100`
- `internal/health/checker.go:103-129`
- `internal/libvirt/domain.go:223-245`

Every ticker interval launches new goroutines even when previous domain checks are still waiting for an IP. `WaitForDomainIP` sleeps without accepting a context. With a 30-second interval and a two-minute timeout, multiple checks and recovery attempts can overlap for each domain.

Make polling context-aware, serialize checks per domain, bound concurrency, and avoid starting a new cycle before the previous cycle completes.

### 14. GitHub App HTTP requests can hang indefinitely

References:

- `internal/github/app.go:75-90`
- `internal/github/app.go:114-129`
- `internal/github/app.go:144-160`

The GitHub App implementation creates HTTP clients without timeouts and requests without caller contexts. A stalled DNS lookup, TLS connection, or response can block runner creation indefinitely.

Accept a context and shared HTTP client, use `NewRequestWithContext`, and configure appropriate dial, TLS, response-header, and total timeouts.

### 15. Debian images selected by the catalog cannot be cached

References:

- `internal/images/debian.go:154-176`
- `internal/libvirt/storage.go:575-595`

Debian image records use `sha512-base64` and omit `Size`, while `imageCacheName` accepts only SHA-256 and requires a nonzero size. Passing a selected Debian image to `CacheImage` always fails.

Normalize Debian metadata to a supported digest and size, or extend cache validation and streaming verification to support the emitted SHA-512 representation.

## Low

### 16. The status command cannot display an IP address

References:

- `internal/libvirt/domain.go:182-220`
- `cmd/shuttle/main.go:288-295`

`GetDomainStatus` never populates `DomainStatus.IP`, although the status command conditionally prints it. Query interface addresses when building status or remove the unused output.

### 17. Manager and logger resources have no explicit lifecycle

References:

- `internal/runner/manager.go:27-45`
- `internal/runner/manager.go:66-84`
- `internal/logging/logger.go`

The manager does not expose a method to stop its vsock listener and goroutine. The logger does not expose ownership-aware closure for file output. Repeated use from a daemon or tests can leak listeners, goroutines, and file descriptors.

Add idempotent `Close` methods and arrange CLI defers where the component owns the resource.

### 18. Lifecycle and security-critical behavior lacks tests

References:

- `internal/runner/manager.go`
- `internal/libvirt/domain.go`
- `internal/health/checker.go`
- `internal/github/app.go`

There are no automated tests for managed-domain filtering, duplicate names, failed destroy/undefine, creation rollback, phone-home authentication and limits, repository runners, configuration precedence, or GitHub App timeout behavior.

Add a fake libvirt transport for failure-order tests and isolated integration tests for define, start, phone-home, job completion, recycle, and destroy behavior.

## Dependency Review

### Obsolete libvirt XML module

`go.mod` uses `github.com/libvirt/libvirt-go-xml v7.4.0+incompatible`. This obsolete module was replaced by the API-compatible `libvirt.org/go/libvirtxml` module.

Migrate the import path and dependency, then run the complete test and static-analysis suite. This is a maintenance and supportability recommendation rather than the cause of the lifecycle bugs above.

### Available dependency updates

`go list -m -u all` reported updates including:

- `golang.org/x/crypto`: `v0.51.0` to `v0.54.0`
- `golang.org/x/net`: `v0.55.0` to `v0.57.0`
- `golang.org/x/sync`: `v0.20.0` to `v0.22.0`
- `golang.org/x/sys`: `v0.45.0` to `v0.47.0`
- `github.com/klauspost/compress`: `v1.18.5` to `v1.19.2`
- `github.com/pierrec/lz4/v4`: `v4.1.26` to `v4.1.28`

Refresh these through supported direct dependencies, then rerun tests, the race detector, linting, and vulnerability analysis.

### CI enforcement

No `.github/workflows` files exist. Local pre-commit hooks run tests, linting, repository policy checks, and secret scanning, but pull requests and pushes do not enforce these checks remotely.

Add CI for tests, race detection, linting, module verification, vulnerability scanning, and builds of both host and guest binaries.

## Verification

The following checks passed:

- `go test -race ./...`
- `go vet ./...`
- `golangci-lint run`
- `go mod verify`

The worktree was clean before this report was added.

`govulncheck` was unavailable in the current environment, so vulnerability reachability was not independently verified.

Coverage results highlight risk in the lifecycle code:

- `cmd/shuttle`: 0.0%
- `internal/runner`: 0.0%
- `internal/health`: 0.0%
- `internal/config`: 0.0%
- `internal/libvirt`: 10.2%
- `internal/github`: 13.4%

No live libvirt lifecycle operation was performed because creation, destruction, health recovery, and storage operations are mutating and potentially destructive.

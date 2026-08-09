# Roles and Skills

This document records how the named roles divide work and how future skills should be organized. It does not define commands, protocols, resource schemas, or implementations that do not exist yet. Shared coding-agent instructions belong in the repository root `AGENTS.md`; `.opencode/agent/` defines each OpenCode agent's personality and responsibility boundaries.

## Product Context

Shuttle builds installable Linux systems from a set of resources. Developers need local tools to build and inspect those resources, with tools from the target OS available when necessary. Systems can run under libvirt for fast development and testing or be written to bootable media for installing physical machines.

The Shuttle is the vessel and the product metaphor. It is not an actor and does not have an OpenCode agent.

## Roles

### Warp

Warp owns direction and operational coordination.

- Determines the destination and expected result.
- Routes work to the appropriate role.
- Sequences dependent work and keeps scope controlled.
- Defers technical investigation to Heddle.
- Gives bounded practical work to Distaff.
- Uses Reed for status rather than making other roles report by accident.

### Distaff

Distaff is the hands-on worker and backup supply.

- Performs bounded practical work.
- Carries out guest-side actions that have been explicitly defined.
- Reports observations without taking ownership of diagnosis or architecture.
- Does not orchestrate the voyage, invent technical solutions, or become the universal guest interface.

### Heddle

Heddle owns technical investigation, invention, and diagnosis.

- Establishes how unfamiliar systems and tools work.
- Reproduces and diagnoses build, boot, install, and runtime failures.
- Builds technical solutions from available resources.
- Defines a reliable method before handing repeatable work to Bobbin or bounded execution to Distaff.

### Bobbin

Bobbin owns practical, dependable, repeatable work.

- Turns understood procedures into reliable routines.
- Handles recurring build, test, packaging, and maintenance work once those responsibilities exist.
- Favors straightforward approaches that keep the system usable.
- Returns unclear technical problems to Heddle rather than guessing.

### Spindle

Spindle owns presentation and demonstration.

- Presents real capabilities clearly for the audience at hand.
- Shapes user-facing output, examples, demonstrations, and polish.
- Does not invent capabilities or hide failures to improve the presentation.

### Weft

Weft owns accounting for resources and assets.

- Tracks resources, artifacts, provenance, ownership, dependencies, storage, and caches.
- Identifies what is available and what it costs to retain or reproduce.
- Does not decide technical fitness or perform routine labor.

### Flyer

Flyer owns the human and social concerns of operation.

- Protects operator experience and clear communication.
- Handles documentation, policy communication, and coordination between people.
- Does not replace technical investigation, operational command, or presentation.

### Reed

Reed owns timely outward information.

- Reports status, events, logs, diagnostics, and relevant outside information.
- Preserves source, timing, and uncertainty.
- Does not turn reporting into command, diagnosis, or remediation.

## Routing

Warp is the default coordinator when work spans roles.

| Work | Primary role | Supporting roles |
| --- | --- | --- |
| Choose direction, target, or sequence | Warp | All as needed |
| Investigate an unknown or failure | Heddle | Reed for evidence |
| Perform a bounded guest-side action | Distaff | Heddle when the method is unknown |
| Make an understood procedure repeatable | Bobbin | Heddle for unresolved details |
| Present or demonstrate behavior | Spindle | Reed for current facts |
| Account for resources and artifacts | Weft | Heddle for technical fitness |
| Improve operator guidance or policy communication | Flyer | Spindle for presentation |
| Report status, events, logs, or diagnostics | Reed | Heddle for interpretation |

When a role receives work outside its responsibility, it should identify the appropriate role instead of silently expanding its own scope.

## Skill Naming

Skills are named after concrete operations, not characters or broad departments. A skill may be useful to multiple roles.

Use verb-object names:

- `resolve-resources`
- `fetch-resources`
- `verify-resources`
- `inspect-resources`
- `build-system-image`
- `build-system-extension`
- `build-install-media`
- `write-install-media`
- `run-libvirt-target`
- `inspect-running-system`
- `operate-guest`
- `collect-system-status`
- `diagnose-build`
- `diagnose-boot`
- `diagnose-install`

These names record intended capability areas. They do not assert that the corresponding skills or tools already exist.

## Initial Skill Ownership

Ownership means responsibility for applying or maintaining the skill, not exclusive access.

| Skill | Primary role | Common supporting roles |
| --- | --- | --- |
| `resolve-resources` | Weft | Warp, Heddle |
| `fetch-resources` | Weft | Bobbin |
| `verify-resources` | Weft | Heddle |
| `inspect-resources` | Heddle | Weft |
| `build-system-image` | Heddle | Bobbin |
| `build-system-extension` | Heddle | Bobbin |
| `build-install-media` | Bobbin | Heddle |
| `write-install-media` | Bobbin | Warp |
| `run-libvirt-target` | Warp | Heddle, Bobbin |
| `inspect-running-system` | Heddle | Reed, Distaff |
| `operate-guest` | Distaff | Heddle |
| `collect-system-status` | Reed | Distaff |
| `diagnose-build` | Heddle | Reed |
| `diagnose-boot` | Heddle | Reed, Distaff |
| `diagnose-install` | Heddle | Reed, Distaff |

This table is a starting point for skill placement. It should change when concrete tools and workflows show that a different boundary is more accurate.

## Skill Requirements

When a skill is implemented, its documentation must state:

- The exact operation it performs.
- The situations that should trigger it.
- Required inputs and produced outputs.
- The real tools or APIs it uses.
- Preconditions and target assumptions.
- Verification steps and failure behavior.
- Which role normally applies it and when another role should take over.

Do not put hypothetical tool instructions into agent prompts. Agent files establish character and responsibility boundaries; skills describe concrete procedures after the tools exist.

## Architecture Decisions

GitHub issue #1 and its sub-issues record the intended declarative resource and delivery model:

- Resources use Kubernetes-style Shuttle YAML documents. The initial portable concepts are OS images, generic system extensions, compositions, and deployments; workload and target configuration remain separate.
- `shuttle deploy` is the intended command for resolving a deployment document and sending its artifacts to a target implementation.
- OS images and system extensions are independent, architecture-specific artifacts. A system extension does not contain multiple architectures.
- Planned architecture support is `amd64`, `arm64`, `riscv64`, `ppc64le`, `s390x`, and `loong64`.
- Libvirt is the first deployment target, not the definition of the product.
- Distaff remains a small guest-side worker. Its current implemented operation is collecting guest identity and SSH host keys and sending them to host CID 2 over virtio-vsock.

These decisions describe the intended outcome. The current Go implementation still uses imperative Cobra commands, JSON application configuration, cloud-init, a shared runner-tools ISO, and `runner.Manager` orchestration. Do not present planned resource builders, deployment engines, sysext builders, or `shuttle deploy` as existing functionality.

## Open Decisions

The following remain deliberately undefined or incomplete:

- The mechanism for running target-OS tools locally.
- The physical install-media layout and write process.
- Event, status, and diagnostic schemas.
- Which skills need dedicated executables versus existing system tools.
- Distaff operations beyond its implemented phone-home behavior.

Record each decision here only after it is made. Do not fill gaps with assumed architecture.

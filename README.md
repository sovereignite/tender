# Shuttle

A loom is only as good as its parts and the thread running through them.

Shuttle builds installable Linux systems from composable resources and delivers
them to a destination — bare metal, cloud, libvirt, a rack of Raspberry Pis.
It doesn't matter where. The shuttle carries the weft where it needs to go,
and each part of the loom knows its job.

## The Idea

You have a destination. You have resources — OS images, system extensions,
workloads, configurations. You need them composed, built, and delivered as a
complete system. One artifact per architecture, in the right size, for the
right place, at the right time.

That's what Shuttle does. It doesn't care what you're building. It cares that
it gets there.

## The Parts

Every part has a job. No one does everything. Everyone does something.

| Name | Role |
|------|------|
| **Warp** | Host CLI — `shuttle`. Sets the course. The backbone everything threads through. |
| **Distaff** | Guest agent — phones home from inside the system. Reports readiness. Intentionally small, intentionally simple. |
| **Heddle** | Builds system extensions from upstream sources. Lifts and separates, shapes the output. |
| **Bobbin** | Deployment targets — practical, dependable, holds what's needed. Libvirt is first because it's what we have. |
| **Spindle** | Discovers and caches cloud images and base OS artifacts. Draws raw material from outside. |
| **Weft** | Composes resources. Owns the accounting. The thread that ties everything together. |
| **Flyer** | Keeps compositions coherent. Checks tension, catches flaws, makes sure nothing's wasted. |
| **Reed** | Status, health checks, ready signals from the outside world. Beats the weft into place. |

## The Design

No one part is the loom. The loom is all the parts working together.

Warp doesn't build extensions. Heddle doesn't throw the shuttle.
Distaff doesn't tell anyone what to do — he just reports what he sees. Each
part is bounded, focused, and good at one thing.

When you add a new capability, you add a new part to the loom. You don't make
existing ones do more. The frame stays the same size. The parts grow.

## License

GPLv2 — see [LICENSE.md](LICENSE.md).

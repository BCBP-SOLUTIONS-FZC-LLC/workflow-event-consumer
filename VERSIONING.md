# Versioning and releases

`workflow-events-consumer` is a deployable service, not a library. A release is a Git tag `vMAJOR.MINOR.PATCH` (pre-releases `-rc.N`) that triggers the `Release` workflow, which builds, scans and signs the container image. Each release is described in [CHANGELOG.md](./CHANGELOG.md).

## Semantic versioning

| Bump | When |
| --- | --- |
| MAJOR | A breaking change to a public contract: the topology file format, the forwarded request shape (`POST` of the unmodified envelope with `x-internal-token`), or the routing of an event type to a target |
| MINOR | A backward-compatible capability: a new endpoint, an optional field, a new event type |
| PATCH | A backward-compatible fix, a dependency bump, or a docs-only correction |

## What is public

| In scope | Out of scope |
| --- | --- |
| Environment variables and their defaults | Prometheus metric names beyond the platform standard |
| The topology file format and the routing catalogue | Anything under `internal/` |
| The forwarded request and the status codes it treats as success | Container internals |

## Cutting a release

1. Move `[Unreleased]` in `CHANGELOG.md` to a `## [X.Y.Z] - YYYY-MM-DD` heading and merge to `main`.
2. Tag the merged commit: `git tag vX.Y.Z && git push origin vX.Y.Z`.
3. The `Release` workflow refuses a tag that has no CHANGELOG entry.

Tags are never cut inside a pull request.

# ADR 004: One repository with an apps folder

- Status: Accepted
- Date: 2026-11-16
- Scope: How the code of the CLI, the server, and the web UI is organized
- Related: [ADR 005](005-separate-cli-and-server.md), [ADR 008](008-web-ui-served-by-server.md)

## Context

DevLog gains a server and a web UI beside the existing CLI. They could live in
separate repositories, one per application, or together.

## Decision

One repository, with one folder per application under `apps/`:

```
apps/cli      Go module
apps/server   Go module
apps/web      Vite + React app
docs/         documentation for all of them
```

Each app has its own build files, README, and CI job. Documentation that
spans apps lives in `docs/`, with decisions in `docs/decisions/`.

All apps share one version and one `v*` tag, so a release ships a CLI and a
server that speak the same API.

## Consequences

- A change to an endpoint and the screen or command that uses it lands in one
  pull request, with no coordinated releases between repositories.
- CI runs each app's job only when its folder changes, once there is more than
  one app.
- Separate repositories would only pay off with separate owners or release
  cycles, which a one-person project does not have.

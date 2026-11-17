# ADR 005: Separate CLI and server

- Status: Proposed
- Date: 2026-11-16
- Scope: Whether the server and the CLI are one program or two
- Related: [ADR 004](004-monorepo-with-apps.md), [ADR 006](006-docker-compose-deployment.md), [API](../api.md)

## Context

The first design had one `devlog` binary in two roles: `devlog serve` on the
server and every other command as its client. Two later decisions changed the
trade-off: the server runs from a Docker image ([ADR 006](006-docker-compose-deployment.md)),
and collection runs only on the user's machines
([ADR 007](007-collection-on-devices.md)).

## Decision

Two programs in two Go modules:

- `apps/server` builds the server image. It owns the database, the API, MCP,
  and serving the web UI. It contains no collectors.
- `apps/cli` builds the `devlog` command for the user's machines. It owns the
  collectors and talks to the server only through the HTTP API.

The API, documented in [docs/api.md](../api.md), is the contract between
them. The activity payload is defined on both sides rather than in a shared
module.

## Consequences

- The server image carries no Git, collector, or scheduler code, and the CLI
  carries no database driver.
- The activity payload is duplicated in two small structs; a mismatch shows up
  as a rejected request, and both ship at the same version
  ([ADR 004](004-monorepo-with-apps.md)).
- Running DevLog without a droplet means running the server container locally.
- If more code ends up shared, a Go workspace with a shared module is the next
  step.

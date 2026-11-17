# ADR 007: Collection runs on the user's devices

- Status: Accepted
- Date: 2026-11-16
- Scope: Where activity is collected and how it reaches the server
- Related: [features F5 and F6](../features.md), [ADR 005](005-separate-cli-and-server.md)

## Context

v0.7 collects in two places: the work machines read Git, Claude Code, and
opencode, and an always-on machine reads GitHub. Moving the GitHub collector
into the server looked simpler, but personal GitHub tokens often cannot read
private repositories of company organizations, so in practice the local
collectors are the ones that see the work. They also give the data that
matters most for time: the intervals of coding agent sessions, where GitHub
only gives points in time.

## Decision

Collection runs only in the CLI, on the machines where the user enables it.
`devlog collect` posts each batch to `POST /api/activity`, and the server
merges it by source with the v0.7 rules. The `github` source stays available
in the CLI, for repositories the token can read.

Each run collects from the last time its device covered, not only the current
day, so a machine that was off or could not reach the server catches up.

The server never collects by itself.

## Consequences

- Without the CLI installed, DevLog works with manual time entries and no
  suggestions.
- The company machine, which cannot reach the server, contributes only what
  the `github` source can read.
- The server needs no GitHub token, Git, or access to any repository.

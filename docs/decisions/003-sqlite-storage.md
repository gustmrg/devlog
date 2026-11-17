# ADR 003: SQLite storage

- Status: Accepted
- Date: 2026-11-16
- Scope: Where the server keeps its data, and the end of Git synchronization
- Related: [data model](../data-model.md), [ADR 001](001-personal-time-tracker.md)

## Context

v0.7 stores one JSON file per entry and synchronizes devices through a private
Git repository. That works without a server because each device writes only
its own files. A time tracker changes the picture: the user edits the same
time entries from several devices, reports aggregate and join across projects
and tickets, and renaming or archiving a project must reach its entries. With
a server as the single writer, the reasons for files and Git no longer apply.

## Decision

The server keeps everything in one SQLite database: projects, time entries,
collected activity, collector runs, and sessions.

- Driver: `modernc.org/sqlite`, written in Go, so builds stay `CGO_ENABLED=0`
  for every platform.
- Access: `database/sql` with hand-written SQL; no ORM.
- Migrations: numbered `.sql` files embedded in the binary, applied at start
  and tracked with `PRAGMA user_version`.
- WAL mode, one connection pool, foreign keys on.

The JSON store, the Git synchronization (`devlog remote`, `devlog sync`), and
the summaries are removed in phase 4.

## Consequences

- One file to back up. The server writes consistent copies with `VACUUM INTO`.
- No database service to run, unlike PostgreSQL, which would only pay off with
  several users or an existing shared instance.
- Clients cannot work offline against the server; the collector catches up on
  its next run instead.
- `modernc.org/sqlite` must build for every release target; this is checked
  in phase 1.

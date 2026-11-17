# Roadmap

Each phase is one or more pull requests that leave `main` working. Phases 1 to
3 build the server and the UI beside the current CLI; phase 4 moves collection
to the server and removes the v0.7 storage. v1.0.0 breaks compatibility with
v0.7 ([ADR 001](decisions/001-personal-time-tracker.md)).

| Phase | Delivers | Status |
| --- | --- | --- |
| 0. Repository layout | `apps/` folders, documentation, decisions | In progress |
| 1. Server core | An API you can call with `curl`, running in Docker | Planned |
| 2. Web UI | Daily use from the browser, deployed on the droplet | Planned |
| 3. Reports | Week and report pages, CSV export | Planned |
| 4. Collection on the server | The CLI as a client; v0.7 storage removed | Planned |
| 5. Suggestions | Proposed time entries on the Day page | Planned |
| 6. Release | v1.0.0 | Planned |

## 0. Repository layout

- Move the CLI into `apps/cli`; adapt CI and GoReleaser.
- Write the architecture, features, roadmap, and decision records.

## 1. Server core

- `apps/server` Go module with `net/http`, SQLite (`modernc.org/sqlite`), and
  embedded migrations.
- Token authentication and browser sessions.
- Endpoints for projects, time entries, and the timer.
- Dockerfile and a Compose file with the Tailscale container; the image is
  published to Docker Hub on release.
- Scheduled database backup.
- CI job for the server.

## 2. Web UI

- `apps/web` with Vite, React, TypeScript, Zod, shadcn/ui, and React Router.
- The server image builds the UI and serves it from the same origin.
- Sign-in, Day page with the timer, and Projects page.
- Deploy on the droplet. From here DevLog is usable every day.
- CI job for the web app.

## 3. Reports

- Week grid against the daily target.
- Reports page grouped by project, ticket, client, or day; CSV export.
- Hours against each project's estimate.

## 4. Collection on the server

- Activity tables and `POST /api/activity` with the v0.7 merge rules.
- `devlog login`; `devlog collect` posts to the server and catches up from the
  last covered time instead of only today.
- `devlog start`, `stop`, and `status`.
- MCP moves to the server's `/mcp`.
- Devices page.
- One-time `devlog import` of v0.7 data, if it is worth keeping.
- Remove the JSON store, Git synchronization, summaries, and the other v0.7
  commands.

## 5. Suggestions

- Collectors record the active intervals of sessions instead of only the first
  and last time of the day.
- The suggestion algorithm ([F6](features.md#f6-suggestions)) and
  `GET /api/suggestions`.
- Accept and adjust suggestions on the Day page; MCP tools to suggest and save
  time entries.
- Optional LLM-written descriptions.

This phase is a heuristic to tune against real days, so it carries the most
risk.

## 6. Release

- `devlog setup` for the new configuration (server URL, token, sources).
- Deployment guide for the droplet.
- Release notes and v1.0.0.

## Later

Considered and left out of v1: time entry, project, and report commands in
the CLI (the web UI covers them), access from outside the tailnet with
password sign-in, and per-device tokens.

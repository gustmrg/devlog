# ADR 008: The server serves the web UI

- Status: Proposed
- Date: 2026-11-16
- Scope: How the web UI is built, delivered, and talks to the API
- Related: [ADR 002](002-private-access-over-tailscale.md), [ADR 004](004-monorepo-with-apps.md)

## Context

The web UI could run as its own container (a static file server) next to the
API, or be served by the API itself.

## Decision

The server image builds `apps/web` in a Node stage and copies the static
files into the Go stage, where the server serves them at `/` and the API at
`/api`, from one origin.

Stack: Vite, React, TypeScript, Zod, shadcn/ui, and React Router. API calls go
through a small `fetch` wrapper that validates responses with Zod; there is no
global state library until a screen needs one.

In development, the Vite dev server proxies `/api` to a local server.

## Consequences

- One origin means no CORS and lets the session cookie be `SameSite=Strict`.
- One image and one container to deploy, and the UI always matches the API
  version.
- The UI cannot be deployed without a server release; with one user and one
  version for all apps, that is not a constraint.

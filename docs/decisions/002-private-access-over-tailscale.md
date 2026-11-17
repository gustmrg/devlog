# ADR 002: Private access over Tailscale

- Status: Accepted
- Date: 2026-11-16
- Scope: Who can reach the server and how clients authenticate
- Related: [ADR 006](006-docker-compose-deployment.md), [architecture](../architecture.md#network-boundary)

## Context

The server holds commit messages, branch names, and pull request titles from
work repositories. It runs on a DigitalOcean droplet that also serves public
applications through a reverse proxy. The user's devices are already on a
Tailscale network, except the company machine, which cannot join it.

## Decision

The server is reachable only on the tailnet. It publishes no port on the
host; Tailscale serves it over HTTPS, and the public reverse proxy has no
route to it.

Inside the tailnet, one random token authenticates every client:

- The CLI, the collector, and MCP clients send `Authorization: Bearer <token>`.
- The browser signs in by entering the token once and receives an `HttpOnly`,
  `SameSite=Strict` session cookie. Changes made with the cookie must come
  from the server's own origin.

## Consequences

- The company machine is not a client: it cannot open the UI or send
  activity. Its time is logged from another device.
- A leaked token gives full access until it is replaced; there is no
  per-device revocation.
- Opening the server to the internet needs a new decision with password
  sign-in, per-device tokens, and rate limiting. Keeping authentication in the
  application, rather than relying only on the network, keeps that change
  small.

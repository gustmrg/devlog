# ADR 006: Docker Compose deployment

- Status: Accepted
- Date: 2026-11-16
- Scope: How the server is packaged, published, and run on the droplet
- Related: [ADR 002](002-private-access-over-tailscale.md), [ADR 003](003-sqlite-storage.md)

## Context

The server runs on a DigitalOcean droplet that already hosts other
applications with Docker. The Raspberry Pi that runs the v0.7 MCP service is
sometimes unavailable and stops serving DevLog.

## Decision

The server ships as a Docker image published to Docker Hub with each release.
On the droplet, Docker Compose runs two containers:

- `tailscale`, which joins the tailnet and serves HTTPS with
  `tailscale serve`.
- `devlog`, which uses the Tailscale container's network
  (`network_mode: service:tailscale`) and keeps the database on a volume.

Neither publishes a port on the host. A sketch, to be completed in phase 1:

```yaml
services:
  tailscale:
    image: tailscale/tailscale
    hostname: devlog
    environment:
      - TS_AUTHKEY=${TS_AUTHKEY}
      - TS_STATE_DIR=/var/lib/tailscale
      - TS_SERVE_CONFIG=/config/serve.json
    volumes:
      - tailscale-state:/var/lib/tailscale
      - ./serve.json:/config/serve.json:ro
  devlog:
    image: <docker-hub-user>/devlog-server
    network_mode: service:tailscale
    volumes:
      - devlog-data:/data
    env_file: .env
```

## Consequences

- The server is unreachable from the internet even if the public proxy is
  misconfigured, because no port exists on the host.
- The CLI keeps its own GoReleaser archives and installers; only the server
  goes through Docker.
- Backups write to the data volume, and copying them off the droplet is a host
  job.
- Image tags and the Docker Hub account are set up in phase 1.

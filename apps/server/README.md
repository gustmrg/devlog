# DevLog server

Planned. The server stores projects, time entries, and collected activity in
SQLite, and serves the JSON API, the MCP endpoint, and the [web UI](../web/).
It runs with Docker Compose on a private host reachable only over Tailscale.

- [Architecture](../../docs/architecture.md)
- [API](../../docs/api.md)
- [Data model](../../docs/data-model.md)
- [Deployment decision](../../docs/decisions/006-docker-compose-deployment.md)

# Development

## Local checks

```bash
go test -race ./...
go vet ./...
go build ./...
```

Run the server with disposable data:

```bash
export DEVLOG_ADMIN_PASSWORD=development-only
export DEVLOG_PAIRING_CODE=DEV-CODE
export DEVLOG_PUBLIC_URL=http://127.0.0.1:8080
go run . serve --data-dir /tmp/devlog-server --listen 127.0.0.1:8080 --allow-insecure
```

Pair a client with `--allow-insecure`. Keep fixtures free of real tokens, repository content, and personal data.

SQLite schema changes must be additive migrations, covered by both empty-database and upgrade tests. Collector changes require contract tests. HTTP handlers should use temporary databases and fake external services.

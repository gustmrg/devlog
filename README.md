# DevLog

DevLog reconstructs a developer's day from local Git and remote collaboration signals, keeps a reviewable timeline, and produces evidence-backed daily summaries. It remains useful as a standalone CLI and can also synchronize multiple devices through a self-hosted server.

## How it works

```text
macOS / Linux devices                    Self-hosted server
┌────────────────────┐                  ┌─────────────────────────────┐
│ devlog             │                  │ devlog serve                │
│ ├─ CLI entries     │── event sync ───▶│ ├─ API + scheduler          │
│ ├─ Git collector   │                  │ ├─ SQLite                    │
│ ├─ offline queue   │                  │ ├─ correlation + summaries  │
│ └─ local agent     │                  │ ├─ review web UI             │
└────────────────────┘                  │ └─ GitHub + Discord adapters │
                                        └─────────────────────────────┘
```

Collectors produce immutable facts. Deterministic correlation turns those facts into draft activities with evidence and confidence. The LLM only polishes the final wording; it never invents projects, timestamps, commits, or verification.

## Project organization

```text
devlog
├── cmd/                  CLI, agent, sync, migration, and serve commands
├── internal/
│   ├── collector/        extensible local and remote source adapters
│   ├── correlation/      event-to-activity reconstruction
│   ├── database/         SQLite schema and repositories
│   ├── server/           API, scheduler, and web handlers
│   ├── summary/          deterministic and OpenAI-compatible summaries
│   ├── syncapi/          device pairing and event synchronization
│   └── notify/           output adapters such as Discord
├── web/                  embedded server-rendered UI
├── deploy/               Raspberry Pi Docker Compose setup
└── docs/                 setup, architecture, configuration, and operations
```

## Quick start

### Standalone CLI

```bash
devlog init
devlog add "Fixed pagination on the transactions list" -p bitfinance -t frontend
devlog list
devlog summary create
```

### Raspberry Pi server

```bash
cd deploy
cp .env.example .env
cp config.example.json config.json
# Edit .env and config.json before starting.
docker compose up -d
docker compose logs devlog
```

Open the private HTTPS URL configured in `DEVLOG_PUBLIC_URL`, sign in, and copy the pairing code. On each client:

```bash
devlog connect https://devlog.example.private --pairing-code CODE
devlog migrate legacy --dry-run
devlog migrate legacy
devlog source discover ~/Dev
devlog source enable ~/Dev/my-project --project-id my-project
devlog agent install
devlog agent status
```

The server command is `devlog serve`. There is no separate worker, web, or bot deployment: all central components run in that process.

## Main commands

```text
devlog add / list / summary       manual and local workflows
devlog connect / sync / status    central server connection
devlog source ...                 repository discovery and enablement
devlog agent ...                  background collector management
devlog migrate legacy             assisted JSON/Markdown import
devlog serve                      central API, jobs, web, and integrations
devlog version / update           release management
```

## Documentation

- [Raspberry Pi server setup](docs/setup/raspberry-pi.md)
- [macOS and Linux client setup](docs/setup/client.md)
- [Architecture and data flow](docs/architecture.md)
- [Configuration reference](docs/configuration.md)
- [Collector adapter contract](docs/adapters.md)
- [Backups, upgrades, and troubleshooting](docs/operations.md)
- [Development guide](docs/development.md)

## Privacy defaults

Local Git collection sends branch names, commit metadata, changed paths, and statistics. It does not send file contents or diffs. Secrets stay in environment variables or the local credentials file, and every device receives a revocable token.

## Requirements

- Go version declared in `go.mod` for source builds
- Git available on client devices
- macOS or Linux for background agent installation
- Docker Compose for the primary server setup
- A private network and HTTPS hostname for synchronization

## License

MIT

# DevLog

![Latest Release](https://img.shields.io/github/v/release/gustmrg/devlog)
![License](https://img.shields.io/github/license/gustmrg/devlog)

A self-hosted personal time tracker for developers. Log your hours from the
browser or the terminal, see them by project and ticket, and optionally let
DevLog collect your Git and coding-agent activity to suggest time entries.

> DevLog is moving from a command-line activity log (v0.7) to the time tracker
> described in the [docs](docs/). Only the CLI is released today; see the
> [roadmap](docs/roadmap.md).

## Apps

| App | What it is | Status |
| --- | --- | --- |
| [`apps/cli`](apps/cli/) | Command-line client and activity collector for your machines | Released (v0.7) |
| [`apps/server`](apps/server/) | API, MCP server, and web UI host, deployed with Docker | Planned |
| [`apps/web`](apps/web/) | React web UI | Planned |

## Documentation

- [Architecture](docs/architecture.md)
- [Features](docs/features.md)
- [Roadmap](docs/roadmap.md)
- [Architecture decisions](docs/decisions/)
- [CLI reference](docs/cli/commands.md)

## License

[MIT](LICENSE)

# DevLog

![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![Latest Release](https://img.shields.io/github/v/release/gustmrg/devlog)
![License](https://img.shields.io/github/license/gustmrg/devlog)

A small command-line tool for recording daily development work and turning it
into useful Markdown summaries.

DevLog works directly from the terminal and can also be called by coding agents
and automation scripts.

## Installation

Install the latest release on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/gustmrg/devlog/main/scripts/install.sh | sh
```

The installer detects your platform, verifies the release checksum, and places
`devlog` in `/usr/local/bin`. Windows builds are available on the
[releases page](https://github.com/gustmrg/devlog/releases).

Then create the local configuration:

```bash
devlog init
```

Running `devlog init` again is safe and does not overwrite existing settings.

## Quick start

```bash
# Record some work
devlog add "Fixed pagination on the transactions page" -p bitfinance -t frontend

# Review today's entries
devlog list

# Create and display a Markdown summary
devlog summary create
devlog summary show
```

Use `--date YYYY-MM-DD` with these commands to work with another day.

## Features

- Dated activity entries organized by project and tags
- Markdown summaries with optional AI-generated writing styles
- GitHub activity import for commits, pull requests, and reviews
- Daily automatic sync on macOS and Linux
- Multi-device synchronization through a private Git repository
- Local, conflict-resistant storage that remains easy to inspect
- Self-updates with release checksum verification

## Documentation

- [Command reference](docs/commands.md)
- [Configuration and integrations](docs/configuration.md)

Run `devlog --help` or `devlog <command> --help` for help in the terminal.

## Data and privacy

DevLog keeps its configuration and data under `~/.devlog/` by default. API keys
are read from environment variables and are never stored in the configuration
file. If you enable multi-device sync, use a private repository because entries
may contain sensitive work information.

## License

[MIT](LICENSE)

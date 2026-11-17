# DevLog CLI

The `devlog` command. Today (v0.7) it records your daily development work on
your machine and turns it into Markdown summaries. In v1 it becomes a client of
the [DevLog server](../server/): it collects activity on your machines and
sends it to the server, and starts and stops the timer. See the
[roadmap](../../docs/roadmap.md).

## Installation

On macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/gustmrg/devlog/main/apps/cli/scripts/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://raw.githubusercontent.com/gustmrg/devlog/main/apps/cli/scripts/install.ps1 | iex
```

The installers verify the release checksum. Then configure the device, one
question at a time:

```bash
devlog setup
```

## Quick start

```bash
devlog add "Fixed pagination on the transactions page" -p bitfinance -t frontend
devlog list
devlog collect --dry-run
devlog summary create
```

## Development

Requires Go 1.26 or later.

```bash
cd apps/cli
go build ./...
go test ./...
```

Releases are built by GoReleaser from this folder when a `v*` tag is pushed.

## Documentation

- [Command reference](../../docs/cli/commands.md)
- [Configuration and integrations](../../docs/cli/configuration.md)
- [Setting up your devices](../../docs/cli/devices.md)

## Data and privacy

Configuration and data live under `~/.devlog/`. API keys are read from
environment variables and never stored in the configuration file. Collected
entries include commit messages, branch and repository names, pull request
titles, and the titles of coding agent sessions; prompts and replies are never
stored.

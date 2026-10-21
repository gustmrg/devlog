# DevLog

![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![Latest Release](https://img.shields.io/github/v/release/gustmrg/devlog)
![License](https://img.shields.io/github/license/gustmrg/devlog)

A small command-line tool that keeps a record of your daily development work and
turns it into useful Markdown summaries. Write entries yourself, or let DevLog
collect them from Git, GitHub, and your Claude Code and opencode sessions.

DevLog works directly from the terminal, can be called by automation scripts,
and serves the collected activity to AI agents over MCP, so an agent can draft
timesheet or status descriptions from what you actually did.

## Installation

Install the latest release on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/gustmrg/devlog/main/scripts/install.sh | sh
```

The installer detects your platform, verifies the release checksum, and places
`devlog` in `/usr/local/bin`. On Windows, use the PowerShell installer; it
installs to `%LOCALAPPDATA%\Programs\devlog` and adds that folder to your user
`PATH`:

```powershell
irm https://raw.githubusercontent.com/gustmrg/devlog/main/scripts/install.ps1 | iex
```

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

To collect activity automatically, point DevLog at your repositories:

```bash
devlog config set collect.git.roots ~/dev
devlog config set collect.git.authorEmails you@example.com

# Preview what today's collection finds, then store it
devlog collect --dry-run
devlog collect

# Collect on weekdays at the end of each work block
devlog collect schedule --at 12:15,18:15
```

Running a collection again is safe: stored entries are completed, not duplicated.
See [Setting up your devices](docs/devices.md) to gather several machines in one
repository and serve the result to an agent with `devlog mcp`.

## Features

- Dated activity entries organized by project and tags
- Markdown summaries with optional AI-generated writing styles
- Automatic activity collection from Git, GitHub, Claude Code, and opencode
- Scheduled collection on macOS, Linux, and Windows
- An MCP server that lets agents read the day's activity
- Multi-device synchronization through a private Git repository
- Local, conflict-resistant storage that remains easy to inspect
- Self-updates with release checksum verification

## Documentation

- [Command reference](docs/commands.md)
- [Configuration and integrations](docs/configuration.md)
- [Setting up your devices](docs/devices.md)

Run `devlog --help` or `devlog <command> --help` for help in the terminal.

## Data and privacy

DevLog keeps its configuration and data under `~/.devlog/` by default. API keys
are read from environment variables and are never stored in the configuration
file. If you enable multi-device sync, use a private repository because entries
may contain sensitive work information.

Collected entries include commit messages, branch and repository names, pull
request titles, and the titles of coding agent sessions. Sessions are read for
their times, places, and titles only; prompts and replies are never stored. The
MCP server's bearer token is kept in `~/.devlog/mcp-token`, readable only by
you.

## License

[MIT](LICENSE)

# Command reference

This page covers DevLog's commands and commonly used options. Run
`devlog <command> --help` for the authoritative help available in your installed
version.

## Entries

### `devlog add <description>`

Record an activity. The explicit form `devlog entry add` is equivalent.

| Option | Description |
| --- | --- |
| `-p, --project <name>` | Set the project. |
| `-t, --tags <list>` | Add comma-separated tags. |
| `--date <YYYY-MM-DD>` | Record the entry on another date. |

```bash
devlog add "Implemented token rotation" -p echo -t backend,auth
```

### `devlog list`

Display entries. The explicit form `devlog entry list` is equivalent.

| Option | Description |
| --- | --- |
| `--date <YYYY-MM-DD>` | Show a specific date. |
| `-w, --week` | Show the current week. |
| `-p, --project <name>` | Filter by project. |
| `--tag <name>` | Filter by tag. |
| `--json` | Print the day's entries as JSON, including collected activity. |

## Summaries

### `devlog summary create`

Create a Markdown summary from recorded entries.

| Option | Description |
| --- | --- |
| `--date <YYYY-MM-DD>` | Summarize a specific date. |
| `--ai` | Generate a polished summary with the configured LLM. |
| `-s, --style <style>` | Use `concise`, `detailed`, `formal`, or `impersonal` with `--ai`. |

### `devlog summary show`

Display a saved summary. Use `--date <YYYY-MM-DD>` to select another day.

### `devlog summary list`

List saved summaries. With no options, it shows the current week.

| Option | Description |
| --- | --- |
| `-w, --week` | Show the current week. |
| `-m, --month` | Show the current month. |
| `--from <YYYY-MM-DD>` | Set the start of a custom range. |
| `--to <YYYY-MM-DD>` | Set the end of a custom range. |

## Activity collection

### `devlog collect`

Collect development activity from this device and store it on the day it
happened. Entries keep their repository, branch, ticket keys, and time, and
their text is never rewritten. Running it again completes stored entries
instead of duplicating them.

| Source | What it records |
| --- | --- |
| `git` | Commits you authored on every branch, branch activity from the reflog, and uncommitted or unpushed work on the checked-out branch, for the repositories under `collect.git.roots`. |
| `github` | Commits you pushed to any branch, pull requests you opened or that were merged (with their head branch), reviews, comments, and created branches. |
| `claude-code` | Claude Code sessions: when they ran on each day, in which repository and branch, their title, and how many prompts they had. |
| `opencode` | The same for opencode sessions, listed through the `opencode` CLI for the repositories under `collect.git.roots`. |

Sessions are read for their times, places, and titles only; prompts and
replies are never stored.

| Option | Description |
| --- | --- |
| `--date <YYYY-MM-DD>` | Collect a specific day (defaults to today). |
| `--from <YYYY-MM-DD>`, `--to <YYYY-MM-DD>` | Collect a range of days. |
| `--sources <list>` | Comma-separated sources (defaults to `collect.sources`). |
| `--dry-run` | Print what would be collected without writing it. |
| `--remote` | Pull and push the configured data repository around the collection. |
| `--json` | Print the collection report as JSON. |

Each run also records, per device and source, when it last ran and which
window it covered, under `devices/` in the data directory.

```bash
devlog collect --dry-run
devlog collect --from 2026-08-17 --to 2026-08-21
```

### `devlog collect schedule`

Run `devlog collect` automatically, Monday to Friday unless `--every-day` is
set. macOS uses `launchd`, which runs a collection missed while the Mac slept
once it wakes; Linux uses the user crontab; Windows uses Task Scheduler, which
runs a missed collection as soon as it can.

| Option | Description |
| --- | --- |
| `--at <times>` | Comma-separated `HH:MM` times (default `12:15,18:15`). |
| `--every-day` | Run on weekends too. |
| `--sources <list>` | Sources to collect (defaults to `collect.sources` when the job runs). |
| `--remote` | Pull and push the configured data repository around each run. |
| `--env-file <path>` | Load tokens from a protected `KEY=VALUE` file. |

```bash
devlog collect schedule --at 12:15,18:15 --remote
devlog collect schedule show
devlog collect schedule remove
```

Output is appended to `~/.devlog/collect.log`.

## GitHub activity

### `devlog sync`

Import GitHub activity for a date. It is the same as
`devlog collect --sources github`, plus `--polish` to rewrite new descriptions
with the configured LLM. Commits pushed to any branch come from your event
feed, which GitHub keeps for 30 days; older dates still get pull requests,
reviews, and commits on default branches. Merge commits and commits GitHub
creates, such as squash merges, are skipped because the pull request already
stands for them. Imported PR entries include the PR body when it is not empty.
Already imported activity is skipped.

| Option | Description |
| --- | --- |
| `--date <YYYY-MM-DD>` | Import activity for a specific date. |
| `--dry-run` | Preview entries without writing them. |
| `--polish` | Rewrite descriptions with the configured LLM. |
| `--remote` | Pull and push the configured data repository around the import. |
| `--env-file <path>` | Load credentials from a protected `KEY=VALUE` file. |

See [Configuration and integrations](configuration.md#github) for authentication
setup.

### `devlog sync schedule`

Install a daily GitHub import using `launchd` on macOS, the user crontab on
Linux, or Task Scheduler on Windows.

```bash
devlog sync schedule --daily-at 23:55
devlog sync schedule show
devlog sync schedule remove
```

The schedule command also accepts `--polish`, `--remote`, and `--env-file`.

## MCP server

### `devlog mcp`

Serve collected activity to AI agents over the
[Model Context Protocol](https://modelcontextprotocol.io).

| Tool | Description |
| --- | --- |
| `get_activity` | Entries of a day or a range of up to 31 days, in time order with kind, repository, branch, ticket keys, and local times; the tickets seen each day; and when each device last collected. Read-only. |
| `add_entry` | Record work the user describes, such as a meeting, as a note at a given time. |

Without flags the server speaks MCP over stdin and stdout:

```json
{ "mcpServers": { "devlog": { "command": "devlog", "args": ["mcp"] } } }
```

With `--http` it serves Streamable HTTP at `/mcp`, and every request must send
`Authorization: Bearer <token>`. The token comes from `DEVLOG_MCP_TOKEN` or
from `~/.devlog/mcp-token`, created with a random token (mode `0600`) on first
use; `--token-file` names another file, which must exist.

```bash
devlog mcp --http 127.0.0.1:8766
```

When remote synchronization is configured, reads first pull the other devices'
activity, at most once a minute, and added notes are pushed.

### `devlog mcp install`

On Linux, install the HTTP server as the systemd user service
`devlog-mcp.service`, listening on `127.0.0.1:8766` unless `--http` says
otherwise. Run `loginctl enable-linger "$USER"` so it keeps running while you
are logged out, and remove it with `devlog mcp uninstall`, which keeps the
token file and data. See [Setting up your devices](devices.md) for exposing it
over HTTPS with Tailscale.

## Multi-device sync

DevLog can synchronize its data directory through a private Git repository.

```bash
devlog remote init git@github.com:you/devlog-data.git
devlog remote status
devlog remote sync
devlog remote remove
```

Use `devlog remote init --branch <name> <repository-url>` for a branch other than
`main`. Removing a remote disconnects it without deleting local or server data.

## Configuration

```bash
devlog config list
devlog config get <key>
devlog config set <key> <value>
```

See [Configuration and integrations](configuration.md) for available settings.

## Maintenance

| Command | Description |
| --- | --- |
| `devlog init` | Create the local data directory and default configuration. Safe to repeat. |
| `devlog migrate` | Migrate legacy daily logs to the current storage format. |
| `devlog version` | Print version and build information. Add `--check` to check for updates. |
| `devlog update` | Install the latest release. Add `-y` to skip confirmation. |

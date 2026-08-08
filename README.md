# DevLog

![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go)
![Latest Release](https://img.shields.io/github/v/release/gustmrg/devlog)
![License](https://img.shields.io/github/license/gustmrg/devlog)

A small command-line tool for developers to record daily work notes and generate simple Markdown summaries.

DevLog is intended to be useful both directly from the terminal and through coding-agent harnesses such as Pi, OpenCode, or similar tools. The CLI remains the source of truth; agent integrations can call the CLI to log work or create summaries.

---

## Current Features

- Initialize a local `~/.devlog/config.json`
- Add dated activity entries with project and tags
- List entries for a specific day
- Generate a basic Markdown summary for a specific day
- Show a previously generated summary
- List previously generated summaries with date-range filters
- Store all data locally under `~/.devlog/`
- Import GitHub activity (commits, authored PRs, PR reviews) as entries with `devlog sync`
- Generate AI-polished summaries (`--ai`, four style templates) and LLM-rewritten sync descriptions (`--polish`) via any OpenAI-compatible endpoint
- Report version/build info and self-update from GitHub releases

---

## Quick Start

```bash
# Initialize DevLog data/config
devlog init

# Log an activity
devlog add "Fixed pagination bug on transactions list" -p bitfinance -t frontend --date 2026-04-14

# View today's entries
devlog list

# View entries for a specific day
devlog list --date 2026-04-14

# Generate today's summary
devlog summary create

# Generate a summary for a specific day
devlog summary create --date 2026-04-14

# Show a saved summary
devlog summary show --date 2026-04-14
```

Example summary output:

```md
---
date: 2026-04-14
style: concise
projects: Echo, BitFinance
---
**Echo**
- Implemented JWT auth middleware
- Built refresh token rotation logic

**BitFinance**
- Fixed budget category filter bug
```

---

## Installation

### Download binary

Download the latest release for your platform from the [releases page](https://github.com/gustmrg/devlog/releases).

### Build from source

Prerequisite: Go compatible with the version declared in `go.mod`.

```bash
git clone https://github.com/gustmrg/devlog
cd devlog
go build -o bin/devlog .
```

Then move the binary somewhere in your `PATH`, for example:

```bash
mv bin/devlog /usr/local/bin/
```

Initialize DevLog:

```bash
devlog init
```

---

## Data Storage

DevLog stores data locally under:

```text
~/.devlog/
├── config.json
├── entries/
│   └── 2026-04-14.json
└── summaries/
    └── 2026-04-14.md
```

Entry files are JSON. Summary files are Markdown with YAML frontmatter.

---

## Commands

### `devlog init`

Creates the `~/.devlog/` directory and a default `config.json`.

```bash
devlog init
```

Current note: this command uses safe config creation and will not overwrite an existing config file.

---

### `devlog add`

Logs a new activity entry.

```bash
devlog add <description> [options]
```

Equivalent explicit form: `devlog entry add <description> [options]`.

Options currently implemented:

| Option | Short | Description |
|---|---:|---|
| `--project <name>` | `-p` | Project name. Uses `defaults.project` from config if omitted. |
| `--tags <list>` | `-t` | Comma-separated tags. |
| `--date <YYYY-MM-DD>` | | Override entry date. Defaults to today. |

Examples:

```bash
devlog add "Implemented refresh token rotation" -p echo -t backend,auth
devlog add "Reviewed checkout API" -p shop --date 2026-04-14
```

---

### `devlog sync`

Imports your GitHub activity for a date — commits, pull requests you authored, and pull requests you reviewed — as entries for that day. Private repositories are included as long as the token can access them.

```bash
devlog sync [options]
```

Options currently implemented:

| Option | Description |
|---|---|
| `--date <YYYY-MM-DD>` | Date to sync. Defaults to today. |
| `--dry-run` | Print what would be imported without writing entries. |
| `--polish` | Rewrite descriptions with an LLM before saving (requires `llm.enabled` in config). On LLM failure the raw descriptions are kept. |

Setup:

- Set `github.username` in `~/.devlog/config.json`.
- Put a GitHub token in the environment variable named by `github.tokenEnvVar` (default `GITHUB_TOKEN`). The token needs the `repo` scope (classic) or Contents read access (fine-grained); authorize it for SSO if your organization requires it.

Re-running `sync` for the same date skips already-imported items, so it is safe to schedule (e.g. a daily cron job).

Examples:

```bash
devlog sync
devlog sync --date 2026-08-05 --dry-run
```

---

### `devlog list`

Displays entries for today or for a specific date.

```bash
devlog list [options]
```

Equivalent explicit form: `devlog entry list [options]`.

Options currently implemented:

| Option | Description |
|---|---|
| `--date <YYYY-MM-DD>` | Show entries for a specific date. Defaults to today. |

Examples:

```bash
devlog list
devlog list --date 2026-04-14
```

---

### `devlog summary create`

Generates a basic Markdown summary from logged entries and saves it to `~/.devlog/summaries/`.

```bash
devlog summary create [options]
```

Options currently implemented:

| Option | Short | Description |
|---|---:|---|
| `--date <YYYY-MM-DD>` | | Summarize a specific date. Defaults to today. |
| `--ai` | | Use an LLM to produce a polished narrative (requires `llm.enabled` in config). |
| `--style <style>` | `-s` | Output style: concise, detailed, formal, impersonal. Only valid with `--ai`; defaults to `defaults.style` from config. |

Examples:

```bash
devlog summary create
devlog summary create --date 2026-04-14
devlog summary create --ai --style formal
```

---

### `devlog summary list`

Lists previously generated summaries. With no flags, shows summaries from the current week.

```bash
devlog summary list [options]
```

Options currently implemented:

| Option | Short | Description |
|---|---:|---|
| `--week` | `-w` | Show summaries from the current week (Monday–Sunday). This is the default when no flag is given. |
| `--month` | `-m` | Show summaries from the current month. |
| `--from <YYYY-MM-DD>` | | Start of date range. |
| `--to <YYYY-MM-DD>` | | End of date range. |

Examples:

```bash
devlog summary list
devlog summary list --week
devlog summary list --month
devlog summary list --from 2026-04-01 --to 2026-04-14
```

---

### `devlog summary show`

Displays a previously generated summary.

```bash
devlog summary show [options]
```

Options currently implemented:

| Option | Description |
|---|---|
| `--date <YYYY-MM-DD>` | Show summary for a specific date. Defaults to today. |

Examples:

```bash
devlog summary show
devlog summary show --date 2026-04-14
```

---

### `devlog version`

Prints the installed version, commit, and build date.

```bash
devlog version [options]
```

Options currently implemented:

| Option | Description |
|---|---|
| `--check` | Also query GitHub and report whether a newer release is available. |

Examples:

```bash
devlog version
devlog version --check
```

---

### `devlog update`

Checks GitHub for a newer release and, if found, downloads it, verifies its checksum, and replaces the running binary in place.

```bash
devlog update [options]
```

Options currently implemented:

| Option | Short | Description |
|---|---:|---|
| `--yes` | `-y` | Skip the confirmation prompt. |

Examples:

```bash
devlog update
devlog update --yes
```

---

## Configuration

Configuration is stored at:

```text
~/.devlog/config.json
```

Default shape:

```json
{
  "defaults": {
    "project": "default",
    "style": "concise",
    "language": "pt-BR"
  },
  "github": {
    "username": "",
    "tokenEnvVar": "GITHUB_TOKEN"
  },
  "llm": {
    "enabled": false,
    "provider": "openrouter",
    "model": "openai/gpt-oss-120b:free",
    "apiKeyEnvVar": "OPENROUTER_API_KEY"
  }
}
```

### LLM settings

The `llm` section powers `devlog summary create --ai` and `devlog sync --polish`:

| Key | Description |
|---|---|
| `enabled` | Must be `true` for LLM features to run. |
| `provider` | `openrouter` or `openai`. For any other OpenAI-compatible endpoint, set `baseURL` instead. |
| `baseURL` | Optional. Overrides the provider's API base URL (e.g. a local or self-hosted endpoint). |
| `model` | Model identifier passed to the API. |
| `apiKeyEnvVar` | Name of the environment variable holding the API key. The key itself is never stored in the config. |

The config command interface is planned but not fully implemented yet.

---

## Planned Features

The following features are planned or partially scaffolded, but should not be treated as stable yet.

### Entry Management

- `devlog add --duration <minutes>` / `-d <minutes>`
- `devlog add -i` interactive entry creation
- `devlog entry edit <id>`
- `devlog entry delete <id>`
- `devlog list --week`
- `devlog list --project <name>`
- `devlog list --tag <name>`

### Summaries

- `devlog summary create --week`
- `devlog summary create --format <template>`
- summary templates from `~/.devlog/templates/`

### Configuration

- `devlog config list`
- `devlog config get <key>`
- `devlog config set <key> <value>`

### AI-Enhanced Summaries

- output language based on config, e.g. `pt-BR` or `en-US`

---

## Built With

- [Go](https://go.dev/) — language
- [cobra](https://github.com/spf13/cobra) — CLI framework
- [viper](https://github.com/spf13/viper) — configuration
- [fatih/color](https://github.com/fatih/color) — terminal colors
- [google/uuid](https://github.com/google/uuid) — entry IDs
- [go-yaml](https://github.com/go-yaml/yaml) — summary frontmatter parsing
- [GoReleaser](https://goreleaser.com) — release automation

---

## License

MIT

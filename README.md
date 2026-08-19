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
- Store entries as conflict-resistant individual files under `~/.devlog/data/`
- Synchronize data across machines through a private Git repository
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
aiGenerated: false
generatedAt: 2026-04-14T18:20:00Z
deviceId: 62c47f36-08f8-4d43-9f48-a303481b18fc
---
**Echo**
- Implemented JWT auth middleware
- Built refresh token rotation logic

**BitFinance**
- Fixed budget category filter bug
```

---

## Installation

Install the latest release on macOS or Linux with:

```bash
curl -fsSL https://raw.githubusercontent.com/gustmrg/devlog/main/scripts/install.sh | sh
```

The script detects the operating system and CPU architecture, verifies the
release checksum, and installs `devlog` in `/usr/local/bin`. It has no options
or configuration variants. If necessary, it requests `sudo` only when copying
the binary.

Initialize DevLog after installation:

```bash
devlog init
```

Verify the installation:

```bash
devlog version
```

The installer requires `curl`, `tar`, `install`, and either `sha256sum` or
`shasum`. Windows users should download the release ZIP from the
[releases page](https://github.com/gustmrg/devlog/releases).

---

## Data Storage

DevLog stores data locally under:

```text
~/.devlog/
├── config.json
├── device-id
├── data/
│   ├── .devlog-version
│   ├── entries/
│   │   └── 2026-04-14/
│   │       └── <entry-id>.json
│   └── summaries/
│       └── 2026-04-14.md
└── backups/
```

Entry files are JSON. Summary files are Markdown with YAML frontmatter. Configuration, API credentials, logs, and the local device ID remain outside the synchronized `data/` repository.

Older daily JSON logs are migrated automatically on first use. Run `devlog migrate` explicitly to perform the migration ahead of time. Legacy data is copied to a timestamped directory under `~/.devlog/backups/` before conversion.

---

## Commands

### `devlog init`

Creates the `~/.devlog/` directory and a default `config.json`.

```bash
devlog init
```

Current note: this command uses safe config creation and will not overwrite an existing config file.

---

### `devlog migrate`

Converts legacy daily JSON logs to the conflict-resistant per-entry layout. Normal commands also perform this migration automatically when needed.

```bash
devlog migrate
```

The original data is copied to a timestamped backup before the current data-version marker is written.

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
| `--remote` | Pull before importing GitHub activity and push the resulting entries afterward. |

Setup:

- Set the username with `devlog config set github.username <username>`, or leave it empty to use the account authenticated by GitHub CLI.
- Put a GitHub token in the environment variable named by `github.tokenEnvVar` (default `GITHUB_TOKEN`). If it is absent and an authenticated GitHub CLI is installed, DevLog uses `gh auth token` automatically. The token needs the `repo` scope (classic) or Contents read and Pull requests read access (fine-grained); authorize it for SSO if your organization requires it.

Re-running `sync` for the same date skips already-imported items, so it is safe to schedule.

#### Automatic sync

Install a daily job using `launchd` on macOS or the user crontab on Linux:

```bash
devlog sync schedule --daily-at 23:55
devlog sync schedule --daily-at 18:00 --polish --remote --env-file ~/.devlog/sync.env
devlog sync schedule show
devlog sync schedule remove
```

Scheduled jobs do not normally inherit an interactive shell's environment. To provide credentials, create a protected environment file:

```bash
mkdir -p ~/.devlog
printf 'GITHUB_TOKEN=your-token\nDEEPSEEK_API_KEY=your-key\n' > ~/.devlog/sync.env
chmod 600 ~/.devlog/sync.env
```

Pass it with `--env-file` when installing the schedule. The file accepts `KEY=VALUE` lines and is read directly by DevLog; it is never copied into the cron or launchd definition. Job output is appended to `~/.devlog/sync.log`.

With `--remote`, the scheduled job pulls remote DevLog changes, imports GitHub activity, and pushes the combined result. Remote failures return a nonzero exit status and local entries remain available for the next retry.

Examples:

```bash
devlog sync
devlog sync --date 2026-08-05 --dry-run
```

---

### `devlog remote`

Synchronizes the local data directory through a private Git repository. DevLog uses the installed `git` executable so existing SSH keys and Git credential helpers continue to work.

```bash
# First machine
devlog remote init git@github.com:you/devlog-data.git

# Additional machines use the same command and repository
devlog remote init git@github.com:you/devlog-data.git

devlog remote status
devlog remote sync
devlog remote remove
```

`remote init` migrates existing data, commits it locally, merges existing remote data, and pushes the result. `remote remove` only disconnects the remote; it does not delete local files or the Git repository on the server.

Remote synchronization:

- merges entries by stable UUID or external `source`;
- gives GitHub-imported entries deterministic IDs, preventing duplicate imports across machines;
- retries concurrent non-fast-forward pushes up to three times;
- keeps the newer summary during a conflict and preserves the other version under `summaries/conflicts/`;
- operates without an interactive Git credential prompt, making authentication failures visible to scheduled jobs.

Use a private repository because DevLog entries may contain sensitive work information. For unattended scheduling, configure SSH or a Git credential helper that works outside an interactive shell.

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
  },
  "remote": {
    "enabled": false,
    "url": "",
    "branch": "main"
  },
  "storage": {
    "path": ""
  }
}
```

An empty `storage.path` uses `~/.devlog/data`. A custom path must be a dedicated data directory and cannot contain `~/.devlog/config.json` or credential files.

### LLM settings

The `llm` section powers `devlog summary create --ai` and `devlog sync --polish`:

| Key | Description |
|---|---|
| `enabled` | Must be `true` for LLM features to run. |
| `provider` | `openrouter`, `openai`, or `deepseek`. For any other OpenAI-compatible endpoint, set `baseURL` instead. |
| `baseURL` | Optional. Overrides the provider's API base URL (e.g. a local or self-hosted endpoint). |
| `model` | Model identifier passed to the API. |
| `apiKeyEnvVar` | Name of the environment variable holding the API key. The key itself is never stored in the config. |

`defaults.language` is included in LLM prompts for both AI summaries and polished sync entries. Use a language tag such as `pt-BR` or `en-US`.

For DeepSeek, set `provider` to `deepseek`, choose a DeepSeek model (for example `deepseek-chat`), and set `apiKeyEnvVar` to the environment variable containing your DeepSeek API key.

Use the config command to inspect and update settings without editing JSON:

```bash
devlog config list
devlog config get github.username
devlog config set github.username your-login
devlog config set github.tokenEnvVar GITHUB_TOKEN
devlog config set llm.enabled true
```

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

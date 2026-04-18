# DevLog CLI

A command-line tool for developers to track daily activities and generate formatted timesheet summaries.

---

## What it does

DevLog acts as a developer memory system. You log activities throughout the day as you work, then generate a structured summary at the end of your session — ready to paste into a timesheet.

```
$ devlog add "Implemented JWT auth middleware" -p echo -t backend,auth -d 45
✔ Entry added (id: a1b2c3d4)

$ devlog summary --style concise

## 2026-04-14 — 2h 30min

### Echo
- Implemented JWT auth middleware (45min)
- Built refresh token rotation logic (1h 15min)

### BitFinance
- Fixed budget category filter bug (30min)

✔ Summary saved to ~/.devlog/summaries/2026-04-14.md
```

---

## Installation

**Prerequisites:** [Rust](https://rustup.rs/) must be installed.

```bash
git clone https://github.com/your-username/devlog
cd devlog
cargo build --release
cp target/release/devlog /usr/local/bin/
```

Then initialize the data directory:

```bash
devlog init
```

---

## Quick Start

```bash
# Log an activity
devlog add "Fixed pagination bug on transactions list" -p bitfinance -t frontend -d 30

# Log interactively
devlog add -i

# View today's entries
devlog list

# Generate today's summary
devlog summary

# Generate an AI-polished summary in formal style
devlog summary --ai --style formal
```

---

## Commands

### `devlog init`

Creates the `~/.devlog/` directory structure and a default `config.json`. Safe to run multiple times — will not overwrite existing data.

---

### `devlog add`

Logs a new activity entry.

```
devlog add <description> [options]
```

| Option | Short | Description |
|---|---|---|
| `--project <name>` | `-p` | Project name (uses config default if omitted) |
| `--tags <list>` | `-t` | Comma-separated tags |
| `--duration <minutes>` | `-d` | Time spent in minutes |
| `--date <YYYY-MM-DD>` | | Override date (defaults to today) |
| `-i` | | Interactive mode — prompts for each field |

```bash
devlog add "Implemented refresh token rotation" -p echo -t backend,auth -d 75
devlog add -i
```

---

### `devlog list`

Displays logged entries with optional filters.

| Option | Short | Description |
|---|---|---|
| `--date <YYYY-MM-DD>` | | Show entries for a specific date |
| `--week` | `-w` | Show entries for the current week |
| `--project <name>` | `-p` | Filter by project |
| `--tag <name>` | | Filter by tag |

```bash
devlog list
devlog list --week
devlog list --project echo
devlog list --date 2026-04-13
```

---

### `devlog edit <id>`

Opens an interactive prompt to modify an existing entry. Current values are shown as defaults — press Enter to keep them.

```bash
devlog edit a1b2c3d4
```

---

### `devlog delete <id>`

Removes an entry after confirmation.

```bash
devlog delete a1b2c3d4
```

---

### `devlog summary`

Generates a structured summary from logged entries and saves it to `~/.devlog/summaries/`.

| Option | Short | Description |
|---|---|---|
| `--date <YYYY-MM-DD>` | | Summarize a specific date (defaults to today) |
| `--week` | `-w` | Generate a weekly summary |
| `--style <style>` | `-s` | Output style (see [Summary Styles](#summary-styles)) |
| `--ai` | | Use an LLM to produce a polished narrative |
| `--format <template>` | `-f` | Template from `~/.devlog/templates/` |

```bash
devlog summary
devlog summary --style formal
devlog summary --week --style detailed
devlog summary --ai --style impersonal
```

---

### `devlog config`

Reads and writes configuration values.

```bash
devlog config --set defaults.project echo
devlog config --set defaults.language pt-BR
devlog config --get defaults.style
devlog config --list
```

---

## Summary Styles

Styles control how summaries are formatted. They work with both template-based and AI-enhanced generation.

| Style | Description |
|---|---|
| `concise` | One line per activity, minimal detail. Best for quick timesheet entries. |
| `detailed` | Full breakdown with durations, tags, and timestamps. |
| `formal` | Third-person, complete sentences. Suitable for corporate timesheets. |
| `impersonal` | Passive voice, no pronouns. Common in consulting contexts. |

Set a default style so you never have to type it:

```bash
devlog config --set defaults.style concise
```

---

## AI-Enhanced Summaries

When you pass `--ai`, DevLog groups your entries and sends them to an LLM, which rewrites them as a polished narrative according to the chosen style.

**Setup:**

```bash
# Set your OpenAI API key as an environment variable
export DEVLOG_OPENAI_API_KEY=sk-...

# Enable AI in config
devlog config --set ai.enabled true
```

**Example:**

```
$ devlog summary --ai --style formal

## 2026-04-14 — 2h 30min

Worked on authentication infrastructure for the Echo project, implementing JWT
middleware with role-based claims and secure refresh token rotation. Additionally,
resolved a filtering defect in the BitFinance budget module.

✔ Summary saved to ~/.devlog/summaries/2026-04-14.md
```

Without `--ai`, summaries are generated instantly from a template — no API key required.

---

## Configuration

Configuration is stored at `~/.devlog/config.json`.

```json
{
  "defaults": {
    "project": "echo",
    "style": "concise",
    "language": "pt-BR"
  },
  "ai": {
    "enabled": false,
    "provider": "openai",
    "model": "gpt-4o-mini",
    "apiKeyEnvVar": "DEVLOG_OPENAI_API_KEY"
  },
  "reminder": {
    "enabled": false,
    "time": "18:00"
  }
}
```

| Key | Description |
|---|---|
| `defaults.project` | Default project when `--project` is omitted |
| `defaults.style` | Default summary style |
| `defaults.language` | Language for AI-generated summaries (e.g. `pt-BR`, `en-US`) |
| `ai.enabled` | Enable AI-powered summary generation |
| `ai.model` | LLM model to use |
| `ai.apiKeyEnvVar` | Environment variable holding the API key |
| `reminder.enabled` | Enable end-of-day reminder notifications |
| `reminder.time` | Time to trigger reminder (HH:MM) |

---

## Data Storage

All data is stored locally under `~/.devlog/`.

```
~/.devlog/
├── config.json
├── entries/
│   ├── 2026-04-13.json
│   └── 2026-04-14.json
├── summaries/
│   ├── 2026-04-13.md
│   └── 2026-04-14.md
└── templates/
    └── timesheet.md
```

Entries are stored as JSON (one file per day) and summaries are saved as Markdown.

---

## Built With

- [Rust](https://www.rust-lang.org/)
- [clap](https://github.com/clap-rs/clap) — CLI argument parsing
- [serde](https://serde.rs/) — JSON serialization
- [chrono](https://github.com/chronotope/chrono) — Date/time handling
- [reqwest](https://github.com/seanmonstar/reqwest) — HTTP client for AI integration
- [tokio](https://tokio.rs/) — Async runtime
- [colored](https://github.com/colored-rs/colored) — Terminal output styling

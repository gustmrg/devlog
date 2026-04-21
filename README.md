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

**Prerequisites:** [Go](https://go.dev/dl/) 1.21+ must be installed.

```bash
git clone https://github.com/gustmrg/devlog
cd devlog
go build -o bin/devlog .
mv bin/devlog /usr/local/bin/
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

Reads and writes configuration values using `set`, `get`, and `list` subcommands.

```bash
devlog config set <key> <value>
devlog config get <key>
devlog config list
```

| Key | Description |
|---|---|
| `defaultProject` | Default project when `-p` is omitted on `devlog entry add` |
| `style` | Default summary style |
| `language` | Output language for AI summaries (e.g. `pt-BR`, `en-US`) |
| `llm.enabled` | Enable or disable AI-powered summaries (`true` / `false`) |
| `llm.model` | LLM model to use (e.g. `openai/gpt-4o-mini`) |
| `llm.provider` | LLM provider (e.g. `openrouter`) |
| `llm.apiKeyEnvVar` | Environment variable holding the API key |

```bash
devlog config set defaultProject myapp
devlog config set language en-US
devlog config set style formal
devlog config set llm.enabled true
devlog config set llm.model "openai/gpt-4o-mini"
devlog config get defaultProject
devlog config list
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
devlog config set style concise
```

---

## AI-Enhanced Summaries

When you pass `--ai`, DevLog groups your entries and sends them to an LLM, which rewrites them as a polished narrative according to the chosen style.

**Setup:**

```bash
# Set your OpenRouter API key as an environment variable
export OPENROUTER_API_KEY=sk-or-xxx

# Enable AI in config
devlog config set llm.enabled true
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
    "project": "",
    "style": "concise",
    "language": "pt-BR"
  },
  "llm": {
    "enabled": false,
    "provider": "openrouter",
    "model": "openai/gpt-oss-120b:free",
    "apiKeyEnvVar": "OPENROUTER_API_KEY"
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
| `llm.enabled` | Enable AI-powered summary generation |
| `llm.model` | LLM model to use |
| `llm.apiKeyEnvVar` | Environment variable holding the API key |
| `reminder.enabled` | Enable end-of-day reminder notifications |
| `reminder.time` | Time to trigger reminder (HH:MM) |

### LLM Integration

devlog supports AI-powered summary generation via [OpenRouter](https://openrouter.ai), which gives you access to multiple models through a single API key.

**1. Get an API key** at [openrouter.ai/keys](https://openrouter.ai/keys)

**2. Set the environment variable** in your shell:

```sh
export OPENROUTER_API_KEY=sk-or-xxx
```

Add it to `~/.zshrc` (or `~/.bashrc`) to make it permanent:

```sh
echo 'export OPENROUTER_API_KEY=sk-or-xxx' >> ~/.zshrc
source ~/.zshrc
```

**3. Enable LLM in config** (`~/.devlog/config.json`):

```json
"llm": {
  "enabled": true,
  "provider": "openrouter",
  "model": "openai/gpt-4o-mini",
  "apiKeyEnvVar": "OPENROUTER_API_KEY"
}
```

You can browse available models at [openrouter.ai/models](https://openrouter.ai/models) and set your preferred one via `llm.model`.

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

- [Go](https://go.dev/)
- [cobra](https://github.com/spf13/cobra) — CLI framework
- [viper](https://github.com/spf13/viper) — Configuration management

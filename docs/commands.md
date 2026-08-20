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

## GitHub activity

### `devlog sync`

Import commits, authored pull requests, and pull-request reviews from GitHub.
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

Install a daily GitHub import using `launchd` on macOS or the user crontab on
Linux.

```bash
devlog sync schedule --daily-at 23:55
devlog sync schedule show
devlog sync schedule remove
```

The schedule command also accepts `--polish`, `--remote`, and `--env-file`.

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

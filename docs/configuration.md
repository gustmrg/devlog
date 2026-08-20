# Configuration and integrations

DevLog stores its configuration in `~/.devlog/config.json`. Inspect and update
settings without editing the file directly:

```bash
devlog config list
devlog config get defaults.project
devlog config set defaults.project my-project
```

## Settings

| Key | Description | Default |
| --- | --- | --- |
| `defaults.project` | Project used when `devlog add` omits `--project`. | `default` |
| `defaults.style` | Style used for AI summaries. | `concise` |
| `defaults.language` | Language requested from the LLM. | `pt-BR` |
| `storage.path` | Custom data directory; empty uses `~/.devlog/data`. | empty |
| `github.username` | GitHub account to import activity from. | empty |
| `github.tokenEnvVar` | Environment variable containing a GitHub token. | `GITHUB_TOKEN` |
| `llm.enabled` | Enable LLM-backed features. | `false` |
| `llm.provider` | LLM provider name. | `openrouter` |
| `llm.baseURL` | Optional OpenAI-compatible API base URL. | empty |
| `llm.model` | Model sent to the provider. | `openai/gpt-oss-120b:free` |
| `llm.apiKeyEnvVar` | Environment variable containing the LLM API key. | `OPENROUTER_API_KEY` |
| `remote.enabled` | Whether Git synchronization is configured. | `false` |
| `remote.url` | Data repository URL. | empty |
| `remote.branch` | Data repository branch. | `main` |

A custom `storage.path` must be a dedicated data directory. It cannot contain
DevLog's configuration or credential files.

## GitHub

Set a username explicitly or leave it empty to use the account authenticated by
the GitHub CLI:

```bash
devlog config set github.username your-login
```

DevLog reads the token from `GITHUB_TOKEN` by default. If that variable is not
set, it tries `gh auth token`. The token needs access to repository contents and
pull requests, including any private repositories you want to import.

```bash
devlog sync --dry-run
devlog sync
```

## AI summaries and polished imports

LLM settings power `devlog summary create --ai` and `devlog sync --polish`.
DevLog supports OpenRouter, OpenAI, DeepSeek, and other OpenAI-compatible APIs
through `llm.baseURL`.

```bash
devlog config set llm.enabled true
devlog config set llm.provider openai
devlog config set llm.model gpt-4.1-mini
devlog config set llm.apiKeyEnvVar OPENAI_API_KEY
```

Export the API key using the environment-variable name configured in
`llm.apiKeyEnvVar`. The key itself is not written to `config.json`.

## Scheduled sync credentials

Scheduled jobs do not normally inherit credentials from an interactive shell.
Create a protected environment file and pass it when installing the schedule:

```bash
mkdir -p ~/.devlog
printf 'GITHUB_TOKEN=your-token\nOPENAI_API_KEY=your-key\n' > ~/.devlog/sync.env
chmod 600 ~/.devlog/sync.env
devlog sync schedule --daily-at 23:55 --env-file ~/.devlog/sync.env
```

The file is read by DevLog and is not copied into the cron or `launchd`
definition. Scheduled output is appended to `~/.devlog/sync.log`.

## Remote data synchronization

Connect each machine to the same private Git repository:

```bash
devlog remote init git@github.com:you/devlog-data.git
```

DevLog uses the installed `git` executable, including its SSH keys and credential
helpers. For unattended jobs, authentication must work without an interactive
prompt. Use a private repository because activity entries may contain sensitive
work information.

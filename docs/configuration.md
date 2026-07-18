# Configuration

Clients use `~/.devlog/config.json`. The Compose server mounts `deploy/config.json` read-only and stores mutable data in `/data/devlog.db`.

## Server configuration

```json
{
  "github": {"enabled": true, "tokenEnvVar": "DEVLOG_GITHUB_TOKEN", "interval": "15m"},
  "llm": {"enabled": true, "baseUrl": "https://openrouter.ai/api/v1", "model": "openai/gpt-oss-120b:free", "apiKeyEnvVar": "DEVLOG_LLM_API_KEY"},
  "discord": {"enabled": true, "tokenEnvVar": "DEVLOG_DISCORD_BOT_TOKEN", "guildId": "...", "channelId": "...", "userId": "..."},
  "schedules": {"timezone": "America/Fortaleza", "correlateInterval": "30m", "finalizeAt": "17:45", "summaryAt": "18:00"},
  "retentionDays": 30,
  "projects": [{"id": "devlog", "name": "DevLog", "remote": "github.com/gustmrg/devlog", "enabled": true}]
}
```

Secrets are environment variables, never JSON values. Schedules are interpreted in `schedules.timezone`. GitHub polling overlaps the previous cursor and relies on event deduplication.

## Client configuration

```json
{
  "device": {"id": "generated-by-pairing", "name": "macbook-personal"},
  "server": {"url": "https://devlog.example.private"},
  "discovery": {"roots": ["~/Dev"], "exclude": ["~/Dev/archived"], "maxDepth": 3},
  "projects": [{"id": "devlog", "name": "DevLog", "path": "~/Dev/devlog", "remote": "github.com/gustmrg/devlog", "enabled": true}]
}
```

Changing a project path affects only that device. Keep the same project ID and canonical remote across devices.

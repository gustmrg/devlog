# Operations

## Backup

Stop writes or use the provided SQLite-safe backup command before copying the database:

```bash
docker compose exec devlog devlog admin backup --output /data/backups/devlog-$(date +%F).db
```

Copy the backup off the Pi and retain the matching `config.json` separately. Secrets remain in `.env` and should use an independent encrypted backup.

## Restore

Stop the service, preserve the current volume, place the verified backup at `/data/devlog.db`, fix ownership, and restart. Check `/readyz`, server logs, device status, and the latest timeline before removing the old copy.

## Token rotation

- Change GitHub, Discord, and LLM secrets in `.env`, then restart the service.
- Pair a replacement device before revoking the previous device.
- Rotate `DEVLOG_PAIRING_CODE` after bootstrap or suspected exposure.

## Troubleshooting

- **Agent has queued events:** run `devlog sync`; verify the private hostname, HTTPS trust, and device token.
- **Repository is silent:** check `devlog source list`, its local path, Git availability, and agent logs.
- **GitHub is stale:** verify PAT permissions and rate-limit errors in server logs. A failed run does not advance its cursor.
- **No Discord message:** confirm bot token, channel access, configured user/channel IDs, and Gateway connectivity.
- **LLM fails:** the deterministic summary remains available; inspect logs without printing the API key.
- **Summary is incomplete:** check device last-seen state and synchronize offline machines; late events create another revision.

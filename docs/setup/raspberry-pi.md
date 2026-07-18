# Raspberry Pi server setup

The supported production path is one Docker Compose service and one persistent volume. The image must match the Pi architecture (`linux/arm64` for most current models).

## 1. Network and prerequisites

Install Docker Engine and the Compose plugin. Give the Pi a stable hostname on your private network and configure HTTPS before pairing clients. `DEVLOG_PUBLIC_URL` must be the URL clients and Discord links can reach.

Do not expose port 8080 directly to the public internet. If TLS terminates at a private reverse proxy, proxy to the Compose port and preserve `Host`, `X-Forwarded-Proto`, and client addresses.

## 2. Prepare configuration

```bash
mkdir -p ~/services/devlog
cd ~/services/devlog
cp /path/to/devlog/deploy/{compose.yml,.env.example,config.example.json} .
mv .env.example .env
mv config.example.json config.json
chmod 600 .env
chmod 644 config.json
```

Set at least:

```dotenv
DEVLOG_ADMIN_PASSWORD=a-long-unique-password
DEVLOG_PAIRING_CODE=a-temporary-bootstrap-code
DEVLOG_PUBLIC_URL=https://devlog.example.private
```

The bootstrap pairing code is valid for ten minutes and one successful pairing. Generate another one from the Devices page for each additional client; rotating the environment value is not required.

## 3. Optional integrations

### GitHub

Create a fine-grained personal access token with read-only access to the enabled repositories. Grant repository metadata, contents, pull requests, and issues read permissions. Set `DEVLOG_GITHUB_TOKEN` in `.env`, enable GitHub in `config.json`, and list each repository in `projects` using its canonical remote. DevLog resolves the authenticated login and retains only that account's commits, authored issues/PRs, reviews, and comments.

### Discord

Create a bot, invite it to the target server with permission to view the channel, send messages, and use message components. Enable the Message Content intent only if another future command needs it; summary buttons do not. Set the bot token in `.env` and set `channelId` plus the only permitted `userId` in `config.json`.

The bot connects through the Discord Gateway, so it does not require a public callback endpoint.

### LLM

Set an OpenAI-compatible base URL and model in `config.json`, then place the API key in `DEVLOG_LLM_API_KEY`. If disabled or unavailable, DevLog produces a deterministic summary.

## 4. Start and verify

```bash
docker compose up -d
docker compose ps
docker compose logs devlog
curl --fail http://127.0.0.1:8080/healthz
curl --fail http://127.0.0.1:8080/readyz
```

Open `DEVLOG_PUBLIC_URL`, sign in with `DEVLOG_ADMIN_PASSWORD`, inspect Projects, and open Devices to verify the pairing code.

## 5. Pair clients

Follow [client setup](client.md) on every workstation. Verify that the web timeline receives an event before enabling optional integrations.

## Updating

```bash
docker compose pull
docker compose up -d
docker compose logs --tail=100 devlog
```

Back up the volume before upgrades. See [operations](../operations.md).

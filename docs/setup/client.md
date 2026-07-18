# Client setup

Install the same `devlog` binary on every macOS or Linux device. The CLI and agent are not separate applications.

## 1. Initialize and connect

```bash
devlog init
devlog connect https://devlog.example.private --pairing-code CODE --name macbook-personal
devlog sync
```

For local development over trusted HTTP only, add `--allow-insecure`. Never use it over an untrusted network.

Connection state is stored under `~/.devlog/`. `credentials.json` is mode `0600`; do not copy it between machines because each device must have a separately revocable token.

## 2. Import existing data

```bash
devlog migrate legacy --dry-run
devlog migrate legacy
devlog sync
```

Import is idempotent. Existing `entries/*.json` and `summaries/*.md` remain untouched and can be archived after the central timeline is verified.

## 3. Enable repositories

```bash
devlog source discover ~/Dev
devlog source enable ~/Dev/hobby-projects/devlog --project-id devlog
devlog source list
```

Only enabled repositories are collected. The local collector sends metadata, not file contents or diffs.

## 4. Install the background agent

```bash
devlog agent install
devlog agent status
```

macOS uses a user LaunchAgent; Linux uses a `systemd --user` service. To debug interactively:

```bash
devlog agent uninstall
devlog agent run --interval 30s
```

Reinstall the service after moving or replacing the binary because the service file records its absolute path.

## 5. Daily checks

```bash
devlog status
devlog sync
devlog list
```

Manual entries continue to work offline and synchronize when the server becomes reachable.

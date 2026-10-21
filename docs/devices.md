# Setting up your devices

DevLog collects on every device where you work and gathers the results in one
private Git repository. One always-on machine, such as a home server, also
collects your GitHub activity and serves everything to an agent over MCP.

```
work machines                       private data repository        always-on machine
devlog collect (git, claude-code,   ─ push ─▶  entries/  ◀─ pull ─  devlog collect (github)
opencode) on weekdays at 12:15 and             devices/    push     devlog mcp --http
18:15                                                                tailscale serve (HTTPS)
```

Each device only writes its own entries and its own status under `devices/`,
so they never conflict. A commit collected both locally and from GitHub is
stored once.

## The data repository

Create an empty **private** repository, such as `you/devlog-data`. Entries
contain commit messages, branch names, and session titles from your work, so
check that your employer allows keeping them there.

## Work machines (macOS, Linux, Windows)

Install DevLog with the script for your platform (see the [README](../README.md)),
then run `devlog setup` and choose **Work machine**. It asks for the device
name, time zone, ticket key pattern, the folders with your repositories, your
commit emails, and the `opencode` executable; then it connects the data
repository, schedules collection at 12:15 and 18:15 on weekdays, and runs a
test collection.

Without questions, the same configuration is:

```bash
devlog init
devlog config set device.name laptop
devlog config set collect.timezone America/Los_Angeles
devlog config set collect.git.roots ~/dev/work-projects
devlog config set collect.git.authorEmails you@company.com,you@example.com
devlog config set collect.ticketPattern 'PROJ-[0-9]+'
devlog remote init git@github.com:you/devlog-data.git

devlog collect --dry-run
devlog collect schedule --at 12:15,18:15 --remote
```

The default sources, `git`, `claude-code`, and `opencode`, skip themselves on a
machine without the tool. Git must reach the data repository without a prompt,
through an SSH key or a credential helper, because the scheduled job runs
unattended.

On Windows, run the commands in PowerShell; `~` works in DevLog's paths. The
job is a Task Scheduler task named `DevLog collect` that runs while you are
logged on and catches up on collections missed while the computer was off.

## The always-on machine

Install the Linux build with the install script, then run `devlog setup` and
choose **Always-on machine**. It checks for a GitHub token, asks for the
account to collect, connects the same data repository, schedules collection at
12:20 and 18:20 on weekdays, a few minutes after the work machines, and
installs the MCP service. When you are signed in with `gh auth login`, the
scheduled runs use its token; a token that only lives in your shell's
environment is offered to be saved in `~/.devlog/collect.env`.

Without questions, the same configuration is:

```bash
devlog init
devlog config set device.name server
devlog config set collect.timezone America/Los_Angeles
devlog config set collect.sources github
devlog config set collect.ticketPattern 'PROJ-[0-9]+'
devlog config set github.username your-login
devlog remote init git@github.com:you/devlog-data.git
```

Give it a GitHub token that can read your work repositories, in a file only
you can read, and schedule the collection:

```bash
printf 'GITHUB_TOKEN=your-token\n' > ~/.devlog/collect.env
chmod 600 ~/.devlog/collect.env
devlog collect --env-file ~/.devlog/collect.env --dry-run
devlog collect schedule --at 12:20,18:20 --remote --env-file ~/.devlog/collect.env
```

Organizations may need to approve a fine-grained token, or authorize a classic
token for single sign-on, before it sees their private repositories.

Then serve the activity over MCP (`devlog setup` installs the service for you)
and expose it over HTTPS on your tailnet:

```bash
devlog mcp install                 # listens on 127.0.0.1:8766
loginctl enable-linger "$USER"     # keeps it running while you are logged out
tailscale serve --bg --https=8444 http://127.0.0.1:8766
```

Add `https://<machine>.<tailnet>.ts.net:8444/mcp` to your agent as a remote MCP
server with header authentication: header `Authorization`, value
`Bearer <token>`, where the token is in `~/.devlog/mcp-token`. Check the
service with `systemctl --user status devlog-mcp` and
`journalctl --user -u devlog-mcp`. `devlog update`, or `sudo devlog update`
when DevLog is installed in `/usr/local/bin`, restarts it on the new version.

## Checking coverage

`get_activity` reports when each device last collected each source. The same
information is in `devices/<device-id>.json` in the data repository, and each
scheduled run appends its output to `~/.devlog/collect.log`.

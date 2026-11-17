# Features

The features planned for v1. Each lists where it is available; the
[roadmap](roadmap.md) says when it lands.

| # | Feature | Web | CLI | MCP | Status |
| --- | --- | --- | --- | --- | --- |
| F1 | [Time entries](#f1-time-entries) | yes | no | yes | Planned |
| F2 | [Timer](#f2-timer) | yes | yes | no | Planned |
| F3 | [Projects](#f3-projects) | yes | no | read | Planned |
| F4 | [Reports](#f4-reports) | yes | no | read | Planned |
| F5 | [Activity collection](#f5-activity-collection) | status | yes | read | Exists in v0.7, moves to the server |
| F6 | [Suggestions](#f6-suggestions) | yes | no | yes | Planned |
| F7 | [Backup](#f7-backup) | no | no | no | Planned |

## F1. Time entries

Create, edit, and delete the time spent on work. An entry has a project, a
description, ticket keys, and either a start and end time or only a duration
("2h30 on Tuesday"). Entries without a project are allowed and show up in
reports as unassigned.

## F2. Timer

Start a timer with a project, description, and ticket, and stop it to turn it
into a time entry. One timer runs at a time; starting another stops the
current one. The timer lives on the server, so it can be started in the
browser and stopped from the terminal.

## F3. Projects

A project has a name, a client, a color, an optional estimate in hours, and
the repositories whose activity belongs to it. Archived projects keep their
time entries but leave the pickers.

## F4. Reports

Totals for a date range, grouped by project, ticket, client, or day, with a
CSV export. The week view compares each day with a configurable daily target,
and the project view shows the hours spent against the estimate.

CSV columns: date, project, client, tickets, description, start, end, hours.

## F5. Activity collection

Optional. `devlog collect` reads, on the machine it runs on:

| Source | What it records |
| --- | --- |
| `git` | Commits, branch activity from the reflog, uncommitted and unpushed work |
| `claude-code` | Claude Code sessions: active intervals, repository, branch, title, prompt count |
| `opencode` | The same for opencode sessions |
| `github` | Pushed commits, pull requests, reviews, comments, created branches, when the token can read the repositories |

Prompts and replies are never read into DevLog. The Devices page shows when
each machine last collected each source, so a quiet day can be told apart from
a machine that did not collect.

## F6. Suggestions

The Day page proposes time entries built from the day's activity:

1. Take the activity as points (commits, pull requests, reviews) and intervals
   (session spans, branch activity).
2. Merge them into blocks while the gap between neighbours is at most the idle
   gap (default 30 minutes). A lone point becomes a block of the minimum length
   (default 15 minutes) ending at that point.
3. Split a block where the project changes.
4. Remove the intervals already covered by time entries.
5. Round to the rounding step (default 15 minutes).
6. Propose the project (from the repository), the most frequent ticket, and a
   description from commit messages and pull request titles, optionally
   rewritten by an LLM.

Suggestions are computed on request and never stored. Nothing becomes a time
entry until the user accepts or edits it. Meetings and reading leave no trace,
so the Day page also shows the gaps without evidence.

## F7. Backup

The server writes a consistent copy of the database (`VACUUM INTO`) on a
schedule, to a folder that can be copied off the host.

## Out of scope

Multiple users or workspaces, approvals, invoicing and hourly rates, tags on
time entries, mobile apps, and access from outside the tailnet. See
[ADR 001](decisions/001-personal-time-tracker.md).

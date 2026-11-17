# ADR 001: A personal time tracker

- Status: Accepted
- Date: 2026-11-16
- Scope: What DevLog is for and what it leaves out
- Related: [features](../features.md), [roadmap](../roadmap.md)

## Context

Up to v0.7, DevLog is a command-line tool that records development activity
(commits, pull requests, coding agent sessions) and turns it into Markdown
summaries and timesheet descriptions. Filling in timesheets still means
working out the hours by hand, and tools such as Clockify do that part well
but know nothing about the work itself.

## Decision

DevLog becomes a self-hosted time tracker for one person: log hours, run a
timer, manage projects, and read reports, in the browser first and from the
terminal for the timer.

Activity collection becomes optional. Its job is to suggest time entries, not
to be the record: the user logs or accepts every hour.

Left out: multiple users and workspaces, approvals, invoicing and hourly
rates, tags on time entries, and mobile apps. A ticket key stands in for a
task.

v1 does not keep compatibility with v0.7 data, configuration, or commands.
The project has no other users, and keeping both storages would double the
maintenance.

## Consequences

- One user means no accounts, roles, or tenant boundaries in the schema or
  the API.
- The release that ships it is v1.0.0. A one-time import of v0.7 activity is
  optional.
- Adding users later would touch every table and every route; it would need a
  new decision.

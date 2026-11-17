# API

Draft. The server's HTTP API, to be refined while it is built. Routes are
under `/api`, take and return JSON, and report failures as:

```json
{ "error": { "code": "invalid_input", "message": "end must be after start" } }
```

Requests carry `Authorization: Bearer <token>`, or the session cookie from the
browser. Changes made with a cookie must come from the server's own origin.

| Route | Purpose | Phase |
| --- | --- | --- |
| `GET /status` | Server version and time zone | 1 |
| `POST /session`, `DELETE /session` | Browser sign-in with the token, and sign-out | 1 |
| `GET /projects`, `POST /projects` | List and create projects | 1 |
| `PATCH /projects/{id}`, `DELETE /projects/{id}` | Edit, archive, or delete a project | 1 |
| `GET /time-entries` | List by date range, project, or ticket | 1 |
| `POST /time-entries` | Create an entry | 1 |
| `PATCH /time-entries/{id}`, `DELETE /time-entries/{id}` | Edit or delete an entry | 1 |
| `GET /timer`, `POST /timer/start`, `POST /timer/stop` | The running timer | 1 |
| `GET /reports/summary` | Totals for a range, grouped by project, ticket, client, or day | 3 |
| `GET /reports/export.csv` | The entries of a range as CSV | 3 |
| `GET /activity` | Collected activity for a range | 4 |
| `POST /activity` | Upsert a batch of activity with the device's collector runs | 4 |
| `GET /devices` | When each device last collected each source | 4 |
| `GET /suggestions?date=` | Suggested time entries for a day | 5 |

The CLI sends its version, and the server answers with its own, so the CLI can
warn when they differ.

## MCP

`/mcp` serves the Model Context Protocol over Streamable HTTP with the same
bearer token. Planned tools: `get_timesheet`, `get_activity`,
`list_projects`, `suggest_time_entries`, `save_time_entry`, and
`delete_time_entry`.

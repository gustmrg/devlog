# Architecture

DevLog is one Go release with two runtime roles. On a workstation it is a CLI and background agent. On the central host, `devlog serve` provides the API, scheduler, review UI, and integrations.

## Data flow

```text
Git local ─┐
Manual CLI ├─▶ local SQLite outbox ─▶ sync API ─┐
Future     ┘                                    │
                                                 ├─▶ immutable events
GitHub collector ────────────────────────────────┘          │
                                                            ▼
                                               deterministic correlation
                                                            │
                                                            ▼
                                               draft activities + evidence
                                                            │
                                  ┌─────────────────────────┴──────────┐
                                  ▼                                    ▼
                            web review                         summary generator
                                                                       │
                                                                       ▼
                                                               Discord + web
```

## Boundaries

- **Collectors** only turn a source cursor into normalized events and a new cursor.
- **Events** are immutable, globally identified, timestamped, attributed to a source, and deduplicated.
- **Correlation** groups events using project identity, timing, branches, and external references. It does not call an LLM.
- **Activities** are editable projections with evidence, confidence, and review status.
- **Summary generation** receives activities, never raw diffs or command history. A deterministic generator remains available when the LLM fails.
- **Notifications** deliver an existing summary and mutate it only through explicit reviewed actions.

## Project identity

Paths are device-local aliases. The stable identity is an explicit project ID associated with a normalized remote such as `github.com/owner/repository`. Repositories without a known mapping remain unclassified until enabled or corrected.

## Offline and late data

The local agent queues events before attempting a network request. The server acknowledges event IDs only after the transaction commits, and a retry is safe because IDs, external IDs, and fingerprints are unique. Events arriving after a summary create a new reviewable revision rather than overwriting history.

## Extension model

Adapters are compiled into the binary and implement the collector contract. GitHub is the first remote adapter. GitLab, calendar, shell, or agent-session adapters must emit the same event model and maintain their own cursor; they do not change activities, summaries, or sync storage.

## Trust model

DevLog is single-user. Admin access uses the configured server password and session cookies. Devices exchange a short-lived pairing code for independent high-entropy tokens. Run the service behind HTTPS on a private network; plain HTTP is accepted only with an explicit development override.

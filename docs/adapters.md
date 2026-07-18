# Collector adapters

A collector identifies its type, validates configuration, and implements:

```go
Collect(context.Context, cursor string) (events []domain.Event, nextCursor string, err error)
```

Rules:

1. Do not advance the cursor when collection or persistence fails.
2. Give every upstream object a stable external ID and fingerprint.
3. Use UTC internally and preserve the actual occurrence time.
4. Emit metadata only unless the user explicitly opts into content collection.
5. Do not create activities or summaries in an adapter.
6. Treat retries and overlapping cursor windows as normal.

## Adding GitLab

Create a collector registered as `gitlab`, authenticate from an environment variable, map GitLab commits/MRs/issues/reviews to normalized event kinds, and add fixture-driven contract tests for pagination, cursor overlap, rate limiting, and retry. Project mapping continues to use `gitlab.com/owner/repository`; no schema or correlator fork should be necessary.

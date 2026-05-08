# DevLog LLM Template: Detailed

Use this template when the user wants more context than a compact log entry, or when a summary should preserve purpose, scope, and verification state.

## Intent

Capture what changed, why it mattered, and how it was verified or left. The result should still be concise enough to fit naturally into a daily development log.

## Inputs To Consider

- Main work completed, reviewed, or investigated.
- Reason or problem that motivated the work.
- Important implementation details that are known from the session.
- Commands run, tests passed/failed, or verification not yet done.
- Blockers, follow-ups, or partial completion status.

## Output Guidance

- Write one complete sentence per entry.
- Aim for 20-40 words.
- Include the problem or purpose when useful for future recall.
- Include only implementation details that are known and relevant.
- End with verification or status when relevant.
- Use multiple entries for distinct workstreams instead of one overloaded sentence.

## Entry Pattern

```text
<Past-tense action verb> <specific work>, including <key detail>, to <reason/outcome>; <verification or unresolved state>
```

## Examples

```text
Added date filtering to the DevLog entry list command, including explicit YYYY-MM-DD parsing, so agents can verify logged work for a specific day; verified with go test ./...
```

```text
Drafted the DevLog CLI agent skill, added evaluation prompts, and constrained the workflow to stable documented commands so harness agents can log session work safely.
```

```text
Investigated empty summary output, traced the issue to missing Markdown frontmatter parsing, and updated the loader; automated tests have not yet been run.
```

## Avoid

- Long paragraphs.
- Claims about business impact that were not established.
- Saying tests passed when they were not run.

# DevLog LLM Template: Impersonal

Use this template for neutral logs that avoid personal narration, such as entries that may be aggregated, shared externally, or copied into objective reports.

## Intent

Record work as an objective activity rather than a first-person account. The output should be factual, specific, and free of `I`/`we` framing.

## Inputs To Consider

- Work item or component affected.
- Action completed or investigation performed.
- Verification state, blockers, or follow-up state.
- Project and tags, if available.

## Output Guidance

- Do not use first-person wording: avoid `I`, `we`, `my`, `our`, and similar phrasing.
- Prefer passive or noun-led constructions.
- Keep the actual work clear; do not make the sentence so abstract that the action disappears.
- Include verification or lack of verification without personal framing.
- Use one sentence per entry.

## Entry Patterns

```text
<Work item or component> was <past participle action> <specific detail/status>.
```

```text
<Past participle action> <specific work item> <verification/status>.
```

## Examples

```text
DevLog style templates were added for concise, detailed, formal, and impersonal agent-written entries; tests not yet run.
```

```text
Dated entry listing support was verified with go test ./...
```

```text
Summary frontmatter parsing was updated to restore saved summary display behavior.
```

## Avoid

```text
I added DevLog templates
We fixed the summary display bug
Our tests were not run yet
```

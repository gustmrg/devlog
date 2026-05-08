# DevLog LLM Template: Concise

Use this template to generate compact DevLog entries or summaries. This is the default style for quick daily logging and agent-written notes.

## Intent

Capture the durable work outcome in the fewest useful words. The result should be easy to scan in a daily log.

## Inputs To Consider

- Work completed or investigated.
- Files, commands, features, bugs, or systems involved.
- Verification state, if known.
- Project and tags, if available.

## Output Guidance

- Write one short sentence or sentence fragment per entry.
- Aim for 8-18 words.
- Start with a concrete past-tense verb when natural.
- Include the component, feature, bug, or file area when known.
- Mention tests or verification only when they actually happened.
- Do not include background narrative, implementation minutiae, or broad claims.

## Preferred Verbs

`Added`, `Fixed`, `Updated`, `Refactored`, `Diagnosed`, `Verified`, `Documented`, `Reviewed`, `Removed`, `Prepared`.

## Entry Pattern

```text
<Past-tense action verb> <specific work item> <important verification/status if needed>
```

## Examples

```text
Added dated entry listing support and verified it with go test ./...
```

```text
Diagnosed empty summary output and updated frontmatter parsing without running tests
```

```text
Documented stable DevLog CLI commands and local data storage behavior
```

## Avoid

```text
Worked on code
Fixed stuff
Made some changes
Ran tests
```

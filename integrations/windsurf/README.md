# Windsurf — review-replay rule

Windsurf reads rule files under `.windsurf/rules/`. Each rule is a markdown file with frontmatter telling Cascade when to apply it.

## Install

```bash
mkdir -p .windsurf/rules
cp review-replay.md .windsurf/rules/review-replay.md
```

The `trigger: model_decision` frontmatter lets Cascade decide when to invoke it based on the description.

## Prerequisites

- `review-replay` binary on PATH:
  ```bash
  go install github.com/alejandroSuch/review-replay/cmd/review-replay@latest
  ```
- `GITHUB_TOKEN` env var.
- One LLM API key.

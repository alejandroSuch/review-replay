# Cursor — review-replay rule

Cursor reads `.cursor/rules/*.mdc` files at the project root. Drop the rule in there and Cursor's agent will know when to invoke review-replay.

## Install

```bash
mkdir -p .cursor/rules
cp review-replay.mdc .cursor/rules/review-replay.mdc
```

For global rules (all projects), add the same file under `~/.cursor/rules/`.

## Prerequisites

- `review-replay` binary on PATH:
  ```bash
  go install github.com/alejandroSuch/review-replay/cmd/review-replay@latest
  ```
- `GITHUB_TOKEN` env var.
- One LLM API key.

## Triggering it

The rule is `alwaysApply: false`, so it activates when the user explicitly mentions PR verification ("did I address the reviews", "re-check the PR", etc). If you want it always-on for a project, edit the frontmatter to `alwaysApply: true`.

# Claude Code — review-replay skill

A skill that lets Claude Code invoke review-replay when the user wants to verify PR feedback was addressed.

## Install (global, all projects)

```bash
mkdir -p ~/.claude/skills
cp review-replay.md ~/.claude/skills/review-replay.md
```

## Install (single project)

```bash
mkdir -p .claude/skills
cp review-replay.md .claude/skills/review-replay.md
```

Claude Code picks it up automatically based on the description in the frontmatter. The skill triggers when the user says things like:

- "did I address all the reviews?"
- "re-check the PR before I tell the reviewer it's done"
- "verify Copilot's feedback"
- runs `/review-replay`

It also runs proactively after any address-reviews flow to confirm the fixes actually resolved the concerns.

## Prerequisites

- `review-replay` binary on PATH:
  ```bash
  go install github.com/alejandroSuch/review-replay/cmd/review-replay@latest
  ```
- `GITHUB_TOKEN` / `GH_TOKEN` env var.
- One LLM API key (`OPENROUTER_API_KEY` / `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GEMINI_API_KEY`).

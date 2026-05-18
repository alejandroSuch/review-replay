# Claude Code — review-replay skill

A skill that lets Claude Code invoke review-replay when the user wants to verify PR feedback was addressed.

## Install (recommended) — as a Claude Code plugin

The whole repo is also a single-plugin Claude Code marketplace. Two commands inside Claude Code:

```
/plugin marketplace add alejandroSuch/review-replay
/plugin install review-replay@review-replay
```

The skill gets installed in the current Claude Code session. Triggered automatically whenever the user says things like:

- "did I address all the reviews?"
- "re-check the PR before I tell the reviewer it's done"
- "verify Copilot's feedback"
- runs `/review-replay:check`

The plugin only ships the skill (instructions for Claude). The actual `review-replay` binary still needs to be installed on PATH — see [Prerequisites](#prerequisites) below.

## Manual install — drop in `~/.claude/skills/`

If you prefer not to use the plugin system:

```bash
mkdir -p ~/.claude/skills
mkdir -p ~/.claude/skills/review-replay
curl -fsSL https://raw.githubusercontent.com/alejandroSuch/review-replay/main/plugins/review-replay/skills/review-replay/SKILL.md \
  -o ~/.claude/skills/review-replay/SKILL.md
```

Or per-project, drop the same file into `.claude/skills/review-replay/SKILL.md`.

Note: the manual route ships the skill only (description-triggered). The `/review-replay:check` slash command is only available via the plugin install above.

## Prerequisites

- `review-replay` binary on PATH:
  ```bash
  GOPROXY=direct go install github.com/alejandroSuch/review-replay/cmd/review-replay@main
  ```
- `GITHUB_TOKEN` / `GH_TOKEN` env var.
- One LLM API key (`OPENROUTER_API_KEY` / `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GEMINI_API_KEY`).

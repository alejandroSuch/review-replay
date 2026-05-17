# Integrations

Drop-in configs so your AI coding assistant can invoke `review-replay` when you ask it to verify PR feedback. Each subfolder has the ready-to-copy file plus a per-tool README explaining how to install it.

| Assistant | Folder | Install location |
|---|---|---|
| Claude Code | [claude-code/](./claude-code/) | `~/.claude/skills/review-replay.md` (global) or `.claude/skills/` (project) |
| Codex CLI | [codex/](./codex/) | Append to `~/.codex/AGENTS.md` or `./AGENTS.md` |
| Cursor | [cursor/](./cursor/) | `.cursor/rules/review-replay.mdc` |
| Windsurf | [windsurf/](./windsurf/) | `.windsurf/rules/review-replay.md` |
| Continue.dev | [continue/](./continue/) | Merge into `~/.continue/config.json` |
| Aider | [aider/](./aider/) | `/run` command + optional `CONVENTIONS.md` |

## Common prerequisites (any assistant)

1. Install the binary:
   ```bash
   go install github.com/alejandroSuch/review-replay/cmd/review-replay@latest
   ```
2. GitHub auth:
   ```bash
   export GITHUB_TOKEN="$(gh auth token)"
   ```
3. One LLM API key:
   ```bash
   export OPENROUTER_API_KEY=sk-or-v1-...
   # or OPENAI_API_KEY / ANTHROPIC_API_KEY / GEMINI_API_KEY
   ```

## What each integration does

All of them teach the assistant the same pattern:

- **Trigger phrases**: "did I address the reviews?", "re-check the PR", "verify Copilot's feedback", "before I ping the reviewer back".
- **Action**: resolve the current PR via `gh pr view --json url --jq .url`, then run `review-replay <url>` (or with `--check` / `--reviewer` based on intent).
- **Report**: surface the summary, the pending / partial entries, and the draft replies.

The difference between integrations is purely how each assistant discovers and invokes the rule — the underlying workflow is identical.

## Don't generate, don't implement

review-replay is the **verifier**. It does not:

- Generate review comments (use CodeRabbit, Copilot Review, Greptile).
- Implement fixes (use Claude Code skills, Cursor agent mode, Aider, Cursor / Codex agents).

Each integration explicitly tells the assistant not to confuse roles.

---
name: review-replay
description: Use this skill to verify whether reviewer concerns on a GitHub pull request were actually addressed by subsequent commits and replies. Trigger when the user says things like "did I address the reviews", "verify the PR feedback", "re-check the PR before pinging the reviewer", "did the address-reviews run actually work", or runs /review-replay. Also use proactively as a self-check after running address-reviews / similar fix-implementing skills.
---

# review-replay

`review-replay` is a CLI that classifies every review comment on a GitHub PR as `addressed`, `partial`, `pending`, or `needs-discussion`, based on the code at HEAD plus the thread replies. Use it whenever the user wants confirmation that PR feedback has landed.

## When to invoke

- **Self-check after fixing reviews**: right after running an `address-reviews` skill or any flow that modifies the PR in response to feedback. Run with `--check`; if it exits 1, iterate.
- **Pre-ping audit**: before the user tells a reviewer "all done", run review-replay to surface anything missed.
- **Targeted verification**: if the user names a specific reviewer ("did we cover Copilot's bullets?"), use `--reviewer <substring>`.
- **CI gate hint**: if the user is wiring this into branch protection, suggest the `--check` mode and the `alejandroSuch/review-replay/action@main` Action.

Do NOT invoke for: generating new review comments (that's CodeRabbit / Copilot Review), or for *implementing* fixes (that's address-reviews skills).

## Prerequisites (one-time)

```bash
# Binary
GOPROXY=direct go install github.com/alejandroSuch/review-replay/cmd/review-replay@main

# GitHub auth (one of)
export GITHUB_TOKEN="$(gh auth token)"

# LLM provider key (one of)
export OPENROUTER_API_KEY=sk-or-v1-...
export OPENAI_API_KEY=sk-...
export ANTHROPIC_API_KEY=sk-ant-...
export GEMINI_API_KEY=...
```

If the binary is missing, `go install` it before running. If no LLM key is present, ask the user which provider they want to use.

## How to run

1. Resolve the PR target. If the user didn't pass one, infer from the current branch:
   ```bash
   gh pr view --json url --jq .url
   ```
2. Run review-replay:
   ```bash
   review-replay <pr-url>
   ```
3. Read the verdict table and summary. Per-comment fields:
   - `status` — addressed / partial / pending / needs-discussion.
   - `source` — `rule` (short-circuit, no LLM call) or `llm`.
   - `evidence` — commit SHA that addresses the comment (when applicable).
   - `draft reply` — message the user can post; null when no action needed.

4. If the user wants a gate / strict pass, run with `--check` (exit 1 on any `pending`).

5. If the user wants to filter to one reviewer, append `--reviewer <substring>` (case-insensitive).

## What to report back to the user

- The summary line (`N addressed · M partial · K pending · L needs-discussion`).
- The list of `pending` and `partial` comments with their location and short body.
- The draft replies, so the user can copy them.
- If `--check` exited 1, suggest concrete next actions: iterate or override.

## Caveats to know

- `review-replay` reads the GitHub state at the moment of invocation. If the user just pushed a commit, give GitHub a few seconds before re-running.
- Confidence below 0.6 is suspicious — flag those cases to the user instead of treating the verdict as final.
- `--post` posts inline reply drafts directly to GitHub. Default is interactive (prompts `y/N/q` per draft). Use `--yes` to post all without prompting, or `--dry-run` to preview without touching the API. Only inline drafts are posted; review summaries and issue comments are skipped since they have no reply target.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `no GitHub token found` | Run `export GITHUB_TOKEN="$(gh auth token)"`. |
| `no API key for provider` | Set one of `OPENROUTER_API_KEY` / `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GEMINI_API_KEY`. |
| `Classifier output could not be parsed` | Re-run with `--model gpt-4o` or `--model anthropic/claude-haiku-4.5` for stricter JSON. |
| Output shows `0 inline (0 issue-level)` | The PR has no substantive review feedback. Nothing to verify. |

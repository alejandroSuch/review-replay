---
description: Verify reviewer concerns on a GitHub PR were addressed by later commits and replies. Uses the review-replay CLI.
---

You are running the `review-replay` slash command. The user wants to verify whether the reviewer concerns on a PR have been addressed.

## Steps

1. Identify the target PR. If the user passed an argument, use it. Otherwise infer from the current branch:
   ```bash
   gh pr view --json url --jq .url
   ```
2. Verify the binary is on PATH:
   ```bash
   command -v review-replay
   ```
   If missing, suggest:
   ```bash
   GOPROXY=direct go install github.com/alejandroSuch/review-replay/cmd/review-replay@main
   ```
3. Run the classifier:
   ```bash
   review-replay <pr-url>
   ```
   Add flags based on what the user asked for:
   - `--check` for CI-gate semantics (exit 1 on pending).
   - `--reviewer <substring>` to filter to one author (e.g. `copilot`).
   - `--json` for machine-parseable output.
   - `--no-llm` for evidence-only (no classifier calls).
4. Report back:
   - Summary line: `N addressed · M partial · K pending · L needs-discussion`.
   - The `pending` and `partial` entries with their location and short body.
   - The draft replies, so the user can copy them.
   - Confidence buckets: `high` / `medium` / `low`. Flag any `low` to the user.

## Don't

- Don't use `--post` unless the user explicitly asks. Posting writes to the PR.
- Don't trust verdicts with `low` confidence without surfacing the uncertainty.
- Don't try to also generate review comments — this tool only verifies existing ones.

## Prerequisites the user must have set

- `GITHUB_TOKEN` or `GH_TOKEN` (for read access to the PR).
- One LLM key: `OPENROUTER_API_KEY` / `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GEMINI_API_KEY`.

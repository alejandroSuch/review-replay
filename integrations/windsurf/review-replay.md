---
trigger: model_decision
description: Verify reviewer concerns on a GitHub PR were addressed by later commits and replies. Use when the user asks to re-check the PR before pinging the reviewer, or to verify a specific reviewer's feedback.
---

When the user wants to verify whether PR review feedback has been addressed, run `review-replay`.

## How

1. If `review-replay` is missing:
   ```bash
   GOPROXY=direct go install github.com/alejandroSuch/review-replay/cmd/review-replay@main
   ```
2. Ensure env: `GITHUB_TOKEN` plus one of `OPENROUTER_API_KEY` / `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GEMINI_API_KEY`.
3. Resolve the PR (current branch via `gh pr view --json url --jq .url` or use what the user gave).
4. Run `review-replay <pr-url>` for a report, or `review-replay <pr-url> --check` to gate on pending.
5. Add `--reviewer <substring>` to filter by author (e.g. `copilot`).

## Report back

- Summary line: `N addressed · M partial · K pending · L needs-discussion`.
- Pending / partial entries with location and body snippet.
- Draft replies for the user to copy.

## Don't

- Don't use `--post` — live posting is intentionally disabled.
- Don't trust verdicts with confidence < 0.6 without flagging.
- Don't use this tool to *generate* review feedback; it only verifies existing.

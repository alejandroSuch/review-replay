# review-replay

review-replay is a CLI tool that verifies whether reviewer concerns on a GitHub PR have been addressed by subsequent commits and replies. Use it whenever the user asks to verify, re-check, or audit PR review feedback.

## When to invoke

- The user asks "did I address the reviews?" or similar.
- The user is about to ping the reviewer back ("tell them I'm done") — verify first.
- After running any address-reviews flow, as a self-check.
- The user asks specifically about one reviewer ("did we cover Copilot's bullets?") — use `--reviewer <substring>`.

Do NOT use to:
- Generate new review comments (different lane: CodeRabbit, Copilot Review).
- Implement review fixes (different lane: code-edit agents).

## How to invoke

1. Ensure the binary is installed:
   ```bash
   go install github.com/alejandroSuch/review-replay/cmd/review-replay@latest
   ```
2. Set env if not already:
   ```bash
   export GITHUB_TOKEN="$(gh auth token)"
   # one LLM key:
   export OPENROUTER_API_KEY=sk-or-v1-...
   ```
3. Run:
   ```bash
   review-replay <pr-url-or-shortform>
   ```
   For the current branch's PR:
   ```bash
   review-replay "$(gh pr view --json url --jq .url)"
   ```
4. If the user wants a gate, add `--check` (exit 1 on `pending`).
5. If the user wants reviewer filter, add `--reviewer <substring>`.
6. For machine-parseable output, add `--json`.

## What to report

- The summary line (`N addressed · M partial · K pending · L needs-discussion`).
- The `pending` and `partial` entries with their location and body snippet.
- The draft replies, so the user can copy them.
- When `--check` exited 1, surface that as the blocker.

## Don't

- Don't pass `--post` without `--dry-run`. Live posting is intentionally disabled.
- Don't run repeatedly inside loops — each call costs LLM tokens.
- Don't trust verdicts with confidence < 0.6 without surfacing the uncertainty.

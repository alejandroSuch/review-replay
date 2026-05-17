# Aider — review-replay command

Aider supports shell commands inline via `/run`. There's no skill system; you just teach yourself the muscle memory.

## One-off invocation

Inside an aider session:

```
/run review-replay $(gh pr view --json url --jq .url)
```

Or with a gate:

```
/run review-replay $(gh pr view --json url --jq .url) --check
```

## Shell alias (recommended)

Add this to `~/.zshrc` / `~/.bashrc`:

```bash
alias rr='review-replay "$(gh pr view --json url --jq .url)"'
alias rr-check='review-replay "$(gh pr view --json url --jq .url)" --check'
```

Then in aider:

```
/run rr
/run rr-check
```

## Conventions file

If you want aider to suggest invoking it, drop a `CONVENTIONS.md` in your project root:

```markdown
# Conventions

When the user asks to verify PR feedback was addressed (e.g. "did I cover the reviews?", "re-check the PR before pinging"), run:

```bash
review-replay "$(gh pr view --json url --jq .url)"
```

Use `--check` for CI-gate semantics, `--reviewer <substring>` to filter by author.
```

Then invoke aider with:

```bash
aider --read CONVENTIONS.md
```

## Prerequisites

- `review-replay` binary on PATH.
- `GITHUB_TOKEN` env var.
- One LLM API key.

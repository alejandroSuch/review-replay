# Continue.dev — review-replay slash command

Continue lets you register shell-backed slash commands in `~/.continue/config.json` (or per-project `.continuerc.json`).

## Install

Merge `config.snippet.json` into your existing Continue config:

```jsonc
{
  // ...your existing config...
  "slashCommands": [
    // ...existing commands...
    {
      "name": "review-replay",
      "description": "Verify reviewer concerns on a GitHub PR were addressed",
      "step": "ShellCommandStep",
      "params": {
        "command": "review-replay \"$(gh pr view --json url --jq .url)\""
      }
    }
  ]
}
```

Then in the chat, type `/review-replay` and Continue will run the binary against the current branch's PR.

## Variants

Gate-mode (exit 1 on pending):
```json
"command": "review-replay \"$(gh pr view --json url --jq .url)\" --check"
```

Filter by reviewer:
```json
"command": "review-replay \"$(gh pr view --json url --jq .url)\" --reviewer copilot"
```

## Prerequisites

- `review-replay` binary on PATH.
- `GITHUB_TOKEN` env var.
- One LLM API key.

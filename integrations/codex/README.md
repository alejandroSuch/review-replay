# Codex CLI — review-replay agent instructions

Codex reads `AGENTS.md` files at the project root (and globally at `~/.codex/AGENTS.md`). Append the contents of `AGENTS.md` here to one of those files and Codex will know to invoke `review-replay` when the user asks about PR feedback.

## Install (global, all projects)

```bash
cat AGENTS.md >> ~/.codex/AGENTS.md
```

Create the file first if it doesn't exist:

```bash
mkdir -p ~/.codex
touch ~/.codex/AGENTS.md
cat AGENTS.md >> ~/.codex/AGENTS.md
```

## Install (single project)

```bash
cat AGENTS.md >> ./AGENTS.md
```

## Prerequisites

- `review-replay` binary on PATH:
  ```bash
  GOPROXY=direct go install github.com/alejandroSuch/review-replay/cmd/review-replay@main
  ```
- `GITHUB_TOKEN` env var.
- One LLM API key.

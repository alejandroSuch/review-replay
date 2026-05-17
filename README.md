# review-replay

> The CI for PR feedback. Did every comment get an answer?

`review-replay` verifies whether the concerns raised in a GitHub PR review have actually been addressed — by code changes, by explicit acknowledgments in the thread, or by the reviewer marking it resolved. It is **not** a review generator (that's CodeRabbit, Copilot Review). It is **not** a fix implementer (that's Claude Code, Cursor agent). It is the verifier that closes the loop.

```bash
$ review-replay owner/repo#42

owner/repo#42 · head a7c1e3d
3 addressed · 1 partial · 1 pending · 1 needs-discussion

#     │ kind   │ author      │ status         │ src   │ conf │ evidence │ comment                       │ draft reply
──────┼────────┼─────────────┼────────────────┼───────┼──────┼──────────┼───────────────────────────────┼────────────────────────
1     │ inline │ alice       │ addressed      │ rule  │ 0.95 │ -        │ extract this into a helper    │ -
2     │ inline │ alice       │ pending        │ rule  │ 0.95 │ -        │ this should be debounced      │ -
3     │ inline │ alice       │ addressed      │ llm   │ 0.92 │ e4f5g6h  │ rename `tmp` to descriptive   │ Renamed to pendingWriteBuffer.
...
```

## Why it exists

| Role | Tools | What it does |
|---|---|---|
| Generation | CodeRabbit, Copilot Review, Greptile | Generates review feedback |
| Implementation | Claude Code skills, Cursor agent, Ellipsis | Implements review fixes |
| **Verification** | **review-replay** | **Verifies the fixes actually resolve the comments** |

As more code gets written by agents that claim "I addressed it", you need an independent check. `review-replay` reads the PR conversation + current code state, classifies each comment, and gates the merge if anything is still pending.

## Install

```bash
go install github.com/alejandroSuch/review-replay/cmd/review-replay@latest
```

Binary lands in `$(go env GOPATH)/bin`. Add that to your `PATH` if it isn't already.

### Pinning a version (or installing from `main`)

`@latest` resolves to the most recent semver tag. If you want to follow the development branch (no tag is required), or you just pushed a fix and want it immediately, install from `main` and bypass the Go module proxy cache:

```bash
GOPROXY=direct go install github.com/alejandroSuch/review-replay/cmd/review-replay@main
```

Other useful pins:

```bash
go install github.com/alejandroSuch/review-replay/cmd/review-replay@v0.1.0   # tag
go install github.com/alejandroSuch/review-replay/cmd/review-replay@1e079c3  # commit
```

### Verify the installed build

```bash
which review-replay
go version -m "$(which review-replay)" | grep vcs.revision
```

The revision should match the commit you intended to install.

### From source

```bash
git clone https://github.com/alejandroSuch/review-replay
cd review-replay
go build -o review-replay ./cmd/review-replay
```

## GitHub auth

`review-replay` reads from `GITHUB_TOKEN` or `GH_TOKEN`. How you get one depends on where you're running it.

### Local use (CLI)

The simplest path is to reuse the token from your `gh` CLI session:

```bash
gh auth login    # one-time, browser flow
export GITHUB_TOKEN="$(gh auth token)"
```

That's enough for **read-only** runs (the default and `--check`). The default `gh auth` scopes already include what review-replay needs to read PR conversations.

### Personal Access Token (PAT)

For scripts, CI outside Actions, or shared environments, generate a **fine-grained PAT** at <https://github.com/settings/personal-access-tokens> with these repository permissions:

| Permission | Level | Needed for |
|---|---|---|
| Contents | Read | Commit metadata, file content at HEAD |
| Pull requests | Read | Reviews, threads, issue comments |
| Pull requests | **Read and write** | Only if you plan to use `--post` to reply to comments |
| Metadata | Read | (auto) |

Then:

```bash
export GITHUB_TOKEN="github_pat_..."
```

Classic PATs (`ghp_...`) also work; the relevant scope is `repo` (or `public_repo` for public repos only).

### GitHub Actions

You do **not** need to create a token. The runner provides one automatically:

```yaml
- uses: alejandroSuch/review-replay/action@main
  with:
    api-key: ${{ secrets.OPENROUTER_API_KEY }}
    # github-token defaults to ${{ github.token }}, no setup needed
```

The auto-provided token has read access to the PR by default. Make sure the job has:

```yaml
permissions:
  pull-requests: read
  contents: read
  # plus pull-requests: write if you ever enable --post
```

## Usage

```bash
review-replay <pr> [flags]
```

`<pr>` accepts a GitHub URL or `owner/repo#N`.

### Modes

| Flag | Behavior |
|---|---|
| (default) | Classify and print a report |
| `--check` | Exit 1 if any comment is classified `pending`. Use as a required CI check. |
| `--post` | Post draft replies to inline review threads. Default is interactive: prompts `y/N/q` per comment. Add `--yes` to skip prompts, `--dry-run` to preview without hitting the GitHub API. |
| `--no-llm` | Skip classification, only show deterministic evidence packets. |
| `--ping-llm` | Probe LLM provider config and exit. |
| `--json` | Emit full JSON output instead of the table. |

### LLM providers

| Provider | Default model | Notes |
|---|---|---|
| `openrouter` (default) | `openai/gpt-4o-mini` | OpenAI-compatible wire format, broadest model catalog. |
| `openai` | `gpt-4o-mini` | Official OpenAI endpoint. |
| `anthropic` | `claude-haiku-4-5` | Native Messages API (not OpenAI-compatible). |
| `gemini` | `gemini-2.5-flash` | Native generateContent API. |

For any **OpenAI-compatible** endpoint (Together, Groq, Fireworks, vLLM, LM Studio, Ollama via `/v1`, etc.), use `--provider openai --base-url <your-endpoint>`.

### Env vars

| Variable | Purpose |
|---|---|
| `GITHUB_TOKEN` / `GH_TOKEN` | GitHub auth (required) |
| `RR_PROVIDER` | Default provider (overridden by `--provider`) |
| `RR_MODEL` | Default model (overridden by `--model`) |
| `RR_API_KEY` | Generic LLM API key |
| `RR_BASE_URL` | Custom base URL for OpenAI-compatible endpoints |
| `OPENROUTER_API_KEY` / `OPENAI_API_KEY` / `ANTHROPIC_API_KEY` / `GEMINI_API_KEY` | Provider-specific fallbacks |

### Filter by reviewer

Block merge only on a specific reviewer (e.g., Copilot, your tech lead):

```bash
review-replay owner/repo#42 --reviewer copilot --check
```

Case-insensitive substring match on author login.

## How it works

1. **Fetch** the PR via the GitHub GraphQL + REST APIs: review threads (with `isResolved` and `resolvedBy`), review summaries, issue comments, commits.
2. **Build deterministic evidence packets** per comment:
   - For inline comments: did the region change at HEAD? Which later commits touched the file? Are there inline replies? Is the thread resolved by the opener?
   - For review summaries and issue comments: which commits and replies came after? Filter chitchat (`LGTM`, `thanks`, emoji-only) and PR-author self-reviews. Dedupe near-identical bodies.
3. **Short-circuit rules** (no LLM call):
   - Thread resolved by the reviewer who opened it → `addressed`.
   - No region change, no replies, no later commits → `pending`.
4. **Classify the rest** with your chosen LLM provider.
5. **Report**: per-comment status + confidence + evidence commit + draft reply, plus a summary line.

## GitHub Action

```yaml
- uses: alejandroSuch/review-replay/action@main
  with:
    mode: check
    api-key: ${{ secrets.OPENROUTER_API_KEY }}
```

Make the job a required check to block merge on `pending` comments.

See [`action/examples/copilot-followup.yml`](./action/examples/copilot-followup.yml) for a workflow that only blocks on Copilot's reviews.

## AI assistant integrations

Drop-in skills/rules so Claude Code, Cursor, Codex CLI, Windsurf, Continue.dev or Aider can invoke `review-replay` when you ask them to verify PR feedback.

See [`integrations/`](./integrations/) for ready-to-copy files per tool.

## Eval harness

Capture fixtures from real PRs, label by hand, run the classifier, compare runs across model swaps.

```bash
# install the eval binary
go install github.com/alejandroSuch/review-replay/cmd/review-replay-eval@latest

# capture fixtures (preserves any existing labels)
review-replay-eval capture owner/repo#42

# label the JSON files by hand: set label.status to addressed | partial | pending | needs-discussion

# run the classifier and save the run
review-replay-eval run --save baseline

# swap model and compare
RR_MODEL=anthropic/claude-haiku-4-5 review-replay-eval run --save haiku
review-replay-eval diff baseline haiku
```

## Scope

`review-replay` processes:
- **Inline review threads** (comments anchored to a line of code).
- **Review summary bodies** (the text of an Approve / Request changes review).
- **Structured issue comments** (general PR comments with substantive content; chitchat is filtered).

Out of scope: GitHub Discussions, commit comments, file-level non-line comments, GitLab/Bitbucket.

## License

MIT.

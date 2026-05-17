// review-replay verifies whether reviewer concerns on a GitHub PR were
// addressed by subsequent commits and replies.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alejandroSuch/review-replay/internal/evidence"
	"github.com/alejandroSuch/review-replay/internal/github"
	"github.com/alejandroSuch/review-replay/internal/llm"
	"github.com/alejandroSuch/review-replay/internal/pipeline"
	"github.com/alejandroSuch/review-replay/internal/render"
	"github.com/alejandroSuch/review-replay/internal/types"
)

const usage = `review-replay <pr> [flags]

Arguments:
  <pr>             PR URL (https://github.com/owner/repo/pull/N) or owner/repo#N

Modes:
  (default)        Classify and print a report
  --check          Exit 1 if any comment is classified 'pending' (CI gate)
  --post           Post draft replies. Without --yes, prints them and asks per comment
  --no-llm         Skip classification, only show evidence
  --ping-llm       Probe LLM provider config and exit
  --json           Print full JSON result

Flags:
  -y, --yes        Skip confirmation prompt for --post
  --dry-run        For --post: never call the GitHub API even with --yes
  --provider <p>   openrouter | openai | anthropic | gemini
  --model <m>      Model id (overrides provider default)
  --api-key <k>    API key (overrides env)
  --base-url <u>   Base URL (for OpenAI-compatible custom endpoints)
  --reviewer <u>   Only classify comments whose author matches (case-insensitive substring)
  -h, --help       Show this message

Env vars:
  GITHUB_TOKEN / GH_TOKEN     — GitHub auth (required)
  RR_PROVIDER, RR_MODEL,
  RR_API_KEY, RR_BASE_URL     — provider config
  OPENROUTER_API_KEY,
  OPENAI_API_KEY,
  ANTHROPIC_API_KEY,
  GEMINI_API_KEY              — provider-specific fallbacks
`

type args struct {
	pr       string
	jsonOut  bool
	noLLM    bool
	pingLLM  bool
	check    bool
	post     bool
	yes      bool
	dryRun   bool
	provider string
	model    string
	apiKey   string
	baseURL  string
	reviewer string
	help     bool
}

func parseArgs() (args, error) {
	fs := flag.NewFlagSet("review-replay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var a args
	fs.BoolVar(&a.jsonOut, "json", false, "")
	fs.BoolVar(&a.noLLM, "no-llm", false, "")
	fs.BoolVar(&a.pingLLM, "ping-llm", false, "")
	fs.BoolVar(&a.check, "check", false, "")
	fs.BoolVar(&a.post, "post", false, "")
	fs.BoolVar(&a.yes, "yes", false, "")
	fs.BoolVar(&a.yes, "y", false, "")
	fs.BoolVar(&a.dryRun, "dry-run", false, "")
	fs.StringVar(&a.provider, "provider", "", "")
	fs.StringVar(&a.model, "model", "", "")
	fs.StringVar(&a.apiKey, "api-key", "", "")
	fs.StringVar(&a.baseURL, "base-url", "", "")
	fs.StringVar(&a.reviewer, "reviewer", "", "")
	fs.BoolVar(&a.help, "help", false, "")
	fs.BoolVar(&a.help, "h", false, "")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return a, err
	}
	if a.help {
		return a, nil
	}
	if a.pingLLM {
		return a, nil
	}
	rest := fs.Args()
	if len(rest) < 1 {
		return a, errors.New("missing PR argument")
	}
	a.pr = rest[0]
	return a, nil
}

func main() {
	a, err := parseArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(1)
	}
	if a.help {
		fmt.Print(usage)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if a.pingLLM {
		os.Exit(pingLLM(ctx, a))
	}

	if a.post && a.noLLM {
		fmt.Fprintln(os.Stderr, "Error: --post requires classification (--no-llm conflicts).")
		os.Exit(1)
	}

	pr, err := github.ParsePrRef(a.pr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ghClient, err := github.NewClient(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	builder := evidence.New(ghClient)

	res, err := pipeline.Run(ctx, pipeline.Options{
		Client:   ghClient,
		Builder:  builder,
		PR:       pr,
		Reviewer: a.reviewer,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}

	if len(res.Inline) == 0 && len(res.IssueLevel) == 0 {
		if a.jsonOut {
			writeJSON(map[string]any{"snapshot": res.Snapshot, "inline": res.Inline, "issueLevel": res.IssueLevel})
		} else {
			fmt.Print(render.NothingToClassify(res.Snapshot))
		}
		return
	}

	if a.noLLM {
		if a.jsonOut {
			writeJSON(res)
		} else {
			fmt.Print(render.Evidence(res.Snapshot, res.Inline, res.IssueLevel))
		}
		return
	}

	provider, resolved, err := llm.Resolve(llm.Config{
		Provider: a.provider,
		Model:    a.model,
		APIKey:   a.apiKey,
		BaseURL:  a.baseURL,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "\x1b[2musing %s/%s\x1b[0m\n", resolved.Provider, resolved.Model)
	if a.reviewer != "" {
		fmt.Fprintf(os.Stderr, "\x1b[2mfilter: only comments by %q (matched %d)\x1b[0m\n", a.reviewer, len(res.Inline)+len(res.IssueLevel))
	}

	classified, err := pipeline.Classify(ctx, res, pipeline.ClassifyOptions{
		LLM:   provider,
		Model: resolved.Model,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}

	if a.jsonOut {
		writeJSON(classified)
	} else {
		fmt.Print(render.Report(res.Snapshot, res.Inline, res.IssueLevel, classified.Outcomes))
	}

	exitCode := 0
	if a.check {
		pending := 0
		for _, o := range classified.Outcomes {
			if o.Classification.Status == types.StatusPending {
				pending++
			}
		}
		if pending > 0 {
			fmt.Fprintf(os.Stderr, "\n--check: %d pending comment(s); failing.\n", pending)
			exitCode = 1
		} else {
			fmt.Fprintln(os.Stderr, "\n--check: 0 pending. ok.")
		}
	}

	if a.post {
		code := postDrafts(ctx, ghClient, pr, classified.Outcomes, a.yes, a.dryRun)
		if code != 0 {
			exitCode = code
		}
	}

	os.Exit(exitCode)
}

func pingLLM(ctx context.Context, a args) int {
	provider, resolved, err := llm.Resolve(llm.Config{
		Provider: a.provider,
		Model:    a.model,
		APIKey:   a.apiKey,
		BaseURL:  a.baseURL,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ping failed:", err)
		return 2
	}
	fmt.Printf("provider: %s\nmodel: %s\napiKey: source=%s length=%d\napiBase: %s\n",
		resolved.Provider, resolved.Model, resolved.APIKeySource, resolved.APIKeyLen, resolved.BaseURL)
	start := time.Now()
	out, err := provider.Complete(ctx, llm.Request{
		System:    "Reply with the single word OK.",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: "ping"}},
		Model:     resolved.Model,
		MaxTokens: 8,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ping failed:", err)
		return 2
	}
	fmt.Printf("response: %s\nlatency: %dms\nusage: in=%d out=%d\n",
		strings.TrimSpace(out.Text), time.Since(start).Milliseconds(), out.Usage.InputTokens, out.Usage.OutputTokens)
	return 0
}

func postDrafts(ctx context.Context, _ *github.Client, _ types.PrRef, outcomes []types.Outcome, yes, dryRun bool) int {
	candidates := make([]types.Outcome, 0)
	for _, o := range outcomes {
		if o.Classification.DraftReply != nil && strings.TrimSpace(*o.Classification.DraftReply) != "" {
			candidates = append(candidates, o)
		}
	}
	if len(candidates) == 0 {
		fmt.Println("\n--post: no draft replies to post.")
		return 0
	}
	fmt.Printf("\n--post: %d draft replies.\n", len(candidates))

	reader := bufio.NewReader(os.Stdin)
	posted, skipped, failed := 0, 0, 0
	for _, o := range candidates {
		fmt.Printf("\ncomment %d [%s]:\n  %s\n", o.Classification.CommentID, o.Classification.Status, *o.Classification.DraftReply)
		approve := yes
		if !yes {
			fmt.Print("post? [y/N/q] ")
			line, _ := reader.ReadString('\n')
			ans := strings.ToLower(strings.TrimSpace(line))
			if ans == "q" {
				fmt.Println("aborted by user.")
				break
			}
			approve = ans == "y" || ans == "yes"
		}
		if !approve {
			skipped++
			continue
		}
		if dryRun {
			fmt.Println("  [dry-run] would post.")
			posted++
			continue
		}
		// Phase 2: posting against the real GitHub API is intentionally
		// disabled while we still verify the classifier. Use --dry-run.
		fmt.Println("  [error] live posting not enabled yet; rerun with --dry-run.")
		failed++
	}
	fmt.Printf("\n--post summary: %d posted, %d skipped, %d failed.\n", posted, skipped, failed)
	if failed > 0 {
		return 1
	}
	return 0
}

func writeJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

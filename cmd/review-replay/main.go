// review-replay verifies whether reviewer concerns on a GitHub PR were
// addressed by subsequent commits and replies.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/alejandroSuch/review-replay/internal/evidence"
	"github.com/alejandroSuch/review-replay/internal/github"
	"github.com/alejandroSuch/review-replay/internal/pipeline"
	"github.com/alejandroSuch/review-replay/internal/render"
)

const usage = `review-replay <pr> [flags]

Arguments:
  <pr>             PR URL (https://github.com/owner/repo/pull/N) or owner/repo#N

Flags:
  --no-llm         Skip classification, only show evidence (only mode in phase 1)
  --json           Print full JSON result
  --reviewer <u>   Only classify comments whose author matches (case-insensitive substring)
  -h, --help       Show this message

Auth:
  Set GITHUB_TOKEN or GH_TOKEN in env.
`

type args struct {
	pr       string
	jsonOut  bool
	noLLM    bool
	reviewer string
	help     bool
}

func parseArgs() (args, error) {
	fs := flag.NewFlagSet("review-replay", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var a args
	fs.BoolVar(&a.jsonOut, "json", false, "")
	fs.BoolVar(&a.noLLM, "no-llm", false, "")
	fs.StringVar(&a.reviewer, "reviewer", "", "")
	fs.BoolVar(&a.help, "help", false, "")
	fs.BoolVar(&a.help, "h", false, "")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return a, err
	}
	if a.help {
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

	// Phase 1: --no-llm is implicit (LLM not implemented yet).
	if !a.noLLM {
		fmt.Fprintln(os.Stderr, "phase 1 only supports --no-llm; running evidence-only")
	}

	pr, err := github.ParsePrRef(a.pr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client, err := github.NewClient(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	builder := evidence.New(client)
	res, err := pipeline.Run(ctx, pipeline.Options{
		Client:   client,
		Builder:  builder,
		PR:       pr,
		Reviewer: a.reviewer,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}

	if a.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		return
	}

	if len(res.Inline) == 0 && len(res.IssueLevel) == 0 {
		fmt.Print(render.NothingToClassify(res.Snapshot))
		return
	}
	fmt.Print(render.Evidence(res.Snapshot, res.Inline, res.IssueLevel))
}

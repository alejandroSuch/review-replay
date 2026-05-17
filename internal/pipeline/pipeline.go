// Package pipeline orchestrates the snapshot fetch + evidence build.
package pipeline

import (
	"context"

	"github.com/alejandroSuch/review-replay/internal/evidence"
	"github.com/alejandroSuch/review-replay/internal/github"
	"github.com/alejandroSuch/review-replay/internal/types"
)

// Result aggregates everything we know about a PR before classification.
type Result struct {
	Snapshot   *types.PrSnapshot
	Inline     []types.CommentEvidence
	IssueLevel []types.IssueLevelEvidence
}

// Options control which steps run.
type Options struct {
	Client   *github.Client
	Builder  *evidence.Builder
	PR       types.PrRef
	Reviewer string // case-insensitive substring filter on author; empty = no filter
}

// Run fetches the snapshot and builds evidence packets. No LLM yet.
func Run(ctx context.Context, opts Options) (*Result, error) {
	snap, err := opts.Client.FetchPrSnapshot(ctx, opts.PR)
	if err != nil {
		return nil, err
	}
	inline, issueLevel, err := opts.Builder.BuildAll(ctx, snap)
	if err != nil {
		return nil, err
	}
	if opts.Reviewer != "" {
		inline = filterInlineByReviewer(inline, opts.Reviewer)
		issueLevel = filterIssueByReviewer(issueLevel, opts.Reviewer)
	}
	return &Result{Snapshot: snap, Inline: inline, IssueLevel: issueLevel}, nil
}

func filterInlineByReviewer(in []types.CommentEvidence, needle string) []types.CommentEvidence {
	out := in[:0]
	needle = lower(needle)
	for _, e := range in {
		if contains(lower(e.Comment.Author), needle) {
			out = append(out, e)
		}
	}
	return out
}

func filterIssueByReviewer(in []types.IssueLevelEvidence, needle string) []types.IssueLevelEvidence {
	out := in[:0]
	needle = lower(needle)
	for _, e := range in {
		if contains(lower(e.Comment.Author), needle) {
			out = append(out, e)
		}
	}
	return out
}

// avoid pulling strings just for ToLower/Contains in a tight loop helper.
func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func contains(hay, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(hay) {
		return false
	}
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alejandroSuch/review-replay/internal/classifier"
	"github.com/alejandroSuch/review-replay/internal/llm"
	"github.com/alejandroSuch/review-replay/internal/types"
)

// ClassifyOptions configures the classification phase.
type ClassifyOptions struct {
	LLM       llm.Provider
	Model     string
	MaxTokens int
	// Parallelism caps the number of in-flight LLM calls. Defaults to 4.
	Parallelism int
}

// Outcome is a per-comment classification with its provenance.
type Outcome = types.Outcome

// Classified is a Result plus the per-comment outcomes and a final report.
type Classified struct {
	*Result
	Outcomes []Outcome
	Report   types.Report
}

// Classify runs the LLM (and short-circuit rules) over the evidence packets in
// a Result. Inline and issue-level both go through the same provider; the
// difference is the system prompt used by the classifier.
func Classify(ctx context.Context, r *Result, opts ClassifyOptions) (*Classified, error) {
	if opts.Parallelism <= 0 {
		opts.Parallelism = 4
	}
	if opts.LLM == nil {
		// Allow rule-only runs: inline gets short-circuits, issue-level errors.
		if len(r.IssueLevel) > 0 {
			return nil, fmt.Errorf("issue-level evidence requires an LLM provider")
		}
	}

	outcomes := make([]Outcome, len(r.Inline)+len(r.IssueLevel))
	jobs := make(chan func() error)
	errs := make(chan error, len(outcomes))
	var wg sync.WaitGroup
	for i := 0; i < opts.Parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				if err := job(); err != nil {
					errs <- err
				}
			}
		}()
	}

	for i, e := range r.Inline {
		i, e := i, e
		jobs <- func() error {
			res, err := classifier.ClassifyComment(ctx, e, classifier.Options{
				LLM: opts.LLM, Model: opts.Model, MaxTokens: opts.MaxTokens,
			})
			if err != nil {
				return err
			}
			outcomes[i] = Outcome{
				Classification:   res.Classification,
				Source:           res.Diagnostics.Source,
				RuleName:         res.Diagnostics.RuleName,
				InputTokens:      res.Diagnostics.Usage.InputTokens,
				OutputTokens:     res.Diagnostics.Usage.OutputTokens,
				CacheReadTokens:  res.Diagnostics.Usage.CacheReadTokens,
				CacheWriteTokens: res.Diagnostics.Usage.CacheWriteTokens,
			}
			return nil
		}
	}
	for i, e := range r.IssueLevel {
		i, e := i, e
		offset := len(r.Inline) + i
		jobs <- func() error {
			res, err := classifier.ClassifyIssueLevel(ctx, e, classifier.Options{
				LLM: opts.LLM, Model: opts.Model, MaxTokens: opts.MaxTokens,
			})
			if err != nil {
				return err
			}
			outcomes[offset] = Outcome{
				Classification:   res.Classification,
				Source:           res.Diagnostics.Source,
				RuleName:         res.Diagnostics.RuleName,
				InputTokens:      res.Diagnostics.Usage.InputTokens,
				OutputTokens:     res.Diagnostics.Usage.OutputTokens,
				CacheReadTokens:  res.Diagnostics.Usage.CacheReadTokens,
				CacheWriteTokens: res.Diagnostics.Usage.CacheWriteTokens,
			}
			return nil
		}
	}
	close(jobs)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}

	summary := map[types.ClassificationStatus]int{
		types.StatusAddressed:       0,
		types.StatusPartial:         0,
		types.StatusPending:         0,
		types.StatusNeedsDiscussion: 0,
	}
	classifications := make([]types.Classification, len(outcomes))
	usage := types.UsageTotals{}
	for i, o := range outcomes {
		classifications[i] = o.Classification
		summary[o.Classification.Status]++
		usage.InputTokens += o.InputTokens
		usage.OutputTokens += o.OutputTokens
		usage.CacheReadTokens += o.CacheReadTokens
		usage.CacheWriteTokens += o.CacheWriteTokens
		if o.Source == types.SourceLLM {
			usage.LLMCalls++
		} else {
			usage.RuleCalls++
		}
	}
	if est, ok := estimateUSD(opts.Model, usage.InputTokens, usage.OutputTokens); ok {
		usage.EstimatedUSD = est
		usage.PriceModel = opts.Model
	}
	report := types.Report{
		PR:              r.Snapshot.PR,
		GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
		Classifications: classifications,
		Summary:         summary,
		Usage:           usage,
	}
	return &Classified{Result: r, Outcomes: outcomes, Report: report}, nil
}

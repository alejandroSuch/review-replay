// Package classifier maps deterministic evidence packets to per-comment
// classifications via short-circuit rules and an LLM provider.
package classifier

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alejandroSuch/review-replay/internal/filters"
	"github.com/alejandroSuch/review-replay/internal/llm"
	"github.com/alejandroSuch/review-replay/internal/prompts"
	"github.com/alejandroSuch/review-replay/internal/types"
)

// Options configures a single classification call.
type Options struct {
	LLM       llm.Provider
	Model     string
	MaxTokens int
}

// Diagnostics carries provenance metadata about a classification.
type Diagnostics struct {
	Source     types.ClassificationSource
	RuleName   string
	RawText    string
	ParseError string
}

// Result wraps a classification with its diagnostics.
type Result struct {
	Classification types.Classification
	Diagnostics    Diagnostics
}

// ClassifyComment classifies a single inline comment evidence packet.
func ClassifyComment(ctx context.Context, e types.CommentEvidence, opts Options) (*Result, error) {
	if hit := applyShortCircuit(e); hit != nil {
		return &Result{
			Classification: hit.classification,
			Diagnostics:    Diagnostics{Source: types.SourceRule, RuleName: hit.ruleName},
		}, nil
	}
	if opts.LLM == nil {
		return nil, fmt.Errorf("classifier: no LLM provider configured and no short-circuit match")
	}

	user := prompts.InlineUser(e)
	resp, err := opts.LLM.Complete(ctx, llm.Request{
		System:    prompts.SystemInline,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: user}},
		Model:     opts.Model,
		MaxTokens: defaultInt(opts.MaxTokens, 600),
		JSON:      true,
	})
	if err != nil {
		return nil, err
	}
	parsed, perr := parseClassification(resp.Text)
	if perr != nil {
		return &Result{
			Classification: fallback(e.Comment.ID, types.OriginInline),
			Diagnostics: Diagnostics{
				Source:     types.SourceLLM,
				RawText:    resp.Text,
				ParseError: perr.Error(),
			},
		}, nil
	}
	return &Result{
		Classification: types.Classification{
			CommentID:         e.Comment.ID,
			Origin:            types.OriginInline,
			Status:            parsed.Status,
			EvidenceCommitSha: parsed.EvidenceCommitSha,
			DraftReply:        parsed.DraftReply,
			Confidence:        parsed.Confidence,
			Rationale:         parsed.Rationale,
		},
		Diagnostics: Diagnostics{Source: types.SourceLLM, RawText: resp.Text},
	}, nil
}

// ClassifyIssueLevel classifies a review summary or issue comment evidence
// packet.
func ClassifyIssueLevel(ctx context.Context, e types.IssueLevelEvidence, opts Options) (*Result, error) {
	if opts.LLM == nil {
		return nil, fmt.Errorf("classifier: no LLM provider configured for issue-level classification")
	}
	user := prompts.IssueLevelUser(e)
	resp, err := opts.LLM.Complete(ctx, llm.Request{
		System:    prompts.SystemIssueLevel,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: user}},
		Model:     opts.Model,
		MaxTokens: defaultInt(opts.MaxTokens, 600),
		JSON:      true,
	})
	if err != nil {
		return nil, err
	}
	origin := types.OriginReviewSummary
	if e.Comment.Kind == types.KindIssueComment {
		origin = types.OriginIssueComment
	}
	parsed, perr := parseClassification(resp.Text)
	if perr != nil {
		return &Result{
			Classification: fallback(e.Comment.ID, origin),
			Diagnostics: Diagnostics{
				Source:     types.SourceLLM,
				RawText:    resp.Text,
				ParseError: perr.Error(),
			},
		}, nil
	}
	return &Result{
		Classification: types.Classification{
			CommentID:         e.Comment.ID,
			Origin:            origin,
			Status:            parsed.Status,
			EvidenceCommitSha: parsed.EvidenceCommitSha,
			DraftReply:        parsed.DraftReply,
			Confidence:        parsed.Confidence,
			Rationale:         parsed.Rationale,
		},
		Diagnostics: Diagnostics{Source: types.SourceLLM, RawText: resp.Text},
	}, nil
}

type ruleHit struct {
	classification types.Classification
	ruleName       string
}

// ApplyShortCircuit is exposed for tests.
func ApplyShortCircuit(e types.CommentEvidence) (string, *types.Classification) {
	hit := applyShortCircuit(e)
	if hit == nil {
		return "", nil
	}
	return hit.ruleName, &hit.classification
}

func applyShortCircuit(e types.CommentEvidence) *ruleHit {
	if e.ResolvedByThreadOpener {
		rationale := "Thread marked resolved by the reviewer who opened it."
		if e.ResolvedByLogin != nil {
			rationale = fmt.Sprintf("Thread marked resolved by %s (the reviewer who opened it).", *e.ResolvedByLogin)
		}
		return &ruleHit{
			ruleName: "resolved-by-opener",
			classification: types.Classification{
				CommentID:  e.Comment.ID,
				Origin:     types.OriginInline,
				Status:     types.StatusAddressed,
				Confidence: types.ConfidenceHigh,
				Rationale:  rationale,
			},
		}
	}
	if e.Resolved && filters.IsBotAuthor(e.Comment.Author) {
		resolver := "a maintainer"
		if e.ResolvedByLogin != nil {
			resolver = *e.ResolvedByLogin
		}
		return &ruleHit{
			ruleName: "resolved-bot-thread",
			classification: types.Classification{
				CommentID:  e.Comment.ID,
				Origin:     types.OriginInline,
				Status:     types.StatusAddressed,
				Confidence: types.ConfidenceHigh,
				Rationale:  fmt.Sprintf("Bot-authored thread marked resolved by %s (bots do not reply to confirm; the resolve click is the final ack).", resolver),
			},
		}
	}
	if !e.RegionChanged && len(e.ChangedInCommits) == 0 && len(e.ThreadReplies) == 0 && !e.Resolved {
		return &ruleHit{
			ruleName: "no-signal",
			classification: types.Classification{
				CommentID:  e.Comment.ID,
				Origin:     types.OriginInline,
				Status:     types.StatusPending,
				Confidence: types.ConfidenceHigh,
				Rationale:  "No code changes touched the path and no thread replies exist.",
			},
		}
	}
	return nil
}

type parsedClassification struct {
	Status            types.ClassificationStatus `json:"status"`
	EvidenceCommitSha *string                    `json:"evidenceCommitSha"`
	DraftReply        *string                    `json:"draftReply"`
	Confidence        types.ConfidenceLevel      `json:"confidence"`
	Rationale         string                     `json:"rationale"`
}

// ParseClassification is exposed for tests.
func ParseClassification(text string) (*parsedClassification, error) {
	return parseClassification(text)
}

func parseClassification(text string) (*parsedClassification, error) {
	stripped := stripFences(strings.TrimSpace(text))
	var p parsedClassification
	if err := json.Unmarshal([]byte(stripped), &p); err == nil {
		if err := validate(&p); err != nil {
			return nil, err
		}
		return &p, nil
	}
	// Best-effort: find the first {...} block.
	start := strings.Index(stripped, "{")
	end := strings.LastIndex(stripped, "}")
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(stripped[start:end+1]), &p); err == nil {
			if err := validate(&p); err != nil {
				return nil, err
			}
			return &p, nil
		}
	}
	return nil, fmt.Errorf("could not parse JSON classification")
}

func stripFences(text string) string {
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}

var validStatuses = map[types.ClassificationStatus]bool{
	types.StatusAddressed:       true,
	types.StatusPartial:         true,
	types.StatusPending:         true,
	types.StatusNeedsDiscussion: true,
}

var validConfidence = map[types.ConfidenceLevel]bool{
	types.ConfidenceHigh:   true,
	types.ConfidenceMedium: true,
	types.ConfidenceLow:    true,
}

func validate(p *parsedClassification) error {
	if !validStatuses[p.Status] {
		return fmt.Errorf("invalid status %q", p.Status)
	}
	if !validConfidence[p.Confidence] {
		return fmt.Errorf("invalid confidence %q (expected high | medium | low)", p.Confidence)
	}
	return nil
}

func fallback(commentID int64, origin types.CommentOrigin) types.Classification {
	return types.Classification{
		CommentID:  commentID,
		Origin:     origin,
		Status:     types.StatusNeedsDiscussion,
		Confidence: types.ConfidenceLow,
		Rationale:  "Classifier output could not be parsed; flagged for human review.",
	}
}

func defaultInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

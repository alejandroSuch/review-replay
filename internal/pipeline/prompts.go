package pipeline

import (
	"fmt"

	"github.com/alejandroSuch/review-replay/internal/classifier"
	"github.com/alejandroSuch/review-replay/internal/prompts"
	"github.com/alejandroSuch/review-replay/internal/types"
)

// PromptBundle is the --emit-prompts output: everything a host that owns the
// model call (e.g. a Claude Code mod) needs to classify a PR itself, with the
// same rules and prompts the CLI uses. Field names are a stable JSON contract.
type PromptBundle struct {
	PR               string       `json:"pr"`
	URL              string       `json:"url"`
	HeadSHA          string       `json:"headSha"`
	PRAuthor         string       `json:"prAuthor"`
	Commits          int          `json:"commits"`
	HistoryRewritten bool         `json:"historyRewritten"`
	Items            []PromptItem `json:"items"`
}

// PromptItem is one comment to classify. Exactly one of Rule or (System, User)
// is set: Rule when a short-circuit already decided it, prompts otherwise.
type PromptItem struct {
	CommentID int64  `json:"commentId"`
	Origin    string `json:"origin"`
	Author    string `json:"author"`
	Path      string `json:"path,omitempty"`
	Line      *int   `json:"line,omitempty"`
	Body      string `json:"body"`
	HTMLURL   string `json:"htmlUrl"`
	State     string `json:"reviewState,omitempty"`
	Rule      *Ruled `json:"rule,omitempty"`
	System    string `json:"system,omitempty"`
	User      string `json:"user,omitempty"`
}

// Ruled is a short-circuit verdict.
type Ruled struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Confidence string `json:"confidence"`
	Rationale  string `json:"rationale"`
}

// Prompts builds the PromptBundle for a Result without calling any model.
func Prompts(r *Result) PromptBundle {
	s := r.Snapshot
	b := PromptBundle{
		PR:               fmt.Sprintf("%s/%s#%d", s.PR.Owner, s.PR.Repo, s.PR.Number),
		URL:              fmt.Sprintf("https://github.com/%s/%s/pull/%d", s.PR.Owner, s.PR.Repo, s.PR.Number),
		HeadSHA:          s.HeadSHA,
		PRAuthor:         s.PRAuthor,
		Commits:          len(s.Commits),
		HistoryRewritten: s.HistoryRewritten,
		Items:            make([]PromptItem, 0, len(r.Inline)+len(r.IssueLevel)),
	}
	for _, e := range r.Inline {
		c := e.Comment
		item := PromptItem{
			CommentID: c.ID,
			Origin:    string(types.OriginInline),
			Author:    c.Author,
			Path:      c.Path,
			Line:      c.Line,
			Body:      c.Body,
			HTMLURL:   c.HTMLURL,
		}
		if name, cl := classifier.ApplyShortCircuit(e); cl != nil {
			item.Rule = &Ruled{
				Name:       name,
				Status:     string(cl.Status),
				Confidence: string(cl.Confidence),
				Rationale:  cl.Rationale,
			}
		} else {
			item.System = prompts.SystemInline
			item.User = prompts.InlineUser(e, s.HistoryRewritten)
		}
		b.Items = append(b.Items, item)
	}
	for _, e := range r.IssueLevel {
		c := e.Comment
		origin := types.OriginReviewSummary
		if c.Kind == types.KindIssueComment {
			origin = types.OriginIssueComment
		}
		item := PromptItem{
			CommentID: c.ID,
			Origin:    string(origin),
			Author:    c.Author,
			Body:      c.Body,
			HTMLURL:   c.HTMLURL,
			System:    prompts.SystemIssueLevel,
			User:      prompts.IssueLevelUser(e, s.HistoryRewritten),
		}
		if c.ReviewState != nil {
			item.State = string(*c.ReviewState)
		}
		b.Items = append(b.Items, item)
	}
	return b
}

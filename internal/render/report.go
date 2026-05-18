package render

import (
	"fmt"
	"strings"

	"github.com/alejandroSuch/review-replay/internal/types"
)

// Report formats the full classified report with per-comment status, source
// and draft replies. inline + issueLevel are passed so we can look up the
// original body for the comment column.
func Report(
	snap *types.PrSnapshot,
	inline []types.CommentEvidence,
	issueLevel []types.IssueLevelEvidence,
	outcomes []types.Outcome,
) string {
	inlineByID := make(map[int64]types.CommentEvidence, len(inline))
	for _, e := range inline {
		inlineByID[e.Comment.ID] = e
	}
	issueByID := make(map[int64]types.IssueLevelEvidence, len(issueLevel))
	for _, e := range issueLevel {
		issueByID[e.Comment.ID] = e
	}

	cols := []column{
		{"#", 6},
		{"kind", 8},
		{"author", 14},
		{"status", 16},
		{"src", 5},
		{"conf", 6},
		{"evidence", 9},
		{"comment", 32},
		{"draft reply", 32},
	}
	rows := make([][]string, 0, len(outcomes))
	for _, o := range outcomes {
		author := "?"
		body := ""
		if ev, ok := inlineByID[o.Classification.CommentID]; ok {
			author = ev.Comment.Author
			body = oneLine(ev.Comment.Body)
		} else if ev, ok := issueByID[o.Classification.CommentID]; ok {
			author = ev.Comment.Author
			body = oneLine(ev.Comment.Body)
		}
		evidence := "-"
		if o.Classification.EvidenceCommitSha != nil {
			evidence = short(*o.Classification.EvidenceCommitSha, 7)
		}
		draft := "-"
		if o.Classification.DraftReply != nil && *o.Classification.DraftReply != "" {
			draft = *o.Classification.DraftReply
		}
		rows = append(rows, []string{
			short(fmt.Sprintf("%d", o.Classification.CommentID), 6),
			kindLabel(o.Classification.Origin),
			author,
			colorStatus(o.Classification.Status),
			colorSource(o.Source),
			colorConfidence(o.Classification.Confidence),
			evidence,
			truncate(body, 32),
			truncate(draft, 32),
		})
	}

	var b strings.Builder
	summary := summarize(outcomes)
	b.WriteString(bold(fmt.Sprintf("\n%s/%s#%d · head %s\n", snap.PR.Owner, snap.PR.Repo, snap.PR.Number, short(snap.HeadSHA, 7))))
	b.WriteString(summary)
	b.WriteString("\n")
	if line := usageLine(outcomes); line != "" {
		b.WriteString(dim(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(renderTable(cols, rows))
	return b.String()
}

func usageLine(outcomes []types.Outcome) string {
	var in, out, llm, rule int
	for _, o := range outcomes {
		in += o.InputTokens
		out += o.OutputTokens
		if o.Source == types.SourceLLM {
			llm++
		} else {
			rule++
		}
	}
	if llm == 0 {
		return ""
	}
	return fmt.Sprintf("usage: %d LLM calls, %d rule calls · %d in / %d out tokens",
		llm, rule, in, out)
}

func summarize(outcomes []types.Outcome) string {
	counts := map[types.ClassificationStatus]int{}
	for _, o := range outcomes {
		counts[o.Classification.Status]++
	}
	return fmt.Sprintf("%s · %s · %s · %s",
		green(fmt.Sprintf("%d addressed", counts[types.StatusAddressed])),
		yellow(fmt.Sprintf("%d partial", counts[types.StatusPartial])),
		red(fmt.Sprintf("%d pending", counts[types.StatusPending])),
		cyan(fmt.Sprintf("%d needs-discussion", counts[types.StatusNeedsDiscussion])),
	)
}

func colorStatus(s types.ClassificationStatus) string {
	switch s {
	case types.StatusAddressed:
		return green(string(s))
	case types.StatusPartial:
		return yellow(string(s))
	case types.StatusPending:
		return red(string(s))
	case types.StatusNeedsDiscussion:
		return cyan(string(s))
	}
	return string(s)
}

func colorConfidence(c types.ConfidenceLevel) string {
	switch c {
	case types.ConfidenceHigh:
		return green(string(c))
	case types.ConfidenceMedium:
		return yellow(string(c))
	case types.ConfidenceLow:
		return dim(string(c))
	}
	return string(c)
}

func colorSource(s types.ClassificationSource) string {
	if s == types.SourceRule {
		return dim(string(s))
	}
	return string(s)
}

func kindLabel(origin types.CommentOrigin) string {
	switch origin {
	case types.OriginInline:
		return "inline"
	case types.OriginReviewSummary:
		return "summary"
	case types.OriginIssueComment:
		return "issue"
	}
	return string(origin)
}

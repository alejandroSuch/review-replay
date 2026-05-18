// Package render formats snapshots, evidence and reports for the CLI.
package render

import (
	"fmt"
	"strings"

	"github.com/alejandroSuch/review-replay/internal/types"
)

// Evidence prints the deterministic evidence packets for the snapshot.
func Evidence(snap *types.PrSnapshot, inline []types.CommentEvidence, issue []types.IssueLevelEvidence) string {
	var b strings.Builder
	replies := len(snap.ReviewComments) - len(inline)
	b.WriteString(bold(fmt.Sprintf("\n%s/%s#%d · head %s\n", snap.PR.Owner, snap.PR.Repo, snap.PR.Number, short(snap.HeadSHA, 7))))
	b.WriteString(fmt.Sprintf("%d inline (%d replies) · %d issue-level · %d issue comments raw · %d review summaries raw · %d commits\n\n",
		len(inline), replies, len(issue), len(snap.IssueComments), len(snap.ReviewSummaries), len(snap.Commits)))

	cols := []column{
		{"#", 6},
		{"author", 14},
		{"path:line", 30},
		{"flags", 14},
		{"after", 5},
		{"replies", 7},
		{"body", 56},
	}
	rows := make([][]string, 0, len(inline))
	for _, e := range inline {
		flags := []string{}
		if e.Outdated {
			flags = append(flags, "outdated")
		}
		if e.RegionChanged {
			flags = append(flags, "changed")
		}
		flagStr := strings.Join(flags, ",")
		if flagStr == "" {
			flagStr = "-"
		}
		line := "?"
		if e.Comment.Line != nil {
			line = fmt.Sprintf("%d", *e.Comment.Line)
		}
		rows = append(rows, []string{
			short(fmt.Sprintf("%d", e.Comment.ID), 6),
			e.Comment.Author,
			truncate(fmt.Sprintf("%s:%s", e.Comment.Path, line), 30),
			flagStr,
			fmt.Sprintf("%d", len(e.ChangedInCommits)),
			fmt.Sprintf("%d", len(e.ThreadReplies)),
			truncate(oneLine(e.Comment.Body), 56),
		})
	}
	b.WriteString(renderTable(cols, rows))

	if len(issue) > 0 {
		b.WriteString("\n\nIssue-level comments (no diff anchor):\n")
		issueRows := make([][]string, 0, len(issue))
		for _, e := range issue {
			kind := "[summary]"
			if e.Comment.Kind == types.KindIssueComment {
				kind = "[issue]"
			}
			issueRows = append(issueRows, []string{
				short(fmt.Sprintf("%d", e.Comment.ID), 6),
				e.Comment.Author,
				kind,
				"-",
				fmt.Sprintf("%d", len(e.LaterCommits)),
				fmt.Sprintf("%d", len(e.LaterReplies)),
				truncate(oneLine(e.Comment.Body), 56),
			})
		}
		b.WriteString(renderTable(cols, issueRows))
	}
	return b.String()
}

// NothingToClassify is shown when neither inline nor issue-level work survives
// filtering.
func NothingToClassify(snap *types.PrSnapshot) string {
	var b strings.Builder
	b.WriteString(bold(fmt.Sprintf("\n%s/%s#%d · head %s\n\n", snap.PR.Owner, snap.PR.Repo, snap.PR.Number, short(snap.HeadSHA, 7))))
	b.WriteString("Nothing to classify on this PR.\n")
	b.WriteString("Inline threads: 0. Substantive review summaries: 0. Substantive issue comments: 0.\n")
	b.WriteString("Short messages (LGTM, thanks, emojis) are filtered out as non-classifiable noise.\n\n")
	b.WriteString("Exiting cleanly.\n")
	return b.String()
}

type column struct {
	header string
	width  int
}

func renderTable(cols []column, rows [][]string) string {
	var b strings.Builder
	header := make([]string, len(cols))
	sep := make([]string, len(cols))
	for i, c := range cols {
		header[i] = bold(pad(c.header, c.width))
		sep[i] = strings.Repeat("─", c.width)
	}
	b.WriteString(strings.Join(header, " │ "))
	b.WriteString("\n")
	b.WriteString(strings.Join(sep, "─┼─"))
	b.WriteString("\n")
	for _, r := range rows {
		cells := make([]string, len(cols))
		for i, val := range r {
			cells[i] = pad(val, cols[i].width)
		}
		b.WriteString(strings.Join(cells, " │ "))
		b.WriteString("\n")
	}
	return b.String()
}

func pad(s string, n int) string {
	visible := stripANSI(s)
	r := []rune(visible)
	switch {
	case len(r) > n:
		// Truncate at the visible rune boundary. We drop ANSI codes from
		// the truncated cell — colored cells are short enough in practice
		// that this isn't a real loss.
		return string(r[:n])
	case len(r) == n:
		// Exact fit: keep the ANSI wrappers intact.
		return s
	default:
		return s + strings.Repeat(" ", n-len(r))
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

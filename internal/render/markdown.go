package render

import (
	"fmt"
	"strings"

	"github.com/alejandroSuch/review-replay/internal/types"
)

// Marker is the hidden HTML comment that lets an upsert (the Action) find and
// replace the previous review-replay comment instead of posting a new one.
const Marker = "<!-- review-replay -->"

// Markdown renders the classified report as a PR comment body (GitHub-flavored
// markdown). It leads with Marker so a sticky-comment upsert can locate it. Only
// the unresolved comments (pending / partial / needs-discussion) get a row; the
// addressed ones are summarized in the header.
func Markdown(
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

	counts := map[types.ClassificationStatus]int{}
	for _, o := range outcomes {
		counts[o.Classification.Status]++
	}

	var b strings.Builder
	b.WriteString(Marker)
	b.WriteString("\n### review-replay\n\n")
	b.WriteString(fmt.Sprintf("`%s/%s#%d` at `%s`: %d addressed, %d partial, %d pending, %d needs-discussion.\n",
		snap.PR.Owner, snap.PR.Repo, snap.PR.Number, short(snap.HeadSHA, 7),
		counts[types.StatusAddressed], counts[types.StatusPartial],
		counts[types.StatusPending], counts[types.StatusNeedsDiscussion]))

	unresolved := counts[types.StatusPartial] + counts[types.StatusPending] + counts[types.StatusNeedsDiscussion]
	if unresolved == 0 {
		b.WriteString("\nAll review feedback looks addressed.\n")
		return b.String()
	}

	b.WriteString("\n| status | author | comment | evidence | draft reply |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, o := range outcomes {
		c := o.Classification
		if c.Status == types.StatusAddressed {
			continue
		}
		author, body, url := "?", "", ""
		if ev, ok := inlineByID[c.CommentID]; ok {
			author, body, url = ev.Comment.Author, oneLine(ev.Comment.Body), ev.Comment.HTMLURL
		} else if ev, ok := issueByID[c.CommentID]; ok {
			author, body, url = ev.Comment.Author, oneLine(ev.Comment.Body), ev.Comment.HTMLURL
		}
		comment := mdCell(truncate(body, 100))
		if url != "" {
			comment = fmt.Sprintf("[%s](%s)", comment, url)
		}
		evidence := "-"
		if c.EvidenceCommitSha != nil {
			evidence = "`" + short(*c.EvidenceCommitSha, 7) + "`"
		}
		draft := "-"
		if c.DraftReply != nil && *c.DraftReply != "" {
			draft = mdCell(*c.DraftReply)
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			string(c.Status), mdCell(author), comment, evidence, draft))
	}
	b.WriteString("\n<sub>review-replay: draft replies are suggestions, edit before posting.</sub>\n")
	return b.String()
}

// mdCell makes a string safe for a single markdown table cell.
func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

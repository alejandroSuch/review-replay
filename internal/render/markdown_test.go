package render

import (
	"strings"
	"testing"

	"github.com/alejandroSuch/review-replay/internal/types"
)

func ptr(s string) *string { return &s }

func TestMarkdown_UnresolvedOnly(t *testing.T) {
	snap := &types.PrSnapshot{PR: types.PrRef{Owner: "o", Repo: "r", Number: 7}, HeadSHA: "abcdef1234567"}
	inline := []types.CommentEvidence{
		{Comment: types.ReviewComment{ID: 1, Author: "rev", Body: "Add a nil check|here", HTMLURL: "https://gh/c/1"}},
		{Comment: types.ReviewComment{ID: 2, Author: "rev", Body: "typo in name", HTMLURL: "https://gh/c/2"}},
	}
	outcomes := []types.Outcome{
		{Classification: types.Classification{CommentID: 1, Origin: types.OriginInline, Status: types.StatusPending,
			DraftReply: ptr("Fixed the nil check in the guard."), EvidenceCommitSha: ptr("deadbeefcafe")}},
		{Classification: types.Classification{CommentID: 2, Origin: types.OriginInline, Status: types.StatusAddressed}},
	}

	out := Markdown(snap, inline, nil, outcomes)

	if !strings.HasPrefix(out, Marker) {
		t.Fatalf("body must start with the upsert marker; got:\n%s", out)
	}
	if !strings.Contains(out, "o/r#7") || !strings.Contains(out, "abcdef1") {
		t.Errorf("missing PR ref / head sha header:\n%s", out)
	}
	if !strings.Contains(out, "1 addressed, 0 partial, 1 pending") {
		t.Errorf("summary counts wrong:\n%s", out)
	}
	// unresolved row: linked comment, pipe escaped, evidence sha, draft reply
	if !strings.Contains(out, "https://gh/c/1") || !strings.Contains(out, `nil check\|here`) {
		t.Errorf("pending row missing link or unescaped pipe:\n%s", out)
	}
	if !strings.Contains(out, "deadbee") || !strings.Contains(out, "Fixed the nil check") {
		t.Errorf("pending row missing evidence/draft:\n%s", out)
	}
	// addressed comment must NOT get a row
	if strings.Contains(out, "typo in name") {
		t.Errorf("addressed comment should be summarized, not rowed:\n%s", out)
	}
}

func TestMarkdown_AllAddressed(t *testing.T) {
	snap := &types.PrSnapshot{PR: types.PrRef{Owner: "o", Repo: "r", Number: 9}, HeadSHA: "0123456789"}
	outcomes := []types.Outcome{
		{Classification: types.Classification{CommentID: 1, Status: types.StatusAddressed}},
	}
	out := Markdown(snap, nil, nil, outcomes)
	if !strings.HasPrefix(out, Marker) {
		t.Fatalf("missing marker")
	}
	if !strings.Contains(out, "All review feedback looks addressed.") {
		t.Errorf("expected all-addressed message:\n%s", out)
	}
	if strings.Contains(out, "| status |") {
		t.Errorf("no table expected when all addressed:\n%s", out)
	}
}

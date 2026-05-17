package classifier

import (
	"testing"

	"github.com/alejandroSuch/review-replay/internal/types"
)

func baseEvidence() types.CommentEvidence {
	return types.CommentEvidence{
		Comment: types.ReviewComment{ID: 1, Author: "rev", Body: "fix"},
	}
}

func TestShortCircuitNoSignal(t *testing.T) {
	rule, c := ApplyShortCircuit(baseEvidence())
	if rule != "no-signal" || c == nil || c.Status != types.StatusPending {
		t.Fatalf("expected no-signal pending, got %q %+v", rule, c)
	}
}

func TestShortCircuitSkipsWhenRegionChanged(t *testing.T) {
	e := baseEvidence()
	e.RegionChanged = true
	if r, c := ApplyShortCircuit(e); r != "" || c != nil {
		t.Fatalf("expected no rule, got %q %+v", r, c)
	}
}

func TestShortCircuitSkipsWhenResolved(t *testing.T) {
	e := baseEvidence()
	e.Resolved = true
	if r, c := ApplyShortCircuit(e); r != "" || c != nil {
		t.Fatalf("expected no rule, got %q %+v", r, c)
	}
}

func TestShortCircuitAddressedByOpener(t *testing.T) {
	e := baseEvidence()
	login := "rev"
	e.Resolved = true
	e.ResolvedByLogin = &login
	e.ResolvedByThreadOpener = true
	rule, c := ApplyShortCircuit(e)
	if rule != "resolved-by-opener" || c == nil || c.Status != types.StatusAddressed {
		t.Fatalf("expected resolved-by-opener addressed, got %q %+v", rule, c)
	}
}

func TestParseClassification(t *testing.T) {
	happy := `{"status":"addressed","evidenceCommitSha":"abc1234","draftReply":"Done in abc1234.","confidence":0.9,"rationale":"ok"}`
	t.Run("clean", func(t *testing.T) {
		p, err := ParseClassification(happy)
		if err != nil || p.Status != types.StatusAddressed {
			t.Fatalf("got %+v err=%v", p, err)
		}
	})
	t.Run("fenced", func(t *testing.T) {
		wrapped := "```json\n" + happy + "\n```"
		if _, err := ParseClassification(wrapped); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("with prose", func(t *testing.T) {
		noisy := "Sure, here you go:\n" + happy + "\nLet me know."
		if _, err := ParseClassification(noisy); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid status", func(t *testing.T) {
		bad := `{"status":"maybe","confidence":0.5,"rationale":"x"}`
		if _, err := ParseClassification(bad); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("malformed", func(t *testing.T) {
		if _, err := ParseClassification("not json"); err == nil {
			t.Fatal("expected error")
		}
	})
}

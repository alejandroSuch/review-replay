package evidence

import (
	"strings"
	"testing"

	"github.com/alejandroSuch/review-replay/internal/types"
)

func TestPickChangedCommits(t *testing.T) {
	comment := types.ReviewComment{
		Path:      "src/foo.ts",
		CreatedAt: "2026-05-01T10:00:00Z",
	}
	commits := []types.Commit{
		{SHA: "before", CommittedAt: "2026-05-01T09:00:00Z"},
		{SHA: "after", CommittedAt: "2026-05-01T11:00:00Z"},
	}
	files := map[string]map[string]struct{}{
		"before": {"src/foo.ts": {}},
		"after":  {"src/foo.ts": {}},
	}
	got := PickChangedCommits(commits, comment, files)
	if len(got) != 1 || got[0].SHA != "after" {
		t.Fatalf("expected [after], got %+v", got)
	}
}

func TestPickChangedCommitsFiltersByPath(t *testing.T) {
	comment := types.ReviewComment{Path: "src/foo.ts", CreatedAt: "2026-05-01T10:00:00Z"}
	commits := []types.Commit{{SHA: "after", CommittedAt: "2026-05-01T11:00:00Z"}}
	files := map[string]map[string]struct{}{"after": {"src/bar.ts": {}}}
	if got := PickChangedCommits(commits, comment, files); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestExtractHunk(t *testing.T) {
	file := strings.Join([]string{"a", "b", "c", "d", "e", "f", "g", "h"}, "\n")
	h := ExtractHunk(file, 4, 4)
	if !strings.Contains(h, "   1  a") {
		t.Fatalf("missing first line: %q", h)
	}
	if !strings.Contains(h, "   4  d") {
		t.Fatalf("missing target line: %q", h)
	}
	if !strings.Contains(h, "   8  h") {
		t.Fatalf("missing last line: %q", h)
	}
}

func TestComputeRegionChanged(t *testing.T) {
	t.Run("outdated", func(t *testing.T) {
		if !ComputeRegionChanged(true, nil, "", nil) {
			t.Fatal("outdated should be true")
		}
	})
	t.Run("no later commit", func(t *testing.T) {
		hunk := "   1  hello"
		if ComputeRegionChanged(false, nil, "@@\n hello", &hunk) {
			t.Fatal("no later commits should be false")
		}
	})
	t.Run("hunk diverged", func(t *testing.T) {
		commits := []types.Commit{{CommittedAt: "2026-05-01T11:00:00Z"}}
		hunk := "   1  goodbye"
		if !ComputeRegionChanged(false, commits, "@@\n hello", &hunk) {
			t.Fatal("diverged content should be true")
		}
	})
	t.Run("hunk intact", func(t *testing.T) {
		commits := []types.Commit{{CommittedAt: "2026-05-01T11:00:00Z"}}
		hunk := "   1  hello world"
		if ComputeRegionChanged(false, commits, "@@\n hello world", &hunk) {
			t.Fatal("intact content should be false")
		}
	})
}

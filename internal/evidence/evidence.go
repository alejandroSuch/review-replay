// Package evidence builds the deterministic per-comment data packets the
// classifier consumes.
package evidence

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alejandroSuch/review-replay/internal/filters"
	"github.com/alejandroSuch/review-replay/internal/github"
	"github.com/alejandroSuch/review-replay/internal/types"
)

const (
	hunkContextLines = 5
	// dupSameAuthorWindow caps how close in time two comments by the same
	// author have to be to count as a duplicate. Wider than this and we
	// assume the second one is a deliberate restatement (the author
	// circling back), not a double-post we should silently drop.
	dupSameAuthorWindow = 30 * time.Minute
)

// Build assembles inline and issue-level evidence for a PR snapshot.
type Builder struct {
	gh *github.Client
}

// New returns a Builder.
func New(client *github.Client) *Builder {
	return &Builder{gh: client}
}

// BuildAll returns both inline and issue-level evidence sets for a PR.
func (b *Builder) BuildAll(ctx context.Context, snap *types.PrSnapshot) ([]types.CommentEvidence, []types.IssueLevelEvidence, error) {
	inline, err := b.buildInline(ctx, snap)
	if err != nil {
		return nil, nil, err
	}
	issueComments := buildIssueLevelComments(snap)
	issueEvidence := buildIssueLevelEvidence(snap, issueComments)
	return inline, issueEvidence, nil
}

func (b *Builder) buildInline(ctx context.Context, snap *types.PrSnapshot) ([]types.CommentEvidence, error) {
	commitFiles, err := b.loadCommitFiles(ctx, snap)
	if err != nil {
		return nil, err
	}
	repliesByParent := groupRepliesByParent(snap.ReviewComments)
	threadByRoot := make(map[int64]types.ReviewThread, len(snap.ReviewThreads))
	for _, t := range snap.ReviewThreads {
		threadByRoot[t.RootCommentID] = t
	}

	out := make([]types.CommentEvidence, 0, len(snap.ReviewComments))
	for _, c := range snap.ReviewComments {
		if c.InReplyToID != nil {
			continue // skip child replies; they show up as ThreadReplies on the root
		}
		changed := pickChangedCommits(snap.Commits, c, commitFiles)
		outdated := c.Line == nil
		currentHunk, err := b.fetchCurrentHunk(ctx, snap, c, outdated)
		if err != nil {
			return nil, err
		}
		regionChanged := computeRegionChanged(outdated, changed, c.DiffHunk, currentHunk)

		var resolved bool
		var resolvedByLogin *string
		var resolvedByOpener bool
		if t, ok := threadByRoot[c.ID]; ok {
			resolved = t.IsResolved
			resolvedByLogin = t.ResolvedByLogin
			if resolved && resolvedByLogin != nil && *resolvedByLogin == t.OpenerLogin {
				resolvedByOpener = true
			}
		}

		out = append(out, types.CommentEvidence{
			Comment:                c,
			Outdated:               outdated,
			RegionChanged:          regionChanged,
			ChangedInCommits:       changed,
			ThreadReplies:          repliesByParent[c.ID],
			CurrentHunk:            currentHunk,
			Resolved:               resolved,
			ResolvedByThreadOpener: resolvedByOpener,
			ResolvedByLogin:        resolvedByLogin,
		})
	}
	return out, nil
}

func (b *Builder) loadCommitFiles(ctx context.Context, snap *types.PrSnapshot) (map[string]map[string]struct{}, error) {
	type result struct {
		sha   string
		files []string
		err   error
	}
	ch := make(chan result, len(snap.Commits))
	var wg sync.WaitGroup
	for _, c := range snap.Commits {
		wg.Add(1)
		go func(sha string) {
			defer wg.Done()
			files, err := b.gh.FetchCommitFiles(ctx, snap.PR, sha)
			ch <- result{sha: sha, files: files, err: err}
		}(c.SHA)
	}
	wg.Wait()
	close(ch)
	out := make(map[string]map[string]struct{}, len(snap.Commits))
	for r := range ch {
		if r.err != nil {
			return nil, fmt.Errorf("commit %s files: %w", r.sha, r.err)
		}
		set := make(map[string]struct{}, len(r.files))
		for _, f := range r.files {
			set[f] = struct{}{}
		}
		out[r.sha] = set
	}
	return out, nil
}

func groupRepliesByParent(comments []types.ReviewComment) map[int64][]types.ReviewComment {
	out := make(map[int64][]types.ReviewComment)
	for _, c := range comments {
		if c.InReplyToID == nil {
			continue
		}
		out[*c.InReplyToID] = append(out[*c.InReplyToID], c)
	}
	return out
}

// PickChangedCommits is exposed for testing.
func PickChangedCommits(commits []types.Commit, comment types.ReviewComment, files map[string]map[string]struct{}) []types.Commit {
	return pickChangedCommits(commits, comment, files)
}

func pickChangedCommits(commits []types.Commit, comment types.ReviewComment, files map[string]map[string]struct{}) []types.Commit {
	out := make([]types.Commit, 0)
	for _, c := range commits {
		if c.CommittedAt <= comment.CreatedAt {
			continue
		}
		set, ok := files[c.SHA]
		if !ok {
			continue
		}
		if _, hit := set[comment.Path]; hit {
			out = append(out, c)
		}
	}
	return out
}

func (b *Builder) fetchCurrentHunk(ctx context.Context, snap *types.PrSnapshot, c types.ReviewComment, outdated bool) (*string, error) {
	if outdated || c.Line == nil {
		return nil, nil
	}
	content, err := b.gh.FetchFileAtRef(ctx, snap.PR, c.Path, snap.HeadSHA)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, nil
	}
	startLine := *c.Line
	if c.StartLine != nil {
		startLine = *c.StartLine
	}
	hunk := ExtractHunk(*content, startLine, *c.Line)
	return &hunk, nil
}

// ExtractHunk returns a numbered window of ±hunkContextLines around [start,end].
func ExtractHunk(fileContent string, startLine, endLine int) string {
	lines := strings.Split(fileContent, "\n")
	from := startLine - hunkContextLines
	if from < 1 {
		from = 1
	}
	to := endLine + hunkContextLines
	if to > len(lines) {
		to = len(lines)
	}
	parts := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		var text string
		if i-1 < len(lines) {
			text = lines[i-1]
		}
		parts = append(parts, fmt.Sprintf("%4d  %s", i, text))
	}
	return strings.Join(parts, "\n")
}

// ComputeRegionChanged tells whether the commented region likely shifted in HEAD.
func ComputeRegionChanged(outdated bool, changedCommits []types.Commit, diffHunk string, currentHunk *string) bool {
	return computeRegionChanged(outdated, changedCommits, diffHunk, currentHunk)
}

func computeRegionChanged(outdated bool, changedCommits []types.Commit, diffHunk string, currentHunk *string) bool {
	if outdated {
		return true
	}
	if len(changedCommits) == 0 {
		return false
	}
	if currentHunk == nil {
		return true
	}
	return !sameContent(diffHunk, *currentHunk)
}

func sameContent(diffHunk, currentHunk string) bool {
	fromHunk := stripDiffMarkers(diffHunk)
	fromCurrent := stripLineNumbers(currentHunk)
	return strings.Contains(fromCurrent, fromHunk) || strings.Contains(fromHunk, fromCurrent)
}

func stripDiffMarkers(diffHunk string) string {
	lines := strings.Split(diffHunk, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, "@@") || strings.HasPrefix(l, "-") {
			continue
		}
		if strings.HasPrefix(l, "+") || strings.HasPrefix(l, " ") {
			l = l[1:]
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func stripLineNumbers(hunk string) string {
	lines := strings.Split(hunk, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		// Format is "%4d  text"; skip the leading number+2 spaces.
		if idx := indexOfSeparator(l); idx >= 0 {
			out = append(out, l[idx+2:])
		} else {
			out = append(out, l)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func indexOfSeparator(s string) int {
	// Look for the "  " separator after a leading number block (digits + spaces).
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ':
			if i+1 < len(s) && s[i+1] == ' ' {
				return i
			}
		default:
			if !(s[i] >= '0' && s[i] <= '9') {
				return -1
			}
		}
	}
	return -1
}

// buildIssueLevelComments collects review summaries and issue comments that
// pass the classifiability filter, removing PR-author self-reviews and
// near-duplicates.
func buildIssueLevelComments(snap *types.PrSnapshot) []types.IssueLevelComment {
	all := make([]types.IssueLevelComment, 0, len(snap.ReviewSummaries)+len(snap.IssueComments))
	for _, r := range snap.ReviewSummaries {
		if r.Author == snap.PRAuthor {
			continue
		}
		if !filters.IsClassifiableText(r.Body) {
			continue
		}
		state := r.State
		all = append(all, types.IssueLevelComment{
			Kind:        types.KindReviewSummary,
			ID:          r.ID,
			Author:      r.Author,
			Body:        filters.StripQuotedLines(r.Body),
			CreatedAt:   r.SubmittedAt,
			HTMLURL:     r.HTMLURL,
			ReviewState: &state,
		})
	}
	for _, c := range snap.IssueComments {
		if c.Author == snap.PRAuthor {
			continue
		}
		if !filters.IsClassifiableText(c.Body) {
			continue
		}
		all = append(all, types.IssueLevelComment{
			Kind:      types.KindIssueComment,
			ID:        c.ID,
			Author:    c.Author,
			Body:      filters.StripQuotedLines(c.Body),
			CreatedAt: c.CreatedAt,
			HTMLURL:   c.HTMLURL,
		})
	}
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].CreatedAt < all[j].CreatedAt
	})
	return dedupeAccidentalRepost(all)
}

// dedupeAccidentalRepost drops follow-up comments only when they are clearly
// an accidental double-post: same author, same comment Kind, same review
// state, byte-equal body after quote-stripping, posted within a short window.
// Anything paraphrased, time-separated, or kind-distinct (e.g. a review
// summary followed minutes later by an issue comment that materially changes
// the ask) MUST reach the classifier — silently dropping it loses real
// feedback.
func dedupeAccidentalRepost(in []types.IssueLevelComment) []types.IssueLevelComment {
	kept := make([]types.IssueLevelComment, 0, len(in))
	for _, c := range in {
		dup := false
		cBody := strings.TrimSpace(c.Body)
		for _, k := range kept {
			if k.Author != c.Author {
				continue
			}
			if k.Kind != c.Kind {
				continue
			}
			if !reviewStateMatches(k.ReviewState, c.ReviewState) {
				continue
			}
			if strings.TrimSpace(k.Body) != cBody {
				continue
			}
			if !withinWindow(k.CreatedAt, c.CreatedAt, dupSameAuthorWindow) {
				continue
			}
			dup = true
			break
		}
		if !dup {
			kept = append(kept, c)
		}
	}
	return kept
}

func reviewStateMatches(a, b *types.ReviewState) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func withinWindow(a, b string, window time.Duration) bool {
	ta, err1 := time.Parse(time.RFC3339, a)
	tb, err2 := time.Parse(time.RFC3339, b)
	if err1 != nil || err2 != nil {
		// If we can't parse, fall back to "not within window" to avoid
		// accidental dedupe.
		return false
	}
	diff := ta.Sub(tb)
	if diff < 0 {
		diff = -diff
	}
	return diff <= window
}

func buildIssueLevelEvidence(snap *types.PrSnapshot, comments []types.IssueLevelComment) []types.IssueLevelEvidence {
	out := make([]types.IssueLevelEvidence, 0, len(comments))
	for _, c := range comments {
		laterCommits := make([]types.Commit, 0)
		for _, k := range snap.Commits {
			if k.CommittedAt > c.CreatedAt {
				laterCommits = append(laterCommits, k)
			}
		}
		laterReplies := make([]types.IssueLevelComment, 0)
		for _, other := range comments {
			if other.ID != c.ID && other.CreatedAt > c.CreatedAt {
				laterReplies = append(laterReplies, other)
			}
		}
		out = append(out, types.IssueLevelEvidence{
			Comment:      c,
			LaterCommits: laterCommits,
			LaterReplies: laterReplies,
		})
	}
	return out
}

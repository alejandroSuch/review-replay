// Package types holds the domain model shared across the review-replay engine.
package types

// PrRef identifies a pull request.
type PrRef struct {
	Owner  string
	Repo   string
	Number int
}

// ReviewComment is an inline review comment anchored to a line of code.
type ReviewComment struct {
	ID                int64
	Author            string
	Body              string
	Path              string
	Line              *int
	StartLine         *int
	OriginalCommitSha string
	CreatedAt         string
	DiffHunk          string
	HTMLURL           string
	InReplyToID       *int64
}

// ReviewThread is a group of inline comments on the same code location.
type ReviewThread struct {
	RootCommentID   int64
	IsResolved      bool
	ResolvedByLogin *string
	OpenerLogin     string
	CommentIDs      []int64
}

// ReviewState enumerates the GitHub PR review states we care about.
type ReviewState string

const (
	ReviewApproved         ReviewState = "APPROVED"
	ReviewChangesRequested ReviewState = "CHANGES_REQUESTED"
	ReviewCommented        ReviewState = "COMMENTED"
	ReviewDismissed        ReviewState = "DISMISSED"
	ReviewPending          ReviewState = "PENDING"
)

// ReviewSummary is the body of an Approve / Request changes review.
type ReviewSummary struct {
	ID          int64
	Author      string
	Body        string
	State       ReviewState
	SubmittedAt string
	HTMLURL     string
}

// IssueComment is a general PR comment (not anchored to code).
type IssueComment struct {
	ID        int64
	Author    string
	Body      string
	CreatedAt string
	HTMLURL   string
}

// Commit is a commit on the PR branch.
type Commit struct {
	SHA         string
	Message     string
	AuthoredAt  string
	CommittedAt string
}

// PrSnapshot is the immutable view of a PR at fetch time.
type PrSnapshot struct {
	PR              PrRef
	PRAuthor        string
	HeadSHA         string
	ReviewComments  []ReviewComment
	ReviewThreads   []ReviewThread
	ReviewSummaries []ReviewSummary
	IssueComments   []IssueComment
	Commits         []Commit
}

// IssueLevelKind discriminates between review summaries and issue comments.
type IssueLevelKind string

const (
	KindReviewSummary IssueLevelKind = "review-summary"
	KindIssueComment  IssueLevelKind = "issue-comment"
)

// IssueLevelComment is a normalised view of a review summary or issue comment.
type IssueLevelComment struct {
	Kind        IssueLevelKind
	ID          int64
	Author      string
	Body        string
	CreatedAt   string
	HTMLURL     string
	ReviewState *ReviewState
}

// CommentEvidence is the per-inline-comment packet the classifier consumes.
type CommentEvidence struct {
	Comment                ReviewComment
	Outdated               bool
	RegionChanged          bool
	ChangedInCommits       []Commit
	ThreadReplies          []ReviewComment
	CurrentHunk            *string
	Resolved               bool
	ResolvedByThreadOpener bool
	ResolvedByLogin        *string
}

// IssueLevelEvidence is the packet for review summaries / issue comments.
type IssueLevelEvidence struct {
	Comment      IssueLevelComment
	LaterCommits []Commit
	LaterReplies []IssueLevelComment
}

// ClassificationStatus is the verdict per comment.
type ClassificationStatus string

const (
	StatusAddressed       ClassificationStatus = "addressed"
	StatusPartial         ClassificationStatus = "partial"
	StatusPending         ClassificationStatus = "pending"
	StatusNeedsDiscussion ClassificationStatus = "needs-discussion"
)

// CommentOrigin distinguishes where a classification came from.
type CommentOrigin string

const (
	OriginInline         CommentOrigin = "inline"
	OriginReviewSummary  CommentOrigin = "review-summary"
	OriginIssueComment   CommentOrigin = "issue-comment"
)

// ConfidenceLevel is a discrete confidence bucket. LLMs cluster their
// numerical outputs in ~5 values anyway, so we ask them for one of three
// explicit buckets that map to concrete evidence quality.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// Classification is the output for a single comment.
type Classification struct {
	CommentID         int64
	Origin            CommentOrigin
	Status            ClassificationStatus
	EvidenceCommitSha *string
	DraftReply        *string
	Confidence        ConfidenceLevel
	Rationale         string
}

// Report aggregates classifications for a PR.
type Report struct {
	PR              PrRef
	GeneratedAt     string
	Classifications []Classification
	Summary         map[ClassificationStatus]int
	Usage           UsageTotals
}

// ClassificationSource is the upstream that produced the classification.
type ClassificationSource string

const (
	SourceRule ClassificationSource = "rule"
	SourceLLM  ClassificationSource = "llm"
)

// Outcome wraps a classification with its provenance.
type Outcome struct {
	Classification Classification
	Source         ClassificationSource
	RuleName       string
	// Token usage for this outcome. Zero when the outcome came from a rule.
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
}

// UsageTotals aggregates token counts across outcomes.
type UsageTotals struct {
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	LLMCalls         int
	RuleCalls        int
	EstimatedUSD     float64 // 0 if no price known for the model
	PriceModel       string  // model id used to compute EstimatedUSD; "" if unknown
}

package github

import (
	"context"
	"fmt"
	"net/url"

	"github.com/alejandroSuch/review-replay/internal/types"
	gh "github.com/google/go-github/v66/github"
)

const reviewThreadsQuery = `
query ReviewThreads($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          isResolved
          resolvedBy { login }
          comments(first: 100) {
            nodes {
              databaseId
              author { login }
              body
              path
              line
              startLine
              originalLine
              originalStartLine
              originalCommit { oid }
              createdAt
              diffHunk
              url
            }
          }
        }
      }
    }
  }
}`

const reviewsQuery = `
query Reviews($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviews(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          databaseId
          author { login }
          body
          state
          submittedAt
          url
        }
      }
    }
  }
}`

type gqlAuthor struct {
	Login string `json:"login"`
}

type gqlComment struct {
	DatabaseID        int64      `json:"databaseId"`
	Author            *gqlAuthor `json:"author"`
	Body              string     `json:"body"`
	Path              string     `json:"path"`
	Line              *int       `json:"line"`
	StartLine         *int       `json:"startLine"`
	OriginalLine      *int       `json:"originalLine"`
	OriginalStartLine *int       `json:"originalStartLine"`
	OriginalCommit    *struct {
		OID string `json:"oid"`
	} `json:"originalCommit"`
	CreatedAt string `json:"createdAt"`
	DiffHunk  string `json:"diffHunk"`
	URL       string `json:"url"`
}

type gqlReviewThread struct {
	IsResolved bool       `json:"isResolved"`
	ResolvedBy *gqlAuthor `json:"resolvedBy"`
	Comments   struct {
		Nodes []gqlComment `json:"nodes"`
	} `json:"comments"`
}

type gqlReviewThreadsPage struct {
	Repository struct {
		PullRequest struct {
			ReviewThreads struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []gqlReviewThread `json:"nodes"`
			} `json:"reviewThreads"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

type gqlReview struct {
	DatabaseID  int64      `json:"databaseId"`
	Author      *gqlAuthor `json:"author"`
	Body        string     `json:"body"`
	State       string     `json:"state"`
	SubmittedAt *string    `json:"submittedAt"`
	URL         string     `json:"url"`
}

type gqlReviewsPage struct {
	Repository struct {
		PullRequest struct {
			Reviews struct {
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
				Nodes []gqlReview `json:"nodes"`
			} `json:"reviews"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

func (c *Client) fetchReviewThreads(ctx context.Context, pr types.PrRef) ([]gqlReviewThread, error) {
	all := []gqlReviewThread{}
	var cursor *string
	for {
		var page gqlReviewThreadsPage
		vars := map[string]any{
			"owner":  pr.Owner,
			"repo":   pr.Repo,
			"number": pr.Number,
			"cursor": cursor,
		}
		if err := c.graphqlQuery(ctx, reviewThreadsQuery, vars, &page); err != nil {
			return nil, fmt.Errorf("fetch review threads: %w", err)
		}
		all = append(all, page.Repository.PullRequest.ReviewThreads.Nodes...)
		if !page.Repository.PullRequest.ReviewThreads.PageInfo.HasNextPage {
			break
		}
		endCursor := page.Repository.PullRequest.ReviewThreads.PageInfo.EndCursor
		cursor = &endCursor
	}
	return all, nil
}

func (c *Client) fetchReviews(ctx context.Context, pr types.PrRef) ([]gqlReview, error) {
	all := []gqlReview{}
	var cursor *string
	for {
		var page gqlReviewsPage
		vars := map[string]any{
			"owner":  pr.Owner,
			"repo":   pr.Repo,
			"number": pr.Number,
			"cursor": cursor,
		}
		if err := c.graphqlQuery(ctx, reviewsQuery, vars, &page); err != nil {
			return nil, fmt.Errorf("fetch reviews: %w", err)
		}
		all = append(all, page.Repository.PullRequest.Reviews.Nodes...)
		if !page.Repository.PullRequest.Reviews.PageInfo.HasNextPage {
			break
		}
		endCursor := page.Repository.PullRequest.Reviews.PageInfo.EndCursor
		cursor = &endCursor
	}
	return all, nil
}

func (c *Client) listIssueComments(ctx context.Context, pr types.PrRef) ([]*gh.IssueComment, error) {
	opt := &gh.IssueListCommentsOptions{
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	all := []*gh.IssueComment{}
	for {
		comments, resp, err := c.REST.Issues.ListComments(ctx, pr.Owner, pr.Repo, pr.Number, opt)
		if err != nil {
			return nil, fmt.Errorf("list issue comments: %w", err)
		}
		all = append(all, comments...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return all, nil
}

func (c *Client) listCommits(ctx context.Context, pr types.PrRef) ([]*gh.RepositoryCommit, error) {
	opt := &gh.ListOptions{PerPage: 100}
	all := []*gh.RepositoryCommit{}
	for {
		commits, resp, err := c.REST.PullRequests.ListCommits(ctx, pr.Owner, pr.Repo, pr.Number, opt)
		if err != nil {
			return nil, fmt.Errorf("list commits: %w", err)
		}
		all = append(all, commits...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return all, nil
}

// FetchPrSnapshot retrieves all the data review-replay needs for a single PR.
func (c *Client) FetchPrSnapshot(ctx context.Context, pr types.PrRef) (*types.PrSnapshot, error) {
	prData, _, err := c.REST.PullRequests.Get(ctx, pr.Owner, pr.Repo, pr.Number)
	if err != nil {
		return nil, fmt.Errorf("get PR: %w", err)
	}

	threads, err := c.fetchReviewThreads(ctx, pr)
	if err != nil {
		return nil, err
	}
	reviews, err := c.fetchReviews(ctx, pr)
	if err != nil {
		return nil, err
	}
	issueComments, err := c.listIssueComments(ctx, pr)
	if err != nil {
		return nil, err
	}
	commits, err := c.listCommits(ctx, pr)
	if err != nil {
		return nil, err
	}

	snap := &types.PrSnapshot{
		PR:       pr,
		PRAuthor: derefLogin(prData.User),
		HeadSHA:  derefString(prData.Head.SHA),
	}

	// Map review threads -> ReviewComment + ReviewThread.
	for _, t := range threads {
		nodes := t.Comments.Nodes
		if len(nodes) == 0 {
			continue
		}
		root := nodes[0]
		rootID := root.DatabaseID
		opener := authorLogin(root.Author)
		ids := make([]int64, 0, len(nodes))
		for i, c := range nodes {
			var inReplyTo *int64
			if i > 0 {
				rid := rootID
				inReplyTo = &rid
			}
			origSha := ""
			if c.OriginalCommit != nil {
				origSha = c.OriginalCommit.OID
			}
			line := c.Line
			if line == nil {
				line = c.OriginalLine
			}
			startLine := c.StartLine
			if startLine == nil {
				startLine = c.OriginalStartLine
			}
			snap.ReviewComments = append(snap.ReviewComments, types.ReviewComment{
				ID:                c.DatabaseID,
				Author:            authorLogin(c.Author),
				Body:              c.Body,
				Path:              c.Path,
				Line:              line,
				StartLine:         startLine,
				OriginalCommitSha: origSha,
				CreatedAt:         c.CreatedAt,
				DiffHunk:          c.DiffHunk,
				HTMLURL:           c.URL,
				InReplyToID:       inReplyTo,
			})
			ids = append(ids, c.DatabaseID)
		}
		var resolvedBy *string
		if t.ResolvedBy != nil && t.ResolvedBy.Login != "" {
			rb := t.ResolvedBy.Login
			resolvedBy = &rb
		}
		snap.ReviewThreads = append(snap.ReviewThreads, types.ReviewThread{
			RootCommentID:   rootID,
			IsResolved:      t.IsResolved,
			ResolvedByLogin: resolvedBy,
			OpenerLogin:     opener,
			CommentIDs:      ids,
		})
	}

	// Review summaries (filter out reviews without submittedAt — pending).
	for _, r := range reviews {
		if r.SubmittedAt == nil {
			continue
		}
		snap.ReviewSummaries = append(snap.ReviewSummaries, types.ReviewSummary{
			ID:          r.DatabaseID,
			Author:      authorLogin(r.Author),
			Body:        r.Body,
			State:       normalizeReviewState(r.State),
			SubmittedAt: *r.SubmittedAt,
			HTMLURL:     r.URL,
		})
	}

	// Issue comments.
	for _, c := range issueComments {
		snap.IssueComments = append(snap.IssueComments, types.IssueComment{
			ID:        c.GetID(),
			Author:    c.GetUser().GetLogin(),
			Body:      c.GetBody(),
			CreatedAt: c.GetCreatedAt().Format("2006-01-02T15:04:05Z"),
			HTMLURL:   c.GetHTMLURL(),
		})
	}

	// Commits.
	for _, c := range commits {
		snap.Commits = append(snap.Commits, types.Commit{
			SHA:         c.GetSHA(),
			Message:     c.GetCommit().GetMessage(),
			AuthoredAt:  c.GetCommit().GetAuthor().GetDate().Format("2006-01-02T15:04:05Z"),
			CommittedAt: c.GetCommit().GetCommitter().GetDate().Format("2006-01-02T15:04:05Z"),
		})
	}

	return snap, nil
}

// FetchFileAtRef returns the content of a file at the given ref, or nil if it
// does not exist or cannot be decoded.
func (c *Client) FetchFileAtRef(ctx context.Context, pr types.PrRef, path, ref string) (*string, error) {
	file, _, _, err := c.REST.Repositories.GetContents(ctx, pr.Owner, pr.Repo, url.PathEscape(path), &gh.RepositoryContentGetOptions{Ref: ref})
	if err != nil {
		// Treat 404 as "file gone" rather than a hard error.
		return nil, nil //nolint:nilerr
	}
	if file == nil || file.GetType() != "file" {
		return nil, nil
	}
	raw, err := file.GetContent()
	if err != nil {
		return nil, nil //nolint:nilerr
	}
	return &raw, nil
}

// FetchCommitFiles returns the list of file paths a commit touches.
func (c *Client) FetchCommitFiles(ctx context.Context, pr types.PrRef, sha string) ([]string, error) {
	commit, _, err := c.REST.Repositories.GetCommit(ctx, pr.Owner, pr.Repo, sha, nil)
	if err != nil {
		return nil, fmt.Errorf("get commit %s: %w", sha, err)
	}
	out := make([]string, 0, len(commit.Files))
	for _, f := range commit.Files {
		out = append(out, f.GetFilename())
	}
	return out, nil
}

func derefLogin(u *gh.User) string {
	if u == nil {
		return "unknown"
	}
	return u.GetLogin()
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func authorLogin(a *gqlAuthor) string {
	if a == nil {
		return "unknown"
	}
	return a.Login
}

var validReviewStates = map[string]types.ReviewState{
	"APPROVED":           types.ReviewApproved,
	"CHANGES_REQUESTED":  types.ReviewChangesRequested,
	"COMMENTED":          types.ReviewCommented,
	"DISMISSED":          types.ReviewDismissed,
	"PENDING":            types.ReviewPending,
}

func normalizeReviewState(s string) types.ReviewState {
	if r, ok := validReviewStates[s]; ok {
		return r
	}
	return types.ReviewCommented
}

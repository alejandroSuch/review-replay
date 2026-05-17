package github

import (
	"context"
	"fmt"

	"github.com/alejandroSuch/review-replay/internal/types"
)

// PostedReply describes a successfully posted reply.
type PostedReply struct {
	ID      int64
	HTMLURL string
}

// PostInlineReply posts a reply to an inline review thread whose root comment
// is rootCommentID. Returns the new comment's id + html_url on success.
func (c *Client) PostInlineReply(ctx context.Context, pr types.PrRef, rootCommentID int64, body string) (*PostedReply, error) {
	comment, _, err := c.REST.PullRequests.CreateCommentInReplyTo(ctx, pr.Owner, pr.Repo, pr.Number, body, rootCommentID)
	if err != nil {
		return nil, fmt.Errorf("post inline reply to %d: %w", rootCommentID, err)
	}
	return &PostedReply{
		ID:      comment.GetID(),
		HTMLURL: comment.GetHTMLURL(),
	}, nil
}

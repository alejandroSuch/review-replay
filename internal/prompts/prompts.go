// Package prompts assembles the system and user prompts the classifier sends
// to the LLM provider.
package prompts

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/alejandroSuch/review-replay/internal/types"
)

//go:embed system_inline.txt
var SystemInline string

//go:embed system_issue_level.txt
var SystemIssueLevel string

const (
	maxHunkChars = 2000
	maxBodyChars = 1500
)

// InlineUser builds the user message for an inline comment classification.
// historyRewritten signals a force-pushed branch: the prompt then forbids
// citing commit SHAs in the draft reply.
func InlineUser(e types.CommentEvidence, historyRewritten bool) string {
	var b strings.Builder
	c := e.Comment

	if historyRewritten {
		fmt.Fprintln(&b, rewriteWarning)
	}
	fmt.Fprintln(&b, "# Review comment")
	fmt.Fprintf(&b, "author: %s\n", c.Author)
	fmt.Fprintf(&b, "path: %s\n", c.Path)
	startLine := "unknown"
	if c.StartLine != nil {
		startLine = fmt.Sprintf("%d", *c.StartLine)
	}
	endLine := "unknown"
	if c.Line != nil {
		endLine = fmt.Sprintf("%d", *c.Line)
	}
	fmt.Fprintf(&b, "line: %s-%s\n", startLine, endLine)
	fmt.Fprintf(&b, "createdAt: %s\n", c.CreatedAt)
	fmt.Fprintf(&b, "outdated: %v\n", e.Outdated)
	fmt.Fprintf(&b, "regionChanged: %v\n", e.RegionChanged)
	if e.Resolved && e.ResolvedByLogin != nil && !e.ResolvedByThreadOpener {
		fmt.Fprintf(&b, "resolvedBy: %s (NOT the thread opener — weak signal)\n", *e.ResolvedByLogin)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Body")
	fmt.Fprintln(&b, truncate(c.Body, maxBodyChars))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Original diff hunk (state when comment was posted)")
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b, truncate(c.DiffHunk, maxHunkChars))
	fmt.Fprintln(&b, "```")
	fmt.Fprintln(&b)

	if e.CurrentHunk != nil {
		fmt.Fprintln(&b, "## Current state at HEAD (region around the comment)")
		fmt.Fprintln(&b, "```")
		fmt.Fprintln(&b, truncate(*e.CurrentHunk, maxHunkChars))
		fmt.Fprintln(&b, "```")
	} else {
		fmt.Fprintln(&b, "## Current state at HEAD")
		fmt.Fprintln(&b, "(file or region no longer exists)")
	}
	fmt.Fprintln(&b)

	if len(e.ChangedInCommits) > 0 {
		fmt.Fprintln(&b, "## Commits after the comment that touch this file")
		for _, k := range e.ChangedInCommits {
			fmt.Fprintf(&b, "- %s (%s): %s\n", shortSHA(k.SHA), k.CommittedAt, oneLine(k.Message))
		}
	} else {
		fmt.Fprintln(&b, "## Commits after the comment that touch this file")
		fmt.Fprintln(&b, "(none)")
	}
	fmt.Fprintln(&b)

	if len(e.ThreadReplies) > 0 {
		fmt.Fprintln(&b, "## Thread replies")
		for _, r := range e.ThreadReplies {
			fmt.Fprintf(&b, "- %s (%s):\n", r.Author, r.CreatedAt)
			fmt.Fprint(&b, indent(truncate(r.Body, maxBodyChars), "    "))
			fmt.Fprintln(&b)
		}
	} else {
		fmt.Fprintln(&b, "## Thread replies")
		fmt.Fprintln(&b, "(none)")
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Return the JSON classification now. No prose, no markdown fences.")
	return b.String()
}

const rewriteWarning = "# History rewritten\n" +
	"This PR has been force-pushed at least once. Commit SHAs listed below may be\n" +
	"unreachable from HEAD. DO NOT cite, reference, or include any commit SHA in\n" +
	"the `draftReply` field. Describe what changed in prose, not by commit hash."

// IssueLevelUser builds the user message for an issue-level (review summary or
// issue comment) classification.
// historyRewritten signals a force-pushed branch: the prompt then forbids
// citing commit SHAs in the draft reply.
func IssueLevelUser(e types.IssueLevelEvidence, historyRewritten bool) string {
	var b strings.Builder
	c := e.Comment

	if historyRewritten {
		fmt.Fprintln(&b, rewriteWarning)
	}

	header := "Review summary"
	if c.Kind == types.KindIssueComment {
		header = "Issue comment"
	}
	fmt.Fprintf(&b, "# %s\n", header)
	fmt.Fprintf(&b, "author: %s\n", c.Author)
	if c.ReviewState != nil {
		fmt.Fprintf(&b, "reviewState: %s\n", *c.ReviewState)
	}
	fmt.Fprintf(&b, "createdAt: %s\n", c.CreatedAt)
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Body")
	fmt.Fprintln(&b, truncate(c.Body, 2000))
	fmt.Fprintln(&b)

	if len(e.LaterCommits) > 0 {
		fmt.Fprintln(&b, "## Commits authored after this comment")
		for _, k := range e.LaterCommits {
			fmt.Fprintf(&b, "- %s (%s): %s\n", shortSHA(k.SHA), k.CommittedAt, oneLine(k.Message))
		}
	} else {
		fmt.Fprintln(&b, "## Commits after this comment")
		fmt.Fprintln(&b, "(none)")
	}
	fmt.Fprintln(&b)

	if len(e.LaterReplies) > 0 {
		fmt.Fprintln(&b, "## Subsequent issue-level comments")
		for _, r := range e.LaterReplies {
			fmt.Fprintf(&b, "- %s (%s, %s):\n", r.Author, r.Kind, r.CreatedAt)
			fmt.Fprint(&b, indent(truncate(r.Body, 2000), "    "))
			fmt.Fprintln(&b)
		}
	} else {
		fmt.Fprintln(&b, "## Subsequent issue-level comments")
		fmt.Fprintln(&b, "(none)")
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "Return the JSON classification now. No prose, no markdown fences.")
	return b.String()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n…[truncated]"
}

func oneLine(s string) string {
	line := strings.SplitN(s, "\n", 2)[0]
	r := []rune(line)
	if len(r) > 120 {
		return string(r[:120])
	}
	return line
}

func shortSHA(s string) string {
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

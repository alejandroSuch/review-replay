// Package github fetches a PR snapshot from GitHub.
package github

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/alejandroSuch/review-replay/internal/types"
)

var (
	urlRef   = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)
	shortRef = regexp.MustCompile(`^([^/]+)/([^/#]+)#(\d+)$`)
)

// ParsePrRef accepts either a PR URL or owner/repo#N short form.
func ParsePrRef(input string) (types.PrRef, error) {
	if m := urlRef.FindStringSubmatch(input); m != nil {
		n, err := strconv.Atoi(m[3])
		if err != nil {
			return types.PrRef{}, fmt.Errorf("invalid PR number: %s", m[3])
		}
		return types.PrRef{Owner: m[1], Repo: m[2], Number: n}, nil
	}
	if m := shortRef.FindStringSubmatch(input); m != nil {
		n, err := strconv.Atoi(m[3])
		if err != nil {
			return types.PrRef{}, fmt.Errorf("invalid PR number: %s", m[3])
		}
		return types.PrRef{Owner: m[1], Repo: m[2], Number: n}, nil
	}
	return types.PrRef{}, fmt.Errorf("invalid PR reference %q: use a GitHub URL or owner/repo#N", input)
}

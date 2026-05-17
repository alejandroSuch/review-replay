// Package filters provides deterministic helpers to discard non-classifiable
// noise (chit-chat, quoted-only bodies) and to detect near-duplicate texts.
package filters

import (
	"regexp"
	"strings"
)

const minBodyLength = 30

var chitchatPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*(lgtm|sgtm|ack|ok|thanks?|ty|tnx|cheers|yay|nice|great|cool|👍+|👏+|🙏+|🚀+|✅+|🎉+|❤️+)[.!\s]*$`),
	regexp.MustCompile(`(?i)^\s*(approved|approve|done|merged?)[.!\s]*$`),
}

// IsClassifiableText returns true when the body has enough signal to be worth
// classifying. It strips markdown quote lines (>...) before checking length
// and matching against the chitchat patterns.
func IsClassifiableText(body string) bool {
	trimmed := strings.TrimSpace(StripQuotedLines(body))
	if len(trimmed) < minBodyLength {
		return false
	}
	for _, p := range chitchatPatterns {
		if p.MatchString(trimmed) {
			return false
		}
	}
	return true
}

var leadingQuote = regexp.MustCompile(`^\s*>+\s?`)
var multipleBlankLines = regexp.MustCompile(`\n{3,}`)

// StripQuotedLines removes leading-> markdown quote lines from a body so the
// classifier sees only the author's own content.
func StripQuotedLines(body string) string {
	lines := strings.Split(body, "\n")
	out := lines[:0]
	for _, line := range lines {
		if !leadingQuote.MatchString(line) {
			out = append(out, line)
		}
	}
	joined := strings.Join(out, "\n")
	joined = multipleBlankLines.ReplaceAllString(joined, "\n\n")
	return strings.TrimSpace(joined)
}

// Known bot logins that GitHub sometimes returns without the [bot] suffix.
// Keep this short and conservative — we only want to skip the LLM call for
// truly noisy / formulaic bots.
var knownBots = map[string]struct{}{
	"github-actions":                {},
	"dependabot":                    {},
	"renovate":                      {},
	"renovate-bot":                  {},
	"codecov":                       {},
	"codecov-commenter":             {},
	"vercel":                        {},
	"vercel-preview-bot":            {},
	"netlify":                       {},
	"copilot-pull-request-reviewer": {},
	"coderabbitai":                  {},
	"greptile-apps":                 {},
	"snyk-bot":                      {},
	"gitkraken-services":            {},
	"mergify":                       {},
	"mergify-bot":                   {},
	"pre-commit-ci":                 {},
}

// IsBotAuthor returns true when the login looks like a GitHub bot. Handles
// both the canonical `<name>[bot]` suffix and the de-suffixed form that
// go-github occasionally returns on REST endpoints.
func IsBotAuthor(login string) bool {
	if login == "" {
		return false
	}
	if strings.HasSuffix(login, "[bot]") {
		return true
	}
	lower := strings.ToLower(login)
	if _, ok := knownBots[lower]; ok {
		return true
	}
	// Some clients drop the suffix to `<name>` but keep a [bot] marker
	// elsewhere; tolerate the prefix form too.
	for known := range knownBots {
		if strings.HasPrefix(lower, known+"[") {
			return true
		}
	}
	return false
}

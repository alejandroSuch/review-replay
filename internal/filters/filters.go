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

// BodySimilarity returns a Jaccard similarity between two bodies based on
// lowercased word tokens of length >= 4. Cheap and good enough to detect
// quoted-or-restated reviews.
func BodySimilarity(a, b string) float64 {
	ta := tokenize(a)
	tb := tokenize(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for tok := range ta {
		if _, ok := tb[tok]; ok {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

var nonWord = regexp.MustCompile(`[^\w\s]`)

func tokenize(s string) map[string]struct{} {
	clean := nonWord.ReplaceAllString(strings.ToLower(s), " ")
	tokens := strings.Fields(clean)
	out := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		if len(t) >= 4 {
			out[t] = struct{}{}
		}
	}
	return out
}

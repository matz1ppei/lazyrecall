package ai

import (
	"strings"
	"unicode"

	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func normalizeAccents(s string) string {
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

// MatchAnswer returns true if input matches correct after trimming whitespace,
// lowercasing, and stripping accent marks.
func MatchAnswer(input, correct string) bool {
	return strings.EqualFold(
		normalizeAccents(strings.TrimSpace(input)),
		normalizeAccents(strings.TrimSpace(correct)),
	)
}

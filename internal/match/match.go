// Package match applies the keyword/exclude rules and the deadline check
// to a listing. Plain rules only: no model call, nothing fuzzy.
package match

import (
	"regexp"
	"strings"
	"time"

	"github.com/taktekhq/bidwatch/internal/model"
)

// Rules holds the word lists a listing is checked against.
type Rules struct {
	Keywords []string
	Excludes []string
}

// Result explains why a listing did or didn't match, for the per-source
// counts and for tests.
type Result struct {
	Matched  bool
	Keyword  string // the keyword that matched, when Matched is true
	Excluded string // the exclude word that killed it, when Matched is false because of one
	PastDue  bool   // true if the only reason it failed is a deadline already gone
}

// Check runs a listing's title (and, if given, a short description) against
// the rules, then against today's date. A listing with no excluded word,
// at least one keyword hit, and a deadline that is either unknown or still
// in the future, matches.
func Check(l model.Listing, description string, rules Rules, now time.Time) Result {
	haystack := l.Title + " " + description

	for _, ex := range rules.Excludes {
		if ex == "" {
			continue
		}
		if wordMatch(haystack, ex) {
			return Result{Matched: false, Excluded: ex}
		}
	}

	var hitKeyword string
	for _, kw := range rules.Keywords {
		if kw == "" {
			continue
		}
		if wordMatch(haystack, kw) {
			hitKeyword = kw
			break
		}
	}
	if hitKeyword == "" {
		return Result{Matched: false}
	}

	if l.Deadline != nil && l.Deadline.Before(now) {
		return Result{Matched: false, Keyword: hitKeyword, PastDue: true}
	}

	return Result{Matched: true, Keyword: hitKeyword}
}

// wordMatch reports whether phrase appears in haystack as a whole word (or
// phrase of words), case-insensitive, rather than as a substring of a
// longer word. Plain substring matching lets a short keyword like "ai"
// fire on "Water Main Relocations" (the "ai" inside "Main") or "app" fire
// on "Appraisal Services"; a word boundary on each side of the phrase
// rules that out while still matching "AI Engineer" or "App Development".
func wordMatch(haystack, phrase string) bool {
	re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(phrase) + `\b`)
	if err != nil {
		// A phrase that somehow can't compile (shouldn't happen for plain
		// words) falls back to a substring check rather than silently
		// never matching.
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(phrase))
	}
	return re.MatchString(haystack)
}

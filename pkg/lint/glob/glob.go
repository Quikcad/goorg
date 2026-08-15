// Package glob provides the two pattern matchers goorg uses.
//
// They are deliberately different. Paths are hierarchical, so `*` must not
// cross a separator and `**` is needed to span segments. Rule IDs are flat
// identifiers that merely contain a slash, so `*` must cross it — otherwise a
// config key of `*` would match no rule at all, which is the one thing anyone
// writing it cannot mean. Do not merge them.
package glob

import (
	"path"
	"strings"
)

// MatchPath reports whether a slash-separated path matches a glob pattern.
//
// It extends path.Match with `**`, which matches any number of path segments
// including none.
func MatchPath(pattern, name string) bool {
	return matchSegments(splitPath(pattern), splitPath(name))
}

// MatchRuleID reports whether a rule ID matches a config key pattern, where `*`
// matches any run of characters — including `/` — and `?` matches exactly one.
func MatchRuleID(pattern, id string) bool {
	// Iterative wildcard match, backtracking on the most recent `*`.
	var patIdx, idIdx, mark int
	star := -1
	for idIdx < len(id) {
		switch {
		case patIdx < len(pattern) && (pattern[patIdx] == '?' || pattern[patIdx] == id[idIdx]):
			patIdx++
			idIdx++
		case patIdx < len(pattern) && pattern[patIdx] == '*':
			star, mark = patIdx, idIdx
			patIdx++
		case star >= 0:
			// Let the last `*` absorb one more character.
			patIdx = star + 1
			mark++
			idIdx = mark
		default:
			return false
		}
	}
	for patIdx < len(pattern) && pattern[patIdx] == '*' {
		patIdx++
	}
	return patIdx == len(pattern)
}

// Specificity scores how precisely a path pattern is written, so that the most
// specific of several matching patterns can win. It is the number of literal
// characters before the first wildcard.
func Specificity(pattern string) int {
	if i := strings.IndexAny(pattern, "*?"); i >= 0 {
		return i
	}
	return len(pattern)
}

func splitPath(s string) []string {
	s = strings.Trim(s, "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func matchSegments(pattern, name []string) bool {
	if len(pattern) == 0 {
		return len(name) == 0
	}
	if pattern[0] == "**" {
		// Try consuming 0, 1, 2 … segments with the wildcard.
		for i := 0; i <= len(name); i++ {
			if matchSegments(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	if ok, err := path.Match(pattern[0], name[0]); err != nil || !ok {
		return false
	}
	return matchSegments(pattern[1:], name[1:])
}

package config

import (
	"path"
	"strings"
)

// MatchPath reports whether a slash-separated path matches a glob pattern.
//
// It extends path.Match with `**`, which matches any number of path segments
// including none. path.Match alone never lets `*` cross a separator, which
// makes recursive patterns impossible to express.
func MatchPath(pattern, name string) bool {
	return matchSegments(splitPath(pattern), splitPath(name))
}

// MatchRuleID reports whether a rule ID matches a config key pattern, where `*`
// matches any run of characters and `?` matches exactly one.
//
// Unlike MatchPath, `*` here crosses the `/` in a rule ID, so a key of `*`
// means every rule — the only thing anyone writing it could intend. The two
// matchers are different on purpose and must not be merged.
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

package diag

import "sort"

// Diagnostic is a single rule violation.
//
// A rule populates only Position, Message and Help. The runner stamps on RuleID
// and Severity from the rule set and the active configuration, which is what
// keeps a rule from ever needing to know how it was configured.
type Diagnostic struct {
	Position
	RuleID   string
	Severity Severity
	// Message states what is wrong, lowercase and without trailing
	// punctuation, matching Go's convention for error strings.
	Message string
	// Help states how to fix it, not what is wrong. Reporters with room for a
	// second line render it; terse formats drop it.
	Help string
}

// Sort orders diagnostics by path, line, column, then rule ID.
//
// Determinism here is not cosmetic: without a total order, two runs over an
// identical tree can print findings in different orders, and CI diffs become
// unreadable noise.
func Sort(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		switch {
		case a.Path != b.Path:
			return a.Path < b.Path
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.Col != b.Col:
			return a.Col < b.Col
		default:
			return a.RuleID < b.RuleID
		}
	})
}

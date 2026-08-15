package rule

import "github.com/Quikcad/goorg/pkg/lint/diag"

// CheckFunc inspects a loaded project and returns any violations. It fills in
// only Position, Message and Help; the runner supplies RuleID and Severity.
type CheckFunc func(*Context) []diag.Diagnostic

// Rule is one check goorg can run.
type Rule struct {
	// ID is the stable identifier used in configuration, output and
	// suppression comments. It is always "<category>/<kebab-case-name>", and
	// it is contractual: renaming one silently breaks every consumer's config.
	ID string
	// Category is the family this rule belongs to, matching the first segment
	// of ID.
	Category Category
	// Tier is what the rule needs in order to run.
	Tier Tier
	// Placement records what the rule's outcome depends on, which is what the
	// what-if pass uses to tell a real objection to a relocation from a
	// cosmetic one. The zero value makes a rule participate.
	Placement Placement
	// Summary is a single lowercase line, shown by `goorg rules`.
	Summary string
	// Doc is the long-form explanation shown by `goorg explain <id>`. It must
	// argue for the rule, not merely restate it.
	Doc string
	// Default is the severity applied when configuration does not mention the
	// rule. A rule shipped Off is opt-in.
	Default diag.Severity
	// Check performs the analysis.
	Check CheckFunc
}

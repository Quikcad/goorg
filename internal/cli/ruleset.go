package cli

import (
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// buildRuleSet composes every rule family into the set goorg runs.
//
// This is the one place families are wired together. There is no global
// registry and no init-time side effects, so the set is explicit, ordered, and
// trivially replaceable in a test. Each family will be added here as it lands:
//
//	directory.Rules(), organization.Rules(), logic.Rules(), pattern.Rules()
//
// Phase 1 ships the engine with no families, so the set is deliberately empty
// and `goorg check` reports nothing.
func buildRuleSet() (*rule.Set, error) {
	return rule.NewSet()
}

package cli

import (
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/rules/directory"
)

// buildRuleSet composes every rule family into the set goorg runs.
//
// This is the one place families are wired together. There is no global
// registry and no init-time side effects, so the set is explicit, ordered, and
// trivially replaceable in a test. Each family will be added here as it lands:
//
//	directory.Rules(), organization.Rules(), logic.Rules(), pattern.Rules()
//
// Families land here as they are written; organization, logic and pattern are
// still to come.
func buildRuleSet() (*rule.Set, error) {
	return rule.NewSet(
		directory.Rules(),
	)
}

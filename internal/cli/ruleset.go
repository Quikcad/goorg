package cli

import (
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/rules/directory"
	"github.com/Quikcad/goorg/pkg/rules/logic"
	"github.com/Quikcad/goorg/pkg/rules/organization"
	"github.com/Quikcad/goorg/pkg/rules/pattern"
)

// buildRuleSet composes every rule family into the set goorg runs.
//
// This is the one place families are wired together. There is no global
// registry and no init-time side effects, so the set is explicit, ordered, and
// trivially replaceable in a test. Each family will be added here as it lands:
//
//	directory.Rules(), organization.Rules(), logic.Rules(), pattern.Rules()
//
// All four syntax-tier families are wired in. The type tier adds five more
// rules to logic/ and org/ in phase 5.
func buildRuleSet() (*rule.Set, error) {
	return rule.NewSet(
		directory.Rules(),
		organization.Rules(),
		logic.Rules(),
		pattern.Rules(),
	)
}

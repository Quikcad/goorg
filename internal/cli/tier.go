package cli

import (
	"fmt"

	"github.com/Quikcad/goorg/pkg/lint/config"
	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/typed"
)

// TypeLoadRule is the meta-diagnostic ID for a type tier that could not run.
// The goorg/ namespace is reserved for the tool's own diagnostics.
const TypeLoadRule = "goorg/type-tier-unavailable"

// tierPlan is what the CLI resolved about which tiers will run.
type tierPlan struct {
	// tiers is handed to the runner.
	tiers map[rule.Tier]bool
	// program is the type-checked view, nil when the type tier is not running.
	program *typed.Program
	// gaps are diagnostics about coverage the run could not provide.
	gaps []diag.Diagnostic
	// failed reports that the type tier was wanted but could not be loaded.
	failed bool
}

// planTiers decides which tiers run, loading type information only when a
// type-tier rule is actually enabled.
//
// The cost of the type tier is real — comparable to a build — so a project
// whose type rules are all off never pays it. That is also what keeps
// `--syntax-only` honest: it is the same code path, not a second one.
func planTiers(root string, cfg *config.Config, set *rule.Set, syntaxOnly bool) tierPlan {
	plan := tierPlan{tiers: map[rule.Tier]bool{rule.Syntax: true}}

	wanted := enabledTypeRules(cfg, set)
	switch {
	case len(wanted) == 0:
		return plan
	case syntaxOnly:
		plan.gaps = append(plan.gaps, skipped(fmt.Sprintf(
			"--syntax-only skipped %s that need type information", plural(len(wanted), "rule"))))
		return plan
	}

	program, err := typed.Load(root)
	if err != nil {
		// Reporting the gap rather than passing silently is the whole point of
		// the tier split: a linter that quietly checks nothing is worse than
		// one that refuses to run. See docs/decisions.md D1.
		plan.failed = true
		plan.gaps = append(plan.gaps, diag.Diagnostic{
			Position: diag.Position{Path: "."},
			RuleID:   TypeLoadRule,
			Severity: diag.Error,
			Message: fmt.Sprintf("%s could not run: %v",
				plural(len(wanted), "type-tier rule"), err),
			Help: "fix the build, or pass --syntax-only to skip these rules deliberately",
		})
		return plan
	}

	for _, broken := range program.Broken() {
		plan.gaps = append(plan.gaps, diag.Diagnostic{
			Position: diag.Position{Path: broken.Dir},
			RuleID:   TypeLoadRule,
			Severity: diag.Error,
			Message:  "package did not type-check, so type-tier rules skipped it",
			Help:     "fix the build error; goorg cannot reason about types it could not resolve",
		})
	}

	plan.tiers[rule.Types] = true
	plan.program = program
	return plan
}

// enabledTypeRules returns the type-tier rules the configuration switches on.
func enabledTypeRules(cfg *config.Config, set *rule.Set) []*rule.Rule {
	var out []*rule.Rule
	for _, r := range set.All() {
		if r.Tier == rule.Types && cfg.Severity(r) != diag.Off {
			out = append(out, r)
		}
	}
	return out
}

func skipped(message string) diag.Diagnostic {
	return diag.Diagnostic{
		Position: diag.Position{Path: "."},
		RuleID:   TypeLoadRule,
		Severity: diag.Warning,
		Message:  message,
		Help:     "run without --syntax-only for full coverage",
	}
}

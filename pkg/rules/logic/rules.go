package logic

import (
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// Rules returns the logic/ family.
//
// Three further rules are specified but need type information and land in
// phase 5: logic/interface-registry, logic/any-should-be-generic and
// logic/ideal-numeric-type.
func Rules() []*rule.Rule {
	return []*rule.Rule{
		iotaCandidate,
		maxConditionOperands,
		maxObjectMembers,
		preferGuardClause,
	}
}

// files returns the non-test files the family inspects.
//
// Test files are exempt from the shape rules per docs/decisions.md D6: a
// table-driven test's helper functions and fixture structs are written for the
// test, not for a reader arriving from another package.
func files(c *rule.Context) []*project.File {
	var out []*project.File
	for _, f := range c.Project.Files() {
		if f.IsTest {
			continue
		}
		out = append(out, f)
	}
	return out
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// settingsError converts a settings decode failure into a diagnostic pointing
// at the config file, so a typo in .goorg.yaml surfaces the same way a
// violation does instead of crashing the run.
func settingsError(err error) []diag.Diagnostic {
	return []diag.Diagnostic{{
		Position: diag.Position{Path: ".goorg.yaml", Line: 1},
		Message:  fmt.Sprintf("invalid rule settings: %v", strings.TrimSpace(err.Error())),
		Help:     "check the settings block for this rule against `goorg explain <rule>`",
	}}
}

package organization

import (
	"fmt"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// fileDecls pairs a file with its classified declarations, so the ordering
// rules classify once rather than once each.
type fileDecls struct {
	file  *project.File
	decls []member
}

// orderedFiles returns the files the ordering and budget rules apply to.
//
// Test files are exempt per docs/decisions.md D6: table-driven tests carry many
// small helpers and would violate the budgets constantly, and their declaration
// order carries no meaning for a reader arriving from another package.
// Singleton files are exempt because org/singleton-layout defines their shape.
func orderedFiles(c *rule.Context) []*fileDecls {
	var out []*fileDecls
	for _, f := range c.Project.Files() {
		if f.IsTest {
			continue
		}
		if detectSingleton(f, defaultInstanceFunc) != nil {
			continue
		}
		out = append(out, &fileDecls{file: f, decls: classify(f)})
	}
	return out
}

// nonTestFiles returns every file the budget rules apply to.
func nonTestFiles(c *rule.Context) []*project.File {
	var out []*project.File
	for _, f := range c.Project.Files() {
		if f.IsTest {
			continue
		}
		out = append(out, f)
	}
	return out
}

// countFuncs tallies a file's top-level functions.
func countFuncs(f *project.File, countMethods bool) (exported, unexported int) {
	for _, d := range classify(f) {
		if !d.isFunc {
			continue
		}
		if d.isMethod && !countMethods {
			continue
		}
		if d.exported {
			exported++
			continue
		}
		unexported++
	}
	return exported, unexported
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

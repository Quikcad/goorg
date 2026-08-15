package ruletest

import (
	"go/ast"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// oneLineStruct is a stand-in rule used to exercise the harness before any real
// family exists. It is deliberately the shape of pat/expand-struct-definition,
// because that rule is the sharpest test of gofmt conformance: gofmt preserves
// whichever struct form the author wrote, which is the only reason the rule has
// room to exist.
var oneLineStruct = &rule.Rule{
	ID:       "pat/expand-struct-definition",
	Category: rule.Pattern,
	Tier:     rule.Syntax,
	Summary:  "struct definitions must span multiple lines",
	Default:  diag.Error,
	Doc: `A struct type with at least one field is written across multiple lines.

	type Point struct{ X, Y int }     violation
	type Point struct {               OK
		X int
		Y int
	}

Rationale: a one-line struct has not been budgeted for. Every field added later
either extends the line or forces a whole-declaration rewrite, so the line grows
instead and the diff of adding a field stops being reviewable.

struct{} is always exempt: the empty struct is a unit value, not a record, and
map[string]struct{} is load-bearing Go idiom.

To fix: put each field on its own line.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, pkg := range c.Project.Packages {
			for _, f := range pkg.Files {
				ast.Inspect(f.Syntax, func(n ast.Node) bool {
					st, ok := n.(*ast.StructType)
					if !ok || st.Fields == nil || st.Fields.NumFields() == 0 {
						return true
					}
					open := c.Project.Position(st.Fields.Opening).Line
					if open != c.Project.Position(st.Fields.Closing).Line {
						return true
					}
					out = append(out, diag.Diagnostic{
						Position: c.Pos(st),
						Message:  "struct definition is on one line",
						Help:     "put each field on its own line",
					})
					return true
				})
			}
		}
		return out
	},
}

func TestHarnessReportsFindings(t *testing.T) {
	got := Run(t, oneLineStruct, map[string]string{
		"pkg/a/a.go": "package a\n\ntype Point struct{ X, Y int }\n",
	})
	Assert(t, got, []string{"struct definition is on one line"})
}

func TestHarnessStaysSilentOnConformingCode(t *testing.T) {
	got := Run(t, oneLineStruct, map[string]string{
		"pkg/a/a.go": "package a\n\ntype Point struct {\n\tX int\n\tY int\n}\n",
	})
	Assert(t, got, nil)
}

// TestEmptyStructIsExempt guards the exemption that makes the rule usable at
// all: map[string]struct{} and chan struct{} must never be reported.
func TestEmptyStructIsExempt(t *testing.T) {
	got := Run(t, oneLineStruct, map[string]string{
		"pkg/a/a.go": "package a\n\nvar seen = map[string]struct{}{}\n\nvar done = make(chan struct{})\n",
	})
	Assert(t, got, nil)
}

// TestGofmtStableHarness proves the conformance check passes for a rule that
// genuinely does not compete with gofmt.
func TestGofmtStableHarness(t *testing.T) {
	AssertGofmtStable(t, oneLineStruct, map[string]string{
		"pkg/a/a.go": "package a\n\ntype Point struct{ X, Y int }\n\ntype Wide struct {\n\tA int\n}\n",
	})
}

// TestGofmtStableCatchesCompetingRule is the negative case: a rule keyed to
// whitespace gofmt normalizes must be caught by the harness, not shipped.
func TestGofmtStableCatchesCompetingRule(t *testing.T) {
	competing := &rule.Rule{
		ID:       "pat/expand-struct-definition",
		Category: rule.Pattern,
		Tier:     rule.Syntax,
		Summary:  "reports on indentation gofmt owns",
		Default:  diag.Error,
		Check: func(c *rule.Context) []diag.Diagnostic {
			var out []diag.Diagnostic
			for _, pkg := range c.Project.Packages {
				for _, f := range pkg.Files {
					for i := 1; i <= f.Lines; i++ {
						if text := f.LineText(i); len(text) > 0 && text[0] == ' ' {
							out = append(out, diag.Diagnostic{
								Position: diag.Position{Path: f.Rel, Line: i},
								Message:  "line starts with a space",
							})
						}
					}
				}
			}
			return out
		},
	}

	fake := &testing.T{}
	AssertGofmtStable(fake, competing, map[string]string{
		"pkg/a/a.go": "package a\n\ntype Point struct {\n    X int\n}\n",
	})
	if !fake.Failed() {
		t.Error("harness accepted a rule that reports on whitespace gofmt rewrites")
	}
}

func TestAssertWellFormed(t *testing.T) {
	AssertWellFormed(t, oneLineStruct)
}

func TestAssertDeterministic(t *testing.T) {
	AssertDeterministic(t, oneLineStruct, map[string]string{
		"pkg/a/a.go": "package a\n\ntype P struct{ X int }\n",
		"pkg/b/b.go": "package b\n\ntype Q struct{ Y int }\n",
	})
}

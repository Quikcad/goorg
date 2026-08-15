package whatif

import (
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// budget is a stand-in rule: it reports a file holding more than max
// declarations. The judge must be testable without depending on a real family.
func budget(max int) *rule.Rule {
	return &rule.Rule{
		ID:       "org/max-functions-per-file",
		Category: rule.Organization,
		Tier:     rule.Syntax,
		Check: func(c *rule.Context) []diag.Diagnostic {
			var out []diag.Diagnostic
			for _, f := range c.Project.Files() {
				if len(f.Syntax.Decls) <= max {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: rule.FilePos(f),
					Message:  "too many declarations",
				})
			}
			return out
		},
	}
}

// ordering is a stand-in rule that only cares about position within a file, so
// it must never veto a move.
func ordering() *rule.Rule {
	return &rule.Rule{
		ID:        "org/member-order",
		Category:  rule.Organization,
		Tier:      rule.Syntax,
		Placement: rule.PlacementOrder,
		Check: func(c *rule.Context) []diag.Diagnostic {
			var out []diag.Diagnostic
			for _, f := range c.Project.Files() {
				// Every file is wrong, always. A participating rule saying this
				// would reject every move.
				out = append(out, diag.Diagnostic{Position: rule.FilePos(f), Message: "bad order"})
			}
			return out
		},
	}
}

func load(t *testing.T, files map[string]string) *project.Project {
	t.Helper()
	return ruletest.Load(t, files)
}

func relocation(name, from, to string, line int) rule.Relocation {
	return rule.Relocation{
		Name: name, From: from, To: to, Line: line,
		Finding: diag.Diagnostic{
			Position: diag.Position{Path: from, Line: line},
			Message:  name + " is used only from " + to,
		},
	}
}

// TestApplyMovesTheDeclaration is the mechanism itself: the source really moves
// between files, and both still parse.
func TestApplyMovesTheDeclaration(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\n// helper does a thing.\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": "package b\n\nfunc Caller() int { return helper() }\n",
	})

	moved, err := Apply(p, relocation("helper", "pkg/a/b/a.go", "pkg/a/b/c.go", 4))
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	from := moved.FileAt("pkg/a/b/a.go")
	to := moved.FileAt("pkg/a/b/c.go")
	if strings.Contains(string(from.Src), "func helper") {
		t.Error("the declaration is still in the source file")
	}
	if !strings.Contains(string(to.Src), "func helper") {
		t.Error("the declaration did not arrive in the destination")
	}
	// The doc comment belongs to the declaration and moves with it.
	if !strings.Contains(string(to.Src), "// helper does a thing.") {
		t.Error("the doc comment was left behind")
	}
	if len(from.Syntax.Decls) != 0 || len(to.Syntax.Decls) != 2 {
		t.Errorf("declaration counts after the move: from=%d to=%d",
			len(from.Syntax.Decls), len(to.Syntax.Decls))
	}
}

func TestSafeMoveIsAccepted(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": "package b\n\nfunc Caller() int { return helper() }\n",
	})

	j := &Judge{Rules: []*rule.Rule{budget(5)}}
	got := j.Evaluate(p, []rule.Relocation{relocation("helper", "pkg/a/b/a.go", "pkg/a/b/c.go", 3)})
	if len(got.Accepted) != 1 {
		t.Fatalf("accepted %d proposals, want 1 (rejected=%d)", len(got.Accepted), got.Rejected)
	}
}

// TestMoveThatBreaksABudgetIsRejected is the whole point: the rule proposing
// the move does not have to know the budget exists.
func TestMoveThatBreaksABudgetIsRejected(t *testing.T) {
	var full strings.Builder
	full.WriteString("package b\n")
	for i := range 3 {
		full.WriteString("\nfunc f")
		full.WriteByte(byte('0' + i))
		full.WriteString("() int { return helper() }\n")
	}

	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": full.String(),
	})

	// The destination is already at the limit, so taking one more breaks it.
	j := &Judge{Rules: []*rule.Rule{budget(3)}}
	got := j.Evaluate(p, []rule.Relocation{relocation("helper", "pkg/a/b/a.go", "pkg/a/b/c.go", 3)})
	if len(got.Accepted) != 0 {
		t.Errorf("accepted a move that breaks a budget: %v", got.Accepted)
	}
	if got.Rejected != 1 {
		t.Errorf("rejected = %d, want 1", got.Rejected)
	}
}

// TestOrderingRulesDoNotVeto pins the Placement distinction. Where the
// declaration lands inside its new file is a separate fix, so a rule about
// ordering must not block the move.
func TestOrderingRulesDoNotVeto(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": "package b\n\nfunc Caller() int { return helper() }\n",
	})

	// The ordering rule reports one finding per file and would otherwise make
	// the count rise as soon as anything changes.
	j := &Judge{Rules: []*rule.Rule{ordering(), budget(5)}}
	got := j.Evaluate(p, []rule.Relocation{relocation("helper", "pkg/a/b/a.go", "pkg/a/b/c.go", 3)})
	if len(got.Accepted) != 1 {
		t.Errorf("an ordering rule vetoed a move: accepted=%d rejected=%d",
			len(got.Accepted), got.Rejected)
	}
}

// TestDisabledRuleCannotVeto: a rule the project switched off has no standing.
func TestDisabledRuleCannotVeto(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": "package b\n\nfunc Caller() int { return helper() }\nfunc d() {}\nfunc e() {}\n",
	})

	off := func(*rule.Rule) diag.Severity { return diag.Off }
	j := &Judge{Rules: []*rule.Rule{budget(1)}, Severity: off}
	got := j.Evaluate(p, []rule.Relocation{relocation("helper", "pkg/a/b/a.go", "pkg/a/b/c.go", 3)})
	if len(got.Accepted) != 1 {
		t.Errorf("a disabled rule vetoed a move: accepted=%d", len(got.Accepted))
	}
}

// TestCycleIsDropped covers the guard: two files each wanting to send a
// declaration to the other would walk the code in a circle.
func TestCycleIsDropped(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc one() int { return 1 }\n",
		"pkg/a/b/c.go": "package b\n\nfunc two() int { return 2 }\n",
	})

	j := &Judge{Rules: []*rule.Rule{budget(9)}}
	got := j.Evaluate(p, []rule.Relocation{
		relocation("one", "pkg/a/b/a.go", "pkg/a/b/c.go", 3),
		relocation("two", "pkg/a/b/c.go", "pkg/a/b/a.go", 3),
	})
	if len(got.Accepted) != 0 {
		t.Errorf("accepted a move from a cycle: %v", got.Accepted)
	}
	if got.Cyclic != 2 {
		t.Errorf("cyclic = %d, want 2", got.Cyclic)
	}
}

// TestEvaluationIsDeterministic guards against map iteration reaching the
// output, which would make CI results flap.
func TestEvaluationIsDeterministic(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc one() int { return 1 }\n\nfunc two() int { return 2 }\n",
		"pkg/a/b/c.go": "package b\n\nfunc Caller() int { return one() + two() }\n",
	})
	proposals := []rule.Relocation{
		relocation("two", "pkg/a/b/a.go", "pkg/a/b/c.go", 5),
		relocation("one", "pkg/a/b/a.go", "pkg/a/b/c.go", 3),
	}

	j := &Judge{Rules: []*rule.Rule{budget(9)}}
	first := j.Evaluate(p, proposals)
	for range 5 {
		got := j.Evaluate(p, proposals)
		if len(got.Accepted) != len(first.Accepted) {
			t.Fatalf("accepted count varied between runs: %d then %d",
				len(first.Accepted), len(got.Accepted))
		}
		for i := range got.Accepted {
			if got.Accepted[i].Message != first.Accepted[i].Message {
				t.Fatalf("order varied between runs at %d", i)
			}
		}
	}
}

// TestUnmodellableMoveIsRejected: goorg must not recommend a move it could not
// even make.
func TestUnmodellableMoveIsRejected(t *testing.T) {
	p := load(t, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/a.go": "package b\n\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": "package b\n",
	})

	j := &Judge{Rules: []*rule.Rule{budget(9)}}
	got := j.Evaluate(p, []rule.Relocation{
		relocation("ghost", "pkg/a/b/a.go", "pkg/a/b/c.go", 99),
	})
	if len(got.Accepted) != 0 || got.Rejected != 1 {
		t.Errorf("accepted=%d rejected=%d, want 0 and 1", len(got.Accepted), got.Rejected)
	}
}

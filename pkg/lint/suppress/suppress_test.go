package suppress

import (
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
	"github.com/Quikcad/goorg/pkg/source/project"
)

func load(t *testing.T, body string) *project.Project {
	t.Helper()
	return ruletest.Load(t, map[string]string{"pkg/a/a.go": body})
}

func finding(line int, ruleID string) diag.Diagnostic {
	return diag.Diagnostic{
		Position: diag.Position{Path: "pkg/a/a.go", Line: line},
		RuleID:   ruleID,
		Severity: diag.Error,
		Message:  "something is wrong",
	}
}

func TestSuppressesFollowingDeclaration(t *testing.T) {
	p := load(t, `package a

//goorg:ignore org/max-private-functions — state machine, splitting hurts
func lex(s string) int {
	return len(s)
}

func other() {}
`)
	set, problems := Scan(p)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if set.Len() != 1 {
		t.Fatalf("found %d directives, want 1", set.Len())
	}

	// The directive covers the whole func declaration, lines 4 to 6.
	for _, line := range []int{4, 5, 6} {
		got := set.Apply([]diag.Diagnostic{finding(line, "org/max-private-functions")})
		if len(got) != 0 {
			t.Errorf("line %d was not suppressed", line)
		}
	}
	// Line 8 is outside the declaration.
	if got := set.Apply([]diag.Diagnostic{finding(8, "org/max-private-functions")}); len(got) != 1 {
		t.Error("suppression leaked past the declaration it precedes")
	}
}

func TestSuppressesOnlyTheNamedRule(t *testing.T) {
	p := load(t, `package a

//goorg:ignore org/member-order — deliberate
func f() {}
`)
	set, _ := Scan(p)
	if got := set.Apply([]diag.Diagnostic{finding(4, "org/member-order")}); len(got) != 0 {
		t.Error("named rule was not suppressed")
	}

	set, _ = Scan(p)
	if got := set.Apply([]diag.Diagnostic{finding(4, "logic/max-object-members")}); len(got) != 1 {
		t.Error("suppression silenced a rule it does not name")
	}
}

func TestTrailingDirectiveCoversItsOwnLineOnly(t *testing.T) {
	p := load(t, `package a

func f() {
	x := 1 //goorg:ignore logic/magic-number — tuned by hand
	y := 2
	_, _ = x, y
}
`)
	set, problems := Scan(p)
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if got := set.Apply([]diag.Diagnostic{finding(4, "logic/magic-number")}); len(got) != 0 {
		t.Error("trailing directive did not suppress its own line")
	}

	set, _ = Scan(p)
	if got := set.Apply([]diag.Diagnostic{finding(5, "logic/magic-number")}); len(got) != 1 {
		t.Error("trailing directive leaked onto the next line")
	}
}

// TestReasonIsMandatory is the point of D4: a suppression nobody can evaluate
// later is worse than the finding it hides.
func TestReasonIsMandatory(t *testing.T) {
	p := load(t, `package a

//goorg:ignore org/member-order
func f() {}
`)
	set, problems := Scan(p)
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want 1", len(problems))
	}
	if problems[0].RuleID != InvalidRule {
		t.Errorf("rule = %q, want %q", problems[0].RuleID, InvalidRule)
	}
	if !strings.Contains(problems[0].Message, "no reason") {
		t.Errorf("message = %q, want it to mention the missing reason", problems[0].Message)
	}
	// A directive without a reason is not honoured.
	if got := set.Apply([]diag.Diagnostic{finding(4, "org/member-order")}); len(got) != 1 {
		t.Error("a reasonless directive suppressed a finding")
	}
}

func TestDirectiveNamingNoRuleIsReported(t *testing.T) {
	p := load(t, "package a\n\n//goorg:ignore\nfunc f() {}\n")
	_, problems := Scan(p)
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "names no rule") {
		t.Fatalf("problems = %v, want one about a missing rule ID", problems)
	}
}

// TestStaleSuppressionIsReported catches the directive left behind after the
// code it excused was fixed.
func TestStaleSuppressionIsReported(t *testing.T) {
	p := load(t, `package a

//goorg:ignore org/member-order — no longer true
func f() {}
`)
	set, _ := Scan(p)
	set.Apply(nil)

	stale := set.Stale()
	if len(stale) != 1 {
		t.Fatalf("got %d stale findings, want 1", len(stale))
	}
	if stale[0].RuleID != StaleRule {
		t.Errorf("rule = %q, want %q", stale[0].RuleID, StaleRule)
	}
	if stale[0].Severity != diag.Warning {
		t.Errorf("severity = %v, want warning", stale[0].Severity)
	}
}

func TestUsedSuppressionIsNotStale(t *testing.T) {
	p := load(t, `package a

//goorg:ignore org/member-order — deliberate
func f() {}
`)
	set, _ := Scan(p)
	set.Apply([]diag.Diagnostic{finding(4, "org/member-order")})
	if stale := set.Stale(); len(stale) != 0 {
		t.Errorf("a directive that fired was reported stale: %v", stale)
	}
}

// TestMetaDiagnosticsAreNotSuppressible stops a broken directive from
// suppressing the report of its own brokenness, which would be unfixable.
func TestMetaDiagnosticsAreNotSuppressible(t *testing.T) {
	p := load(t, `package a

//goorg:ignore goorg/invalid-suppression — trying to hide
func f() {}
`)
	set, _ := Scan(p)
	got := set.Apply([]diag.Diagnostic{{
		Position: diag.Position{Path: "pkg/a/a.go", Line: 4},
		RuleID:   InvalidRule,
		Severity: diag.Error,
		Message:  "suppression gives no reason",
	}})
	if len(got) != 1 {
		t.Error("a meta-diagnostic was suppressed")
	}
}

func TestSeparatorsAreOptional(t *testing.T) {
	for _, form := range []string{
		"//goorg:ignore org/member-order — em dash",
		"//goorg:ignore org/member-order -- double hyphen",
		"//goorg:ignore org/member-order: colon",
		"//goorg:ignore org/member-order plain prose",
	} {
		p := load(t, "package a\n\n"+form+"\nfunc f() {}\n")
		set, problems := Scan(p)
		if len(problems) != 0 {
			t.Errorf("%q was rejected: %v", form, problems)
			continue
		}
		if set.Len() != 1 {
			t.Errorf("%q produced %d directives, want 1", form, set.Len())
		}
	}
}

package cli

import (
	"fmt"
	"strings"
	"testing"
)

// typedProject compiles, so the type tier can run over it.
var typedProject = map[string]string{
	"go.mod": "module example.com/typed\n\ngo 1.25.0\n",
	"pkg/billing/invoice/invoice.go": `package invoice

// Status is an enum whose String method is one character from being a Stringer.
type Status int

const (
	StatusDraft Status = iota
	StatusSent
)

func (s Status) String() (string, error) { return "draft", nil }
`,
}

// brokenProject parses but does not type-check.
var brokenProject = map[string]string{
	"go.mod":             "module example.com/broken\n\ngo 1.25.0\n",
	"pkg/a/b/b.go":       "package b\n\nfunc f() { return undefinedSymbol() }\n",
	"pkg/a/fine/fine.go": "package fine\n",
	"pkg/a/fine/more.go": "package fine\n",
}

// TestTypeTierRuns proves the near-miss check, which is the half of
// logic/interface-registry that catches real bugs.
func TestTypeTierRuns(t *testing.T) {
	root := fixture(t, typedProject)
	stdout, stderr, code := exec(t, nil, "check", "-root", root)
	if code != ExitFindings {
		t.Fatalf("exit = %d, want %d\nstdout: %s\nstderr: %s", code, ExitFindings, stdout, stderr)
	}
	if !strings.Contains(stdout, "logic/interface-registry") {
		t.Errorf("type tier did not run:\n%s", stdout)
	}
	if !strings.Contains(stdout, "wrong signature for fmt.Stringer") {
		t.Errorf("near miss not reported:\n%s", stdout)
	}
}

// TestSyntaxOnlySkipsTypeTier covers the fast path, and proves the skip is
// announced rather than silent.
func TestSyntaxOnlySkipsTypeTier(t *testing.T) {
	root := fixture(t, typedProject)
	stdout, _, code := exec(t, nil, "check", "-root", root, "-syntax-only")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, stdout)
	}
	if strings.Contains(stdout, "logic/interface-registry") {
		t.Errorf("--syntax-only still ran a type rule:\n%s", stdout)
	}
	if !strings.Contains(stdout, "goorg/type-tier-unavailable") {
		t.Errorf("the skipped coverage was not announced:\n%s", stdout)
	}
}

// TestBrokenPackageIsReportedNotSkipped is the guarantee D1 exists for: a
// package that will not type-check must never look like a package that passed.
func TestBrokenPackageIsReportedNotSkipped(t *testing.T) {
	root := fixture(t, brokenProject)
	stdout, _, code := exec(t, nil, "check", "-root", root)
	if code == ExitOK {
		t.Fatalf("a package that did not type-check exited 0:\n%s", stdout)
	}
	if !strings.Contains(stdout, "goorg/type-tier-unavailable") {
		t.Errorf("the coverage gap was not reported:\n%s", stdout)
	}
	if !strings.Contains(stdout, "did not type-check") {
		t.Errorf("the message does not say what happened:\n%s", stdout)
	}
}

// TestSyntaxOnlySurvivesABrokenBuild is the escape hatch the failure message
// points at: the syntax tier still works on a tree that does not compile.
func TestSyntaxOnlySurvivesABrokenBuild(t *testing.T) {
	root := fixture(t, brokenProject)
	stdout, _, code := exec(t, nil, "check", "-root", root, "-syntax-only")
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d — the syntax tier must not need a working build\n%s",
			code, ExitOK, stdout)
	}
}

// TestTypeTierNotLoadedWhenUnused keeps the cost off projects that switched the
// type rules off.
func TestTypeTierNotLoadedWhenUnused(t *testing.T) {
	files := map[string]string{}
	for k, v := range typedProject {
		files[k] = v
	}
	files[".goorg.yaml"] = "version: 1\nrules:\n  logic/*: off\n  org/consumer-locality: off\n  org/global-file-scoped: off\n"

	root := fixture(t, files)
	stdout, _, _ := exec(t, nil, "check", "-root", root)
	if strings.Contains(stdout, "goorg/type-tier-unavailable") {
		t.Errorf("type tier was consulted although no type rule is enabled:\n%s", stdout)
	}
}

// relocationProject has one unexported helper whose only consumer is another
// file. Whether moving it is good advice depends entirely on the destination's
// budget, which is what the what-if pass exists to measure.
func relocationProject(limit string) map[string]string {
	var caller strings.Builder
	caller.WriteString("package b\n")
	for i := range 15 {
		fmt.Fprintf(&caller, "\nfunc F%d() int { return helper() }\n", i)
	}
	return map[string]string{
		"go.mod":       "module example.com/reloc\n\ngo 1.25.0\n",
		"pkg/a/b/a.go": "package b\n\nfunc Exported() int { return 0 }\n\nfunc helper() int { return 1 }\n",
		"pkg/a/b/c.go": caller.String(),
		".goorg.yaml": "version: 1\nrules:\n  \"*\": \"off\"\n" +
			"  org/consumer-locality: warning\n  org/max-functions-per-file: error\n" +
			"settings:\n  org/max-functions-per-file:\n    limit: " + limit + "\n" +
			"  org/consumer-locality:\n    max_target_declarations: 99\n",
	}
}

// TestWhatIfRejectsAMoveThatBreaksABudget is the pass working: the rule
// proposing the move knows nothing about the budget, and the engine finds out
// by making the move and looking.
func TestWhatIfRejectsAMoveThatBreaksABudget(t *testing.T) {
	t.Run("destination is full, so no advice", func(t *testing.T) {
		root := fixture(t, relocationProject("15"))
		stdout, _, _ := exec(t, nil, "check", "-root", root)
		if strings.Contains(stdout, "consumer-locality") {
			t.Errorf("advised a move that would break a budget:\n%s", stdout)
		}
	})

	t.Run("destination has room, so the advice stands", func(t *testing.T) {
		root := fixture(t, relocationProject("20"))
		stdout, _, _ := exec(t, nil, "check", "-root", root)
		if !strings.Contains(stdout, "helper is used only from") {
			t.Errorf("withheld advice for a move that is safe:\n%s", stdout)
		}
	})

	// Without the pass, the rule falls back to its own approximation, which is
	// deliberately conservative rather than measured.
	t.Run("--no-what-if reports without checking", func(t *testing.T) {
		root := fixture(t, relocationProject("15"))
		stdout, _, _ := exec(t, nil, "check", "-root", root, "-no-what-if")
		if !strings.Contains(stdout, "helper is used only from") {
			t.Errorf("the unexamined form should still advise:\n%s", stdout)
		}
	})
}

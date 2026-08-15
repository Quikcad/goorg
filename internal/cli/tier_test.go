package cli

import (
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

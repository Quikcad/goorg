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

// interfaceOrderProject declares one interface and three implementations: one
// in the interface's order, one scrambled, and one that interleaves an
// unrelated method among them.
func interfaceOrderProject(settings string) map[string]string {
	return map[string]string{
		"go.mod": "module example.com/imo\n\ngo 1.25.0\n",
		"pkg/a/b/b.go": `package b

type Store interface {
	Open() error
	Read() ([]byte, error)
	Close() error
}

type Good struct{}

var _ Store = (*Good)(nil)

func (g *Good) Open() error            { return nil }
func (g *Good) Read() ([]byte, error)  { return nil, nil }
func (g *Good) Close() error           { return nil }

type Scrambled struct{}

var _ Store = (*Scrambled)(nil)

func (s *Scrambled) Close() error           { return nil }
func (s *Scrambled) Open() error            { return nil }
func (s *Scrambled) Read() ([]byte, error)  { return nil, nil }

type Interleaved struct{}

var _ Store = (*Interleaved)(nil)

func (i *Interleaved) Open() error           { return nil }
func (i *Interleaved) Flush() error          { return nil }
func (i *Interleaved) Read() ([]byte, error) { return nil, nil }
func (i *Interleaved) Close() error          { return nil }

type Undeclared struct{}

func (u *Undeclared) Close() error           { return nil }
func (u *Undeclared) Open() error            { return nil }
func (u *Undeclared) Read() ([]byte, error)  { return nil, nil }
`,
		".goorg.yaml": "version: 1\nrules:\n  \"*\": \"off\"\n  org/interface-method-order: error\n" + settings,
	}
}

func TestInterfaceMethodOrder(t *testing.T) {
	t.Run("scrambled order is reported, matching order is not", func(t *testing.T) {
		root := fixture(t, interfaceOrderProject(""))
		stdout, _, code := exec(t, nil, "check", "-root", root)
		if code != ExitFindings {
			t.Fatalf("exit = %d, want %d\n%s", code, ExitFindings, stdout)
		}
		if !strings.Contains(stdout, "Scrambled declares Store methods as Close, Open, Read") {
			t.Errorf("the scrambled implementation was not reported:\n%s", stdout)
		}
		// The message names both orders, so the fix needs no second lookup.
		if !strings.Contains(stdout, "the interface declares them Open, Read, Close") {
			t.Errorf("the message does not say what the order should be:\n%s", stdout)
		}
		if strings.Contains(stdout, "Good declares") {
			t.Errorf("an implementation in the right order was reported:\n%s", stdout)
		}
	})

	// A method the interface does not mention may sit anywhere; only the
	// relative order of the interface's own methods is constrained.
	t.Run("interleaving is allowed by default", func(t *testing.T) {
		root := fixture(t, interfaceOrderProject(""))
		stdout, _, _ := exec(t, nil, "check", "-root", root)
		if strings.Contains(stdout, "Interleaved") {
			t.Errorf("interleaving was reported without contiguous set:\n%s", stdout)
		}
	})

	t.Run("contiguous also requires them together", func(t *testing.T) {
		root := fixture(t, interfaceOrderProject("settings:\n  org/interface-method-order:\n    contiguous: true\n"))
		stdout, _, _ := exec(t, nil, "check", "-root", root)
		if !strings.Contains(stdout, "Interleaved interleaves other methods") {
			t.Errorf("contiguous did not report the interleaved implementation:\n%s", stdout)
		}
	})

	// Structural satisfaction is often accidental, so a type that never said it
	// implements the interface owes it no ordering.
	t.Run("undeclared implementations are ignored by default", func(t *testing.T) {
		root := fixture(t, interfaceOrderProject(""))
		stdout, _, _ := exec(t, nil, "check", "-root", root)
		if strings.Contains(stdout, "Undeclared") {
			t.Errorf("a type with no assertion was reported:\n%s", stdout)
		}
	})

	t.Run("include_implicit reaches them", func(t *testing.T) {
		root := fixture(t, interfaceOrderProject("settings:\n  org/interface-method-order:\n    include_implicit: true\n"))
		stdout, _, _ := exec(t, nil, "check", "-root", root)
		if !strings.Contains(stdout, "Undeclared declares Store methods") {
			t.Errorf("include_implicit did not reach an undeclared implementation:\n%s", stdout)
		}
	})

	// One method imposes no order, so a single-method interface must never fire.
	t.Run("a one-method interface constrains nothing", func(t *testing.T) {
		root := fixture(t, map[string]string{
			"go.mod": "module example.com/one\n\ngo 1.25.0\n",
			"pkg/a/b/b.go": `package b

type Closer interface {
	Close() error
}

type T struct{}

var _ Closer = (*T)(nil)

func (t *T) Other() error { return nil }
func (t *T) Close() error { return nil }
`,
			".goorg.yaml": "version: 1\nrules:\n  \"*\": \"off\"\n  org/interface-method-order: error\n",
		})
		_, _, code := exec(t, nil, "check", "-root", root)
		if code != ExitOK {
			t.Errorf("a single-method interface produced a finding")
		}
	})
}

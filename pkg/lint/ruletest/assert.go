package ruletest

import (
	"go/format"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/rule"
)

// minDocLength is the floor enforced by AssertWellFormed.
const minDocLength = 200

// Assert compares findings against expected substrings, one per finding, in
// sorted order.
func Assert(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d\ngot:\n  %s\nwant substrings:\n  %s",
			len(got), len(want), join(got), join(want))
	}
	for i := range got {
		if !strings.Contains(got[i], want[i]) {
			t.Errorf("finding %d = %q, want it to contain %q", i, got[i], want[i])
		}
	}
}

// AssertDeterministic runs a rule repeatedly and fails if the findings differ.
//
// Map iteration order is randomized, so a rule that returns findings derived
// from a map without sorting produces CI results that flap between identical
// runs. That is the single most corrosive bug a linter can have, because it
// trains people to re-run the job rather than read it.
func AssertDeterministic(t *testing.T, r *rule.Rule, files map[string]string) {
	t.Helper()
	first := Run(t, r, files)
	for range 5 {
		if got := Run(t, r, files); !equal(got, first) {
			t.Fatalf("%s produced different findings across runs:\n%v\n%v", r.ID, first, got)
		}
	}
}

// AssertGofmtStable is the conformance check that keeps goorg out of gofmt's
// territory.
//
// It reformats every fixture with gofmt and asserts the rule reports exactly
// the same findings. A rule that fails this is either reporting on something
// gofmt rewrites — in which case gofmt and goorg would fight forever — or its
// finding disappears once the file is formatted, which makes it unactionable.
//
// This runs for every rule, not only the formatting-adjacent ones.
func AssertGofmtStable(t *testing.T, r *rule.Rule, files map[string]string) {
	t.Helper()

	formatted := make(map[string]string, len(files))
	for name, body := range files {
		if filepath.Ext(name) != ".go" {
			formatted[name] = body
			continue
		}
		out, err := format.Source([]byte(body))
		if err != nil {
			// A fixture that gofmt cannot parse is a fixture bug, unless the
			// rule under test is specifically about unparseable input.
			t.Fatalf("gofmt could not parse fixture %s: %v", name, err)
		}
		formatted[name] = string(out)
	}

	before := Run(t, r, files)
	after := Run(t, r, formatted)
	if equal(before, after) {
		return
	}
	t.Errorf("%s reports differently after gofmt, so it competes with the formatter\n"+
		"before gofmt:\n  %s\nafter gofmt:\n  %s", r.ID, join(before), join(after))
}

// AssertWellFormed holds a rule to the documentation standard the CLI promises.
//
// `goorg explain` is the only reason anyone adopts a rule they did not write,
// so a rule that cannot argue for itself will simply be switched off.
func AssertWellFormed(t *testing.T, r *rule.Rule) {
	t.Helper()
	if got, want := r.ID[:len(r.Category)+1], string(r.Category)+"/"; got != want {
		t.Errorf("ID %q does not start with its category %q", r.ID, r.Category)
	}
	if r.Summary == "" {
		t.Error("empty Summary")
	}
	if strings.HasSuffix(r.Summary, ".") {
		t.Errorf("Summary should not end in a period: %q", r.Summary)
	}
	if len(r.Doc) < minDocLength {
		t.Errorf("Doc is %d chars; a rule needs enough explanation to argue for itself", len(r.Doc))
	}
	if !strings.Contains(r.Doc, "Rationale:") {
		t.Error("Doc does not state a rationale")
	}
	if !strings.Contains(r.Doc, "To fix:") {
		t.Error("Doc does not say how to fix a violation")
	}
	if r.Check == nil {
		t.Error("nil Check")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func join(items []string) string {
	return strings.Join(items, "\n  ")
}

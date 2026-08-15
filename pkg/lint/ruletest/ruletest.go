// Package ruletest is the harness every rule family tests against.
//
// Fixtures are in-memory file maps rather than files in testdata/, for two
// reasons: **/testdata/** is excluded by default so a fixture there would never
// be loaded, and keeping the source next to the finding it should produce makes
// a case readable as one unit.
package ruletest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/source/project"
)

// Tree writes an in-memory file map into a temporary directory and returns its
// path.
func Tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Load parses a fixture tree, failing the test if any fixture does not parse.
func Load(t *testing.T, files map[string]string) *project.Project {
	t.Helper()
	proj, err := project.Load(Tree(t, files), project.Options{})
	if err != nil {
		t.Fatalf("load project: %v", err)
	}
	for _, pe := range proj.ParseErrors {
		t.Fatalf("fixture failed to parse: %s: %s", pe.Position, pe.Message)
	}
	return proj
}

// Run executes one rule over a fixture tree and returns its findings as
// "path:line: message" strings, sorted.
func Run(t *testing.T, r *rule.Rule, files map[string]string) []string {
	t.Helper()
	requireSyntaxTier(t, r)
	return formatFindings(r.Check(rule.NewContext(Load(t, files), nil)))
}

// SyntaxTier filters a family down to the rules this harness can run.
//
// A type-tier rule needs a type-checked program, which an in-memory fixture
// does not have; family-wide assertions use this so adding a type rule does not
// silently start nil-dereferencing inside the harness.
func SyntaxTier(rules []*rule.Rule) []*rule.Rule {
	var out []*rule.Rule
	for _, r := range rules {
		if r.Tier == rule.Syntax {
			out = append(out, r)
		}
	}
	return out
}

// RunWith executes a rule over a fixture tree with explicit settings, which are
// round-tripped through YAML exactly as a real .goorg.yaml would be. That keeps
// a settings test honest about the decoding path rather than reaching past it.
//
//goorg:ignore logic/any-should-be-generic — yaml.Marshal erases the type regardless
func RunWith(t *testing.T, r *rule.Rule, files map[string]string, settings any) []string {
	t.Helper()
	requireSyntaxTier(t, r)
	data, err := yaml.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	decode := func(dst any) error { return yaml.Unmarshal(data, dst) }
	return formatFindings(r.Check(rule.NewContext(Load(t, files), decode)))
}

func formatFindings(ds []diag.Diagnostic) []string {
	var out []string
	for _, d := range ds {
		if d.Line > 0 {
			out = append(out, fmt.Sprintf("%s:%d: %s", d.Path, d.Line, d.Message))
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s", d.Path, d.Message))
	}
	sort.Strings(out)
	return out
}

func requireSyntaxTier(t *testing.T, r *rule.Rule) {
	t.Helper()
	if r.Tier != rule.Syntax {
		t.Fatalf("%s is %s-tier; the fixture harness has no type information. "+
			"Filter with ruletest.SyntaxTier, or exercise it through internal/cli.", r.ID, r.Tier)
	}
}

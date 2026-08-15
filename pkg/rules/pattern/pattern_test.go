package pattern

import (
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
)

// conforming satisfies both rules in the family.
var conforming = map[string]string{
	"go.mod": "module example.com/ok\n\ngo 1.25.0\n",
	"pkg/billing/invoice/invoice.go": `package invoice

type Invoice struct {
	ID    string
	Total int
}

type Ref struct {
	Key string
}

func NewInvoice(id string) *Invoice { return &Invoice{ID: id} }

func MakeRef(key string) Ref { return Ref{Key: key} }

func Parse(s string) (Ref, error) { return Ref{Key: s}, nil }

var seen = map[string]struct{}{}

var done = make(chan struct{})
`,
}

func TestExpandStructDefinition(t *testing.T) {
	t.Run("one-line struct", func(t *testing.T) {
		got := ruletest.Run(t, expandStructDefinition, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Point struct{ X, Y int }\n",
		})
		ruletest.Assert(t, got, []string{"struct with 2 fields is written on one line"})
	})

	t.Run("grouped fields on one line", func(t *testing.T) {
		got := ruletest.Run(t, expandStructDefinition, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Point struct {\n\tX, Y int\n\tZ    int\n}\n",
		})
		ruletest.Assert(t, got, []string{"2 fields share one declaration"})
	})

	// struct{} is a unit value, not a record. map[string]struct{} and
	// chan struct{} are load-bearing Go idiom and must never be reported.
	t.Run("empty struct is always exempt", func(t *testing.T) {
		got := ruletest.Run(t, expandStructDefinition, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

type Marker struct{}

var seen = map[string]struct{}{}

var done = make(chan struct{})

func f() { seen["x"] = struct{}{} }
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("anonymous struct in a table test", func(t *testing.T) {
		got := ruletest.Run(t, expandStructDefinition, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nvar rows = []struct{ Name string }{}\n",
		})
		ruletest.Assert(t, got, []string{"struct with 1 field is written on one line"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, expandStructDefinition, conforming), nil)
	})
}

// TestExpandStructSurvivesGofmt is the sharpest test of the conformance
// harness: this is the rule closest to the formatter's territory, and it only
// has room to exist because gofmt preserves whichever form the author wrote.
func TestExpandStructSurvivesGofmt(t *testing.T) {
	ruletest.AssertGofmtStable(t, expandStructDefinition, map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/b.go": "package b\n\ntype Point struct{ X, Y int }\n\ntype Wide struct {\n\tA int\n}\n\ntype Marker struct{}\n",
	})
}

func TestFactoryNaming(t *testing.T) {
	t.Run("New returning a value", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Config struct {\n\tA int\n}\n\nfunc NewConfig() Config { return Config{} }\n",
		})
		ruletest.Assert(t, got, []string{"NewConfig returns a value, so it should be named Make..."})
	})

	t.Run("Make returning a pointer", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Config struct {\n\tA int\n}\n\nfunc MakeConfig() *Config { return &Config{} }\n",
		})
		ruletest.Assert(t, got, []string{"MakeConfig returns a pointer, so it should be named New..."})
	})

	// A trailing error does not change what the function builds.
	t.Run("trailing error is ignored", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Config struct {\n\tA int\n}\n\nfunc NewConfig() (*Config, error) { return &Config{}, nil }\n",
		})
		ruletest.Assert(t, got, nil)
	})

	// The shipped scope judges only names the author already chose, so it
	// cannot fire on Parse, Open, Dial or MustCompile.
	t.Run("unprefixed constructors are not judged", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

type Config struct {
	A int
}

func Parse(s string) *Config { return &Config{} }

func Open(s string) (*Config, error) { return &Config{}, nil }
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("all-factories mode judges everything", func(t *testing.T) {
		got := ruletest.RunWith(t, factoryNaming, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Config struct {\n\tA int\n}\n\nfunc Build() *Config { return &Config{} }\n",
		}, factoryNamingSettings{
			ValuePrefix: "Make", PointerPrefix: "New", Scope: ScopeAllFactories,
		})
		ruletest.Assert(t, got, []string{"Build builds Config but carries no factory prefix"})
	})

	// An imported result type cannot be classified from syntax: an imported
	// interface looks exactly like an imported struct, and guessing would
	// misname every constructor returning an error-like interface.
	t.Run("imported result types are skipped", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

import "bytes"

func NewBuffer() bytes.Buffer { return bytes.Buffer{} }
`,
		})
		ruletest.Assert(t, got, nil)
	})

	// Newton is not a New factory.
	t.Run("prefix must end at a word boundary", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Config struct {\n\tA int\n}\n\nfunc Newton() Config { return Config{} }\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("bare New and Make are accepted", func(t *testing.T) {
		got := ruletest.Run(t, factoryNaming, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype Config struct {\n\tA int\n}\n\nfunc New() *Config { return &Config{} }\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, factoryNaming, conforming), nil)
	})
}

func TestFamilyIsWellFormed(t *testing.T) {
	rules := Rules()
	if len(rules) != 2 {
		t.Fatalf("family has %d rules, want 2", len(rules))
	}
	for _, r := range rules {
		t.Run(r.ID, func(t *testing.T) {
			ruletest.AssertWellFormed(t, r)
			if r.Tier != rule.Syntax {
				t.Errorf("tier = %v, want syntax", r.Tier)
			}
			ruletest.AssertDeterministic(t, r, conforming)
			ruletest.AssertGofmtStable(t, r, conforming)
		})
	}
}

func TestFamilyBuildsAsASet(t *testing.T) {
	if _, err := rule.NewSet(Rules()); err != nil {
		t.Fatalf("Rules() does not form a valid set: %v", err)
	}
}

package organization

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
)

// conforming follows every rule in the family. Each rule asserts it stays
// silent here, which is what stops a rule from being written so broadly that it
// fires on correct code.
var conforming = map[string]string{
	"go.mod": "module example.com/ok\n\ngo 1.25.0\n",
	"pkg/billing/invoice/status.go": `package invoice

type Status int

const (
	StatusDraft Status = iota
	StatusSent
)

func (s Status) String() string { return "status" }
`,
	"pkg/billing/invoice/invoice.go": `package invoice

import "errors"

var ErrNotFound = errors.New("not found")

type Invoice struct {
	ID string
}

func NewInvoice(id string) *Invoice { return &Invoice{ID: id} }

func (i *Invoice) Total() int { return 0 }

func (i *Invoice) normalize() {}

func Parse(s string) string { return s }

func trim(s string) string { return s }
`,
	"pkg/billing/invoice/handler.go": `package invoice

type Handler interface {
	Handle(id string) error
	Close() error
}
`,
	"pkg/billing/invoice/registry.go": `package invoice

import "sync"

var registryOnce sync.Once

var registry *Invoice

func instance() *Invoice {
	registryOnce.Do(func() { registry = &Invoice{} })
	return registry
}

func Default() *Invoice { return instance() }
`,
}

func TestMemberOrder(t *testing.T) {
	t.Run("const after functions", func(t *testing.T) {
		got := ruletest.Run(t, memberOrder, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nfunc F() {}\n\nconst limit = 10\n",
		})
		ruletest.Assert(t, got, []string{"enums limit appears after the functions section"})
	})

	t.Run("grouped var block", func(t *testing.T) {
		got := ruletest.Run(t, memberOrder, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nimport \"errors\"\n\nvar (\n\tErrA = errors.New(\"a\")\n\tErrB = errors.New(\"b\")\n)\n",
		})
		ruletest.Assert(t, got, []string{"var block declares 2 variables"})
	})

	t.Run("type declarations must stay contiguous", func(t *testing.T) {
		got := ruletest.Run(t, memberOrder, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

type A struct{}

type B struct{}

func (a A) M() {}
`,
		})
		ruletest.Assert(t, got, []string{"is separated from the rest of A by B"})
	})

	// A var whose initializer references a local type cannot be hoisted above
	// that type, so it sinks to the type's section rather than being reported.
	t.Run("var depending on a local type is not a violation", func(t *testing.T) {
		got := ruletest.Run(t, memberOrder, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

type Rule struct {
	Name string
}

var defaultRule = &Rule{Name: "x"}
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, memberOrder, conforming), nil)
	})
}

func TestPrivateFunctionsLast(t *testing.T) {
	t.Run("exported after unexported", func(t *testing.T) {
		got := ruletest.Run(t, privateFunctionsLast, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nfunc helper() {}\n\nfunc Exported() {}\n",
		})
		ruletest.Assert(t, got, []string{"exported func Exported appears after unexported helper"})
	})

	t.Run("methods split per receiver, not across the file", func(t *testing.T) {
		got := ruletest.Run(t, privateFunctionsLast, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

type A struct{}

func (a A) Do() {}

func (a A) hide() {}

type B struct{}

func (b B) Do() {}

func (b B) hide() {}
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, privateFunctionsLast, conforming), nil)
	})
}

func TestSingletonLayout(t *testing.T) {
	t.Run("exported accessor before instance", func(t *testing.T) {
		got := ruletest.Run(t, singletonLayout, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

import "sync"

var once sync.Once

var inst *int

func Get() *int { return instance() }

func instance() *int {
	once.Do(func() { inst = new(int) })
	return inst
}
`,
		})
		ruletest.Assert(t, got, []string{"exported Get appears before instance"})
	})

	t.Run("unrelated declaration in a singleton file", func(t *testing.T) {
		got := ruletest.Run(t, singletonLayout, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

import "sync"

var once sync.Once

var inst *int

func instance() *int {
	once.Do(func() { inst = new(int) })
	return inst
}

type Unrelated struct{}
`,
		})
		ruletest.Assert(t, got, []string{"declaration is unrelated to the singleton"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, singletonLayout, conforming), nil)
	})
}

func TestSingletonInstanceFunc(t *testing.T) {
	t.Run("no sync.Once guard", func(t *testing.T) {
		got := ruletest.Run(t, singletonInstanceFunc, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

import "sync"

var once sync.Once

var inst *int

func instance() *int {
	if inst == nil {
		inst = new(int)
	}
	return inst
}
`,
		})
		ruletest.Assert(t, got, []string{"does not guard construction with sync.Once"})
	})

	t.Run("constructed in init", func(t *testing.T) {
		got := ruletest.Run(t, singletonInstanceFunc, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

import "sync"

var once sync.Once

var inst *int

func instance() *int {
	once.Do(func() { inst = new(int) })
	return inst
}

func init() { inst = new(int) }
`,
		})
		// The init function both assigns outside the accessor and constructs in
		// init; both are worth saying, and they name different fixes.
		if len(got) < 1 {
			t.Fatalf("expected a finding, got %v", got)
		}
		if !strings.Contains(strings.Join(got, "\n"), "init function") {
			t.Errorf("findings do not mention init: %v", got)
		}
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, singletonInstanceFunc, conforming), nil)
	})
}

func TestGlobalsSingletonOnly(t *testing.T) {
	t.Run("plain global", func(t *testing.T) {
		got := ruletest.Run(t, globalsSingletonOnly, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nvar counter int\n",
		})
		ruletest.Assert(t, got, []string{"package-level variable counter is not singleton state"})
	})

	// The exemption list is the whole design: Go has no immutable composite
	// constant, so these forms have no other spelling and a rule without them
	// would fire on unavoidable code.
	t.Run("exempt forms", func(t *testing.T) {
		got := ruletest.Run(t, globalsSingletonOnly, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var ErrNotFound = errors.New("not found")

var _ fmt.Stringer = (*thing)(nil)

var pattern = regexp.MustCompile("^x$")

var replacer = strings.NewReplacer("a", "b").Replace

var lookup = map[string]int{"a": 1}

var defaults = &thing{}

type thing struct{}

func (t *thing) String() string { return "" }
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("exported table is mutable by importers", func(t *testing.T) {
		got := ruletest.Run(t, globalsSingletonOnly, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nvar Lookup = map[string]int{\"a\": 1}\n",
		})
		ruletest.Assert(t, got, []string{"exported lookup table Lookup can be mutated"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, globalsSingletonOnly, conforming), nil)
	})
}

func TestBudgets(t *testing.T) {
	many := func(n int, prefix string) string {
		var b strings.Builder
		b.WriteString("package b\n")
		for i := range n {
			fmt.Fprintf(&b, "\nfunc %s%d() {}\n", prefix, i)
		}
		return b.String()
	}

	t.Run("too many functions", func(t *testing.T) {
		got := ruletest.RunWith(t, maxFunctionsPerFile, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": many(6, "F"),
		}, budgetSettings{Limit: 3})
		ruletest.Assert(t, got, []string{"file declares 6 functions, over the limit of 3"})
	})

	t.Run("too many exported", func(t *testing.T) {
		got := ruletest.RunWith(t, maxPublicFunctions, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": many(4, "F"),
		}, budgetSettings{Limit: 2})
		ruletest.Assert(t, got, []string{"file exports 4 functions, over the limit of 2"})
	})

	t.Run("private budget only applies where there are exports", func(t *testing.T) {
		helpers := many(4, "h")
		got := ruletest.RunWith(t, maxPrivateFunctions, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": helpers,
		}, privateBudgetSettings{Limit: 2, WhenFileHasExports: true})
		ruletest.Assert(t, got, nil)

		got = ruletest.RunWith(t, maxPrivateFunctions, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": helpers + "\nfunc Exported() {}\n",
		}, privateBudgetSettings{Limit: 2, WhenFileHasExports: true})
		ruletest.Assert(t, got, []string{"file declares 4 unexported functions"})
	})

	// This is the interaction that makes org/type-cohesion satisfiable: a type
	// with more methods than the file budget must not be forced to split.
	t.Run("methods are not counted against the file", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package b\n\ntype T struct{}\n")
		for i := range 10 {
			fmt.Fprintf(&b, "\nfunc (t T) M%d() {}\n", i)
		}
		got := ruletest.RunWith(t, maxFunctionsPerFile, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": b.String(),
		}, budgetSettings{Limit: 3})
		ruletest.Assert(t, got, nil)
	})
}

func TestTypeCohesion(t *testing.T) {
	t.Run("methods away from their type", func(t *testing.T) {
		got := ruletest.Run(t, typeCohesion, map[string]string{
			"go.mod":         "module x\n",
			"pkg/a/b/t.go":   "package b\n\ntype T struct{}\n",
			"pkg/a/b/ops.go": "package b\n\nfunc (t T) Do() {}\n",
		})
		ruletest.Assert(t, got, []string{"of type T live away from its declaration in t.go"})
	})

	t.Run("build-constrained files are exempt", func(t *testing.T) {
		got := ruletest.Run(t, typeCohesion, map[string]string{
			"go.mod":             "module x\n",
			"pkg/a/b/t.go":       "package b\n\ntype T struct{}\n",
			"pkg/a/b/t_linux.go": "//go:build linux\n\npackage b\n\nfunc (t T) Do() {}\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, typeCohesion, conforming), nil)
	})
}

func TestInterfaceOwnFile(t *testing.T) {
	t.Run("interface shares a file with a struct", func(t *testing.T) {
		got := ruletest.Run(t, interfaceOwnFile, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

type Handler interface {
	Handle() error
}

type Impl struct{}
`,
		})
		ruletest.Assert(t, got, []string{"interface Handler shares a file with 1 other type"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, interfaceOwnFile, conforming), nil)
	})
}

// TestSingletonFilesAreExemptFromOrdering is the precedence mechanism.
//
// A singleton requires the unexported accessor to precede the exported
// functions, which inverts org/private-functions-last. Rather than that rule
// knowing singletons exist, singleton files are classified as a distinct shape
// and the ordering rules skip them.
func TestSingletonFilesAreExemptFromOrdering(t *testing.T) {
	files := map[string]string{
		"go.mod": "module x\n",
		"pkg/a/b/b.go": `package b

import "sync"

var once sync.Once

var inst *int

func instance() *int {
	once.Do(func() { inst = new(int) })
	return inst
}

func Get() *int { return instance() }
`,
	}
	for _, r := range []*rule.Rule{memberOrder, privateFunctionsLast, globalsSingletonOnly} {
		if got := ruletest.Run(t, r, files); len(got) != 0 {
			t.Errorf("%s fired on a well-formed singleton file: %v", r.ID, got)
		}
	}
	// The singleton rules themselves must still accept it.
	for _, r := range []*rule.Rule{singletonLayout, singletonInstanceFunc} {
		if got := ruletest.Run(t, r, files); len(got) != 0 {
			t.Errorf("%s fired on a well-formed singleton file: %v", r.ID, got)
		}
	}
}

// TestNoDefectIsReportedTwice guards the precedence table: each mistake gets one
// owner, so a user never sees two findings giving contradictory advice.
func TestNoDefectIsReportedTwice(t *testing.T) {
	violating := map[string]string{
		"go.mod":       "module x\n",
		"pkg/a/b/b.go": "package b\n\nfunc helper() {}\n\nfunc Exported() {}\n\nconst limit = 10\n",
	}
	seen := map[string][]string{}
	for _, r := range Rules() {
		for _, finding := range ruletest.Run(t, r, violating) {
			path, rest, _ := strings.Cut(finding, " ")
			_ = rest
			seen[path] = append(seen[path], r.ID)
		}
	}
	for pos, ids := range seen {
		if len(ids) > 1 {
			t.Errorf("%s reported by %d rules (%s); each defect needs one owner",
				pos, len(ids), strings.Join(ids, ", "))
		}
	}
}

func TestFamilyIsWellFormed(t *testing.T) {
	rules := Rules()
	if len(rules) != 10 {
		t.Fatalf("family has %d rules, want 10", len(rules))
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

func TestConformingTreeIsSilentForEveryRule(t *testing.T) {
	for _, r := range Rules() {
		if got := ruletest.Run(t, r, conforming); len(got) != 0 {
			t.Errorf("%s fired on a conforming tree: %v", r.ID, got)
		}
	}
}

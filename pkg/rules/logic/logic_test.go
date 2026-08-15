package logic

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
)

// conforming satisfies every rule in the family.
var conforming = map[string]string{
	"go.mod": "module example.com/ok\n\ngo 1.25.0\n",
	"pkg/billing/invoice/status.go": `package invoice

type Status int

const (
	StatusDraft Status = iota
	StatusSent
	StatusPaid
)
`,
	"pkg/billing/invoice/invoice.go": `package invoice

type Invoice struct {
	ID    string
	Total int
}

func NewInvoice(id string) *Invoice { return &Invoice{ID: id} }

func (i *Invoice) Settled() bool {
	if i.Total == 0 {
		return true
	}
	return i.ID != ""
}
`,
}

func TestIotaCandidate(t *testing.T) {
	t.Run("hand-numbered run", func(t *testing.T) {
		got := ruletest.Run(t, iotaCandidate, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nconst (\n\tA = 0\n\tB = 1\n\tC = 2\n)\n",
		})
		ruletest.Assert(t, got, []string{"3 constants are numbered by hand"})
	})

	t.Run("offset and step are still a sequence", func(t *testing.T) {
		got := ruletest.Run(t, iotaCandidate, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nconst (\n\tA = 8\n\tB = 16\n\tC = 24\n)\n",
		})
		ruletest.Assert(t, got, []string{"3 constants are numbered by hand"})
	})

	t.Run("already uses iota", func(t *testing.T) {
		got := ruletest.Run(t, iotaCandidate, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype S int\n\nconst (\n\tA S = iota\n\tB\n\tC\n)\n",
		})
		ruletest.Assert(t, got, nil)
	})

	// Unrelated constants that happen to share a block are not an enum.
	t.Run("non-sequential values are not an enum", func(t *testing.T) {
		got := ruletest.Run(t, iotaCandidate, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nconst (\n\tTimeout = 30\n\tRetries = 3\n\tPort    = 8080\n)\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("string constants are not an integer run", func(t *testing.T) {
		got := ruletest.Run(t, iotaCandidate, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nconst (\n\tA = \"a\"\n\tB = \"b\"\n\tC = \"c\"\n)\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("run shorter than the minimum", func(t *testing.T) {
		got := ruletest.Run(t, iotaCandidate, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\nconst (\n\tA = 0\n\tB = 1\n)\n",
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, iotaCandidate, conforming), nil)
	})
}

func TestMaxConditionOperands(t *testing.T) {
	t.Run("over the limit", func(t *testing.T) {
		got := ruletest.Run(t, maxConditionOperands, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(a, b, c, d, e bool) bool {
	if a && b && c && d && e {
		return true
	}
	return false
}
`,
		})
		ruletest.Assert(t, got, []string{"if condition combines 5 operands, over the limit of 4"})
	})

	// Go's precedence is correct but not obvious: && binds tighter than ||.
	t.Run("mixed operators without parentheses", func(t *testing.T) {
		got := ruletest.Run(t, maxConditionOperands, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(a, b, c bool) bool {
	if a || b && c {
		return true
	}
	return false
}
`,
		})
		ruletest.Assert(t, got, []string{"condition mixes && and || without parentheses"})
	})

	t.Run("parentheses make the grouping explicit", func(t *testing.T) {
		got := ruletest.Run(t, maxConditionOperands, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(a, b, c bool) bool {
	if a || (b && c) {
		return true
	}
	return false
}
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, maxConditionOperands, conforming), nil)
	})
}

func TestMaxObjectMembers(t *testing.T) {
	t.Run("too many fields", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package b\n\ntype Wide struct {\n")
		for i := range 8 {
			fmt.Fprintf(&b, "\tF%d int\n", i)
		}
		b.WriteString("}\n")
		got := ruletest.RunWith(t, maxObjectMembers, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": b.String(),
		}, maxObjectMembersSettings{Fields: 5, Methods: 15})
		ruletest.Assert(t, got, []string{"type Wide has 8 fields, over the limit of 5"})
	})

	// Methods are counted across the package, since org/type-cohesion puts them
	// all in one file but a package may still split by build constraint.
	t.Run("too many methods", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("package b\n\ntype T struct{}\n")
		for i := range 6 {
			fmt.Fprintf(&b, "\nfunc (t T) M%d() {}\n", i)
		}
		got := ruletest.RunWith(t, maxObjectMembers, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": b.String(),
		}, maxObjectMembersSettings{Fields: 12, Methods: 3})
		ruletest.Assert(t, got, []string{"type T has 6 methods, over the limit of 3"})
	})

	t.Run("grouped field names count individually", func(t *testing.T) {
		got := ruletest.RunWith(t, maxObjectMembers, map[string]string{
			"go.mod":       "module x\n",
			"pkg/a/b/b.go": "package b\n\ntype T struct {\n\tA, B, C int\n}\n",
		}, maxObjectMembersSettings{Fields: 2, Methods: 15})
		ruletest.Assert(t, got, []string{"type T has 3 fields, over the limit of 2"})
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, maxObjectMembers, conforming), nil)
	})
}

func TestPreferGuardClause(t *testing.T) {
	t.Run("body wholly wrapped", func(t *testing.T) {
		got := ruletest.Run(t, preferGuardClause, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(ok bool) int {
	if ok {
		a := 1
		b := 2
		return a + b
	}
	return 0
}
`,
		})
		ruletest.Assert(t, got, []string{"function body is wholly wrapped in 1 conditional"})
	})

	// Nested wrappers are one inversion, not two.
	t.Run("nested wrappers collapse into one finding", func(t *testing.T) {
		got := ruletest.Run(t, preferGuardClause, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(a, b bool) int {
	if a {
		if b {
			x := 1
			y := 2
			return x + y
		}
	}
	return 0
}
`,
		})
		ruletest.Assert(t, got, []string{"wholly wrapped in 2 conditionals"})
	})

	t.Run("an else branch has no single inversion", func(t *testing.T) {
		got := ruletest.Run(t, preferGuardClause, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(ok bool) int {
	if ok {
		a := 1
		b := 2
		return a + b
	} else {
		return 9
	}
}
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("short wrapped body is not worth inverting", func(t *testing.T) {
		got := ruletest.Run(t, preferGuardClause, map[string]string{
			"go.mod": "module x\n",
			"pkg/a/b/b.go": `package b

func f(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
`,
		})
		ruletest.Assert(t, got, nil)
	})

	t.Run("conforming tree is silent", func(t *testing.T) {
		ruletest.Assert(t, ruletest.Run(t, preferGuardClause, conforming), nil)
	})
}

func TestFamilyIsWellFormed(t *testing.T) {
	rules := Rules()
	if len(rules) != 35 {
		t.Fatalf("family has %d rules, want 35", len(rules))
	}
	for _, r := range rules {
		t.Run(r.ID, func(t *testing.T) {
			ruletest.AssertWellFormed(t, r)
			// Type-tier rules are exercised end to end through internal/cli,
			// which has a real type-checked program to give them.
			if r.Tier != rule.Syntax {
				return
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

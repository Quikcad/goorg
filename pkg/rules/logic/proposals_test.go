package logic

import (
	"testing"

	"github.com/Quikcad/goorg/pkg/lint/rule"
	"github.com/Quikcad/goorg/pkg/lint/ruletest"
)

// proposalCase is one rule, one fixture, one expectation.
type proposalCase struct {
	name string
	rule *rule.Rule
	src  string
	want []string
}

// fires covers the syntax-tier rules approved in D7. Each is a fixture that
// should produce exactly the listed findings.
var fires = []proposalCase{
	{
		name: "stringly-typed enum",
		rule: stringlyTypedEnum,
		src:  "package b\n\nconst (\n\tA = \"a\"\n\tB = \"b\"\n\tC = \"c\"\n)\n",
		want: []string{"3 string constants form an enum with no named type"},
	},
	{
		name: "enum zero value unnamed",
		rule: enumZeroValueUnnamed,
		src:  "package b\n\ntype S int\n\nconst (\n\tDraft S = iota\n\tSent\n)\n",
		want: []string{"Draft is the zero value but does not say so"},
	},
	{
		name: "duplicate const value",
		rule: duplicateConstValue,
		src:  "package b\n\nconst (\n\tA = 1\n\tB = 2\n\tC = 2\n)\n",
		want: []string{"C repeats the value of B"},
	},
	{
		name: "nesting too deep",
		rule: maxNestingDepth,
		src: `package b

func f(xs [][]int) {
	for _, x := range xs {
		if len(x) > 0 {
			for _, y := range x {
				if y > 0 {
					_ = y
				}
			}
		}
	}
}
`,
		want: []string{"f nests 4 levels deep, over the limit of 3"},
	},
	{
		name: "if chain on one operand",
		rule: ifChainToSwitch,
		src: `package b

func f(s string) int {
	if s == "a" {
		return 1
	} else if s == "b" {
		return 2
	} else if s == "c" {
		return 3
	}
	return 0
}
`,
		want: []string{"3 ifs branches all compare s"},
	},
	{
		name: "negated condition with else",
		rule: negatedCondition,
		src:  "package b\n\nfunc f(ok bool) int {\n\tif !ok {\n\t\treturn 0\n\t} else {\n\t\treturn 1\n\t}\n}\n",
		want: []string{"condition is negated although the if has an else"},
	},
	{
		name: "single case switch",
		rule: singleCaseSwitch,
		src:  "package b\n\nfunc f(s string) {\n\tswitch s {\n\tcase \"a\":\n\t\t_ = s\n\t}\n}\n",
		want: []string{"switch has a single case and no default"},
	},
	{
		name: "identical branches",
		rule: identicalBranches,
		src:  "package b\n\nfunc f(ok bool, x int) int {\n\tif ok {\n\t\treturn x + 1\n\t} else {\n\t\treturn x + 1\n\t}\n}\n",
		want: []string{"both branches of the conditional are identical"},
	},
	{
		name: "empty branch without explanation",
		rule: emptyBranch,
		src:  "package b\n\nfunc f(ok bool) {\n\tif ok {\n\t}\n}\n",
		want: []string{"branch body is empty with no explanation"},
	},
	{
		name: "too many parameters",
		rule: maxFunctionParams,
		src:  "package b\n\nfunc f(a, b, c, d, e, g int) {}\n",
		want: []string{"f takes 6 parameters, over the limit of 5"},
	},
	{
		name: "too many return values",
		rule: maxReturnValues,
		src:  "package b\n\nfunc f() (int, int, int, int, error) { return 0, 0, 0, 0, nil }\n",
		want: []string{"f returns 4 values besides an error, over the limit of 3"},
	},
	{
		name: "too many bool fields",
		rule: booleanFieldCount,
		src:  "package b\n\ntype Job struct {\n\tstarted  bool\n\tfinished bool\n\tfailed   bool\n}\n",
		want: []string{"Job has 3 bool fields, which encode 8 states"},
	},
	{
		name: "pointer to slice",
		rule: pointerToSliceOrMap,
		src:  "package b\n\nfunc f(xs *[]int) {}\n",
		want: []string{"a slice already holds a reference"},
	},
	{
		name: "any struct field",
		rule: emptyInterfaceField,
		src:  "package b\n\ntype Event struct {\n\tPayload any\n}\n",
		want: []string{"struct field is typed any"},
	},
	{
		name: "bare bool parameter",
		rule: booleanParameter,
		src:  "package b\n\nfunc Fetch(id string, retry bool) {}\n",
		want: []string{"Fetch takes a bare bool"},
	},
	{
		name: "oversized interface",
		rule: interfaceSize,
		src: `package b

type Store interface {
	Get() error
	Put() error
	Delete() error
	List() error
	Close() error
}
`,
		want: []string{"interface Store declares 5 methods, over the limit of 4"},
	},
}

// silent covers the cases each rule must not report, which is what stops a rule
// from being written broadly enough to fire on correct code.
var silent = []proposalCase{
	{name: "typed string enum", rule: stringlyTypedEnum,
		src: "package b\n\ntype S string\n\nconst (\n\tA S = \"a\"\n\tB S = \"b\"\n\tC S = \"c\"\n)\n"},
	{name: "named zero value", rule: enumZeroValueUnnamed,
		src: "package b\n\ntype S int\n\nconst (\n\tUnknown S = iota\n\tDraft\n)\n"},
	{name: "distinct const values", rule: duplicateConstValue,
		src: "package b\n\nconst (\n\tA = 1\n\tB = 2\n\tC = 3\n)\n"},
	{name: "shallow nesting", rule: maxNestingDepth,
		src: "package b\n\nfunc f(xs []int) {\n\tfor _, x := range xs {\n\t\tif x > 0 {\n\t\t\t_ = x\n\t\t}\n\t}\n}\n"},
	{name: "chain on different operands", rule: ifChainToSwitch,
		src: "package b\n\nfunc f(a, b, c string) int {\n\tif a == \"x\" {\n\t\treturn 1\n\t} else if b == \"y\" {\n\t\treturn 2\n\t} else if c == \"z\" {\n\t\treturn 3\n\t}\n\treturn 0\n}\n"},
	// A negated guard with no else is what logic/prefer-guard-clause wants.
	{name: "negated guard without else", rule: negatedCondition,
		src: "package b\n\nfunc f(ok bool) int {\n\tif !ok {\n\t\treturn 0\n\t}\n\treturn 1\n}\n"},
	{name: "switch with default", rule: singleCaseSwitch,
		src: "package b\n\nfunc f(s string) {\n\tswitch s {\n\tcase \"a\":\n\t\t_ = s\n\tdefault:\n\t}\n}\n"},
	{name: "differing branches", rule: identicalBranches,
		src: "package b\n\nfunc f(ok bool, x int) int {\n\tif ok {\n\t\treturn x + 1\n\t} else {\n\t\treturn x - 1\n\t}\n}\n"},
	{name: "explained empty branch", rule: emptyBranch,
		src: "package b\n\nfunc f(ok bool) {\n\tif ok {\n\t\t// Deliberate: the caller already handled it.\n\t}\n}\n"},
	// A leading context is conventional and carries no ambiguity.
	{name: "context does not count", rule: maxFunctionParams,
		src: "package b\n\nimport \"context\"\n\nfunc f(ctx context.Context, a, b, c, d, e int) {}\n"},
	{name: "trailing error does not count", rule: maxReturnValues,
		src: "package b\n\nfunc f() (int, int, int, error) { return 0, 0, 0, nil }\n"},
	// An options struct is a bag of independent switches, not a state machine.
	{name: "options struct is exempt", rule: booleanFieldCount,
		src: "package b\n\ntype Settings struct {\n\tA bool\n\tB bool\n\tC bool\n\tD bool\n}\n"},
	// A pointer to a fixed-size array is legitimate.
	{name: "pointer to array", rule: pointerToSliceOrMap,
		src: "package b\n\nfunc f(xs *[4]int) {}\n"},
	{name: "typed struct field", rule: emptyInterfaceField,
		src: "package b\n\ntype Event struct {\n\tPayload string\n}\n"},
	{name: "unexported bool parameter", rule: booleanParameter,
		src: "package b\n\nfunc fetch(id string, retry bool) {}\n"},
	{name: "small interface", rule: interfaceSize,
		src: "package b\n\ntype Getter interface {\n\tGet() error\n}\n"},
}

func TestApprovedProposalsFire(t *testing.T) {
	for _, tc := range fires {
		t.Run(tc.name, func(t *testing.T) {
			got := ruletest.Run(t, tc.rule, map[string]string{
				"go.mod":       "module x\n",
				"pkg/a/b/b.go": tc.src,
			})
			ruletest.Assert(t, got, tc.want)
		})
	}
}

func TestApprovedProposalsStaySilent(t *testing.T) {
	for _, tc := range silent {
		t.Run(tc.name, func(t *testing.T) {
			got := ruletest.Run(t, tc.rule, map[string]string{
				"go.mod":       "module x\n",
				"pkg/a/b/b.go": tc.src,
			})
			ruletest.Assert(t, got, nil)
		})
	}
}

// TestEveryApprovedRuleIsCovered stops a rule from being added to the family
// without a fixture proving it fires.
func TestEveryApprovedRuleIsCovered(t *testing.T) {
	covered := map[string]bool{}
	for _, tc := range fires {
		covered[tc.rule.ID] = true
	}
	// Rules with their own dedicated test function, or exercised end to end
	// through internal/cli because they need type information.
	for _, id := range []string{
		"logic/iota-candidate", "logic/max-condition-operands",
		"logic/max-object-members", "logic/prefer-guard-clause",
		"logic/max-function-lines", "logic/section-spacing",
		"logic/interface-registry", "logic/any-should-be-generic",
		"logic/ideal-numeric-type", "logic/enum-missing-string",
		"logic/interface-at-consumer", "logic/constraint-too-wide",
		"logic/float-equality", "logic/lossy-conversion",
		"logic/unsigned-underflow", "logic/integer-division-to-float",
		"logic/exported-embedded-mutex", "logic/context-in-struct",
		"logic/panic-outside-main", "logic/unused-type-parameter",
	} {
		covered[id] = true
	}
	for _, r := range Rules() {
		if !covered[r.ID] {
			t.Errorf("%s has no fixture proving it fires", r.ID)
		}
	}
}

package logic

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/Quikcad/goorg/pkg/lint/diag"
	"github.com/Quikcad/goorg/pkg/lint/rule"
)

var duplicateConstValue = &rule.Rule{
	ID:       "logic/duplicate-const-value",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "two constants in one block share a value",
	Default:  diag.Error,
	Doc: `Two constants declared in the same block may not have the same literal
value.

	const (                          violation — Sent and Paid are both 2
		StatusDraft = 1
		StatusSent  = 2
		StatusPaid  = 2
	)

Rationale: this is the exact failure logic/iota-candidate exists to prevent,
caught after the fact. Hand-numbered constants get renumbered by hand, and the
line that gets missed produces two names for one value. Nothing about that is a
compile error: a switch on the duplicate has an unreachable branch, comparisons
succeed for the wrong constant, and the bug looks like a logic error anywhere
except where it was introduced.

To fix: renumber the run — or better, convert it to iota so the numbering cannot
drift again.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				d, ok := node.(*ast.GenDecl)
				if !ok || d.Tok != token.CONST {
					continue
				}
				out = append(out, duplicatesIn(c, d)...)
			}
		}
		return out
	},
}

// stringlyEnumSettings is the configurable surface of logic/stringly-typed-enum.
type stringlyEnumSettings struct {
	MinConstants int `yaml:"min_constants"`
}

// zeroValueSettings is the configurable surface of logic/enum-zero-value-unnamed.
type zeroValueSettings struct {
	// Names are the identifiers accepted for the zero value.
	Names []string `yaml:"names"`
}

var stringlyTypedEnum = &rule.Rule{
	ID:       "logic/stringly-typed-enum",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "a run of string constants used as an enum needs a named type",
	Default:  diag.Warning,
	Doc: `A run of string constants that acts as an enumeration is declared with a
named string type.

	const (                          violation — untyped
		StatusDraft = "draft"
		StatusSent  = "sent"
	)

	type Status string               OK
	const (
		StatusDraft Status = "draft"
		StatusSent  Status = "sent"
	)

Rationale: without a named type the set is not a set — it is three strings that
happen to be declared together. func Set(s string) accepts any string at all,
so a typo reaches production as data rather than failing to compile, and there
is nowhere to hang a Valid or a String method. One line of declaration converts
a convention into something the compiler enforces.

To fix: declare a named string type and give the constants that type.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := stringlyEnumSettings{MinConstants: 3}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				d, ok := node.(*ast.GenDecl)
				if !ok || d.Tok != token.CONST || len(d.Specs) < s.MinConstants {
					continue
				}
				if !allUntypedStrings(d) {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(d),
					Message: fmt.Sprintf("%s form an enum with no named type",
						plural(len(d.Specs), "string constant")),
					Help: "declare a named string type and give the constants that type",
				})
			}
		}
		return out
	},
}

var enumZeroValueUnnamed = &rule.Rule{
	ID:       "logic/enum-zero-value-unnamed",
	Category: rule.Logic,
	Tier:     rule.Syntax,
	Summary:  "an iota enum names its zero value",
	Default:  diag.Warning,
	Doc: `An enum whose first constant is a meaningful value leaves the zero value
unnamed, so a variable nobody assigned looks like a legitimate state.

	const (                          violation — Draft is the zero value
		StatusDraft Status = iota
		StatusSent
	)

	const (                          OK
		StatusUnknown Status = iota
		StatusDraft
		StatusSent
	)

Rationale: var s Status is StatusDraft, and so is a Status field nobody set, and
so is the Status in a struct decoded from JSON that omitted it. The zero value
is unavoidable in Go, so an enum that does not name it silently claims that
"never set" and "explicitly the first state" are the same thing. Naming it makes
the difference expressible, and makes a missing assignment visible in a switch.

To fix: add an explicit zero constant — Unknown, Unspecified or None — as the
first value of the run.`,
	Check: func(c *rule.Context) []diag.Diagnostic {
		s := zeroValueSettings{Names: []string{"unknown", "unspecified", "none", "invalid", "undefined"}}
		if err := c.Settings(&s); err != nil {
			return settingsError(err)
		}

		var out []diag.Diagnostic
		for _, f := range files(c) {
			for _, node := range f.Syntax.Decls {
				d, ok := node.(*ast.GenDecl)
				if !ok || d.Tok != token.CONST || !startsAtIota(d) {
					continue
				}
				first := firstConstName(d)
				if first == "" || namesZeroValue(first, s.Names) {
					continue
				}
				out = append(out, diag.Diagnostic{
					Position: c.Pos(d),
					Message:  fmt.Sprintf("%s is the zero value but does not say so", first),
					Help:     "add an explicit Unknown or Unspecified constant as the first value",
				})
			}
		}
		return out
	},
}

// duplicatesIn reports constants in one block sharing a literal value.
func duplicatesIn(c *rule.Context, d *ast.GenDecl) []diag.Diagnostic {
	seen := map[string]string{}
	var out []diag.Diagnostic
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || len(vs.Values) != 1 || len(vs.Names) != 1 {
			continue
		}
		lit, ok := vs.Values[0].(*ast.BasicLit)
		if !ok {
			continue
		}
		key := lit.Kind.String() + ":" + lit.Value
		if first, dup := seen[key]; dup {
			out = append(out, diag.Diagnostic{
				Position: c.Pos(vs.Names[0]),
				Message:  fmt.Sprintf("%s repeats the value of %s", vs.Names[0].Name, first),
				Help:     "renumber the run, or convert it to iota so it cannot drift",
			})
			continue
		}
		seen[key] = vs.Names[0].Name
	}
	return out
}

// allUntypedStrings reports whether every spec is an untyped string constant.
func allUntypedStrings(d *ast.GenDecl) bool {
	for _, spec := range d.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok || vs.Type != nil || len(vs.Values) != 1 {
			return false
		}
		lit, ok := vs.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}
	}
	return len(d.Specs) > 0
}

// startsAtIota reports whether a const block's first spec is a bare iota.
func startsAtIota(d *ast.GenDecl) bool {
	if len(d.Specs) == 0 {
		return false
	}
	vs, ok := d.Specs[0].(*ast.ValueSpec)
	if !ok || len(vs.Values) != 1 {
		return false
	}
	id, ok := vs.Values[0].(*ast.Ident)
	return ok && id.Name == "iota"
}

func firstConstName(d *ast.GenDecl) string {
	vs, ok := d.Specs[0].(*ast.ValueSpec)
	if !ok || len(vs.Names) == 0 {
		return ""
	}
	return vs.Names[0].Name
}

// namesZeroValue reports whether an identifier reads as "no value set".
func namesZeroValue(name string, accepted []string) bool {
	lower := strings.ToLower(name)
	for _, want := range accepted {
		if strings.Contains(lower, strings.ToLower(want)) {
			return true
		}
	}
	return false
}
